// Package server exposes vault operations as MCP tools over streamable
// HTTP, protected by a static bearer token.
package server

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"

	"github.com/andyjmorgan/obsidian-hosted-mcp/internal/search"
	"github.com/andyjmorgan/obsidian-hosted-mcp/internal/vault"
)

// Version is the server version reported to MCP clients.
const Version = "0.7.0"

// Server wires vaults and search into an MCP tool set.
type Server struct {
	vaults   map[string]*vault.Vault
	searcher *search.Searcher
	// syncReady reports whether every vault has a fresh sync heartbeat.
	syncReady func() bool
	// instructions is the vault guidance sent to clients, read once at
	// startup. An edit to a vault's InstructionsFile needs a restart.
	instructions string
}

// New returns a Server over the given vaults.
func New(vaults []*vault.Vault, searcher *search.Searcher, syncReady func() bool) *Server {
	m := make(map[string]*vault.Vault, len(vaults))
	for _, v := range vaults {
		m[v.Name()] = v
	}
	return &Server{
		vaults:       m,
		searcher:     searcher,
		syncReady:    syncReady,
		instructions: loadInstructions(vaults),
	}
}

// Instructions returns the vault guidance advertised to MCP clients.
func (s *Server) Instructions() string { return s.instructions }

// MCPServer builds the MCP server with all tools registered.
func (s *Server) MCPServer() *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "obsidian-hosted-mcp",
		Title:   "Obsidian Hosted MCP",
		Version: Version,
	}, &mcp.ServerOptions{Instructions: s.instructions})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_vaults",
		Description: "List the Obsidian vaults available on this server.",
	}, s.listVaults)

	mcp.AddTool(srv, &mcp.Tool{
		Name: "list_notes",
		Description: "List notes and directories in a vault. Hidden folders such as .obsidian and .trash are excluded, " +
			"but passing dir \".trash\" lists deleted notes explicitly.",
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
		Name:        "replace_section",
		Description: "Replace a Markdown heading section's entire body, including nested subsections, while preserving its heading. content excludes the selected heading. heading_path is an exact case-sensitive suffix of the heading hierarchy; missing or ambiguous matches fail. The section ends at the next heading of equal or higher rank. Adds newline separation when needed before a following heading.",
	}, s.replaceSection)

	mcp.AddTool(srv, &mcp.Tool{
		Name: "search_notes",
		Description: "Search a vault by note name and by content. A plain multi-word query finds notes holding every " +
			"word, in any order; a query with regular-expression characters is read as a regex (ripgrep syntax), and mode " +
			"forces either reading. Results are ranked: notes named after the query first, flagged title_match, then notes " +
			"covering more of the query's words, then notes with more matching lines. Case-insensitive unless " +
			"case_sensitive is set; max_lines_per_note keeps one long note from filling the result.",
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
		Name:        "get_frontmatter",
		Description: "Read one note's frontmatter fields and its tags, without its body.",
	}, s.getFrontmatter)

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
		Name: "delete_note",
		Description: "Delete a note. By default it is moved to the vault's .trash folder (recoverable with restore_note); " +
			"set permanent to remove it outright.",
	}, s.deleteNote)

	mcp.AddTool(srv, &mcp.Tool{
		Name: "restore_note",
		Description: "Restore (undelete) a note from the vault's .trash folder. Restores to the note's path inside .trash " +
			"unless to is set; use list_notes with dir \".trash\" to see what can be restored.",
	}, s.restoreNote)

	return srv
}

// metadataPath is where RFC 9728 protected-resource metadata is served
// when OIDC delegation is enabled.
const metadataPath = "/.well-known/oauth-protected-resource"

// OIDCAuth enables delegating bearer-token validation to a third-party
// OpenID Connect identity provider.
type OIDCAuth struct {
	// Verify validates a provider-issued token (see internal/oidcauth).
	Verify func(ctx context.Context, token string) (*auth.TokenInfo, error)
	// Issuer is advertised to MCP clients as the authorization server.
	Issuer string
	// Scopes are advertised as scopes_supported.
	Scopes []string
	// PublicURL is this server's canonical external URL — the protected
	// resource identifier.
	PublicURL string
}

