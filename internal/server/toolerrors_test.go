package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func testSession(t *testing.T, s *Server) *mcp.ClientSession {
	t.Helper()
	ts := httptest.NewServer(s.Handler(AuthConfig{StaticToken: "secret"}))
	t.Cleanup(ts.Close)
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:   ts.URL,
		HTTPClient: &http.Client{Transport: authTransport{token: "secret"}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func toolError(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) string {
	t.Helper()
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if !res.IsError {
		t.Fatalf("%s succeeded, want a tool error", name)
	}
	text, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("%s: content = %+v", name, res.Content)
	}
	return text.Text
}

func TestUnknownArgumentErrorListsTheToolsParameters(t *testing.T) {
	session := testSession(t, newTestServer(t))

	got := toolError(t, session, "find_notes", map[string]any{"vault": "Work", "query": "plan"})
	for _, want := range []string{`"query"`, "frontmatter_key", "frontmatter_value", "max_results", "tags", "vault"} {
		if !strings.Contains(got, want) {
			t.Errorf("error lacks %q:\n%s", want, got)
		}
	}

	got = toolError(t, session, "read_note", map[string]any{"vault": "Work", "path": "note.md", "limit": 10})
	if !strings.Contains(got, "required: path") {
		t.Errorf("error does not name the required parameters:\n%s", got)
	}
	if !strings.Contains(got, "offset") {
		t.Errorf("error does not list the optional parameters:\n%s", got)
	}
}

func TestMissingNoteIsReportedAsNotFound(t *testing.T) {
	session := testSession(t, newTestServer(t))

	for name, args := range map[string]map[string]any{
		"read_note":       {"vault": "Work", "path": "missing.md"},
		"get_frontmatter": {"vault": "Work", "path": "missing.md"},
		"edit_note":       {"vault": "Work", "path": "missing.md", "find": "a", "replace": "b"},
		"move_note":       {"vault": "Work", "path": "missing.md", "new_path": "other.md"},
		"delete_note":     {"vault": "Work", "path": "missing.md"},
	} {
		got := toolError(t, session, name, args)
		if got != `not_found: no note at "missing.md" in vault "Work"` {
			t.Errorf("%s error = %q", name, got)
		}
	}
}
