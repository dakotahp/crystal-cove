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
const Version = "0.8.0" // x-release-please-version

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

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_vaults",
		Description: "List the Obsidian vaults available on this server.",
	}, s.listVaults)

	mcp.AddTool(srv, &mcp.Tool{
		Name: "list_notes",
		Description: "List notes and directories in a vault. Hidden folders such as .obsidian and .trash are excluded, " +
			"but passing dir \".trash\" lists deleted notes explicitly. Other hidden folders cannot be listed.",
	}, s.listNotes)

	mcp.AddTool(srv, &mcp.Tool{
		Name: "read_note",
		Description: fmt.Sprintf("Read a note from a vault. Returns at most %d characters per call; "+
			"when the response is truncated, call again with offset set to next_offset to continue reading.", vault.ReadPageSize),
	}, s.readNote)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_section",
		Description: "Read a Markdown heading section's body, including nested subsections but excluding its heading. heading_path is an exact case-sensitive suffix of the heading hierarchy (Markdown title text without heading markers); ambiguous matches fail. Only document-level headings count, not headings in code, quotes, lists, or YAML frontmatter. Returns at most 10240 characters; continue with next_offset.",
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
		Name:        "replace_section",
		Description: "Replace a Markdown heading section's entire body, including nested subsections, while preserving its heading. content excludes the selected heading. heading_path is an exact case-sensitive suffix of the heading hierarchy; missing or ambiguous matches fail. The section ends at the next heading of equal or higher rank. Adds newline separation when needed before a following heading.",
	}, s.replaceSection)

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
		Description: "Append content to a note, creating it if it does not exist.",
	}, s.appendNote)

	mcp.AddTool(srv, &mcp.Tool{
		Name: "edit_note",
		Description: "Edit a note by replacing an exact text snippet. The snippet must occur exactly once " +
			"unless replace_all is set; include surrounding lines to make it unique.",
	}, s.editNote)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "move_note",
		Description: "Move or rename a note within a vault. Fails if the destination already exists.",
	}, s.moveNote)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "delete_note",
		Description: s.deleteDescription(),
	}, s.deleteNote)

	mcp.AddTool(srv, &mcp.Tool{
		Name: "restore_note",
		Description: "Restore (undelete) a note from the vault's .trash folder. Restores to the note's path inside .trash " +
			"unless to is set; use list_notes with dir \".trash\" to see what can be restored.",
	}, s.restoreNote)
}

func (s *Server) deleteDescription() string {
	if s.policy.AllowPermanentDelete {
		return "Delete a note. By default it is moved to the vault's .trash folder (recoverable with restore_note); " +
			"set permanent to remove it outright. Deleting a note inside .trash is always permanent."
	}
	return "Delete a note by moving it to the vault's .trash folder, recoverable with restore_note. Permanent " +
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
		return nil, fmt.Errorf("unknown vault %q: use list_vaults to see available vaults", name)
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
