package server

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dakotahp/crystal-cove/internal/notes"
	"github.com/dakotahp/crystal-cove/internal/vault"
)

// DefaultNoteResults caps find_notes when the caller gives no limit.
const DefaultNoteResults = 50

type listTagsInput struct {
	Vault string `json:"vault,omitempty" jsonschema:"the vault to scan; optional when the server holds one vault"`
}

// TagCount is one tag and how many notes carry it.
type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

// TagList is the outcome of list_tags.
type TagList struct {
	// Tags are ordered by count, most used first, then alphabetically.
	Tags []TagCount `json:"tags"`
	// NotesScanned is how many notes were read to build the list.
	NotesScanned int `json:"notes_scanned"`
}

func (s *Server) listTags(_ context.Context, _ *mcp.CallToolRequest, in listTagsInput) (*mcp.CallToolResult, *TagList, error) {
	v, err := s.vault(in.Vault)
	if err != nil {
		return nil, nil, err
	}
	counts := map[string]int{}
	scanned, err := eachNote(v, func(_ string, n *notes.Note) error {
		for _, tag := range n.Tags {
			counts[tag]++
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	out := &TagList{Tags: make([]TagCount, 0, len(counts)), NotesScanned: scanned}
	for tag, count := range counts {
		out.Tags = append(out.Tags, TagCount{Tag: tag, Count: count})
	}
	sort.Slice(out.Tags, func(i, j int) bool {
		if out.Tags[i].Count != out.Tags[j].Count {
			return out.Tags[i].Count > out.Tags[j].Count
		}
		return out.Tags[i].Tag < out.Tags[j].Tag
	})
	return nil, out, nil
}

type findNotesInput struct {
	Vault string   `json:"vault,omitempty" jsonschema:"the vault to search; optional when the server holds one vault"`
	Tags  []string `json:"tags,omitempty" jsonschema:"notes must carry every tag listed"`
	Key   string   `json:"frontmatter_key,omitempty" jsonschema:"notes must carry this frontmatter field"`
	Value string   `json:"frontmatter_value,omitempty" jsonschema:"the field must equal this value; omit to match any value"`
	// MaxResults caps the notes returned; zero means DefaultNoteResults.
	MaxResults int `json:"max_results,omitempty" jsonschema:"maximum notes to return"`
}

// NoteMeta identifies one matching note.
type NoteMeta struct {
	Path string   `json:"path"`
	Tags []string `json:"tags"`
}

// NoteList is the outcome of find_notes.
type NoteList struct {
	Notes []NoteMeta `json:"notes"`
	// NotesScanned is how many notes were read.
	NotesScanned int `json:"notes_scanned"`
	// Truncated reports whether MaxResults cut the list off.
	Truncated bool `json:"truncated"`
}

func (s *Server) findNotes(_ context.Context, _ *mcp.CallToolRequest, in findNotesInput) (*mcp.CallToolResult, *NoteList, error) {
	v, err := s.vault(in.Vault)
	if err != nil {
		return nil, nil, err
	}
	if len(in.Tags) == 0 && in.Key == "" {
		return nil, nil, errors.New("give tags, frontmatter_key, or both: find_notes queries metadata, and search_notes searches text")
	}
	limit := in.MaxResults
	if limit <= 0 {
		limit = DefaultNoteResults
	}

	out := &NoteList{Notes: []NoteMeta{}}
	scanned, err := eachNote(v, func(path string, n *notes.Note) error {
		if !matchesTags(n, in.Tags) || !matchesField(n, in.Key, in.Value) {
			return nil
		}
		if len(out.Notes) == limit {
			out.Truncated = true
			return nil
		}
		tags := n.Tags
		if tags == nil {
			tags = []string{}
		}
		out.Notes = append(out.Notes, NoteMeta{Path: path, Tags: tags})
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	out.NotesScanned = scanned
	return nil, out, nil
}

func matchesTags(n *notes.Note, want []string) bool {
	for _, tag := range want {
		if !slices.Contains(n.Tags, tag) {
			return false
		}
	}
	return true
}

// matchesField reports whether the note carries key, and, when value is
// given, whether the field equals it. A list field matches when any of its
// items does, which is how multi-value fields such as status lists read.
func matchesField(n *notes.Note, key, value string) bool {
	if key == "" {
		return true
	}
	got, ok := n.Frontmatter[key]
	if !ok {
		return false
	}
	if value == "" {
		return true
	}
	if list, ok := got.([]any); ok {
		for _, item := range list {
			if fmt.Sprint(item) == value {
				return true
			}
		}
		return false
	}
	return fmt.Sprint(got) == value
}

type noteRef struct {
	Vault string `json:"vault,omitempty" jsonschema:"the vault holding the note; optional when the server holds one vault"`
	Path  string `json:"path" jsonschema:"vault-relative path of the note"`
}

// Frontmatter is the outcome of get_frontmatter and update_frontmatter.
type Frontmatter struct {
	Path string `json:"path"`
	// Frontmatter holds the note's fields, empty when it has none.
	Frontmatter map[string]any `json:"frontmatter"`
	// HasFrontmatter separates an empty block from no block at all.
	HasFrontmatter bool `json:"has_frontmatter"`
	// Tags are the note's tags, from frontmatter and inline hashtags.
	Tags []string `json:"tags"`
}

func (s *Server) getFrontmatter(_ context.Context, _ *mcp.CallToolRequest, in noteRef) (*mcp.CallToolResult, *Frontmatter, error) {
	v, err := s.note(in.Vault, in.Path)
	if err != nil {
		return nil, nil, err
	}
	n, err := parseNote(v, in.Path)
	if err != nil {
		return nil, nil, err
	}
	return nil, frontmatterOf(in.Path, n), nil
}

type updateFrontmatterInput struct {
	Vault  string         `json:"vault,omitempty" jsonschema:"the vault holding the note; optional when the server holds one vault"`
	Path   string         `json:"path" jsonschema:"vault-relative path of the note"`
	Set    map[string]any `json:"set,omitempty" jsonschema:"fields to add or replace"`
	Remove []string       `json:"remove,omitempty" jsonschema:"field names to delete"`
}

func (s *Server) updateFrontmatter(_ context.Context, _ *mcp.CallToolRequest, in updateFrontmatterInput) (*mcp.CallToolResult, *Frontmatter, error) {
	v, err := s.writableNote(in.Vault, in.Path)
	if err != nil {
		return nil, nil, err
	}
	if len(in.Set) == 0 && len(in.Remove) == 0 {
		return nil, nil, errors.New("give set, remove, or both: update_frontmatter changes nothing otherwise")
	}
	data, err := v.ReadAll(in.Path)
	if err != nil {
		return nil, nil, err
	}
	updated, err := notes.UpdateFrontmatter(data, in.Set, in.Remove)
	if err != nil {
		return nil, nil, fmt.Errorf("updating %q: %w", in.Path, err)
	}
	if err := v.WriteAll(in.Path, updated); err != nil {
		return nil, nil, err
	}
	n, err := notes.Parse(updated)
	if err != nil {
		return nil, nil, fmt.Errorf("re-reading %q: %w", in.Path, err)
	}
	return nil, frontmatterOf(in.Path, n), nil
}

func frontmatterOf(path string, n *notes.Note) *Frontmatter {
	out := &Frontmatter{
		Path:           path,
		Frontmatter:    n.Frontmatter,
		HasFrontmatter: n.HasFrontmatter,
		Tags:           n.Tags,
	}
	if out.Tags == nil {
		out.Tags = []string{}
	}
	return out
}

func parseNote(v *vault.Vault, path string) (*notes.Note, error) {
	data, err := v.ReadAll(path)
	if err != nil {
		return nil, err
	}
	n, err := notes.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("parsing %q: %w", path, err)
	}
	return n, nil
}

// eachNote parses every note in the vault and reports how many it read. A
// note whose frontmatter does not parse is skipped rather than failing the
// whole scan: one malformed note should not hide the rest of the vault.
func eachNote(v *vault.Vault, fn func(path string, n *notes.Note) error) (int, error) {
	paths, err := v.Notes()
	if err != nil {
		return 0, err
	}
	scanned := 0
	for _, path := range paths {
		data, err := v.ReadAll(path)
		if err != nil {
			return scanned, err
		}
		scanned++
		n, err := notes.Parse(data)
		if err != nil {
			continue
		}
		if err := fn(path, n); err != nil {
			return scanned, err
		}
	}
	return scanned, nil
}
