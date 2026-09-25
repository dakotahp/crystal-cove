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

func TestAppendReplacesTheNoteRatherThanWritingIntoIt(t *testing.T) {
	v := newNotesVault(t, "Inbox/today.md")
	target := filepath.Join(v.Root(), "Inbox", "today.md")
	if err := os.WriteFile(target, []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0o640); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}

	if err := v.Append("Inbox/today.md", "two\n"); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(before, after) {
		t.Error("Append wrote into the existing file, so a reader could see it half-written")
	}
	if after.Mode().Perm() != 0o640 {
		t.Errorf("mode = %v, want 0640 kept", after.Mode().Perm())
	}
	if data, _ := os.ReadFile(target); string(data) != "one\ntwo\n" {
		t.Errorf("content = %q", data)
	}
}

func TestCreateAndAppendLeaveNoTemporaryFiles(t *testing.T) {
	v := New("Personal", t.TempDir())
	if err := v.Create("Inbox/new.md", "alpha\n"); err != nil {
		t.Fatal(err)
	}
	if err := v.Append("Inbox/new.md", "beta\n"); err != nil {
		t.Fatal(err)
	}
	if err := v.Append("Inbox/log.md", "gamma\n"); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(v.Root(), "Inbox"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if strings.Join(names, ",") != "log.md,new.md" {
		t.Errorf("directory holds %v, want only the two notes", names)
	}
	if data, _ := os.ReadFile(filepath.Join(v.Root(), "Inbox", "new.md")); string(data) != "alpha\nbeta\n" {
		t.Errorf("new.md = %q", data)
	}
}

func TestCreateRefusesAnExistingNoteAndKeepsIt(t *testing.T) {
	v := newNotesVault(t, "Inbox/today.md")
	target := filepath.Join(v.Root(), "Inbox", "today.md")
	if err := os.WriteFile(target, []byte("keep me\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := v.Create("Inbox/today.md", "replacement\n")
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("Create = %v, want an already-exists error", err)
	}
	if data, _ := os.ReadFile(target); string(data) != "keep me\n" {
		t.Errorf("content = %q, want the original kept", data)
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
