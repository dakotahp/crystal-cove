package server

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func auditedSession(t *testing.T, authCfg AuthConfig, token string) (*mcp.ClientSession, *lockedBuffer) {
	t.Helper()
	s := newTestServer(t)
	logs := &lockedBuffer{}
	s.SetAuditLog(slog.New(slog.NewTextHandler(logs, nil)))
	ts := httptest.NewServer(s.Handler(authCfg))
	t.Cleanup(ts.Close)
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:   ts.URL,
		HTTPClient: &http.Client{Transport: authTransport{token: token}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session, logs
}

func call(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) {
	t.Helper()
	if _, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args}); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
}

func TestAuditLogRecordsWhoTouchedWhichNote(t *testing.T) {
	session, logs := auditedSession(t, AuthConfig{StaticToken: "secret"}, "secret")

	call(t, session, "create_note", map[string]any{"vault": "Work", "path": "ideas/new.md", "content": "SECRET-CONTENT"})
	call(t, session, "read_note", map[string]any{"vault": "Work", "path": "missing.md"})
	call(t, session, "search_notes", map[string]any{"vault": "Work", "query": "SECRET-QUERY"})
	call(t, session, "edit_note", map[string]any{"vault": "Work", "path": "note.md", "find": "hello", "replace": "SECRET-EDIT"})

	got := logs.String()
	for _, want := range []string{
		`tool=create_note caller=api-key outcome=ok`, `vault=Work path=ideas/new.md`,
		`tool=read_note caller=api-key outcome=tool_error`, `path=missing.md`,
		`tool=search_notes`, `tool=edit_note`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("audit log lacks %q:\n%s", want, got)
		}
	}
	for _, secret := range []string{"SECRET-CONTENT", "SECRET-QUERY", "SECRET-EDIT"} {
		if strings.Contains(got, secret) {
			t.Errorf("audit log recorded %s:\n%s", secret, got)
		}
	}
}

func TestAuditLogNamesTheOIDCSubject(t *testing.T) {
	verify := func(context.Context, string) (*auth.TokenInfo, error) {
		return &auth.TokenInfo{UserID: "user-123", Expiration: time.Now().Add(time.Hour)}, nil
	}
	session, logs := auditedSession(t, AuthConfig{OIDC: &OIDCAuth{
		Verify: verify, Issuer: "https://idp.example.com", PublicURL: "https://obsidian.example.com",
	}}, "any-jwt")

	call(t, session, "list_vaults", nil)
	if got := logs.String(); !strings.Contains(got, "tool=list_vaults caller=user-123 outcome=ok") {
		t.Errorf("audit log = %q, want the token subject as caller", got)
	}
}

func TestAuditLogRecordsRejectedCalls(t *testing.T) {
	session, logs := auditedSession(t, AuthConfig{StaticToken: "secret"}, "secret")

	if _, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "no_such_tool"}); err == nil {
		t.Fatal("calling an unknown tool succeeded")
	}
	if got := logs.String(); !strings.Contains(got, "tool=no_such_tool caller=api-key outcome=error") {
		t.Errorf("audit log = %q, want the rejected call", got)
	}
}

func TestAuditLogRecordsRejectedTokens(t *testing.T) {
	s := newTestServer(t)
	logs := &lockedBuffer{}
	s.SetAuditLog(slog.New(slog.NewTextHandler(logs, nil)))
	ts := httptest.NewServer(s.Handler(AuthConfig{StaticToken: "secret"}))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL, strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer GUESSED-TOKEN")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	got := logs.String()
	if res.StatusCode != http.StatusUnauthorized || !strings.Contains(got, "rejected bearer token") || !strings.Contains(got, "remote_addr=127.0.0.1") {
		t.Errorf("status %d, audit log = %q; want a 401 and a logged rejection", res.StatusCode, got)
	}
	if strings.Contains(got, "GUESSED-TOKEN") {
		t.Errorf("audit log recorded the presented token: %q", got)
	}
}

func TestAuditLogCapsLongArguments(t *testing.T) {
	session, logs := auditedSession(t, AuthConfig{StaticToken: "secret"}, "secret")

	call(t, session, "read_note", map[string]any{"vault": "Work", "path": strings.Repeat("a", 1000) + ".md"})
	if got := logs.String(); strings.Contains(got, strings.Repeat("a", 300)) {
		t.Errorf("audit log kept a 1000-character path:\n%s", got)
	}
}
