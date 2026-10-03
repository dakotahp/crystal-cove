// Package server exposes vault operations as MCP tools over streamable
// HTTP, protected by a static bearer token.
package server

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dakotahp/crystal-cove/internal/search"
	"github.com/dakotahp/crystal-cove/internal/vault"
)

// Version is the server version reported to MCP clients.
const Version = "0.11.0" // x-release-please-version

// Server wires vaults and search into an MCP tool set.
type Server struct {
	vaults map[string]*vault.Vault
	// vaultList keeps the configured order for instruction loading.
	vaultList []*vault.Vault
	searcher  *search.Searcher
	// syncReady reports whether every vault has a fresh sync heartbeat.
	syncReady func() bool
	policy    Policy
	audit     *slog.Logger
}

// Policy limits what the tools may do to a vault.
type Policy struct {
	// ReadOnly leaves out every tool that changes a note.
	ReadOnly bool
	// AllowPermanentDelete lets delete_note remove a note outright. Without
	// it, deletes only move notes to the trash, where they stay recoverable.
	AllowPermanentDelete bool
}

// SetPolicy sets what the tools may do. Call it before serving.
func (s *Server) SetPolicy(p Policy) { s.policy = p }

// New returns a Server over the given vaults.
func New(vaults []*vault.Vault, searcher *search.Searcher, syncReady func() bool) *Server {
	m := make(map[string]*vault.Vault, len(vaults))
	for _, v := range vaults {
		m[v.Name()] = v
	}
	return &Server{
		vaults:    m,
		vaultList: vaults,
		searcher:  searcher,
		syncReady: syncReady,
	}
}

// Instructions returns the vault guidance advertised to MCP clients. It is
// read from disk on each call, so guidance edited on another device reaches
// the next session once it syncs, with no restart.
func (s *Server) Instructions() string { return loadInstructions(s.vaultList) }

// MCPServer builds the MCP server with all tools registered.
func (s *Server) MCPServer() *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "crystal-cove",
		Title:   "Crystal Cove",
		Version: Version,
		Icons:   []mcp.Icon{serverIcon},
	}, &mcp.ServerOptions{Instructions: s.Instructions()})

	if len(s.vaults) > 1 {
		mcp.AddTool(srv, &mcp.Tool{
			Name:        "list_vaults",
			Description: "List the Obsidian vaults available on this server.",
		}, s.listVaults)
	}

	mcp.AddTool(srv, &mcp.Tool{
		Name: "list_notes",
		Description: "List notes and directories in a vault. Hidden folders such as .obsidian and .trash are excluded, " +
			"but passing dir \".trash\" with recursive lists deleted notes, which keep their folders there. Other " +
			"hidden folders cannot be listed. Returns at " +
			"most 200 entries unless limit says otherwise (max 1000); when next_offset is not -1, call again with offset " +
			"set to it, or narrow the listing with dir.",
	}, s.listNotes)

	mcp.AddTool(srv, &mcp.Tool{
		Name: "read_note",
		Description: fmt.Sprintf("Read a note from a vault. Returns at most %d characters per call; "+
			"when the response is truncated, call again with offset set to next_offset to continue reading. version "+
			"identifies the note's text; pass it to edit_note so the edit is refused if the note changed since, "+
			"for example through sync from another device.", vault.ReadPageSize),
	}, s.readNote)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_section",
		Description: "Read a Markdown heading section's body, including nested subsections but excluding its heading. heading_path is an exact case-sensitive suffix of the heading hierarchy (Markdown title text without heading markers); ambiguous matches fail. Only document-level headings count, not headings in code, quotes, lists, or YAML frontmatter. Returns at most 10240 characters; continue with next_offset. version identifies this section's text, for edit_section.",
	}, s.getSection)

	mcp.AddTool(srv, &mcp.Tool{
		Name: "search_notes",
		Description: "Search a vault by note name and by content. A plain multi-word query finds notes holding every " +
			"word, in any order; a query with regular-expression characters is read as a regex (ripgrep syntax), and mode " +
			"forces either reading. Only Markdown notes are searched. Results are ranked: notes named after the query first, " +
			"flagged title_match, then notes with more matching lines. Case-insensitive unless " +
			"case_sensitive is set. max_results counts notes, and each note returns at most 5 matching lines unless max_lines_per_note says otherwise; every note reports its own total_matches.",
	}, s.searchNotes)

	mcp.AddTool(srv, &mcp.Tool{
		Name: "recent_notes",
		Description: "List the notes changed most recently, newest first, with their modified time. Pass since as an " +
			"RFC 3339 timestamp to see only what changed after it. Use this to pick up where work left off, which a " +
			"path-ordered listing cannot answer.",
	}, s.recentNotes)

	mcp.AddTool(srv, &mcp.Tool{
		Name: "list_tags",
		Description: "List every tag used in a vault with the number of notes carrying it, most used first. Tags come " +
			"from the frontmatter tags field and from inline hashtags. Use it to learn what a vault is about before searching.",
	}, s.listTags)

	mcp.AddTool(srv, &mcp.Tool{
		Name: "find_notes",
		Description: "Find notes by metadata rather than text: by tags (a note must carry every tag given) and by a " +
			"frontmatter field, with frontmatter_value optional so a key on its own finds every note carrying that field. " +
			"Use search_notes for words in the text.",
	}, s.findNotes)

	mcp.AddTool(srv, &mcp.Tool{
		Name: "get_links",
		Description: "List the wikilinks a note points at, each resolved to the note it names, or flagged unresolved " +
			"when no such note exists yet. Headings, aliases and embeds are reported as written.",
	}, s.getLinks)

	mcp.AddTool(srv, &mcp.Tool{
		Name: "get_backlinks",
		Description: "List the notes that link to a note, with the links they use. This is how notes relate to each " +
			"other in Obsidian, so use it to find the context around a note rather than searching for its name.",
	}, s.getBacklinks)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_frontmatter",
		Description: "Read one note's frontmatter fields and its tags, without its body.",
	}, s.getFrontmatter)

	if !s.policy.ReadOnly {
		s.addWriteTools(srv)
	}
	if s.audit != nil {
		srv.AddReceivingMiddleware(auditMiddleware(s.audit))
	}
	return srv
}

