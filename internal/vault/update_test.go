package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setTo is an Update change that replaces a note's content with data.
func setTo(data string) func([]byte) ([]byte, error) {
	return func([]byte) ([]byte, error) { return []byte(data), nil }
}

func TestUpdateRedoesTheChangeWhenSyncWritesMeanwhile(t *testing.T) {
	v := newNotesVault(t, "Inbox/a.md")
	target := filepath.Join(v.Root(), "Inbox", "a.md")

	calls := 0
	_, err := v.Update("Inbox/a.md", func(old []byte) ([]byte, error) {
		calls++
		if calls == 1 {
			if err := os.WriteFile(target, []byte("synced\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return append(old, "local\n"...), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Errorf("change ran %d times, want 2", calls)
	}
	if data, _ := os.ReadFile(target); string(data) != "synced\nlocal\n" {
		t.Errorf("note = %q, want the synced text kept and the change applied on top", data)
	}
}

func TestUpdateGivesUpWhenTheNoteKeepsChanging(t *testing.T) {
	v := newNotesVault(t, "Inbox/a.md")
	target := filepath.Join(v.Root(), "Inbox", "a.md")

	calls := 0
	_, err := v.Update("Inbox/a.md", func(old []byte) ([]byte, error) {
		calls++
		if err := os.WriteFile(target, []byte(strings.Repeat("x", calls)), 0o644); err != nil {
			t.Fatal(err)
		}
		return []byte("local\n"), nil
	})
	if err == nil || !strings.Contains(err.Error(), "kept changing") {
		t.Fatalf("err = %v, want a kept-changing error", err)
	}
	if data, _ := os.ReadFile(target); string(data) != strings.Repeat("x", calls) {
		t.Errorf("note = %q, want the last synced text, not the local change", data)
	}
}

func TestUpdateWritesNothingWhenNothingChanges(t *testing.T) {
	v := newNotesVault(t, "Inbox/a.md")
	target := filepath.Join(v.Root(), "Inbox", "a.md")
	before, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := v.Update("Inbox/a.md", func(old []byte) ([]byte, error) { return old, nil }); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) {
		t.Error("Update replaced the note although its content did not change")
	}
}

func TestUpdateReportsAMissingNote(t *testing.T) {
	v := New("Personal", t.TempDir())
	if _, err := v.Update("missing.md", func(old []byte) ([]byte, error) { return old, nil }); err == nil {
		t.Error("Update accepted a missing note")
	}
}

func TestAppendDoesNotReplaceANoteCreatedMeanwhile(t *testing.T) {
	v := New("Personal", t.TempDir())
	target := filepath.Join(v.Root(), "log.md")

	calls := 0
	_, err := v.update("log.md", true, func(old []byte) ([]byte, error) {
		calls++
		if calls == 1 {
			if err := os.WriteFile(target, []byte("synced\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return append(old, "local\n"...), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(target); string(data) != "synced\nlocal\n" {
		t.Errorf("note = %q, want the synced note kept", data)
	}
}
