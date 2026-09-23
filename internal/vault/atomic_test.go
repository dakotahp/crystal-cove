package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteAllLeavesNoTemporaryFiles(t *testing.T) {
	v := newNotesVault(t, "Inbox/today.md")

	if err := v.WriteAll("Inbox/today.md", []byte("changed\n")); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(v.Root(), "Inbox"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "today.md" {
			t.Errorf("left behind %q", e.Name())
		}
	}
}

func TestWriteAllReplacesInPlaceForReaders(t *testing.T) {
	v := newNotesVault(t, "Inbox/today.md")
	target := filepath.Join(v.Root(), "Inbox", "today.md")

	before, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.WriteAll("Inbox/today.md", []byte("replaced\n")); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if after.Mode().Perm() != before.Mode().Perm() {
		t.Errorf("mode = %v, want %v kept", after.Mode().Perm(), before.Mode().Perm())
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "replaced\n" {
		t.Errorf("content = %q", data)
	}
}

func TestEditAndReplaceSectionKeepNoTemporaryFiles(t *testing.T) {
	v := newNotesVault(t, "Inbox/today.md")
	if err := v.WriteAll("Inbox/today.md", []byte("## One\n\nalpha\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Edit("Inbox/today.md", "alpha", "beta", false); err != nil {
		t.Fatal(err)
	}
	if err := v.ReplaceSection("Inbox/today.md", []string{"One"}, "gamma\n"); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(v.Root(), "Inbox"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("directory holds %v, want only the note", names)
	}
}

func TestRecentNotesOrdersByModifiedTime(t *testing.T) {
	v := newNotesVault(t, "Inbox/old.md", "Inbox/middle.md", "Inbox/new.md")
	base := time.Now().Add(-72 * time.Hour)
	for name, age := range map[string]time.Duration{"old.md": 0, "middle.md": 24 * time.Hour, "new.md": 48 * time.Hour} {
		p := filepath.Join(v.Root(), "Inbox", name)
		if err := os.Chtimes(p, base.Add(age), base.Add(age)); err != nil {
			t.Fatal(err)
		}
	}

	got, err := v.RecentNotes(0, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Inbox/new.md", "Inbox/middle.md", "Inbox/old.md"}
	for i, path := range want {
		if got[i].Path != path {
			t.Fatalf("RecentNotes = %+v, want newest first: %q", got, want)
		}
	}
	if got[0].Modified.IsZero() {
		t.Error("Modified is zero, want the file's timestamp")
	}
}

func TestRecentNotesAppliesLimitAndSince(t *testing.T) {
	v := newNotesVault(t, "Inbox/old.md", "Inbox/new.md")
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(filepath.Join(v.Root(), "Inbox", "old.md"), old, old); err != nil {
		t.Fatal(err)
	}

	got, err := v.RecentNotes(1, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Path != "Inbox/new.md" {
		t.Errorf("RecentNotes = %+v, want only the newest", got)
	}

	got, err = v.RecentNotes(0, time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Path != "Inbox/new.md" {
		t.Errorf("RecentNotes = %+v, want only notes changed since the cutoff", got)
	}
}

func TestListingsCarryModifiedTime(t *testing.T) {
	v := newNotesVault(t, "Inbox/today.md")

	entries, err := v.List("Inbox", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Modified.IsZero() {
		t.Errorf("List = %+v, want a modified time on the note", entries)
	}
}

func TestRecentNotesSkipsHiddenAndNonNotes(t *testing.T) {
	v := newNotesVault(t, "Inbox/today.md", "Inbox/photo.png", ".trash/gone.md")

	got, err := v.RecentNotes(0, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !strings.HasSuffix(got[0].Path, "today.md") {
		t.Errorf("RecentNotes = %+v, want the one visible note", got)
	}
}
