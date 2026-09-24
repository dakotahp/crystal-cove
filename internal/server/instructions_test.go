package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/andyjmorgan/obsidian-hosted-mcp/internal/search"
	"github.com/andyjmorgan/obsidian-hosted-mcp/internal/vault"
)

func writeInstructions(t *testing.T, v *vault.Vault, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(v.Root(), InstructionsFile), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestInstructionsFromSingleVaultOmitHeading(t *testing.T) {
	v := vault.New("Personal", t.TempDir())
	writeInstructions(t, v, "Daily notes live in Inbox.\n")

	got := New([]*vault.Vault{v}, search.New("rg", nil), func() bool { return true }).Instructions()
	if got != "Daily notes live in Inbox." {
		t.Errorf("instructions = %q", got)
	}
}

func TestInstructionsFromSeveralVaultsAreLabelled(t *testing.T) {
	work := vault.New("Work", t.TempDir())
	personal := vault.New("Personal", t.TempDir())
	writeInstructions(t, work, "Work notes use ticket ids.")
	writeInstructions(t, personal, "Daily notes live in Inbox.")

	got := New([]*vault.Vault{work, personal}, search.New("rg", nil), func() bool { return true }).Instructions()
	for _, want := range []string{"## Vault: Work", "ticket ids", "## Vault: Personal", "Inbox"} {
		if !strings.Contains(got, want) {
			t.Errorf("instructions = %q, want mention of %q", got, want)
		}
	}
}

func TestInstructionsFallBackToSyncedFile(t *testing.T) {
	v := vault.New("Personal", t.TempDir())
	if err := os.WriteFile(filepath.Join(v.Root(), SyncedInstructionsFile), []byte("synced guidance"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := New([]*vault.Vault{v}, search.New("rg", nil), func() bool { return true }).Instructions(); got != "synced guidance" {
		t.Errorf("instructions = %q", got)
	}
}

func TestInstructionsPreferDotfileOverSyncedFile(t *testing.T) {
	v := vault.New("Personal", t.TempDir())
	writeInstructions(t, v, "server copy")
	if err := os.WriteFile(filepath.Join(v.Root(), SyncedInstructionsFile), []byte("synced copy"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := New([]*vault.Vault{v}, search.New("rg", nil), func() bool { return true }).Instructions(); got != "server copy" {
		t.Errorf("instructions = %q", got)
	}
}

func TestInstructionsEmptyWithoutFile(t *testing.T) {
	v := vault.New("Personal", t.TempDir())
	if got := New([]*vault.Vault{v}, search.New("rg", nil), func() bool { return true }).Instructions(); got != "" {
		t.Errorf("instructions = %q, want empty", got)
	}
}

func TestInstructionsIgnoreASymlinkOutOfTheVault(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "secret.md")
	if err := os.WriteFile(outside, []byte("server secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	v := vault.New("Personal", t.TempDir())
	if err := os.Symlink(outside, filepath.Join(v.Root(), InstructionsFile)); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if got := New([]*vault.Vault{v}, search.New("rg", nil), func() bool { return true }).Instructions(); got != "" {
		t.Errorf("instructions = %q, want nothing read through the symlink", got)
	}
}

func TestInstructionsSkipVaultsWithoutFile(t *testing.T) {
	work := vault.New("Work", t.TempDir())
	personal := vault.New("Personal", t.TempDir())
	writeInstructions(t, personal, "Daily notes live in Inbox.")

	got := New([]*vault.Vault{work, personal}, search.New("rg", nil), func() bool { return true }).Instructions()
	if strings.Contains(got, "Work") {
		t.Errorf("instructions = %q, want no section for the vault without a file", got)
	}
	if !strings.Contains(got, "Inbox") {
		t.Errorf("instructions = %q, want the vault with a file", got)
	}
}

func TestInstructionsReachClientOnInitialize(t *testing.T) {
	v := vault.New("Personal", t.TempDir())
	writeInstructions(t, v, "Daily notes live in Inbox.")
	srv := New([]*vault.Vault{v}, search.New("rg", nil), func() bool { return true })

	ts := httptest.NewServer(srv.Handler(AuthConfig{StaticToken: "secret"}))
	defer ts.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "instructions-test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:   ts.URL,
		HTTPClient: &http.Client{Transport: authTransport{token: "secret"}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	if got := session.InitializeResult().Instructions; got != "Daily notes live in Inbox." {
		t.Errorf("Instructions = %q", got)
	}
}
