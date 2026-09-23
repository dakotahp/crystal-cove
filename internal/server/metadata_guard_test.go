package server

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestFrontmatterToolsRejectNonMarkdownFiles(t *testing.T) {
	s, _ := metaServer(t, map[string]string{
		"Areas/theme.css":    "body { color: red }\n",
		".obsidian/app.json": "{}\n",
		"Inbox/a.md":         tagged("idea"),
	})
	ctx := context.Background()

	for _, path := range []string{"Areas/theme.css", ".obsidian/app.json"} {
		if _, _, err := s.getFrontmatter(ctx, &mcp.CallToolRequest{}, noteRef{Vault: "Personal", Path: path}); err == nil {
			t.Errorf("getFrontmatter read %q", path)
		}
		if _, _, err := s.updateFrontmatter(ctx, &mcp.CallToolRequest{}, updateFrontmatterInput{
			Vault: "Personal", Path: path, Set: map[string]any{"status": "done"},
		}); err == nil {
			t.Errorf("updateFrontmatter rewrote %q", path)
		}
	}

	if _, _, err := s.getFrontmatter(ctx, &mcp.CallToolRequest{}, noteRef{Vault: "Personal", Path: "Inbox/a.md"}); err != nil {
		t.Errorf("getFrontmatter rejected a note: %v", err)
	}
}

func TestFrontmatterToolsAcceptUppercaseExtension(t *testing.T) {
	s, _ := metaServer(t, map[string]string{"Inbox/A.MD": tagged("idea")})

	if _, _, err := s.getFrontmatter(context.Background(), &mcp.CallToolRequest{}, noteRef{Vault: "Personal", Path: "Inbox/A.MD"}); err != nil {
		t.Errorf("getFrontmatter rejected an uppercase extension: %v", err)
	}
}

func TestNonMarkdownFileIsLeftUnchanged(t *testing.T) {
	s, v := metaServer(t, map[string]string{"Areas/theme.css": "body { color: red }\n"})

	_, _, _ = s.updateFrontmatter(context.Background(), &mcp.CallToolRequest{}, updateFrontmatterInput{
		Vault: "Personal", Path: "Areas/theme.css", Set: map[string]any{"status": "done"},
	})
	data, err := v.ReadAll("Areas/theme.css")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "body { color: red }\n" {
		t.Errorf("file = %q, want it untouched", data)
	}
}
