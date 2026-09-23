package server

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestLinkToolsRejectAnUnknownVault(t *testing.T) {
	s, _ := metaServer(t, map[string]string{"Inbox/a.md": "body\n"})
	ctx := context.Background()

	if _, _, err := s.getLinks(ctx, &mcp.CallToolRequest{}, noteRef{Vault: "Missing", Path: "Inbox/a.md"}); err == nil {
		t.Error("get_links accepted an unknown vault")
	}
	if _, _, err := s.getBacklinks(ctx, &mcp.CallToolRequest{}, noteRef{Vault: "Missing", Path: "Inbox/a.md"}); err == nil {
		t.Error("get_backlinks accepted an unknown vault")
	}
}

func TestGetBacklinksReportsAMissingNote(t *testing.T) {
	s, _ := metaServer(t, map[string]string{"Inbox/a.md": "body\n"})

	if _, _, err := s.getBacklinks(context.Background(), &mcp.CallToolRequest{}, noteRef{Path: "Inbox/gone.md"}); err == nil {
		t.Error("get_backlinks accepted a missing note")
	}
}

func TestGetLinksOnANoteWithoutLinks(t *testing.T) {
	s, _ := metaServer(t, map[string]string{"Inbox/a.md": "no links here\n"})

	_, res, err := s.getLinks(context.Background(), &mcp.CallToolRequest{}, noteRef{Path: "Inbox/a.md"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Links == nil || len(res.Links) != 0 {
		t.Errorf("Links = %+v, want an empty list", res.Links)
	}
}

func TestGetLinksReportsBrokenFrontmatter(t *testing.T) {
	s, _ := metaServer(t, map[string]string{"Inbox/bad.md": "---\ntags: [unclosed\n---\n\n[[One]]\n"})

	if _, _, err := s.getLinks(context.Background(), &mcp.CallToolRequest{}, noteRef{Path: "Inbox/bad.md"}); err == nil {
		t.Error("get_links accepted a note whose frontmatter does not parse")
	}
}
