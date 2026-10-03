package authserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
)

const (
	testPassword  = "correct-horse-battery-staple"
	testPublicURL = "https://vault.example.com"
)

func newTestAuthServer(tb testing.TB) (*Server, *testClock) {
	tb.Helper()
	clock := newTestClock()
	s, err := New(Options{PublicURL: testPublicURL + "/", Password: testPassword, StoreDir: tb.TempDir(), Now: clock.now})
	if err != nil {
		tb.Fatal(err)
	}
	return s, clock
}

func serve(h http.Handler, method, target string, body io.Reader, header http.Header) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, body)
	for k, vs := range header {
		req.Header[k] = vs
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestMetadata(t *testing.T) {
	s, _ := newTestAuthServer(t)
	rec := serve(s.Handler(), http.MethodGet, "/.well-known/oauth-authorization-server", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var meta map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &meta); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"issuer":                 testPublicURL,
		"authorization_endpoint": testPublicURL + "/authorize",
		"token_endpoint":         testPublicURL + "/token",
		"registration_endpoint":  testPublicURL + "/register",
		"authorization_response_iss_parameter_supported": true,
	}
	for k, v := range want {
		if meta[k] != v {
			t.Errorf("%s = %v, want %v", k, meta[k], v)
		}
	}
	if _, ok := meta["jwks_uri"]; ok {
		t.Error("metadata advertises jwks_uri, but this server has no signing keys")
	}
	for k, v := range map[string]string{
		"code_challenge_methods_supported":      "[S256]",
		"grant_types_supported":                 "[authorization_code refresh_token]",
		"response_types_supported":              "[code]",
		"scopes_supported":                      "[offline_access]",
		"token_endpoint_auth_methods_supported": "[none client_secret_post client_secret_basic]",
	} {
		if got := fmtList(meta[k]); got != v {
			t.Errorf("%s = %s, want %s", k, got, v)
		}
	}
}

func fmtList(v any) string {
	items, _ := v.([]any)
	parts := make([]string, len(items))
	for i, item := range items {
		parts[i], _ = item.(string)
	}
	return "[" + strings.Join(parts, " ") + "]"
}

func TestVerifyRejectsUnknownAndExpiredTokens(t *testing.T) {
	s, clock := newTestAuthServer(t)
	if _, err := s.Verify(context.Background(), "nope"); !errors.Is(err, auth.ErrInvalidToken) {
		t.Errorf("unknown token: err = %v", err)
	}

	if err := s.store.createGrant(&grant{ID: "g1", ClientID: "c1", CreatedAt: clock.now(), RefreshHash: "r"}); err != nil {
		t.Fatal(err)
	}
	s.access[hashToken("good")] = accessToken{grantID: "g1", expires: clock.now().Add(accessLifetime)}
	info, err := s.Verify(context.Background(), "good")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if info.UserID != "owner" || !info.Expiration.Equal(clock.now().Add(accessLifetime)) {
		t.Errorf("TokenInfo = %+v", info)
	}

	clock.advance(accessLifetime)
	if _, err := s.Verify(context.Background(), "good"); !errors.Is(err, auth.ErrInvalidToken) {
		t.Errorf("expired token: err = %v", err)
	}
}

func TestVerifyRejectsTokenOfRevokedGrant(t *testing.T) {
	s, clock := newTestAuthServer(t)
	if err := s.store.createGrant(&grant{ID: "g1", ClientID: "c1", CreatedAt: clock.now(), RefreshHash: "r"}); err != nil {
		t.Fatal(err)
	}
	s.access[hashToken("good")] = accessToken{grantID: "g1", expires: clock.now().Add(accessLifetime)}
	if err := s.store.revokeGrant("g1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Verify(context.Background(), "good"); !errors.Is(err, auth.ErrInvalidToken) {
		t.Errorf("token of a revoked grant: err = %v", err)
	}
}

func TestNewReportsStoreFailureAndPasswordChange(t *testing.T) {
	if _, err := New(Options{PublicURL: testPublicURL, Password: testPassword, StoreDir: "/dev/null/store"}); err == nil {
		t.Error("New succeeded with an unusable store folder")
	}

	dir := t.TempDir()
	if _, err := New(Options{PublicURL: testPublicURL, Password: testPassword, StoreDir: dir}); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	_, err := New(Options{PublicURL: testPublicURL, Password: "a-different-password", StoreDir: dir,
		Audit: slog.New(slog.NewTextHandler(&logs, nil))})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs.String(), "owner password changed") {
		t.Errorf("logs = %q, want a password-change warning", logs.String())
	}
}

func TestPruneDropsExpiredCodesAndTokens(t *testing.T) {
	s, clock := newTestAuthServer(t)
	s.codes["c"] = &pendingCode{expires: clock.now().Add(codeLifetime)}
	s.access["a"] = accessToken{expires: clock.now().Add(accessLifetime)}
	clock.advance(accessLifetime)
	s.mu.Lock()
	s.pruneLocked()
	s.mu.Unlock()
	if len(s.codes) != 0 || len(s.access) != 0 {
		t.Errorf("codes = %d, access = %d after expiry; want both empty", len(s.codes), len(s.access))
	}
}

func TestMatchesResourceIgnoresTrailingSlash(t *testing.T) {
	s, _ := newTestAuthServer(t)
	for _, r := range []string{testPublicURL, testPublicURL + "/"} {
		if !s.matchesResource(r) {
			t.Errorf("matchesResource(%q) = false", r)
		}
	}
	if s.matchesResource("https://other.example.com") {
		t.Error("matchesResource accepted another host")
	}
}

func TestAuditDefaultsAndClock(t *testing.T) {
	s, err := New(Options{PublicURL: testPublicURL, Password: testPassword, StoreDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if s.audit == nil || s.rand == nil || time.Since(s.now()) > time.Minute {
		t.Error("New did not fill in the logger, entropy, and clock defaults")
	}
}
