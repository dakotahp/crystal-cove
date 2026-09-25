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

	got, truncated, err := v.MatchTitles("Bike Maintenance", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if truncated {
		t.Error("truncated = true")
	}
	want := []string{"Areas/Bike Maintenance/Bike Maintenance.md"}
	if !slices.Equal(got, want) {
		t.Errorf("MatchTitles = %q, want %q", got, want)
	}
}

func TestMatchTitlesIgnoresCaseByDefault(t *testing.T) {
	v := newTitleVault(t, "Projects/Harbor Bridge/Harbor Bridge.md")

	got, _, err := v.MatchTitles("harbor bridge", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("MatchTitles = %q, want one match", got)
	}

	got, _, err = v.MatchTitles("harbor bridge", true, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("case-sensitive MatchTitles = %q, want none", got)
	}
}

func TestMatchTitlesIgnoresExtensionAndHiddenEntries(t *testing.T) {
	v := newTitleVault(t, "notes/plan.md", ".obsidian/plan.md", ".trash/plan.md")

	got, _, err := v.MatchTitles("plan", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"notes/plan.md"}
	if !slices.Equal(got, want) {
		t.Errorf("MatchTitles = %q, want %q", got, want)
	}

	if got, _, err := v.MatchTitles(`plan\.md`, false, 0); err != nil || len(got) != 0 {
		t.Errorf("MatchTitles matched the extension: %q, %v", got, err)
	}
}

func TestMatchTitlesRespectsLimit(t *testing.T) {
	v := newTitleVault(t, "a/plan.md", "b/plan.md", "c/plan.md")

	got, truncated, err := v.MatchTitles("plan", false, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !truncated {
		t.Errorf("MatchTitles = %q, truncated = %v; want 2 and true", got, truncated)
	}
}

func TestMatchTitlesRejectsBadPattern(t *testing.T) {
	v := newTitleVault(t, "a/plan.md")
	if _, _, err := v.MatchTitles("([", false, 0); err == nil {
		t.Error("MatchTitles accepted an invalid pattern")
	}
}

func TestMatchTitlesSkipsFilesThatAreNotNotes(t *testing.T) {
	v := newTitleVault(t, "Bike.md", "Bike.png", "Bike.canvas")

	got, _, err := v.MatchTitles("bike", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"Bike.md"}; !slices.Equal(got, want) {
		t.Errorf("MatchTitles = %q, want %q", got, want)
	}
	got, _, err = v.MatchTitleWords([]string{"bike"}, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"Bike.md"}; !slices.Equal(got, want) {
		t.Errorf("MatchTitleWords = %q, want %q", got, want)
	}
}
