package vault

import (
	"reflect"
	"testing"
)

func TestOutlineListsHeadingsInDocumentOrder(t *testing.T) {
	v := newTestVault(t)
	mustWrite(t, v, "note.md", "# Plan\ntext\n## Tasks\n### Soon\n## Log\nUnder\n=====\n")

	got, err := v.Outline("note.md")
	if err != nil {
		t.Fatal(err)
	}
	want := []Heading{
		{HeadingPath: []string{"Plan"}, Level: 1},
		{HeadingPath: []string{"Plan", "Tasks"}, Level: 2},
		{HeadingPath: []string{"Plan", "Tasks", "Soon"}, Level: 3},
		{HeadingPath: []string{"Plan", "Log"}, Level: 2},
		{HeadingPath: []string{"Under"}, Level: 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("outline = %+v, want %+v", got, want)
	}
}

func TestOutlineSkipsTextThatOnlyLooksLikeAHeading(t *testing.T) {
	v := newTestVault(t)
	mustWrite(t, v, "note.md", "---\ntitle: x\n# not a heading: yaml comment\n---\n```\n# code\n```\n> # quote\n\n# Real\n")

	got, err := v.Outline("note.md")
	if err != nil {
		t.Fatal(err)
	}
	want := []Heading{{HeadingPath: []string{"Real"}, Level: 1}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("outline = %+v, want %+v", got, want)
	}
}

func TestOutlineOfANoteWithoutHeadingsIsEmptyNotNil(t *testing.T) {
	v := newTestVault(t)
	mustWrite(t, v, "note.md", "just text\n")

	got, err := v.Outline("note.md")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("outline = %#v, want an empty list", got)
	}
}

func TestOutlineOfAMissingNoteFails(t *testing.T) {
	v := newTestVault(t)
	if _, err := v.Outline("missing.md"); err == nil {
		t.Fatal("outlined a note that does not exist")
	}
}
