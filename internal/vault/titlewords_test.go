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
		got, err := v.MatchTitleWords(words, false)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"Areas/Bike Maintenance/Bike Maintenance.md"}
		if !slices.Equal(got, want) {
			t.Errorf("MatchTitleWords(%q) = %q, want %q", words, got, want)
		}
	}
}

func TestMatchTitleWordsHonoursCaseSensitivity(t *testing.T) {
	v := newTitleVault(t, "Areas/Bike Maintenance.md")

	got, err := v.MatchTitleWords([]string{"bike"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("case-sensitive match = %q, want none", got)
	}

	got, err = v.MatchTitleWords([]string{"Bike"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Errorf("match = %q, want the note", got)
	}
}

func TestMatchTitleWordsSkipsHiddenEntries(t *testing.T) {
	v := newTitleVault(t, ".trash/plan notes.md", "Inbox/plan notes.md")

	got, err := v.MatchTitleWords([]string{"plan"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{"Inbox/plan notes.md"}) {
		t.Errorf("got %q, want the visible note only", got)
	}
}
