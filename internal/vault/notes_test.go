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

func TestReadAllAndWriteAllRoundTrip(t *testing.T) {
	v := newNotesVault(t, "Inbox/today.md")

	if err := v.WriteAll("Inbox/today.md", []byte("changed\n")); err != nil {
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

func TestReadAllAndWriteAllRejectEscapes(t *testing.T) {
	v := newNotesVault(t, "Inbox/today.md")

	if _, err := v.ReadAll("../outside.md"); err == nil {
		t.Error("ReadAll accepted a path outside the vault")
	}
	if err := v.WriteAll("/etc/passwd", []byte("nope")); err == nil {
		t.Error("WriteAll accepted an absolute path")
	}
}
