package vault

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func wantCaseError(t *testing.T, err error, existing string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), "letter case") || !strings.Contains(err.Error(), existing) {
		t.Fatalf("err = %v, want a letter-case error naming %q", err, existing)
	}
}

func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func caseSensitive(t *testing.T) bool {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := os.Lstat(filepath.Join(dir, "A"))
	return err != nil
}

func TestCreateRefusesANameThatDiffersOnlyInCase(t *testing.T) {
	v := newNotesVault(t, "Inbox/Plan.md")

	wantCaseError(t, v.Create("Inbox/plan.md", "other\n"), "Inbox/Plan.md")
	if got := dirNames(t, filepath.Join(v.Root(), "Inbox")); !slices.Equal(got, []string{"Plan.md"}) {
		t.Errorf("Inbox holds %v, want only Plan.md", got)
	}
}

func TestCreateRefusesAFolderThatDiffersOnlyInCase(t *testing.T) {
	v := newNotesVault(t, "Inbox/a.md")

	wantCaseError(t, v.Create("inbox/b.md", "new\n"), "Inbox")
	if got := dirNames(t, v.Root()); !slices.Equal(got, []string{"Inbox"}) {
		t.Errorf("vault root holds %v, want only Inbox", got)
	}
}

func TestCreateAcceptsAPathInsideExistingFolders(t *testing.T) {
	v := newNotesVault(t, "Inbox/a.md")

	if err := v.Create("Inbox/Sub/b.md", "new\n"); err != nil {
		t.Fatal(err)
	}
}

func TestAppendRefusesANewNameThatDiffersOnlyInCase(t *testing.T) {
	v := newNotesVault(t, "Inbox/Plan.md")

	wantCaseError(t, v.Append("Inbox/PLAN.md", "more\n"), "Inbox/Plan.md")
	if data, _ := os.ReadFile(filepath.Join(v.Root(), "Inbox", "Plan.md")); string(data) != "body\n" {
		t.Errorf("Plan.md = %q, want it untouched", data)
	}
}

func TestMoveRefusesADestinationThatDiffersOnlyInCase(t *testing.T) {
	v := newNotesVault(t, "Inbox/a.md", "Inbox/Plan.md")

	wantCaseError(t, v.Move("Inbox/a.md", "Inbox/plan.md"), "Inbox/Plan.md")
}

func TestMoveAllowsChangingOnlyTheCaseOfANote(t *testing.T) {
	if !caseSensitive(t) {
		t.Skip("the filesystem ignores letter case, so it refuses this rename itself")
	}
	v := newNotesVault(t, "Inbox/plan.md")

	if err := v.Move("Inbox/plan.md", "Inbox/Plan.md"); err != nil {
		t.Fatal(err)
	}
	if got := dirNames(t, filepath.Join(v.Root(), "Inbox")); !slices.Equal(got, []string{"Plan.md"}) {
		t.Errorf("Inbox holds %v, want only Plan.md", got)
	}
}
