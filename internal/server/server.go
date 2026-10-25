// Package server exposes vault operations as MCP tools over streamable
// HTTP, protected by a static bearer token.
package server

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dakotahp/crystal-cove/internal/search"
	"github.com/dakotahp/crystal-cove/internal/semantic"
	"github.com/dakotahp/crystal-cove/internal/vault"
)

// Version is the server version reported to MCP clients.
const Version = "0.22.0" // x-release-please-version

// Commit is the source commit the binary was built from, set at build time
// with -ldflags "-X .../internal/server.Commit=<sha>". Version changes only
// at a release, so Commit is what tells two builds between releases apart.
var Commit = "dev"

// BuildInfo names the running build, for the startup log and /health.
func BuildInfo() string {
	return fmt.Sprintf("crystal-cove %s (commit %s)", Version, Commit)
}

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
	// timezone decides which day is today for the daily note tools.
	timezone *time.Location
	now      func() time.Time
	// semantic holds each vault's index when semantic search is on.
	semantic map[string]*semantic.Index
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

// SetTimezone sets the time zone the daily note tools take today from.
// Call it before serving; without it, today is the UTC day.
func (s *Server) SetTimezone(loc *time.Location) { s.timezone = loc }

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
		timezone:  time.UTC,
		now:       time.Now,
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
	params := map[string]string{}

	if len(s.vaults) > 1 {
		addTool(srv, params, &mcp.Tool{
			Name:        "list_vaults",
			Description: "List the Obsidian vaults available on this server.",
		}, s.listVaults)
	}

	addTool(srv, params, &mcp.Tool{
		Name: "list_notes",
		Description: "List notes and directories in a vault. Hidden folders such as .obsidian and .trash are excluded, " +
			"but passing dir \".trash\" with recursive lists deleted notes, which keep their folders there. Other " +
			"hidden folders cannot be listed. Returns at " +
			"most 200 entries unless limit says otherwise (max 1000); when next_offset is not -1, call again with offset " +
			"set to it, or narrow the listing with dir. Entries come in path order; order desc reverses it, so " +
			"{\"dir\": \"Session Logs\", \"order\": \"desc\", \"limit\": 3} lists the three newest notes named by date.",
	}, s.listNotes)

	addTool(srv, params, &mcp.Tool{
		Name: "read_note",
		Description: fmt.Sprintf("Read a note from a vault. Returns at most %d characters per call; "+
			"when the response is truncated, call again with offset set to next_offset to continue reading. The page "+
			"size is fixed: there is no limit parameter. version "+
			"identifies the note's text; pass it to edit_note so the edit is refused if the note changed since, "+
			"for example through sync from another device.", vault.ReadPageSize),
	}, s.readNote)

	readNotesTool := &mcp.Tool{
		Name: "read_notes",
		Description: fmt.Sprintf("Read several notes in one call, such as every note find_notes returned. Each note "+
			"returns its first page of up to %d characters with its version, in the order asked, and the call returns "+
			"at most %d characters in all. A note cut short by either limit reports truncated and next_offset: continue "+
			"it with read_note. A note that cannot be read reports only its path and error, and the others are still "+
			"returned. At most %d paths. max_chars returns only the start of each body, after the whole frontmatter, "+
			"to skim notes cheaply: {\"paths\": [\"Ideas/a.md\", \"Ideas/b.md\"], \"max_chars\": 500}.",
			vault.ReadPageSize, MaxReadNotesChars, MaxReadNotes),
	}
	if schema := readNotesOutputSchema(); schema != nil {
		readNotesTool.OutputSchema = schema
	}
	addTool(srv, params, readNotesTool, s.readNotes)

	addTool(srv, params, &mcp.Tool{
		Name:        "get_section",
		Description: "Read a Markdown heading section's body, including nested subsections but excluding its heading. heading_path is an exact case-sensitive suffix of the heading hierarchy (Markdown title text without heading markers); ambiguous matches fail, and a heading that is not found fails with the note's heading paths, closest first, to retry with. Only document-level headings count, not headings in code, quotes, lists, or YAML frontmatter. Returns at most 10240 characters; continue with next_offset. version identifies this section's text, for edit_section.",
	}, s.getSection)

	addTool(srv, params, &mcp.Tool{
		Name: "grep_notes",
		Description: "Exact search of a vault's note names and content, like grep. A plain multi-word query finds notes holding every " +
			"word, in any order; a query wrapped in double quotes, or mode phrase, finds that exact text with no escaping " +
			"needed; a query with regular-expression characters is read as a regex (ripgrep syntax). mode forces any " +
			"reading. Only Markdown notes are searched. Results are ranked: notes named after the query first, " +
			"flagged title_match, then notes with more matching lines. A note whose name matches is a result even when " +
			"no line in it matches; it reports total_matches 0. glob limits name matches as well as content matches. Case-insensitive unless " +
			"case_sensitive is set. limit counts notes, and each note returns at most 5 matching lines unless max_lines_per_note says otherwise; every note reports its own total_matches. " +
			"Results come in pages: total counts every matching note, and next_offset, passed as offset, gives the next page. " +
			"When the first page holds nothing useful, a better query usually beats the next page. " +
			"A line longer than 500 characters is cut to a window around the match and flagged text_truncated. " +
			"output files leaves the lines out, which is the cheap way to learn which notes match before reading one." +
			s.semanticHint(grepNotesSemanticHint),
	}, s.grepNotes)

	addTool(srv, params, &mcp.Tool{
		Name: "recent_notes",
		Description: "List the notes changed most recently, newest first, with their modified time. Pass since as an " +
			"RFC 3339 timestamp to see only what changed after it. Use this to pick up where work left off, which a " +
			"path-ordered listing cannot answer.",
	}, s.recentNotes)

	addTool(srv, params, &mcp.Tool{
		Name: "list_tags",
		Description: "List every tag used in a vault with the number of notes carrying it, most used first. Tags come " +
			"from the frontmatter tags field and from inline hashtags. Use it to learn what a vault is about before searching; " +
			"a small limit, such as 20, gives the main topics. Long lists come in pages: pass next_offset as offset for the next one.",
	}, s.listTags)

	addTool(srv, params, &mcp.Tool{
		Name: "find_notes",
		Description: "Find notes by metadata rather than text. Give at least one filter; a note must pass all of them. " +
			"tags is a list, and a note must carry every tag in it; frontmatter_key names a frontmatter field the note " +
			"must have, and frontmatter_value, when given, is the value that field must hold (a list field matches when " +
			"any item does); missing_key names a field the note must not have; min_tags and max_tags bound how many tags " +
			"it carries, counted as list_tags counts them; folders and exclude_folders limit where it lives, and folders " +
			"alone lists a folder's notes. fields returns those frontmatter fields with each note. " +
			"Example: {\"tags\": [\"project\"], \"frontmatter_key\": \"status\", \"frontmatter_value\": \"active\"}, or " +
			"{\"folders\": [\"Projects\", \"Areas\"], \"max_tags\": 2, \"fields\": [\"summary\"]}. " +
			"There is no query parameter: use grep_notes for words in the text.",
	}, s.findNotes)

	addTool(srv, params, &mcp.Tool{
		Name: "get_links",
		Description: "List the wikilinks a note points at, each resolved to the note it names, or flagged unresolved " +
			"when no such note exists yet. Headings, aliases and embeds are reported as written.",
	}, s.getLinks)

	addTool(srv, params, &mcp.Tool{
		Name: "get_backlinks",
		Description: "List the notes that link to a note, with the links they use. This is how notes relate to each " +
			"other in Obsidian, so use it to find the context around a note rather than searching for its name." +
			s.semanticHint(backlinksSemanticHint),
	}, s.getBacklinks)

	addTool(srv, params, &mcp.Tool{
		Name: "unresolved_links",
		Description: "List links across the vault that point at no file, which is how to find dead links without " +
			"calling get_links on every note. Links resolve the way Obsidian resolves them: by name or by the end " +
			"of a path, ignoring case, to notes and to any other file such as images, canvases and bases. A link " +
			"without an extension reaches only a note, so [[Board]] does not reach Board.canvas. Wikilinks, " +
			"Markdown links to files, and wikilinks in frontmatter count; links in code do not. folders and " +
			"exclude_folders limit which notes are checked. Example: {\"exclude_folders\": [\"Templates\"], \"limit\": 100}.",
	}, s.unresolvedLinks)

	addTool(srv, params, &mcp.Tool{
		Name: "orphans",
		Description: "List notes that no other note links to, as Obsidian's orphans do, with how many files each " +
			"links to. unlinked keeps only notes that link to nothing either. Links resolve as in unresolved_links, " +
			"and a note linking to itself does not count. folders and exclude_folders choose which notes are " +
			"reported; links from every note still count. Example: {\"folders\": [\"Projects\"], \"unlinked\": true}.",
	}, s.orphans)

	addTool(srv, params, &mcp.Tool{
		Name: "get_frontmatter",
		Description: "Read one note's frontmatter fields and its tags, without its body. Fields are listed " +
			"alphabetically; read_note shows their order in the file.",
	}, s.getFrontmatter)

	addTool(srv, params, &mcp.Tool{
		Name: "daily_note_path",
		Description: "Give the path of a day's daily note, and whether it exists yet, without creating it. The path " +
			"follows the vault's Obsidian Daily notes settings: its folder and date format. date picks the day, as " +
			"YYYY-MM-DD or today, yesterday or tomorrow; it defaults to today in the server's time zone.",
	}, s.dailyNotePath)

	readDailyTool := &mcp.Tool{
		Name: "read_daily_note",
		Description: fmt.Sprintf("Read a day's daily note, found from the vault's Obsidian Daily notes settings; "+
			"date picks the day as in daily_note_path. A note not created yet returns exists false and its path, and "+
			"is not created. Returns at most %d characters per call; continue with offset set to next_offset. version "+
			"is the same one read_note gives, for edit_note.", vault.ReadPageSize),
	}
	if schema := readDailyNoteOutputSchema(); schema != nil {
		readDailyTool.OutputSchema = schema
	}
	addTool(srv, params, readDailyTool, s.readDailyNote)

	if s.semantic != nil {
		s.addSemanticTools(srv, params)
	}
	if !s.policy.ReadOnly {
		s.addWriteTools(srv, params)
	}
	srv.AddReceivingMiddleware(argumentHints(params))
	if s.audit != nil {
		srv.AddReceivingMiddleware(auditMiddleware(s.audit))
	}
	return srv
}

