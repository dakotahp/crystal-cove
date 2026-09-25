package vault

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func newNotesVault(t *testing.T, paths ...string) *Vault {
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

func TestNotesListsMarkdownOnlyAndSkipsHidden(t *testing.T) {
	v := newNotesVault(t, "Inbox/today.md", "Areas/photo.png", ".trash/gone.md", ".obsidian/app.md")

	got, err := v.Notes()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Inbox/today.md"}
	if !slices.Equal(got, want) {
		t.Errorf("Notes = %q, want %q", got, want)
	}
}

func TestReadAllAndUpdateRoundTrip(t *testing.T) {
	v := newNotesVault(t, "Inbox/today.md")

	if _, err := v.Update("Inbox/today.md", setTo("changed\n")); err != nil {
		t.Fatal(err)
	}
	got, err := v.ReadAll("Inbox/today.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "changed\n" {
		t.Errorf("ReadAll = %q", got)
	}
}

func TestReadAllAndUpdateRejectEscapes(t *testing.T) {
	v := newNotesVault(t, "Inbox/today.md")

	if _, err := v.ReadAll("../outside.md"); err == nil {
		t.Error("ReadAll accepted a path outside the vault")
	}
	if _, err := v.Update("/etc/passwd", setTo("nope")); err == nil {
		t.Error("Update accepted an absolute path")
	}
}
