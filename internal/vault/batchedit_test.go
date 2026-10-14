package vault

import (
	"strings"
	"testing"
)

func TestEditAppliesEditsInOrder(t *testing.T) {
	v := newTestVault(t)
	mustWrite(t, v, "note.md", "| a | old |\n\nlast paragraph\n")

	n, version, err := v.Edit("note.md", "",
		Replacement{Find: "| a | old |", Replace: "| a | new |"},
		Replacement{Find: "| a | new |\n", Replace: "| a | new |\n| b | added |\n"},
		Replacement{Find: "last paragraph\n", Replace: "last paragraph\n\nanother\n"},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "| a | new |\n| b | added |\n\nlast paragraph\n\nanother\n"
	if got := mustReadFile(t, v, "note.md"); got != want {
		t.Errorf("content = %q, want %q", got, want)
	}
	if n != 3 || version != Version([]byte(want)) {
		t.Errorf("n = %d, version = %s; want 3 and the new text's version", n, version)
	}
}

func TestEditWritesNothingWhenAnyEditFails(t *testing.T) {
	const note = "alpha beta beta\n"
	for _, tc := range []struct {
		name  string
		edits []Replacement
		want  string
	}{
		{"not found", []Replacement{{Find: "alpha", Replace: "A"}, {Find: "gamma", Replace: "G"}}, "edit 2 of 2: text not found"},
		{"ambiguous", []Replacement{{Find: "alpha", Replace: "A"}, {Find: "beta", Replace: "B"}}, "edit 2 of 2: text occurs 2 times"},
		{"removed by an earlier edit", []Replacement{{Find: "alpha ", Replace: ""}, {Find: "alpha", Replace: "A"}}, "edit 2 of 2: text not found"},
		{"empty find", []Replacement{{Find: "alpha", Replace: "A"}, {Find: "", Replace: "x"}}, "edit 2 of 2: find must not be empty"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := newTestVault(t)
			mustWrite(t, v, "note.md", note)
			if _, _, err := v.Edit("note.md", "", tc.edits...); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want %q", err, tc.want)
			}
			if got := mustReadFile(t, v, "note.md"); got != note {
				t.Errorf("content = %q, want the note untouched", got)
			}
		})
	}
}

func TestEditChecksTheVersionOnceAgainstTheOriginal(t *testing.T) {
	v := newTestVault(t)
	mustWrite(t, v, "note.md", "alpha\n")
	read, err := v.Read("note.md", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := v.Edit("note.md", read.Version,
		Replacement{Find: "alpha", Replace: "beta"},
		Replacement{Find: "beta", Replace: "gamma"},
	); err != nil {
		t.Fatalf("edits after a fresh read: %v", err)
	}
	if _, _, err := v.Edit("note.md", read.Version, Replacement{Find: "gamma", Replace: "delta"}); err == nil {
		t.Error("an edit with the old version was applied")
	}
}

func TestEditNeedsAnEdit(t *testing.T) {
	v := newTestVault(t)
	mustWrite(t, v, "note.md", "alpha\n")
	if _, _, err := v.Edit("note.md", ""); err == nil || !strings.Contains(err.Error(), "at least one edit") {
		t.Errorf("err = %v, want one asking for an edit", err)
	}
}

func TestSingleEditErrorsCarryNoPosition(t *testing.T) {
	v := newTestVault(t)
	mustWrite(t, v, "note.md", "alpha\n")
	_, _, err := v.Edit("note.md", "", Replacement{Find: "gamma", Replace: "x"})
	if err == nil || strings.Contains(err.Error(), "of 1") {
		t.Errorf("err = %v, want a plain error for a single edit", err)
	}
}
