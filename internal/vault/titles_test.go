package vault

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func newTitleVault(t *testing.T, paths ...string) *Vault {
	t.Helper()
	v := New("Personal", t.TempDir())
	for _, p := range paths {
		full := filepath.Join(v.Root(), filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("body\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return v
}

func TestMatchTitlesFindsNoteByName(t *testing.T) {
	v := newTitleVault(t, "Areas/Bike Maintenance/Bike Maintenance.md", "Inbox/today.md")

	got, err := v.MatchTitles("Bike Maintenance", false)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Areas/Bike Maintenance/Bike Maintenance.md"}
	if !slices.Equal(got, want) {
		t.Errorf("MatchTitles = %q, want %q", got, want)
	}
}

func TestMatchTitlesIgnoresCaseByDefault(t *testing.T) {
	v := newTitleVault(t, "Projects/Harbor Bridge/Harbor Bridge.md")

	got, err := v.MatchTitles("harbor bridge", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("MatchTitles = %q, want one match", got)
	}

	got, err = v.MatchTitles("harbor bridge", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("case-sensitive MatchTitles = %q, want none", got)
	}
}

func TestMatchTitlesIgnoresExtensionAndHiddenEntries(t *testing.T) {
	v := newTitleVault(t, "notes/plan.md", ".obsidian/plan.md", ".trash/plan.md")

	got, err := v.MatchTitles("plan", false)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"notes/plan.md"}
	if !slices.Equal(got, want) {
		t.Errorf("MatchTitles = %q, want %q", got, want)
	}

	if got, err := v.MatchTitles(`plan\.md`, false); err != nil || len(got) != 0 {
		t.Errorf("MatchTitles matched the extension: %q, %v", got, err)
	}
}

func TestMatchTitlesRejectsBadPattern(t *testing.T) {
	v := newTitleVault(t, "a/plan.md")
	if _, err := v.MatchTitles("([", false); err == nil {
		t.Error("MatchTitles accepted an invalid pattern")
	}
}

func TestMatchTitlesSkipsFilesThatAreNotNotes(t *testing.T) {
	v := newTitleVault(t, "Bike.md", "Bike.png", "Bike.canvas")

	got, err := v.MatchTitles("bike", false)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"Bike.md"}; !slices.Equal(got, want) {
		t.Errorf("MatchTitles = %q, want %q", got, want)
	}
	got, err = v.MatchTitleWords([]string{"bike"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"Bike.md"}; !slices.Equal(got, want) {
		t.Errorf("MatchTitleWords = %q, want %q", got, want)
	}
}
