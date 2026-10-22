package vault

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func noContent() ([]byte, error) { return nil, nil }

func appendLine(line string) func([]byte) ([]byte, error) {
	return func(old []byte) ([]byte, error) { return append(old, line...), nil }
}

func TestUpsertCreatesAMissingNoteFromItsInitialText(t *testing.T) {
	v := newTestVault(t)
	created, err := v.Upsert("Daily/2026-03-01.md", func() ([]byte, error) { return []byte("# Template\n"), nil }, appendLine("entry\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Error("created = false for a new note")
	}
	if got := mustReadFile(t, v, "Daily/2026-03-01.md"); got != "# Template\nentry\n" {
		t.Errorf("content = %q", got)
	}
}

func TestUpsertLeavesTheInitialTextUnreadForAnExistingNote(t *testing.T) {
	v := newTestVault(t)
	mustWrite(t, v, "log.md", "one\n")
	created, err := v.Upsert("log.md", func() ([]byte, error) {
		t.Error("initial was called for an existing note")
		return nil, nil
	}, appendLine("two\n"))
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Error("created = true for an existing note")
	}
	if got := mustReadFile(t, v, "log.md"); got != "one\ntwo\n" {
		t.Errorf("content = %q", got)
	}
}

func TestUpsertWritesNothingWhenTheInitialTextFails(t *testing.T) {
	v := newTestVault(t)
	boom := errors.New("no template")
	_, err := v.Upsert("new.md", func() ([]byte, error) { return nil, boom }, appendLine("x"))
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
	if _, err := os.Stat(filepath.Join(v.Root(), "new.md")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("note exists after a failed upsert: %v", err)
	}
}

func TestUpsertRefusesPathsOutsideTheVault(t *testing.T) {
	v := newTestVault(t)
	if _, err := v.Upsert("../escape.md", nil, appendLine("x")); err == nil {
		t.Error("Upsert escaped the root")
	}
}

func TestUpsertRefusesACaseVariantOfAnExistingNote(t *testing.T) {
	v := newTestVault(t)
	mustWrite(t, v, "Daily/Today.md", "x")
	if _, err := v.Upsert("daily/today.md", nil, appendLine("y")); err == nil {
		t.Error("Upsert created a note differing only in case")
	}
}

func TestUpsertRefusesAFileAsAParentFolder(t *testing.T) {
	v := newTestVault(t)
	mustWrite(t, v, "log.md", "x")
	if _, err := v.Upsert("log.md/child.md", nil, appendLine("y")); err == nil {
		t.Error("Upsert treated a file as a directory")
	}
}
