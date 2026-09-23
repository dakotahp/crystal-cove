package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMetadataToolsRejectAnUnknownVault(t *testing.T) {
	s, _ := metaServer(t, map[string]string{"Inbox/a.md": tagged("idea")})
	ctx := context.Background()

	if _, _, err := s.listTags(ctx, &mcp.CallToolRequest{}, listTagsInput{Vault: "Missing"}); err == nil {
		t.Error("listTags accepted an unknown vault")
	}
	if _, _, err := s.findNotes(ctx, &mcp.CallToolRequest{}, findNotesInput{Vault: "Missing", Key: "status"}); err == nil {
		t.Error("findNotes accepted an unknown vault")
	}
	if _, _, err := s.getFrontmatter(ctx, &mcp.CallToolRequest{}, noteRef{Vault: "Missing", Path: "Inbox/a.md"}); err == nil {
		t.Error("getFrontmatter accepted an unknown vault")
	}
	if _, _, err := s.updateFrontmatter(ctx, &mcp.CallToolRequest{}, updateFrontmatterInput{
		Vault: "Missing", Path: "Inbox/a.md", Set: map[string]any{"a": 1},
	}); err == nil {
		t.Error("updateFrontmatter accepted an unknown vault")
	}
}

func TestFrontmatterToolsRejectAMissingNote(t *testing.T) {
	s, _ := metaServer(t, map[string]string{"Inbox/a.md": tagged("idea")})
	ctx := context.Background()

	if _, _, err := s.getFrontmatter(ctx, &mcp.CallToolRequest{}, noteRef{Vault: "Personal", Path: "Inbox/gone.md"}); err == nil {
		t.Error("getFrontmatter accepted a missing note")
	}
	if _, _, err := s.updateFrontmatter(ctx, &mcp.CallToolRequest{}, updateFrontmatterInput{
		Vault: "Personal", Path: "Inbox/gone.md", Set: map[string]any{"a": 1},
	}); err == nil {
		t.Error("updateFrontmatter accepted a missing note")
	}
}

func TestGetFrontmatterReportsBrokenYAML(t *testing.T) {
	s, _ := metaServer(t, map[string]string{"Inbox/bad.md": "---\ntags: [unclosed\n---\n\nbody\n"})

	_, _, err := s.getFrontmatter(context.Background(), &mcp.CallToolRequest{}, noteRef{Vault: "Personal", Path: "Inbox/bad.md"})
	if err == nil {
		t.Error("getFrontmatter accepted malformed frontmatter")
	}
}

func TestScanSkipsNotesWithBrokenYAML(t *testing.T) {
	s, _ := metaServer(t, map[string]string{
		"Inbox/good.md": tagged("idea"),
		"Inbox/bad.md":  "---\ntags: [unclosed\n---\n\nbody\n",
	})

	_, res, err := s.listTags(context.Background(), &mcp.CallToolRequest{}, listTagsInput{Vault: "Personal"})
	if err != nil {
		t.Fatalf("one malformed note failed the whole scan: %v", err)
	}
	if res.NotesScanned != 2 || len(res.Tags) != 1 {
		t.Errorf("res = %+v, want both notes read and the good one's tag", res)
	}
}

func TestFindNotesMatchesNonStringFieldValues(t *testing.T) {
	s, _ := metaServer(t, map[string]string{
		"Inbox/a.md": "---\npriority: 2\n---\n\nbody\n",
		"Inbox/b.md": "---\npriority: 5\n---\n\nbody\n",
	})

	_, res, err := s.findNotes(context.Background(), &mcp.CallToolRequest{}, findNotesInput{
		Vault: "Personal", Key: "priority", Value: "2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Notes) != 1 || res.Notes[0].Path != "Inbox/a.md" {
		t.Errorf("Notes = %+v, want the note whose number matches", res.Notes)
	}
}

func TestFindNotesRejectsAListValueThatDoesNotMatch(t *testing.T) {
	s, _ := metaServer(t, map[string]string{"Inbox/a.md": "---\nstatus:\n  - done\n---\n\nbody\n"})

	_, res, err := s.findNotes(context.Background(), &mcp.CallToolRequest{}, findNotesInput{
		Vault: "Personal", Key: "status", Value: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Notes) != 0 {
		t.Errorf("Notes = %+v, want no match", res.Notes)
	}
}

func TestUpdateFrontmatterReportsAnUnreadableNote(t *testing.T) {
	s, v := metaServer(t, map[string]string{"Inbox/bad.md": "---\ntags: [unclosed\n---\n\nbody\n"})

	if _, _, err := s.updateFrontmatter(context.Background(), &mcp.CallToolRequest{}, updateFrontmatterInput{
		Vault: "Personal", Path: "Inbox/bad.md", Set: map[string]any{"status": "done"},
	}); err == nil {
		t.Error("updateFrontmatter rewrote a note whose frontmatter does not parse")
	}
	data, err := os.ReadFile(filepath.Join(v.Root(), "Inbox", "bad.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "---\ntags: [unclosed\n---\n\nbody\n" {
		t.Errorf("file = %q, want it left alone", data)
	}
}
