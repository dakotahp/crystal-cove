package server

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dakotahp/crystal-cove/internal/notes"
	"github.com/dakotahp/crystal-cove/internal/semantic"
	"github.com/dakotahp/crystal-cove/internal/vault"
)

const (
	// DefaultSemanticLimit is how many notes a semantic tool returns when no
	// limit is given: closeness fades quickly down the list.
	DefaultSemanticLimit = 10
	excerptChars         = 300
)

// SetSemantic turns on the semantic tools, with one index per vault name.
// Call it before serving.
func (s *Server) SetSemantic(indexes map[string]*semantic.Index) { s.semantic = indexes }

const (
	grepNotesSemanticHint = " To find notes about a topic, including notes that use other words than the query, use search_notes."
	backlinksSemanticHint = " For notes on the same topic that do not link here, use related_notes."
)

// semanticHint returns hint when the semantic tools are registered, so
// other tools point at them only when they exist.
func (s *Server) semanticHint(hint string) string {
	if s.semantic == nil {
		return ""
	}
	return hint
}

func (s *Server) addSemanticTools(srv *mcp.Server, params map[string]string) {
	addTool(srv, params, &mcp.Tool{
		Name: "search_notes",
		Description: "Find notes about a topic or by name. Start here to find notes. This is hybrid search (keyword + " +
			"semantic): one call ranks notes by name, by key words (BM25, so rare words count most), by any phrase in " +
			"double quotes, and by meaning, so a note that uses other words than the query is found too: \"feeling " +
			"burned out\" can find a note about low energy. The rankings are merged: a note named after the query comes " +
			"first, then notes found in more ways. matched_by says how each note was found (name, phrase, keyword, " +
			"meaning). A note found by meaning alone is a weaker match, and when every top result is meaning only, the " +
			"vault may not hold what was asked for. Each note reports the section that shows the match as heading_path, " +
			"which get_section reads, and an excerpt of it. For exact text, a regex, every matching line, or a line to " +
			"edit, use grep_notes. index reports whether every note is indexed for meaning yet; key words work before " +
			"it is. folders and exclude_folders limit which notes are returned. Results come in pages: pass next_offset " +
			"as offset.",
	}, s.searchNotes)

	addTool(srv, params, &mcp.Tool{
		Name: "related_notes",
		Description: "List the notes closest in meaning to one note, including notes it does not link to, to find " +
			"connections its links miss. Notes are compared section by section, so a long note matches on its closest " +
			"section: heading_path is the other note's section and from_heading_path the section of the given note " +
			"that matched. linked marks notes that already link to it or that it links to; unlinked leaves those out " +
			"to show only new connections. get_links and get_backlinks list the links themselves. Results are ranked " +
			"by closeness and always list the nearest notes, so judge them by their excerpts. folders and " +
			"exclude_folders limit which notes are returned. Results come in pages: pass next_offset as offset.",
	}, s.relatedNotes)
}

type searchNotesInput struct {
	Vault string `json:"vault,omitempty" jsonschema:"name of the vault to search; optional when the server holds one vault"`
	Query string `json:"query" jsonschema:"a name, key words, or a description of the topic in plain words; wrap words in double quotes to require that exact phrase"`
	folderFilter
	Offset int `json:"offset,omitempty" jsonschema:"note to start from; use next_offset from the previous page"`
	Limit  int `json:"limit,omitempty" jsonschema:"maximum notes to return (default 10)"`
}

// FoundNote is one note search_notes found.
type FoundNote struct {
	Path        string   `json:"path"`
	HeadingPath []string `json:"heading_path,omitempty" jsonschema:"the section that shows the match, for get_section; empty for text before the first heading"`
	Excerpt     string   `json:"excerpt" jsonschema:"the start of that section"`
	MatchedBy   []string `json:"matched_by" jsonschema:"how the note was found: name, phrase, keyword, or meaning"`
}

// SearchNotesResult is the outcome of search_notes.
type SearchNotesResult struct {
	Notes      []FoundNote     `json:"notes"`
	Total      int             `json:"total" jsonschema:"notes across every page"`
	NextOffset int             `json:"next_offset" jsonschema:"offset of the next page; -1 when this is the last page"`
	Index      semantic.Status `json:"index"`
}

func (s *Server) searchNotes(ctx context.Context, _ *mcp.CallToolRequest, in searchNotesInput) (*mcp.CallToolResult, *SearchNotesResult, error) {
	v, ix, err := s.semanticIndex(in.Vault)
	if err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(in.Query) == "" {
		return nil, nil, errors.New("query is empty: give a name, key words, or the topic in plain words")
	}
	results, status, err := ix.Search(ctx, in.Query, in.includes)
	if err != nil {
		return nil, nil, err
	}
	if len(results) == 0 && !status.Complete {
		return nil, nil, stillBuilding(v, status)
	}
	found := make([]FoundNote, len(results))
	for i, r := range results {
		found[i] = FoundNote{Path: r.Path, HeadingPath: r.HeadingPath, Excerpt: excerpt(r.Text), MatchedBy: r.MatchedBy}
	}
	page, next, err := pageOf(found, in.Offset, in.Limit, DefaultSemanticLimit)
	if err != nil {
		return nil, nil, err
	}
	return nil, &SearchNotesResult{Notes: page, Total: len(found), NextOffset: next, Index: status}, nil
}