// addWriteTools registers every tool that changes a note. A read-only
// server leaves them out, so clients never see them.
func (s *Server) addWriteTools(srv *mcp.Server, params map[string]string) {
	addTool(srv, params, &mcp.Tool{
		Name: "edit_section",
		Description: "Change one Markdown heading section without quoting its text or rewriting the whole note; the " +
			"heading and the rest of the note are kept. mode append adds lines after the section's own text, before " +
			"its first subheading, which is how to add an entry under a heading such as ## Log. mode prepend adds " +
			"lines right below the heading. mode replace replaces the whole body, subsections included, and needs " +
			"the version get_section returned, so it cannot remove text it has not seen. Any mode refuses the edit " +
			"when a given version no longer matches; changes elsewhere in the note do not count. heading_path is an " +
			"exact case-sensitive suffix of the heading hierarchy; ambiguous matches fail, and a missing heading fails " +
			"with the note's heading paths, closest first.",
	}, s.editSection)

	addTool(srv, params, &mcp.Tool{
		Name: "update_frontmatter",
		Description: "Add, replace or delete frontmatter fields on one note. Fields that are not mentioned keep their " +
			"value and order, and the note's body is left untouched. add_tags and remove_tags change the tags field " +
			"without sending the whole list, so a tag added on another device is not lost: tags compare without case " +
			"or #, new ones go at the end, a list keeps its style, and a tags field written as text becomes a list. " +
			"Example: {\"path\": \"Ideas/Bridge.md\", \"add_tags\": [\"design\"], \"remove_tags\": [\"draft\"]}. " +
			"Returns the note's fields after the change, listed alphabetically; read_note shows their order in the file.",
	}, s.updateFrontmatter)

	addTool(srv, params, &mcp.Tool{
		Name:        "create_note",
		Description: "Create a new note. Parent directories are created automatically; fails if the note already exists. Returns the new note's version, the same one read_note would give, so a following edit_note needs no read first.",
	}, s.createNote)

	addTool(srv, params, &mcp.Tool{
		Name:        "append_note",
		Description: "Append content to the end of a note, creating it if it does not exist. The content starts on a new line.",
	}, s.appendNote)

	addTool(srv, params, &mcp.Tool{
		Name: "append_daily_note",
		Description: "Add content to the end of a day's daily note, on a new line unless inline is set. date picks the " +
			"day as in daily_note_path, so a past or future day's note works too. A note that does not exist yet is " +
			"created first, as Obsidian creates it: in the Daily notes folder, from the Daily notes template, with " +
			"{{title}}, {{date}} and {{time}} filled in for that day. created in the result says whether it was new.",
	}, s.appendDailyNote)

	addTool(srv, params, &mcp.Tool{
		Name: "prepend_daily_note",
		Description: "Add content to the start of a day's daily note, below its frontmatter, followed by a line " +
			"break unless inline is set. date and creating a missing note work as in append_daily_note.",
	}, s.prependDailyNote)

	addTool(srv, params, &mcp.Tool{
		Name: "edit_note",
		Description: "Edit a note by replacing exact text snippets. edits is a list of {find, replace, replace_all}, " +
			"applied in order, each to the text the edits before it left; make several changes to one note in one call. " +
			"Each find must occur exactly once unless replace_all is set; include surrounding lines to make it unique. " +
			"If any edit fails, nothing is written and the error names the edit. Pass the version read_note returned " +
			"to refuse the edits if the note changed since; the result carries the new version for a following edit.",
	}, s.editNote)

	addTool(srv, params, &mcp.Tool{
		Name: "move_note",
		Description: "Move or rename a note within a vault. Fails if the destination already exists. A new name breaks " +
			"links to the note unless update_links is set, which rewrites wikilinks and Markdown links in every note, " +
			"each in its own form: a [[Name]] link stays a bare name unless another note shares the new name, a " +
			"[[folder/Name]] link gets the new path, and a ./ or ../ link stays relative. Relative links inside the " +
			"moved note are rewritten so they still reach their files. " +
			"links_updated in the result says whether links were rewritten. It also restores " +
			"a deleted note: pass its path inside .trash, and leave new_path out to put it back where it was deleted " +
			"from. Use list_notes with dir \".trash\" and recursive to see deleted notes. To move every note in a " +
			"folder, use move_folder instead: one move_note per note leaves the emptied folders behind.",
	}, s.moveNote)

	addTool(srv, params, &mcp.Tool{
		Name: "move_folder",
		Description: "Move or rename a folder with everything in it: notes, attachments and other files of any type, " +
			"and its subfolders, keeping their layout. Use it to archive or reorganize a whole folder in one call. " +
			"Fails if new_path exists, if new_path is inside path, or if either is the vault root or a hidden folder " +
			"such as .trash. update_links rewrites links to every moved file with the same rules as move_note, and " +
			"keeps relative links inside the moved notes working. The parent folders it leaves are kept, even when empty.",
	}, s.moveFolder)

	addTool(srv, params, &mcp.Tool{
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
