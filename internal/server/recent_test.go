package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/andyjmorgan/obsidian-hosted-mcp/internal/search"
	"github.com/andyjmorgan/obsidian-hosted-mcp/internal/vault"
)

func ageNote(t *testing.T, v *vault.Vault, rel string, age time.Duration) {
	t.Helper()
	when := time.Now().Add(-age)
	if err := os.Chtimes(filepath.Join(v.Root(), filepath.FromSlash(rel)), when, when); err != nil {
		t.Fatal(err)
	}
}

func TestRecentNotesReturnsNewestFirst(t *testing.T) {
	s, v := metaServer(t, map[string]string{
		"Inbox/old.md": tagged("idea"),
		"Inbox/new.md": tagged("idea"),
	})
	ageNote(t, v, "Inbox/old.md", 48*time.Hour)

	_, res, err := s.recentNotes(context.Background(), &mcp.CallToolRequest{}, recentNotesInput{Vault: "Personal"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Notes) != 2 || res.Notes[0].Path != "Inbox/new.md" {
		t.Fatalf("Notes = %+v, want the newest note first", res.Notes)
	}
	if res.Notes[0].Modified.IsZero() {
		t.Error("Modified is zero")
	}
}

func TestRecentNotesAcceptsSinceAndLimit(t *testing.T) {
	s, v := metaServer(t, map[string]string{
		"Inbox/old.md": tagged("idea"),
		"Inbox/new.md": tagged("idea"),
	})
	ageNote(t, v, "Inbox/old.md", 48*time.Hour)

	_, res, err := s.recentNotes(context.Background(), &mcp.CallToolRequest{}, recentNotesInput{
		Vault: "Personal", Since: time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Notes) != 1 || res.Notes[0].Path != "Inbox/new.md" {
		t.Errorf("Notes = %+v, want only the recent note", res.Notes)
	}

	_, res, err = s.recentNotes(context.Background(), &mcp.CallToolRequest{}, recentNotesInput{Vault: "Personal", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Notes) != 1 {
		t.Errorf("Notes = %+v, want the limit applied", res.Notes)
	}
}

func TestRecentNotesRejectsAnUnreadableSince(t *testing.T) {
	s, _ := metaServer(t, map[string]string{"Inbox/a.md": tagged("idea")})

	if _, _, err := s.recentNotes(context.Background(), &mcp.CallToolRequest{}, recentNotesInput{
		Vault: "Personal", Since: "last tuesday",
	}); err == nil {
		t.Error("recentNotes accepted a since value that is not a timestamp")
	}
}

func TestVaultDefaultsToTheOnlyVault(t *testing.T) {
	s, _ := metaServer(t, map[string]string{"Inbox/a.md": tagged("idea")})

	_, res, err := s.listTags(context.Background(), &mcp.CallToolRequest{}, listTagsInput{})
	if err != nil {
		t.Fatalf("listTags without a vault name: %v", err)
	}
	if len(res.Tags) != 1 {
		t.Errorf("Tags = %+v", res.Tags)
	}
}

func TestVaultNameRequiredWithSeveralVaults(t *testing.T) {
	work := vault.New("Work", t.TempDir())
	personal := vault.New("Personal", t.TempDir())
	s := New([]*vault.Vault{work, personal}, search.New("rg", nil), func() bool { return true })

	_, _, err := s.listTags(context.Background(), &mcp.CallToolRequest{}, listTagsInput{})
	if err == nil {
		t.Fatal("listTags guessed a vault although this server holds two")
	}
	if got := err.Error(); got == "" || !contains(got, "Personal") || !contains(got, "Work") {
		t.Errorf("err = %q, want it to name the vaults", got)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || len(needle) == 0 ||
		func() bool {
			for i := 0; i+len(needle) <= len(haystack); i++ {
				if haystack[i:i+len(needle)] == needle {
					return true
				}
			}
			return false
		}())
}