// SemanticNote is one note found by meaning.
type SemanticNote struct {
	Path        string   `json:"path"`
	HeadingPath []string `json:"heading_path,omitempty" jsonschema:"the section that matched best, for get_section; empty for text before the first heading"`
	Excerpt     string   `json:"excerpt" jsonschema:"the start of the section that matched"`
	Score       float64  `json:"score" jsonschema:"closeness in meaning, higher is closer; compare scores within one result, not with a fixed scale"`
}

type relatedNotesInput struct {
	Vault    string `json:"vault,omitempty" jsonschema:"name of the vault; optional when the server holds one vault"`
	Path     string `json:"path" jsonschema:"vault-relative path of the note to find related notes for"`
	Unlinked bool   `json:"unlinked,omitempty" jsonschema:"leave out notes that already link to or from this note"`
	folderFilter
	Offset int `json:"offset,omitempty" jsonschema:"note to start from; use next_offset from the previous page"`
	Limit  int `json:"limit,omitempty" jsonschema:"maximum notes to return (default 10)"`
}

// RelatedNote is one note close in meaning to the note asked about.
type RelatedNote struct {
	SemanticNote
	FromHeadingPath []string `json:"from_heading_path,omitempty" jsonschema:"the section of the given note that matched"`
	Linked          bool     `json:"linked" jsonschema:"true when either note already links to the other"`
}

// RelatedNotesResult is the outcome of related_notes.
type RelatedNotesResult struct {
	Path       string          `json:"path"`
	Notes      []RelatedNote   `json:"notes"`
	Total      int             `json:"total" jsonschema:"notes across every page"`
	NextOffset int             `json:"next_offset" jsonschema:"offset of the next page; -1 when this is the last page"`
	Index      semantic.Status `json:"index"`
}

func (s *Server) relatedNotes(_ context.Context, _ *mcp.CallToolRequest, in relatedNotesInput) (*mcp.CallToolResult, *RelatedNotesResult, error) {
	if _, err := s.note(in.Vault, in.Path); err != nil {
		return nil, nil, err
	}
	v, ix, err := s.semanticIndex(in.Vault)
	if err != nil {
		return nil, nil, err
	}
	linked, err := linkedNotes(v, in.Path)
	if err != nil {
		return nil, nil, err
	}
	matches, status, err := ix.Related(in.Path)
	if err != nil {
		if !status.Complete {
			return nil, nil, fmt.Errorf("%w; %w", err, stillBuilding(v, status))
		}
		return nil, nil, err
	}
	found := []RelatedNote{}
	for _, m := range matches {
		if !in.includes(m.Path) || (in.Unlinked && linked[m.Path]) {
			continue
		}
		found = append(found, RelatedNote{SemanticNote: semanticNote(m.Hit), FromHeadingPath: m.FromHeadingPath, Linked: linked[m.Path]})
	}
	page, next, err := pageOf(found, in.Offset, in.Limit, DefaultSemanticLimit)
	if err != nil {
		return nil, nil, err
	}
	return nil, &RelatedNotesResult{Path: in.Path, Notes: page, Total: len(found), NextOffset: next, Index: status}, nil
}

func (s *Server) semanticIndex(vaultName string) (*vault.Vault, *semantic.Index, error) {
	v, err := s.vault(vaultName)
	if err != nil {
		return nil, nil, err
	}
	ix, ok := s.semantic[v.Name()]
	if !ok {
		return nil, nil, fmt.Errorf("vault %q has no semantic index", v.Name())
	}
	return v, ix, nil
}

func stillBuilding(v *vault.Vault, status semantic.Status) error {
	return fmt.Errorf("the semantic index for vault %q is still being built (%d of %d notes so far); try again in a few minutes, or use grep_notes",
		v.Name(), status.NotesIndexed, status.NotesTotal)
}

// linkedNotes returns the notes that rel links to or that link to rel,
// resolved the way get_links and get_backlinks resolve them.
func linkedNotes(v *vault.Vault, rel string) (map[string]bool, error) {
	n, err := parseNote(v, rel)
	if err != nil {
		return nil, err
	}
	resolver, err := resolverFor(v)
	if err != nil {
		return nil, err
	}
	linked := map[string]bool{}
	for _, link := range n.Links() {
		if target, ok := resolver.Resolve(link.Target, rel); ok {
			linked[target] = true
		}
	}
	_, err = eachNote(v, func(path string, n *notes.Note) error {
		for _, link := range n.Links() {
			if target, ok := resolver.Resolve(link.Target, path); ok && target == rel {
				linked[path] = true
				return nil
			}
		}
		return nil
	})
	return linked, err
}

func semanticNote(h semantic.Hit) SemanticNote {
	return SemanticNote{
		Path:        h.Path,
		HeadingPath: h.HeadingPath,
		Excerpt:     excerpt(h.Text),
		Score:       math.Round(float64(h.Score)*1000) / 1000,
	}
}

func excerpt(text string) string {
	flat := []rune(strings.Join(strings.Fields(text), " "))
	if len(flat) <= excerptChars {
		return string(flat)
	}
	return string(flat[:excerptChars]) + "…"
}
