package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func listedTools(t *testing.T, s *Server) []*mcp.Tool {
	t.Helper()
	ts := httptest.NewServer(s.Handler(AuthConfig{StaticToken: "secret"}))
	t.Cleanup(ts.Close)
	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   ts.URL,
		HTTPClient: &http.Client{Transport: authTransport{token: "secret"}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	return tools.Tools
}

func toolNames(tools []*mcp.Tool) []string {
	var names []string
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	slices.Sort(names)
	return names
}

func TestReadOnlyServerOffersNoWriteTools(t *testing.T) {
	s := newTestServer(t)
	s.SetPolicy(Policy{ReadOnly: true})

	want := []string{
		"find_notes", "get_backlinks", "get_frontmatter", "get_links", "get_section",
		"list_notes", "list_tags", "list_vaults", "read_note", "recent_notes", "search_notes",
	}
	if got := toolNames(listedTools(t, s)); !slices.Equal(got, want) {
		t.Errorf("tools = %v, want %v", got, want)
	}
}

func TestSingleVaultServerOffersNoListVaults(t *testing.T) {
	s := rankServer(t, map[string]string{"Inbox/a.md": "body\n"})

	if got := toolNames(listedTools(t, s)); slices.Contains(got, "list_vaults") {
		t.Errorf("tools = %v, want no list_vaults when there is one vault", got)
	}
	_, err := s.vault("Nope")
	if err == nil || !strings.Contains(err.Error(), "holds Personal") || strings.Contains(err.Error(), "list_vaults") {
		t.Errorf("err = %v, want it to name the vault rather than point at list_vaults", err)
	}
}

func TestPermanentDeleteIsOffByDefault(t *testing.T) {
	s, v := metaServer(t, map[string]string{
		"Inbox/a.md":  "body\n",
		".trash/b.md": "trashed\n",
	})
	ctx := context.Background()

	_, _, err := s.deleteNote(ctx, &mcp.CallToolRequest{}, deleteNoteInput{Path: "Inbox/a.md", Permanent: true})
	if err == nil || !strings.Contains(err.Error(), "MCP_ALLOW_PERMANENT_DELETE") {
		t.Errorf("permanent delete err = %v, want a refusal naming MCP_ALLOW_PERMANENT_DELETE", err)
	}
	_, _, err = s.deleteNote(ctx, &mcp.CallToolRequest{}, deleteNoteInput{Path: ".trash/b.md"})
	if err == nil {
		t.Error("deleting from the trash, which is always permanent, was allowed")
	}
	for _, p := range []string{"Inbox/a.md", ".trash/b.md"} {
		if _, err := os.Stat(filepath.Join(v.Root(), filepath.FromSlash(p))); err != nil {
			t.Errorf("%s is gone after a refused delete: %v", p, err)
		}
	}

	_, out, err := s.deleteNote(ctx, &mcp.CallToolRequest{}, deleteNoteInput{Path: "Inbox/a.md"})
	if err != nil || out.TrashedTo != ".trash/a.md" {
		t.Errorf("soft delete = %+v, %v; want it moved to the trash", out, err)
	}
}

func TestPermanentDeleteWhenAllowed(t *testing.T) {
	s, v := metaServer(t, map[string]string{"Inbox/a.md": "body\n"})
	s.SetPolicy(Policy{AllowPermanentDelete: true})

	_, _, err := s.deleteNote(context.Background(), &mcp.CallToolRequest{}, deleteNoteInput{Path: "Inbox/a.md", Permanent: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(v.Root(), "Inbox", "a.md")); !os.IsNotExist(err) {
		t.Errorf("note still exists: %v", err)
	}
}

func TestDeleteToolDescribesThePolicy(t *testing.T) {
	s := newTestServer(t)
	description := func() string {
		for _, tool := range listedTools(t, s) {
			if tool.Name == "delete_note" {
				return tool.Description
			}
		}
		t.Fatal("delete_note is not registered")
		return ""
	}
	if d := description(); strings.Contains(d, "set permanent") {
		t.Errorf("description offers permanent deletion while it is off: %q", d)
	}
	s.SetPolicy(Policy{AllowPermanentDelete: true})
	if d := description(); !strings.Contains(d, "set permanent") {
		t.Errorf("description hides permanent deletion while it is on: %q", d)
	}
}
