package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dakotahp/crystal-cove/internal/search"
	"github.com/dakotahp/crystal-cove/internal/vault"
)

func TestInstructionsAreReadPerSession(t *testing.T) {
	v := vault.New("Personal", t.TempDir())
	file := filepath.Join(v.Root(), InstructionsFile)
	if err := os.WriteFile(file, []byte("first guidance"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := New([]*vault.Vault{v}, search.New("rg", nil), func() bool { return true })

	if got := s.Instructions(); got != "first guidance" {
		t.Fatalf("Instructions = %q", got)
	}

	// An edit that syncs in from another device must reach the next session
	// without restarting the container.
	if err := os.WriteFile(file, []byte("second guidance"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := s.Instructions(); got != "second guidance" {
		t.Errorf("Instructions = %q, want the edited text", got)
	}
}

func TestInstructionsSurviveARemovedFile(t *testing.T) {
	v := vault.New("Personal", t.TempDir())
	file := filepath.Join(v.Root(), InstructionsFile)
	if err := os.WriteFile(file, []byte("guidance"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := New([]*vault.Vault{v}, search.New("rg", nil), func() bool { return true })

	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if got := s.Instructions(); got != "" {
		t.Errorf("Instructions = %q, want empty once the file is gone", got)
	}
}
