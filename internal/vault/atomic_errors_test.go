package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUpdateKeepsAnUnusualFileMode(t *testing.T) {
	v := newNotesVault(t, "Inbox/today.md")
	target := filepath.Join(v.Root(), "Inbox", "today.md")
	if err := os.Chmod(target, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := v.Update("Inbox/today.md", setTo("changed\n")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600 preserved", info.Mode().Perm())
	}
}

func TestUpdateReportsAnUnwritableDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permissions do not restrict root")
	}
	v := newNotesVault(t, "Inbox/today.md")
	dir := filepath.Join(v.Root(), "Inbox")
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	_, err := v.Update("Inbox/today.md", setTo("changed\n"))
	if err == nil || !strings.Contains(err.Error(), "temporary file") {
		t.Fatalf("err = %v, want it to name the temporary file", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "today.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "body\n" {
		t.Errorf("note = %q, want the original kept", data)
	}
}

func TestUpdateCreatesAMissingNoteWhenAllowed(t *testing.T) {
	v := newNotesVault(t, "Inbox/today.md")

	if _, err := v.update("Inbox/fresh.md", true, setTo("new\n")); err != nil {
		t.Fatal(err)
	}
	data, err := v.ReadAll("Inbox/fresh.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new\n" {
		t.Errorf("note = %q", data)
	}
}

func TestRecentNotesOnAnEmptyVault(t *testing.T) {
	v := New("Personal", t.TempDir())

	got, err := v.RecentNotes(0, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("RecentNotes = %+v, want none", got)
	}
}

func TestNotesAndRecentNotesReportAMissingRoot(t *testing.T) {
	v := New("Personal", filepath.Join(t.TempDir(), "gone"))

	if _, err := v.Notes(); err == nil {
		t.Error("Notes accepted a missing vault root")
	}
	if _, err := v.RecentNotes(0, time.Time{}); err == nil {
		t.Error("RecentNotes accepted a missing vault root")
	}
	if _, err := v.ReadAll("a.md"); err == nil || !strings.Contains(err.Error(), "opening vault") {
		t.Errorf("ReadAll err = %v, want it to name the vault", err)
	}
	if _, err := v.List("", false); err == nil || !strings.Contains(err.Error(), "opening vault") {
		t.Errorf("List err = %v, want it to name the vault", err)
	}
}

func TestUpdateReportsATargetThatIsAFolder(t *testing.T) {
	v := newNotesVault(t, "Inbox/today.md/inside.md")

	_, err := v.Update("Inbox/today.md", setTo("changed\n"))
	if err == nil || !strings.Contains(err.Error(), "reading") {
		t.Fatalf("err = %v, want a failed read", err)
	}
	entries, err := os.ReadDir(filepath.Join(v.Root(), "Inbox"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("Inbox holds %d entries, want the temporary file removed", len(entries))
	}
}
