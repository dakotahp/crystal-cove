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
	Vault  string `json:"vault,omitempty" jsonschema:"the vault to scan; optional when the server holds one vault"`
	Offset int    `json:"offset,omitempty" jsonschema:"tag to start from; use next_offset from the previous page"`
	Limit  int    `json:"limit,omitempty" jsonschema:"maximum tags to return (default 200, max 1000); a small limit gives the most used tags"`
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
	Total        int `json:"total" jsonschema:"distinct tags in the whole vault"`
	NextOffset   int `json:"next_offset" jsonschema:"offset of the next page; -1 when this is the last page"`
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

	tags := make([]TagCount, 0, len(counts))
	for tag, count := range counts {
		tags = append(tags, TagCount{Tag: tag, Count: count})
	}
	sort.Slice(tags, func(i, j int) bool {
		if tags[i].Count != tags[j].Count {
			return tags[i].Count > tags[j].Count
		}
		return tags[i].Tag < tags[j].Tag
	})
	page, next, err := pageOf(tags, in.Offset, in.Limit, DefaultListLimit)
	if err != nil {
		return nil, nil, err
	}
	return nil, &TagList{Tags: page, NotesScanned: scanned, Total: len(tags), NextOffset: next}, nil
}

type findNotesInput struct {
	Vault string   `json:"vault,omitempty" jsonschema:"the vault to search; optional when the server holds one vault"`
	Tags  []string `json:"tags,omitempty" jsonschema:"notes must carry every tag listed"`
	Key   string   `json:"frontmatter_key,omitempty" jsonschema:"notes must carry this frontmatter field"`
	Value string   `json:"frontmatter_value,omitempty" jsonschema:"the field must equal this value; omit to match any value"`
	folderFilter
	MissingKey string   `json:"missing_key,omitempty" jsonschema:"notes must not have this frontmatter field"`
	MinTags    *int     `json:"min_tags,omitempty" jsonschema:"notes must carry at least this many tags, counted as list_tags counts them"`
	MaxTags    *int     `json:"max_tags,omitempty" jsonschema:"notes must carry at most this many tags; 0 finds untagged notes"`
	Fields     []string `json:"fields,omitempty" jsonschema:"frontmatter fields to return for each note, so a check needs no extra read"`
	Offset     int      `json:"offset,omitempty" jsonschema:"note to start from; use next_offset from the previous page"`
	Limit      int      `json:"limit,omitempty" jsonschema:"maximum notes to return (default 50, max 1000)"`
}

// NoteMeta identifies one matching note.
type NoteMeta struct {
	Path string   `json:"path"`
	Tags []string `json:"tags"`
	// Fields holds the asked frontmatter fields the note has.
	Fields map[string]any `json:"fields,omitempty"`
}

// NoteList is the outcome of find_notes.
type NoteList struct {
	Notes []NoteMeta `json:"notes"`
	// NotesScanned is how many notes were read.
	NotesScanned int `json:"notes_scanned"`
	Total        int `json:"total" jsonschema:"notes that match, across every page"`
	NextOffset   int `json:"next_offset" jsonschema:"offset of the next page; -1 when this is the last page"`
}

func (s *Server) findNotes(_ context.Context, _ *mcp.CallToolRequest, in findNotesInput) (*mcp.CallToolResult, *NoteList, error) {
	v, err := s.vault(in.Vault)
	if err != nil {
		return nil, nil, err
	}
	if len(in.Tags) == 0 && in.Key == "" && in.MissingKey == "" && in.MinTags == nil && in.MaxTags == nil &&
		len(in.Folders) == 0 && len(in.ExcludeFolders) == 0 {
		return nil, nil, errors.New("give at least one filter, such as tags, frontmatter_key, missing_key, " +
			"max_tags or folders: find_notes queries metadata, and grep_notes searches text")
	}
	if (in.MinTags != nil && *in.MinTags < 0) || (in.MaxTags != nil && *in.MaxTags < 0) {
		return nil, nil, errors.New("min_tags and max_tags cannot be negative")
	}
	matches := []NoteMeta{}
	scanned, err := eachNote(v, func(path string, n *notes.Note) error {
		if !in.includes(path) || !matchesTags(n, in.Tags) || !matchesField(n, in.Key, in.Value) ||
			!in.matchesTagCount(len(n.Tags)) {
			return nil
		}
		if _, ok := n.Frontmatter[in.MissingKey]; ok && in.MissingKey != "" {
			return nil
		}
		tags := n.Tags
		if tags == nil {
			tags = []string{}
		}
		matches = append(matches, NoteMeta{Path: path, Tags: tags, Fields: pickFields(n, in.Fields)})
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	page, next, err := pageOf(matches, in.Offset, in.Limit, DefaultNoteResults)
	if err != nil {
		return nil, nil, err
	}
	return nil, &NoteList{Notes: page, NotesScanned: scanned, Total: len(matches), NextOffset: next}, nil
}

func (in findNotesInput) matchesTagCount(count int) bool {
	return (in.MinTags == nil || count >= *in.MinTags) && (in.MaxTags == nil || count <= *in.MaxTags)
}

// pickFields returns the named frontmatter fields the note has, or nil when
// it has none of them.
func pickFields(n *notes.Note, keys []string) map[string]any {
	var fields map[string]any
	for _, key := range keys {
		if value, ok := n.Frontmatter[key]; ok {
			if fields == nil {
				fields = map[string]any{}
			}
			fields[key] = value
		}
	}
	return fields
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
	Vault      string         `json:"vault,omitempty" jsonschema:"the vault holding the note; optional when the server holds one vault"`
	Path       string         `json:"path" jsonschema:"vault-relative path of the note"`
	Set        map[string]any `json:"set,omitempty" jsonschema:"fields to add or replace"`
	Remove     []string       `json:"remove,omitempty" jsonschema:"field names to delete"`
	AddTags    []string       `json:"add_tags,omitempty" jsonschema:"tags to merge into the tags field, kept in order and not repeated; a leading # is optional"`
	RemoveTags []string       `json:"remove_tags,omitempty" jsonschema:"tags to take out of the tags field; inline hashtags in the body stay"`
}

func (s *Server) updateFrontmatter(_ context.Context, _ *mcp.CallToolRequest, in updateFrontmatterInput) (*mcp.CallToolResult, *Frontmatter, error) {
	v, err := s.writableNote(in.Vault, in.Path)
	if err != nil {
		return nil, nil, err
	}
	changesTags := len(in.AddTags) > 0 || len(in.RemoveTags) > 0
	if len(in.Set) == 0 && len(in.Remove) == 0 && !changesTags {
		return nil, nil, errors.New("give set, remove, add_tags or remove_tags: update_frontmatter changes nothing otherwise")
	}
	if _, setsTags := in.Set["tags"]; changesTags && (setsTags || slices.Contains(in.Remove, "tags")) {
		return nil, nil, errors.New("give tags through set or remove, or through add_tags and remove_tags, not both")
	}
	updated, err := v.Update(in.Path, func(old []byte) ([]byte, error) {
		updated, err := notes.UpdateFrontmatter(old, in.Set, in.Remove)
		if err == nil && changesTags {
			updated, err = notes.ChangeTags(updated, in.AddTags, in.RemoveTags)
		}
		if err != nil {
			return nil, fmt.Errorf("updating %q: %w", in.Path, err)
		}
		return updated, nil
	})
	if err != nil {
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