// addWriteTools registers every tool that changes a note. A read-only
// server leaves them out, so clients never see them.
func (s *Server) addWriteTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "edit_section",
		Description: "Change one Markdown heading section without quoting its text or rewriting the whole note; the " +
			"heading and the rest of the note are kept. mode append adds lines after the section's own text, before " +
			"its first subheading, which is how to add an entry under a heading such as ## Log. mode prepend adds " +
			"lines right below the heading. mode replace replaces the whole body, subsections included, and needs " +
			"the version get_section returned, so it cannot remove text it has not seen. Any mode refuses the edit " +
			"when a given version no longer matches; changes elsewhere in the note do not count. heading_path is an " +
			"exact case-sensitive suffix of the heading hierarchy; missing or ambiguous matches fail.",
	}, s.editSection)

	mcp.AddTool(srv, &mcp.Tool{
		Name: "update_frontmatter",
		Description: "Add, replace or delete frontmatter fields on one note. Fields that are not mentioned keep their " +
			"value and order, and the note's body is left untouched. Returns the note's fields after the change.",
	}, s.updateFrontmatter)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "create_note",
		Description: "Create a new note. Parent directories are created automatically; fails if the note already exists.",
	}, s.createNote)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "append_note",
		Description: "Append content to the end of a note, creating it if it does not exist. The content starts on a new line.",
	}, s.appendNote)

	mcp.AddTool(srv, &mcp.Tool{
		Name: "edit_note",
		Description: "Edit a note by replacing an exact text snippet. The snippet must occur exactly once " +
			"unless replace_all is set; include surrounding lines to make it unique. Pass the version read_note returned " +
			"to refuse the edit if the note changed since; the result carries the new version for a following edit.",
	}, s.editNote)

	mcp.AddTool(srv, &mcp.Tool{
		Name: "move_note",
		Description: "Move or rename a note within a vault. Fails if the destination already exists. A new name breaks " +
			"[[links]] to the note unless update_links is set, which rewrites them in every note. It also restores " +
			"a deleted note: pass its path inside .trash, and leave new_path out to put it back where it was deleted " +
			"from. Use list_notes with dir \".trash\" and recursive to see deleted notes.",
	}, s.moveNote)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "delete_note",
		Description: s.deleteDescription(),
	}, s.deleteNote)
}

func (s *Server) deleteDescription() string {
	if s.policy.AllowPermanentDelete {
		return "Delete a note. By default it is moved to the vault's .trash folder (recoverable with move_note); " +
			"set permanent to remove it outright. Deleting a note inside .trash is always permanent."
	}
	return "Delete a note by moving it to the vault's .trash folder, recoverable with move_note. Permanent " +
		"deletion, including deleting a note already in .trash, is turned off on this server."
}

func (s *Server) vault(name string) (*vault.Vault, error) {
	// Most deployments serve one vault, and naming it on every call is
	// noise a model can get wrong. With several vaults there is nothing to
	// guess, so the error names them.
	if name == "" {
		if len(s.vaults) == 1 {
			for _, v := range s.vaults {
				return v, nil
			}
		}
		return nil, fmt.Errorf("name a vault: this server holds %s", strings.Join(s.vaultNames(), ", "))
	}
	v, ok := s.vaults[name]
	if !ok {
		return nil, fmt.Errorf("unknown vault %q: this server holds %s", name, strings.Join(s.vaultNames(), ", "))
	}
	return v, nil
}

// vaultNames returns the served vault names in a stable order.
func (s *Server) vaultNames() []string {
	names := make([]string, 0, len(s.vaults))
	for name := range s.vaults {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
