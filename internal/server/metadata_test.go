package server

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dakotahp/crystal-cove/internal/search"
	"github.com/dakotahp/crystal-cove/internal/vault"
)

// metaServer builds a one-vault server from path/content pairs.
func metaServer(t *testing.T, notes map[string]string) (*Server, *vault.Vault) {
	t.Helper()
	v := vault.New("Personal", t.TempDir())
	for p, content := range notes {
		full := filepath.Join(v.Root(), filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return New([]*vault.Vault{v}, search.New("rg", nil), func() bool { return true }), v
}

func tagged(tags ...string) string {
	doc := "---\ntags:\n"
	for _, tag := range tags {
		doc += "  - " + tag + "\n"
	}
	return doc + "---\n\nbody\n"
}

func TestListTagsCountsAndOrdersByUse(t *testing.T) {
	s, _ := metaServer(t, map[string]string{
		"Inbox/a.md":   tagged("daily-note", "idea"),
		"Inbox/b.md":   tagged("daily-note"),
		"Areas/c.md":   tagged("daily-note", "upkeep"),
		"Areas/pic.md": "no frontmatter here\n",
	})

	_, res, err := s.listTags(context.Background(), &mcp.CallToolRequest{}, listTagsInput{Vault: "Personal"})
	if err != nil {
		t.Fatal(err)
	}
	if res.NotesScanned != 4 {
		t.Errorf("NotesScanned = %d, want 4", res.NotesScanned)
	}
	if len(res.Tags) != 3 {
		t.Fatalf("Tags = %+v, want three distinct tags", res.Tags)
	}
	if res.Tags[0].Tag != "daily-note" || res.Tags[0].Count != 3 {
		t.Errorf("Tags[0] = %+v, want the most used tag first", res.Tags[0])
	}
}

func TestFindNotesByTagRequiresEveryTag(t *testing.T) {
	s, _ := metaServer(t, map[string]string{
		"Inbox/a.md": tagged("idea", "agent-work"),
		"Inbox/b.md": tagged("idea"),
	})

	_, res, err := s.findNotes(context.Background(), &mcp.CallToolRequest{}, findNotesInput{
		Vault: "Personal",
		Tags:  []string{"idea", "agent-work"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Notes) != 1 || res.Notes[0].Path != "Inbox/a.md" {
		t.Fatalf("Notes = %+v, want only the note carrying both tags", res.Notes)
	}
	if !slices.Equal(res.Notes[0].Tags, []string{"idea", "agent-work"}) {
		t.Errorf("Tags = %q", res.Notes[0].Tags)
	}
}

func TestFindNotesReturnsAnEmptyTagListNotNull(t *testing.T) {
	s, _ := metaServer(t, map[string]string{"Inbox/a.md": "---\nstatus: active\n---\n\nbody\n"})

	_, res, err := s.findNotes(context.Background(), &mcp.CallToolRequest{}, findNotesInput{
		Vault: "Personal", Key: "status",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Notes) != 1 || res.Notes[0].Tags == nil {
		t.Errorf("Notes = %+v, want an empty tag list so it marshals as []", res.Notes)
	}
}

func TestFindNotesByFrontmatterKeyOnly(t *testing.T) {
	s, _ := metaServer(t, map[string]string{
		"Inbox/a.md": "---\nstatus: active\n---\n\nbody\n",
		"Inbox/b.md": "---\ntitle: other\n---\n\nbody\n",
	})

	_, res, err := s.findNotes(context.Background(), &mcp.CallToolRequest{}, findNotesInput{
		Vault: "Personal",
		Key:   "status",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Notes) != 1 || res.Notes[0].Path != "Inbox/a.md" {
		t.Errorf("Notes = %+v, want the note carrying the key", res.Notes)
	}
}

func TestFindNotesByFrontmatterKeyAndValue(t *testing.T) {
	s, _ := metaServer(t, map[string]string{
		"Inbox/a.md": "---\nstatus: active\n---\n\nbody\n",
		"Inbox/b.md": "---\nstatus: done\n---\n\nbody\n",
		"Inbox/c.md": "---\nstatus:\n  - active\n  - urgent\n---\n\nbody\n",
	})

	_, res, err := s.findNotes(context.Background(), &mcp.CallToolRequest{}, findNotesInput{
		Vault: "Personal",
		Key:   "status",
		Value: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, n := range res.Notes {
		paths = append(paths, n.Path)
	}
	want := []string{"Inbox/a.md", "Inbox/c.md"}
	if !slices.Equal(paths, want) {
		t.Errorf("paths = %q, want %q (a list value counts as a match)", paths, want)
	}
}

func TestFindNotesRequiresAFilter(t *testing.T) {
	s, _ := metaServer(t, map[string]string{"Inbox/a.md": tagged("idea")})

	if _, _, err := s.findNotes(context.Background(), &mcp.CallToolRequest{}, findNotesInput{Vault: "Personal"}); err == nil {
		t.Error("findNotes accepted a query with no filter")
	}
}

func TestFindNotesTruncatesAtMaxResults(t *testing.T) {
	s, _ := metaServer(t, map[string]string{
		"Inbox/a.md": tagged("idea"),
		"Inbox/b.md": tagged("idea"),
		"Inbox/c.md": tagged("idea"),
	})

	_, res, err := s.findNotes(context.Background(), &mcp.CallToolRequest{}, findNotesInput{
		Vault: "Personal", Tags: []string{"idea"}, MaxResults: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Notes) != 2 || !res.Truncated {
		t.Errorf("Notes = %d, Truncated = %v; want 2 and true", len(res.Notes), res.Truncated)
	}
}

func TestGetFrontmatterReturnsFieldsAndTags(t *testing.T) {
	s, _ := metaServer(t, map[string]string{
		"Inbox/a.md": "---\nstatus: active\ntags:\n  - idea\n---\n\nbody\n",
		"Inbox/b.md": "plain note\n",
	})

	_, res, err := s.getFrontmatter(context.Background(), &mcp.CallToolRequest{}, noteRef{Vault: "Personal", Path: "Inbox/a.md"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.HasFrontmatter || res.Frontmatter["status"] != "active" {
		t.Errorf("res = %+v", res)
	}
	if !slices.Equal(res.Tags, []string{"idea"}) {
		t.Errorf("Tags = %q", res.Tags)
	}

	_, plain, err := s.getFrontmatter(context.Background(), &mcp.CallToolRequest{}, noteRef{Vault: "Personal", Path: "Inbox/b.md"})
	if err != nil {
		t.Fatal(err)
	}
	if plain.HasFrontmatter || len(plain.Frontmatter) != 0 {
		t.Errorf("plain = %+v, want no fields", plain)
	}
}

func TestUpdateFrontmatterWritesAndLeavesBodyAlone(t *testing.T) {
	s, v := metaServer(t, map[string]string{
		"Inbox/a.md": "---\nstatus: active\ntitle: keep me\n---\n\nbody text\n",
	})

	_, res, err := s.updateFrontmatter(context.Background(), &mcp.CallToolRequest{}, updateFrontmatterInput{
		Vault:  "Personal",
		Path:   "Inbox/a.md",
		Set:    map[string]any{"status": "done"},
		Remove: []string{"title"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Frontmatter["status"] != "done" {
		t.Errorf("Frontmatter = %+v", res.Frontmatter)
	}
	data, err := v.ReadAll("Inbox/a.md")
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if want := "---\nstatus: done\n---\n\nbody text\n"; got != want {
		t.Errorf("file = %q, want %q", got, want)
	}
}

func TestUpdateFrontmatterRejectsAnEmptyChange(t *testing.T) {
	s, _ := metaServer(t, map[string]string{"Inbox/a.md": "---\nstatus: active\n---\n\nbody\n"})

	if _, _, err := s.updateFrontmatter(context.Background(), &mcp.CallToolRequest{}, updateFrontmatterInput{
		Vault: "Personal", Path: "Inbox/a.md",
	}); err == nil {
		t.Error("updateFrontmatter accepted a call that changes nothing")
	}
}
