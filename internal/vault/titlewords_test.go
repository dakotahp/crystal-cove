package vault

import (
	"slices"
	"testing"
)

func TestMatchTitleWordsNeedsEveryWordInAnyOrder(t *testing.T) {
	v := newTitleVault(t,
		"Areas/Bike Maintenance/Bike Maintenance.md",
		"Areas/Bike Rides.md",
		"Inbox/today.md",
	)

	for _, words := range [][]string{{"bike", "maintenance"}, {"maintenance", "bike"}} {
		got, truncated, err := v.MatchTitleWords(words, false, 0)
		if err != nil {
			t.Fatal(err)
		}
		if truncated {
			t.Error("truncated = true")
		}
		want := []string{"Areas/Bike Maintenance/Bike Maintenance.md"}
		if !slices.Equal(got, want) {
			t.Errorf("MatchTitleWords(%q) = %q, want %q", words, got, want)
		}
	}
}

func TestMatchTitleWordsHonoursCaseSensitivity(t *testing.T) {
	v := newTitleVault(t, "Areas/Bike Maintenance.md")

	got, _, err := v.MatchTitleWords([]string{"bike"}, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("case-sensitive match = %q, want none", got)
	}

	got, _, err = v.MatchTitleWords([]string{"Bike"}, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Errorf("match = %q, want the note", got)
	}
}

func TestMatchTitleWordsRespectsLimit(t *testing.T) {
	v := newTitleVault(t, "a/plan notes.md", "b/plan notes.md", "c/plan notes.md")

	got, truncated, err := v.MatchTitleWords([]string{"plan", "notes"}, false, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !truncated {
		t.Errorf("got %q, truncated = %v; want 2 and true", got, truncated)
	}
}

func TestMatchTitleWordsSkipsHiddenEntries(t *testing.T) {
	v := newTitleVault(t, ".trash/plan notes.md", "Inbox/plan notes.md")

	got, _, err := v.MatchTitleWords([]string{"plan"}, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{"Inbox/plan notes.md"}) {
		t.Errorf("got %q, want the visible note only", got)
	}
}