// AuthConfig selects how MCP requests are authenticated: a static bearer
// token (API key), a third-party OIDC provider, or both side by side.
type AuthConfig struct {
	StaticToken string
	OIDC        *OIDCAuth
}

// Handler returns the HTTP handler: process liveness at /livez, sync-aware
// readiness at /readyz and its backwards-compatible /healthz alias, RFC 9728
// protected-resource metadata when OIDC is enabled, and the bearer-protected
// MCP endpoint everywhere else.
func (s *Server) Handler(authCfg AuthConfig) http.Handler {
	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return s.MCPServer()
	}, nil)
	mux := http.NewServeMux()
	mux.HandleFunc("/livez", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})
	ready := func(w http.ResponseWriter, _ *http.Request) {
		if s.syncReady == nil || !s.syncReady() {
			http.Error(w, "sync not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	}
	mux.HandleFunc("/readyz", ready)
	mux.HandleFunc("/healthz", ready)
	opts := &auth.RequireBearerTokenOptions{}
	if authCfg.OIDC != nil {
		opts.ResourceMetadataURL = authCfg.OIDC.PublicURL + metadataPath
		mux.Handle(metadataPath, auth.ProtectedResourceMetadataHandler(&oauthex.ProtectedResourceMetadata{
			Resource:               authCfg.OIDC.PublicURL,
			AuthorizationServers:   []string{authCfg.OIDC.Issuer},
			ScopesSupported:        authCfg.OIDC.Scopes,
			BearerMethodsSupported: []string{"header"},
		}))
	}
	mux.Handle("/", auth.RequireBearerToken(verifyToken(authCfg), opts)(mcpHandler))
	return mux
}

// verifyToken accepts the static token (constant-time compare) when one is
// configured, then falls back to the OIDC verifier when one is configured.
func verifyToken(cfg AuthConfig) auth.TokenVerifier {
	return func(ctx context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		if cfg.StaticToken != "" &&
			subtle.ConstantTimeCompare([]byte(token), []byte(cfg.StaticToken)) == 1 {
			// Static keys do not expire; the middleware requires a bound.
			return &auth.TokenInfo{Expiration: time.Now().Add(time.Hour)}, nil
		}
		if cfg.OIDC != nil {
			return cfg.OIDC.Verify(ctx, token)
		}
		return nil, fmt.Errorf("%w: unknown bearer token", auth.ErrInvalidToken)
	}
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

type listVaultsOutput struct {
	Vaults []string `json:"vaults" jsonschema:"names of the available vaults"`
}

func (s *Server) listVaults(context.Context, *mcp.CallToolRequest, any) (*mcp.CallToolResult, listVaultsOutput, error) {
	names := make([]string, 0, len(s.vaults))
	for name := range s.vaults {
		names = append(names, name)
	}
	sort.Strings(names)
	return nil, listVaultsOutput{Vaults: names}, nil
}

type listNotesInput struct {
	Vault     string `json:"vault,omitempty" jsonschema:"name of the vault to list; optional when the server holds one vault"`
	Dir       string `json:"dir,omitempty" jsonschema:"vault-relative directory to list; defaults to the vault root"`
	Recursive bool   `json:"recursive,omitempty" jsonschema:"list subdirectories recursively"`
}

type listNotesOutput struct {
	Entries []vault.Entry `json:"entries" jsonschema:"files and directories found"`
}

func (s *Server) listNotes(_ context.Context, _ *mcp.CallToolRequest, in listNotesInput) (*mcp.CallToolResult, listNotesOutput, error) {
	v, err := s.vault(in.Vault)
	if err != nil {
		return nil, listNotesOutput{}, err
	}
	entries, err := v.List(in.Dir, in.Recursive)
	if err != nil {
		return nil, listNotesOutput{}, err
	}
	return nil, listNotesOutput{Entries: entries}, nil
}

type readNoteInput struct {
	Vault  string `json:"vault,omitempty" jsonschema:"name of the vault; optional when the server holds one vault"`
	Path   string `json:"path" jsonschema:"vault-relative path of the note"`
	Offset int    `json:"offset,omitempty" jsonschema:"character offset to start reading from; use next_offset from a previous truncated read"`
}

func (s *Server) readNote(_ context.Context, _ *mcp.CallToolRequest, in readNoteInput) (*mcp.CallToolResult, *vault.ReadResult, error) {
	v, err := s.vault(in.Vault)
	if err != nil {
		return nil, nil, err
	}
	res, err := v.Read(in.Path, in.Offset)
	if err != nil {
		return nil, nil, err
	}
	return nil, res, nil
}

type searchNotesInput struct {
	Vault           string `json:"vault,omitempty" jsonschema:"name of the vault to search; optional when the server holds one vault"`
	Query           string `json:"query" jsonschema:"words to look for, or a regular expression in ripgrep syntax"`
	Mode            string `json:"mode,omitempty" jsonschema:"how to read the query: words (every word, any order), regex, or auto (the default: regex when the query holds regex characters, words otherwise)"`
	MaxLinesPerNote int    `json:"max_lines_per_note,omitempty" jsonschema:"cap the lines returned for any one note, so a long note cannot fill the result"`
	Glob            string `json:"glob,omitempty" jsonschema:"restrict the search to paths matching this glob, e.g. *.md or daily/**"`
	CaseSensitive   bool   `json:"case_sensitive,omitempty" jsonschema:"match case exactly instead of the default case-insensitive search"`
	ContextLines    int    `json:"context_lines,omitempty" jsonschema:"lines of context to include around each match"`
	MaxResults      int    `json:"max_results,omitempty" jsonschema:"maximum matching lines to return (default 50, max 500)"`
}

func (s *Server) searchNotes(ctx context.Context, _ *mcp.CallToolRequest, in searchNotesInput) (*mcp.CallToolResult, *search.Result, error) {
	v, err := s.vault(in.Vault)
	if err != nil {
		return nil, nil, err
	}
	mode, err := searchMode(in.Mode)
	if err != nil {
		return nil, nil, err
	}
	opts := search.Options{
		Query:           in.Query,
		Mode:            mode,
		MaxLinesPerFile: in.MaxLinesPerNote,
		Glob:            in.Glob,
		CaseSensitive:   in.CaseSensitive,
		ContextLines:    in.ContextLines,
		MaxResults:      in.MaxResults,
	}
	res, err := s.searcher.Search(ctx, v.Root(), opts)
	if err != nil {
		return nil, nil, err
	}
	limit := in.MaxResults
	if limit <= 0 {
		limit = search.DefaultMaxResults
	}
	words := search.QueryWords(opts)
	titles, truncated, err := matchTitles(v, in.Query, words, in.CaseSensitive, limit)
	if err != nil {
		return nil, nil, err
	}
	res = withTitleMatches(res, titles, truncated)
	rankFiles(res, in.Query, words)
	return nil, res, nil
}

// searchMode converts the tool's mode argument, naming the choices when it
// is not one of them.
func searchMode(mode string) (search.Mode, error) {
	switch search.Mode(mode) {
	case search.ModeAuto, search.ModeWords, search.ModeRegex:
		return search.Mode(mode), nil
	default:
		return "", fmt.Errorf("mode %q is not one of auto, words or regex", mode)
	}
}

// matchTitles finds notes by name the same way the content search read the
// query: every word in any order, or the raw pattern for a regex query.
func matchTitles(v *vault.Vault, query string, words []string, caseSensitive bool, limit int) ([]string, bool, error) {
	if len(words) > 0 {
		return v.MatchTitleWords(words, caseSensitive, limit)
	}
	return v.MatchTitles(query, caseSensitive, limit)
}

// withTitleMatches moves the notes whose name matched to the front of the
// result, keeping any body matches they also have. A note matched only by
// name is added with no lines.
func withTitleMatches(res *search.Result, titles []string, truncated bool) *search.Result {
	if len(titles) == 0 {
		return res
	}
	byPath := make(map[string]search.FileMatches, len(res.Files))
	for _, f := range res.Files {
		byPath[f.Path] = f
	}
	files := make([]search.FileMatches, 0, len(res.Files)+len(titles))
	for _, path := range titles {
		f, ok := byPath[path]
		if !ok {
			f = search.FileMatches{Path: path, Lines: []search.Line{}}
		}
		f.TitleMatch = true
		files = append(files, f)
		delete(byPath, path)
	}
	for _, f := range res.Files {
		if _, ok := byPath[f.Path]; ok {
			files = append(files, f)
		}
	}
	res.Files = files
	res.Truncated = res.Truncated || truncated
	return res
}

type writeNoteInput struct {
	Vault   string `json:"vault,omitempty" jsonschema:"name of the vault; optional when the server holds one vault"`
	Path    string `json:"path" jsonschema:"vault-relative path of the note"`
	Content string `json:"content" jsonschema:"markdown content"`
}

type okOutput struct {
	OK bool `json:"ok"`
}

func (s *Server) createNote(_ context.Context, _ *mcp.CallToolRequest, in writeNoteInput) (*mcp.CallToolResult, okOutput, error) {
	v, err := s.vault(in.Vault)
	if err != nil {
		return nil, okOutput{}, err
	}
	if err := v.Create(in.Path, in.Content); err != nil {
		return nil, okOutput{}, err
	}
	return nil, okOutput{OK: true}, nil
}

func (s *Server) appendNote(_ context.Context, _ *mcp.CallToolRequest, in writeNoteInput) (*mcp.CallToolResult, okOutput, error) {
	v, err := s.vault(in.Vault)
	if err != nil {
		return nil, okOutput{}, err
	}
	if err := v.Append(in.Path, in.Content); err != nil {
		return nil, okOutput{}, err
	}
	return nil, okOutput{OK: true}, nil
}

type editNoteInput struct {
	Vault      string `json:"vault,omitempty" jsonschema:"name of the vault; optional when the server holds one vault"`
	Path       string `json:"path" jsonschema:"vault-relative path of the note"`
	Find       string `json:"find" jsonschema:"exact text to replace; must occur exactly once unless replace_all is set"`
	Replace    string `json:"replace" jsonschema:"replacement text"`
	ReplaceAll bool   `json:"replace_all,omitempty" jsonschema:"replace every occurrence instead of requiring a unique match"`
}

type editNoteOutput struct {
	Replacements int `json:"replacements" jsonschema:"number of replacements made"`
}

func (s *Server) editNote(_ context.Context, _ *mcp.CallToolRequest, in editNoteInput) (*mcp.CallToolResult, editNoteOutput, error) {
	v, err := s.vault(in.Vault)
	if err != nil {
		return nil, editNoteOutput{}, err
	}
	n, err := v.Edit(in.Path, in.Find, in.Replace, in.ReplaceAll)
	if err != nil {
		return nil, editNoteOutput{}, err
	}
	return nil, editNoteOutput{Replacements: n}, nil
}

type moveNoteInput struct {
	Vault   string `json:"vault,omitempty" jsonschema:"name of the vault; optional when the server holds one vault"`
	Path    string `json:"path" jsonschema:"current vault-relative path of the note"`
	NewPath string `json:"new_path" jsonschema:"destination vault-relative path"`
}

func (s *Server) moveNote(_ context.Context, _ *mcp.CallToolRequest, in moveNoteInput) (*mcp.CallToolResult, okOutput, error) {
	v, err := s.vault(in.Vault)
	if err != nil {
		return nil, okOutput{}, err
	}
	if err := v.Move(in.Path, in.NewPath); err != nil {
		return nil, okOutput{}, err
	}
	return nil, okOutput{OK: true}, nil
}

type deleteNoteInput struct {
	Vault     string `json:"vault,omitempty" jsonschema:"name of the vault; optional when the server holds one vault"`
	Path      string `json:"path" jsonschema:"vault-relative path of the note"`
	Permanent bool   `json:"permanent,omitempty" jsonschema:"remove the note outright instead of moving it to .trash"`
}

type deleteNoteOutput struct {
	OK bool `json:"ok"`
	// TrashedTo is the vault-relative path the note was moved to inside
	// .trash; empty for permanent deletions.
	TrashedTo string `json:"trashed_to,omitempty" jsonschema:"where the note was moved inside .trash; empty when deleted permanently"`
}

type restoreNoteInput struct {
	Vault string `json:"vault,omitempty" jsonschema:"name of the vault; optional when the server holds one vault"`
	Path  string `json:"path" jsonschema:"vault-relative path of the note inside .trash"`
	To    string `json:"to,omitempty" jsonschema:"destination vault-relative path; defaults to the note's path inside .trash"`
}

type restoreNoteOutput struct {
	OK bool `json:"ok"`
	// RestoredTo is the vault-relative path the note was restored to.
	RestoredTo string `json:"restored_to" jsonschema:"vault-relative path the note was restored to"`
}

func (s *Server) restoreNote(_ context.Context, _ *mcp.CallToolRequest, in restoreNoteInput) (*mcp.CallToolResult, restoreNoteOutput, error) {
	v, err := s.vault(in.Vault)
	if err != nil {
		return nil, restoreNoteOutput{}, err
	}
	restoredTo, err := v.Restore(in.Path, in.To)
	if err != nil {
		return nil, restoreNoteOutput{}, err
	}
	return nil, restoreNoteOutput{OK: true, RestoredTo: restoredTo}, nil
}

func (s *Server) deleteNote(_ context.Context, _ *mcp.CallToolRequest, in deleteNoteInput) (*mcp.CallToolResult, deleteNoteOutput, error) {
	v, err := s.vault(in.Vault)
	if err != nil {
		return nil, deleteNoteOutput{}, err
	}
	trashedTo, err := v.Delete(in.Path, in.Permanent)
	if err != nil {
		return nil, deleteNoteOutput{}, err
	}
	return nil, deleteNoteOutput{OK: true, TrashedTo: trashedTo}, nil
}

type getSectionInput struct {
	Vault       string   `json:"vault,omitempty" jsonschema:"name of the vault; optional when the server holds one vault"`
	Path        string   `json:"path" jsonschema:"vault-relative path of the note"`
	HeadingPath []string `json:"heading_path" jsonschema:"one or more exact heading titles from ancestor to target; a unique suffix of the full hierarchy"`
	Offset      int      `json:"offset,omitempty" jsonschema:"character offset within the section body; defaults to zero"`
}

func (s *Server) getSection(_ context.Context, _ *mcp.CallToolRequest, in getSectionInput) (*mcp.CallToolResult, *vault.SectionResult, error) {
	v, err := s.vault(in.Vault)
	if err != nil {
		return nil, nil, err
	}
	out, err := v.GetSection(in.Path, in.HeadingPath, in.Offset)
	return nil, out, err
}

type replaceSectionInput struct {
	Vault       string   `json:"vault,omitempty" jsonschema:"name of the vault; optional when the server holds one vault"`
	Path        string   `json:"path" jsonschema:"vault-relative path of the note"`
	HeadingPath []string `json:"heading_path" jsonschema:"one or more exact heading titles from ancestor to target; a unique suffix of the full hierarchy"`
	Content     string   `json:"content" jsonschema:"replacement body including any desired subsections, without the selected heading; empty clears the body"`
}

func (s *Server) replaceSection(_ context.Context, _ *mcp.CallToolRequest, in replaceSectionInput) (*mcp.CallToolResult, okOutput, error) {
	v, err := s.vault(in.Vault)
	if err != nil {
		return nil, okOutput{}, err
	}
	if err := v.ReplaceSection(in.Path, in.HeadingPath, in.Content); err != nil {
		return nil, okOutput{}, err
	}
	return nil, okOutput{OK: true}, nil
}
