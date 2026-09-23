package notes

import (
	"slices"
	"strings"
	"testing"
)

func TestParseReadsFrontmatterAndBody(t *testing.T) {
	doc := "---\ntitle: Bike Maintenance\nstatus: active\n---\n\n# Bike Maintenance\n\nbody text\n"

	n, err := Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if !n.HasFrontmatter {
		t.Error("HasFrontmatter = false")
	}
	if n.Frontmatter["title"] != "Bike Maintenance" || n.Frontmatter["status"] != "active" {
		t.Errorf("Frontmatter = %+v", n.Frontmatter)
	}
	if !strings.HasPrefix(n.Body, "\n# Bike Maintenance") {
		t.Errorf("Body = %q", n.Body)
	}
}

func TestParseWithoutFrontmatter(t *testing.T) {
	n, err := Parse([]byte("# Just a note\n\ntext\n"))
	if err != nil {
		t.Fatal(err)
	}
	if n.HasFrontmatter || len(n.Frontmatter) != 0 {
		t.Errorf("n = %+v, want no frontmatter", n)
	}
	if n.Body != "# Just a note\n\ntext\n" {
		t.Errorf("Body = %q", n.Body)
	}
}

func TestParseRejectsBrokenFrontmatter(t *testing.T) {
	if _, err := Parse([]byte("---\ntags: [unclosed\n---\n\nbody\n")); err == nil {
		t.Error("Parse accepted invalid YAML frontmatter")
	}
}

func TestTagsFromYAMLList(t *testing.T) {
	n, err := Parse([]byte("---\ntags:\n  - daily-note\n  - work/active\n---\n\nbody\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"daily-note", "work/active"}
	if !slices.Equal(n.Tags, want) {
		t.Errorf("Tags = %q, want %q", n.Tags, want)
	}
}

func TestTagsFromInlineAndStringForms(t *testing.T) {
	cases := []struct {
		name string
		doc  string
		want []string
	}{
		{"comma string", "---\ntags: one, two\n---\n\nbody\n", []string{"one", "two"}},
		{"space string", "---\ntags: one two\n---\n\nbody\n", []string{"one", "two"}},
		{"leading hash stripped", "---\ntags:\n  - \"#one\"\n---\n\nbody\n", []string{"one"}},
		{"inline hashtag", "body about #gardening today\n", []string{"gardening"}},
		{"heading is not a tag", "# Heading\n\ntext\n", nil},
		{"numeric is not a tag", "issue #42 filed\n", nil},
		{"url fragment is not a tag", "see https://example.com/page#section\n", nil},
		{"code fence ignored", "```\n#notatag\n```\n", nil},
		{"duplicates collapse", "---\ntags:\n  - one\n---\n\n#one and #one\n", []string{"one"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			n, err := Parse([]byte(c.doc))
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(n.Tags, c.want) {
				t.Errorf("Tags = %q, want %q", n.Tags, c.want)
			}
		})
	}
}

func TestUpdateFrontmatterSetsAndRemovesKeys(t *testing.T) {
	doc := "---\ntitle: Bike Maintenance\nstatus: active\ntags:\n  - upkeep\n---\n\nbody text\n"

	got, err := UpdateFrontmatter([]byte(doc), map[string]any{"status": "done"}, []string{"title"})
	if err != nil {
		t.Fatal(err)
	}
	n, err := Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if n.Frontmatter["status"] != "done" {
		t.Errorf("status = %v", n.Frontmatter["status"])
	}
	if _, ok := n.Frontmatter["title"]; ok {
		t.Error("title was not removed")
	}
	if !slices.Equal(n.Tags, []string{"upkeep"}) {
		t.Errorf("Tags = %q, want the untouched list", n.Tags)
	}
	if n.Body != "\nbody text\n" {
		t.Errorf("Body = %q, want it unchanged", n.Body)
	}
}

func TestUpdateFrontmatterAddsBlockWhenMissing(t *testing.T) {
	got, err := UpdateFrontmatter([]byte("# Just a note\n"), map[string]any{"status": "active"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(got), "---\nstatus: active\n---\n") {
		t.Errorf("got %q, want a new frontmatter block first", got)
	}
	n, err := Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if n.Body != "# Just a note\n" {
		t.Errorf("Body = %q, want it unchanged", n.Body)
	}
}

func TestUpdateFrontmatterKeepsKeyOrder(t *testing.T) {
	doc := "---\nzebra: 1\napple: 2\n---\n\nbody\n"

	got, err := UpdateFrontmatter([]byte(doc), map[string]any{"apple": 3}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(got), "---\nzebra: 1\napple: 3\n---") {
		t.Errorf("got %q, want the original key order", got)
	}
}

func TestUpdateFrontmatterRemovingEveryKeyDropsTheBlock(t *testing.T) {
	got, err := UpdateFrontmatter([]byte("---\nstatus: active\n---\n\nbody\n"), nil, []string{"status"})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "\nbody\n" {
		t.Errorf("got %q, want the frontmatter block gone", got)
	}
}
