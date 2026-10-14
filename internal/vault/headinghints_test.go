package vault

import (
	"fmt"
	"strings"
	"testing"
)

func TestMissingSectionListsTheNoteHeadings(t *testing.T) {
	v := newTestVault(t)
	mustWrite(t, v, "note.md", "# Plan\n## Tasks\n## Log\n")

	_, err := v.GetSection("note.md", []string{"Missing"}, 0)
	if err == nil {
		t.Fatal("found a heading the note does not have")
	}
	for _, want := range []string{`["Plan"]`, `["Plan","Tasks"]`, `["Plan","Log"]`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not offer %s", err, want)
		}
	}
}

func TestMissingSectionPutsCloseHeadingsFirst(t *testing.T) {
	v := newTestVault(t)
	mustWrite(t, v, "note.md", "# Morning\n## BUJO — Personal Evening Capture: every day, 21:30\n## Other\n")

	_, err := v.GetSection("note.md", []string{"bujo - personal evening capture: every day, 21:30"}, 0)
	if err == nil {
		t.Fatal("matched a heading that differs in case and dash")
	}
	msg := err.Error()
	close := strings.Index(msg, `["Morning","BUJO — Personal Evening Capture: every day, 21:30"]`)
	first := strings.Index(msg, `["Morning"]`)
	if close < 0 || first < 0 || close > first {
		t.Errorf("error %q should offer the close heading before the others", msg)
	}
}

func TestMissingSectionInANoteWithoutHeadings(t *testing.T) {
	v := newTestVault(t)
	mustWrite(t, v, "note.md", "just text\n")

	_, err := v.GetSection("note.md", []string{"Tasks"}, 0)
	if err == nil || !strings.Contains(err.Error(), "no headings") {
		t.Errorf("err = %v, want one saying the note has no headings", err)
	}
}

func TestMissingSectionCapsTheHeadingsListed(t *testing.T) {
	v := newTestVault(t)
	var b strings.Builder
	for i := range maxHeadingHints + 5 {
		fmt.Fprintf(&b, "# H%03d\n", i)
	}
	mustWrite(t, v, "note.md", b.String())

	_, err := v.GetSection("note.md", []string{"Missing"}, 0)
	if err == nil {
		t.Fatal("found a missing heading")
	}
	if strings.Contains(err.Error(), fmt.Sprintf(`"H%03d"`, maxHeadingHints)) || !strings.Contains(err.Error(), "5 more") {
		t.Errorf("error %q should list %d headings and count the rest", err, maxHeadingHints)
	}
}

func TestMissingSectionHintAppliesToEdits(t *testing.T) {
	v := newTestVault(t)
	mustWrite(t, v, "note.md", "# Plan\n## Tasks\n")

	_, err := v.EditSection("note.md", []string{"tasks"}, SectionAppend, "- item\n", "")
	if err == nil || !strings.Contains(err.Error(), `["Plan","Tasks"]`) {
		t.Errorf("err = %v, want the headings offered for an edit too", err)
	}
}
