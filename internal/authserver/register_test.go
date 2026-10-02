package authserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/oauthex"
)

func register(t *testing.T, s *Server, meta map[string]any) (int, map[string]any) {
	t.Helper()
	body, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	rec := serve(s.Handler(), http.MethodPost, "/register", bytes.NewReader(body), nil)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestRegisterPublicClient(t *testing.T) {
	s, _ := newTestAuthServer(t)
	code, out := register(t, s, map[string]any{
		"client_name":                "Claude",
		"redirect_uris":              []string{"https://claude.ai/api/mcp/auth_callback"},
		"token_endpoint_auth_method": "none",
		"grant_types":                []string{"authorization_code", "refresh_token"},
	})
	if code != http.StatusCreated {
		t.Fatalf("status = %d, body = %v", code, out)
	}
	id, _ := out["client_id"].(string)
	if id == "" || out["client_secret"] != nil {
		t.Fatalf("response = %v, want a client_id and no secret", out)
	}
	c, ok := s.store.client(id)
	if !ok || c.Name != "Claude" || c.AuthMethod != "none" {
		t.Errorf("stored client = %+v", c)
	}
}

func TestRegisterDefaultsToSecretBasic(t *testing.T) {
	s, _ := newTestAuthServer(t)
	code, out := register(t, s, map[string]any{"redirect_uris": []string{"https://chatgpt.com/cb"}})
	if code != http.StatusCreated {
		t.Fatalf("status = %d, body = %v", code, out)
	}
	secret, _ := out["client_secret"].(string)
	if out["token_endpoint_auth_method"] != "client_secret_basic" || secret == "" {
		t.Fatalf("response = %v, want client_secret_basic with a secret", out)
	}
	c, _ := s.store.client(out["client_id"].(string))
	if c.SecretHash != hashToken(secret) {
		t.Error("stored secret hash does not match the returned secret")
	}
}

func TestRegisterValidation(t *testing.T) {
	cases := []struct {
		name string
		meta map[string]any
		want string
	}{
		{"no redirect uris", map[string]any{}, "invalid_redirect_uri"},
		{"plain http off loopback", map[string]any{"redirect_uris": []string{"http://evil.example.com/cb"}}, "invalid_redirect_uri"},
		{"relative", map[string]any{"redirect_uris": []string{"/cb"}}, "invalid_redirect_uri"},
		{"fragment", map[string]any{"redirect_uris": []string{"https://claude.ai/cb#x"}}, "invalid_redirect_uri"},
		{"custom scheme", map[string]any{"redirect_uris": []string{"myapp://cb"}}, "invalid_redirect_uri"},
		{"unknown auth method", map[string]any{"redirect_uris": []string{"https://claude.ai/cb"}, "token_endpoint_auth_method": "private_key_jwt"}, "invalid_client_metadata"},
		{"unknown grant type", map[string]any{"redirect_uris": []string{"https://claude.ai/cb"}, "grant_types": []string{"password"}}, "invalid_client_metadata"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, _ := newTestAuthServer(t)
			code, out := register(t, s, c.meta)
			if code != http.StatusBadRequest || out["error"] != c.want {
				t.Errorf("status = %d, body = %v; want 400 %s", code, out, c.want)
			}
		})
	}
}

func TestRegisterAllowsLoopbackHTTP(t *testing.T) {
	s, _ := newTestAuthServer(t)
	for _, u := range []string{"http://127.0.0.1:33418/cb", "http://localhost/cb", "http://[::1]/cb"} {
		if code, out := register(t, s, map[string]any{"redirect_uris": []string{u}, "token_endpoint_auth_method": "none"}); code != http.StatusCreated {
			t.Errorf("%s: status = %d, body = %v", u, code, out)
		}
	}
}

func TestRegisterRejectsNonJSON(t *testing.T) {
	s, _ := newTestAuthServer(t)
	rec := serve(s.Handler(), http.MethodPost, "/register", strings.NewReader("{"), nil)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_client_metadata") {
		t.Errorf("status = %d, body = %s", rec.Code, rec.Body)
	}
}

func TestRegisterWhenFull(t *testing.T) {
	s, clock := newTestAuthServer(t)
	for i := range maxClients {
		id := fmt.Sprintf("c%d", i)
		if err := s.store.addClient(&client{ID: id, AuthMethod: "none"}); err != nil {
			t.Fatal(err)
		}
		if err := s.store.createGrant(&grant{ID: "g" + id, ClientID: id, CreatedAt: clock.now(), RefreshHash: "r" + id}); err != nil {
			t.Fatal(err)
		}
	}
	code, out := register(t, s, map[string]any{"redirect_uris": []string{"https://claude.ai/cb"}, "token_endpoint_auth_method": "none"})
	if code != http.StatusServiceUnavailable || out["error"] != "temporarily_unavailable" {
		t.Errorf("status = %d, body = %v", code, out)
	}
}

func TestRegisterWithoutEntropy(t *testing.T) {
	s, _ := newTestAuthServer(t)
	s.rand = failingReader{}
	code, out := register(t, s, map[string]any{"redirect_uris": []string{"https://claude.ai/cb"}})
	if code != http.StatusInternalServerError || out["error"] != "server_error" {
		t.Errorf("status = %d, body = %v", code, out)
	}
}

func TestRegisterResponseParsesWithSDK(t *testing.T) {
	s, _ := newTestAuthServer(t)
	body, _ := json.Marshal(map[string]any{"redirect_uris": []string{"https://claude.ai/cb"}, "client_name": "Claude"})
	rec := serve(s.Handler(), http.MethodPost, "/register", bytes.NewReader(body), nil)
	var resp oauthex.ClientRegistrationResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.ClientID == "" || resp.ClientIDIssuedAt.IsZero() || resp.ClientName != "Claude" {
		t.Errorf("SDK view of the response = %+v", resp)
	}
}
