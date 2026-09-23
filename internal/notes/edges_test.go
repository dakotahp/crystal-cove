package notes

import (
	"strings"
	"testing"
)

func TestParseTreatsUnclosedFenceAsBody(t *testing.T) {
	doc := "---\ntitle: no closing fence\n\nbody\n"

	n, err := Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if n.HasFrontmatter || n.Body != doc {
		t.Errorf("n = %+v, want the whole document as body", n)
	}
}

func TestParseRequiresTheClosingFenceOnItsOwnLine(t *testing.T) {
	n, err := Parse([]byte("---\ntitle: x\n--- trailing\nbody\n"))
	if err != nil {
		t.Fatal(err)
	}
	if n.HasFrontmatter {
		t.Error("HasFrontmatter = true for a fence with trailing text")
	}
}

func TestParseEmptyFrontmatterBlock(t *testing.T) {
	n, err := Parse([]byte("---\n---\nbody\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !n.HasFrontmatter || len(n.Frontmatter) != 0 || n.Body != "body\n" {
		t.Errorf("n = %+v, want an empty block and the body kept", n)
	}
}

func TestParseFrontmatterThatIsNotAMapping(t *testing.T) {
	if _, err := Parse([]byte("---\n- one\n- two\n---\nbody\n")); err == nil {
		t.Error("Parse accepted a frontmatter list where a mapping belongs")
	}
}

func TestTagsIgnoreNonStringListItems(t *testing.T) {
	n, err := Parse([]byte("---\ntags:\n  - one\n  - 42\n---\n\nbody\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Tags) != 1 || n.Tags[0] != "one" {
		t.Errorf("Tags = %q, want the string tag only", n.Tags)
	}
}

func TestUpdateFrontmatterOnNoteWithoutBlockAndNoChangeKeepsFile(t *testing.T) {
	doc := []byte("# Just a note\n")

	got, err := UpdateFrontmatter(doc, nil, []string{"status"})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(doc) {
		t.Errorf("got %q, want the file unchanged", got)
	}
}

func TestUpdateFrontmatterRejectsBrokenYAML(t *testing.T) {
	if _, err := UpdateFrontmatter([]byte("---\ntags: [unclosed\n---\nbody\n"), map[string]any{"a": 1}, nil); err == nil {
		t.Error("UpdateFrontmatter accepted malformed frontmatter")
	}
}

func TestUpdateFrontmatterWritesListValues(t *testing.T) {
	got, err := UpdateFrontmatter([]byte("---\ntitle: x\n---\n\nbody\n"), map[string]any{
		"tags":   []string{"one", "two"},
		"status": "active",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := string(got)
	if !strings.Contains(out, "- one") || !strings.Contains(out, "- two") {
		t.Errorf("got %q, want the list written out", out)
	}
	if strings.Index(out, "status:") > strings.Index(out, "tags:") {
		t.Errorf("got %q, want new keys added in a stable order", out)
	}
	n, err := Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if n.Body != "\nbody\n" {
		t.Errorf("Body = %q", n.Body)
	}
}
