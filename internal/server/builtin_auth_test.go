package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"

	"github.com/dakotahp/crystal-cove/internal/authserver"
)

const ownerPassword = "correct-horse-battery-staple"

// builtinTestServer serves the MCP server with the built-in sign-in at a
// real loopback URL, which is also its public URL.
func builtinTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	s := newTestServer(t)
	var handler http.Handler
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { handler.ServeHTTP(w, r) }))
	t.Cleanup(ts.Close)
	signIn, err := authserver.New(authserver.Options{PublicURL: ts.URL, Password: ownerPassword, StoreDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	handler = s.Handler(AuthConfig{
		StaticToken: "secret",
		Builtin:     &BuiltinAuth{Handler: signIn.Handler(), Verify: signIn.Verify, PublicURL: ts.URL},
	})
	return ts
}

func TestBuiltinProtectedResourceMetadataNamesItself(t *testing.T) {
	ts := builtinTestServer(t)
	res, err := http.Get(ts.URL + "/.well-known/oauth-protected-resource")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var meta struct {
		Resource             string   `json:"resource"`
		AuthorizationServers []string `json:"authorization_servers"`
	}
	if err := json.NewDecoder(res.Body).Decode(&meta); err != nil {
		t.Fatal(err)
	}
	if meta.Resource != ts.URL || fmt.Sprint(meta.AuthorizationServers) != fmt.Sprint([]string{ts.URL}) {
		t.Errorf("metadata = %+v", meta)
	}

	res, err = http.Get(ts.URL + "/.well-known/oauth-authorization-server")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Errorf("authorization server metadata status = %d", res.StatusCode)
	}
}

func TestBuiltinUnauthorizedPointsToMetadata(t *testing.T) {
	ts := builtinTestServer(t)
	res, err := http.Post(ts.URL+"/", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized ||
		!strings.Contains(res.Header.Get("WWW-Authenticate"), ts.URL+"/.well-known/oauth-protected-resource") {
		t.Errorf("status = %d, WWW-Authenticate = %q", res.StatusCode, res.Header.Get("WWW-Authenticate"))
	}
}

// TestBuiltinSignInEndToEnd connects the SDK's own OAuth client: it
// discovers the server, registers, signs in with the owner password,
// exchanges the code, and calls a tool.
func TestBuiltinSignInEndToEnd(t *testing.T) {
	ts := builtinTestServer(t)
	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	fetch := func(_ context.Context, args *auth.AuthorizationArgs) (*auth.AuthorizationResult, error) {
		u, err := url.Parse(args.URL)
		if err != nil {
			return nil, err
		}
		form := u.Query()
		form.Set("password", ownerPassword)
		res, err := noRedirect.PostForm(ts.URL+"/authorize", form)
		if err != nil {
			return nil, err
		}
		res.Body.Close()
		loc, err := url.Parse(res.Header.Get("Location"))
		if err != nil {
			return nil, err
		}
		q := loc.Query()
		if q.Get("code") == "" {
			return nil, fmt.Errorf("sign-in returned status %d and Location %q", res.StatusCode, loc)
		}
		return &auth.AuthorizationResult{Code: q.Get("code"), State: q.Get("state"), Iss: q.Get("iss")}, nil
	}
	oauth, err := auth.NewAuthorizationCodeHandler(&auth.AuthorizationCodeHandlerConfig{
		DynamicClientRegistrationConfig: &auth.DynamicClientRegistrationConfig{
			Metadata: &oauthex.ClientRegistrationMetadata{ClientName: "e2e", RedirectURIs: []string{"http://127.0.0.1/callback"}},
		},
		AuthorizationCodeFetcher: fetch,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "oauth-e2e", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: ts.URL, OAuthHandler: oauth}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "read_note",
		Arguments: map[string]any{"vault": "Work", "path": "note.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if text, ok := res.Content[0].(*mcp.TextContent); res.IsError || !ok || !strings.Contains(text.Text, "hello world") {
		t.Errorf("read_note = %+v", res.Content)
	}
}

func TestBuiltinKeepsStaticToken(t *testing.T) {
	ts := builtinTestServer(t)
	client := mcp.NewClient(&mcp.Implementation{Name: "static", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:   ts.URL,
		HTTPClient: &http.Client{Transport: authTransport{token: "secret"}},
	}, nil)
	if err != nil {
		t.Fatalf("static token rejected beside the built-in sign-in: %v", err)
	}
	session.Close()
}
