package server

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestGetLinksResolvesTargets(t *testing.T) {
	s, _ := metaServer(t, map[string]string{
		"Inbox/a.md":                "Read [[Bike Maintenance]] and [[Nothing Yet]].\n",
		"Areas/Bike Maintenance.md": "body\n",
	})

	_, res, err := s.getLinks(context.Background(), &mcp.CallToolRequest{}, noteRef{Path: "Inbox/a.md"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Links) != 2 {
		t.Fatalf("Links = %+v", res.Links)
	}
	if res.Links[0].Path != "Areas/Bike Maintenance.md" || !res.Links[0].Resolved {
		t.Errorf("Links[0] = %+v, want the note it points at", res.Links[0])
	}
	if res.Links[1].Resolved || res.Links[1].Target != "Nothing Yet" {
		t.Errorf("Links[1] = %+v, want an unresolved link", res.Links[1])
	}
}

func TestGetLinksFollowsAPathLink(t *testing.T) {
	s, _ := metaServer(t, map[string]string{
		"Inbox/a.md":       "[[Areas/Notes]]\n",
		"Areas/Notes.md":   "body\n",
		"Archive/Notes.md": "older\n",
	})

	_, res, err := s.getLinks(context.Background(), &mcp.CallToolRequest{}, noteRef{Path: "Inbox/a.md"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Links) != 1 || res.Links[0].Path != "Areas/Notes.md" {
		t.Errorf("Links = %+v, want the path link resolved exactly", res.Links)
	}
}

func TestGetBacklinksFindsEveryLinkingNote(t *testing.T) {
	s, _ := metaServer(t, map[string]string{
		"Areas/Bike Maintenance.md": "body\n",
		"Inbox/a.md":                "see [[Bike Maintenance]]\n",
		"Inbox/b.md":                "see [[Areas/Bike Maintenance|the guide]]\n",
		"Inbox/c.md":                "unrelated\n",
	})

	_, res, err := s.getBacklinks(context.Background(), &mcp.CallToolRequest{}, noteRef{Path: "Areas/Bike Maintenance.md"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Backlinks) != 2 {
		t.Fatalf("Backlinks = %+v, want both linking notes", res.Backlinks)
	}
	if res.Backlinks[0].Path != "Inbox/a.md" || res.Backlinks[1].Path != "Inbox/b.md" {
		t.Errorf("Backlinks = %+v, want path order", res.Backlinks)
	}
	if res.NotesScanned != 4 {
		t.Errorf("NotesScanned = %d", res.NotesScanned)
	}
}

func TestGetBacklinksIgnoresSelfLinks(t *testing.T) {
	s, _ := metaServer(t, map[string]string{
		"Areas/Notes.md": "this note links to [[Notes]] itself\n",
	})

	_, res, err := s.getBacklinks(context.Background(), &mcp.CallToolRequest{}, noteRef{Path: "Areas/Notes.md"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Backlinks) != 0 {
		t.Errorf("Backlinks = %+v, want none", res.Backlinks)
	}
}

func TestLinkToolsRejectNonNotes(t *testing.T) {
	s, _ := metaServer(t, map[string]string{
		"Inbox/a.md":      "body\n",
		"Areas/theme.css": "body {}\n",
	})
	ctx := context.Background()

	if _, _, err := s.getLinks(ctx, &mcp.CallToolRequest{}, noteRef{Path: "Areas/theme.css"}); err == nil {
		t.Error("get_links accepted a non-note file")
	}
	if _, _, err := s.getBacklinks(ctx, &mcp.CallToolRequest{}, noteRef{Path: "Areas/theme.css"}); err == nil {
		t.Error("get_backlinks accepted a non-note file")
	}
}

func TestGetLinksReportsAMissingNote(t *testing.T) {
	s, _ := metaServer(t, map[string]string{"Inbox/a.md": "body\n"})

	if _, _, err := s.getLinks(context.Background(), &mcp.CallToolRequest{}, noteRef{Path: "Inbox/gone.md"}); err == nil {
		t.Error("get_links accepted a missing note")
	}
}
