package authserver

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Error        string `json:"error"`
}

func postToken(t *testing.T, s *Server, form url.Values) (int, tokenResponse) {
	t.Helper()
	rec := postForm(s, "/token", form)
	var out tokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("token response %q: %v", rec.Body, err)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("token response is cacheable")
	}
	return rec.Code, out
}

func exchangeForm(clientID, code, verifier string) url.Values {
	return url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {clientID},
		"code":          {code},
		"redirect_uri":  {testRedirect},
		"code_verifier": {verifier},
		"resource":      {testPublicURL + "/"},
	}
}

func refreshForm(clientID, refresh string) url.Values {
	return url.Values{"grant_type": {"refresh_token"}, "client_id": {clientID}, "refresh_token": {refresh}}
}

// connect registers a public client and signs it in, returning its tokens.
func connect(t *testing.T, s *Server) (string, tokenResponse) {
	t.Helper()
	id := registerPublicClient(t, s)
	verifier, challenge := pkcePair(t)
	code := signIn(t, s, id, challenge)
	status, out := postToken(t, s, exchangeForm(id, code, verifier))
	if status != http.StatusOK {
		t.Fatalf("exchange: %d %+v", status, out)
	}
	return id, out
}

func TestCodeExchangeIssuesWorkingTokens(t *testing.T) {
	s, _ := newTestAuthServer(t)
	_, out := connect(t, s)
	if out.TokenType != "Bearer" || out.ExpiresIn != 3600 || out.AccessToken == "" || out.RefreshToken == "" {
		t.Fatalf("response = %+v", out)
	}
	if _, err := s.Verify(context.Background(), out.AccessToken); err != nil {
		t.Errorf("issued access token does not verify: %v", err)
	}
}

func TestCodeReuseRevokesTheSignIn(t *testing.T) {
	s, _ := newTestAuthServer(t)
	id := registerPublicClient(t, s)
	verifier, challenge := pkcePair(t)
	code := signIn(t, s, id, challenge)
	_, first := postToken(t, s, exchangeForm(id, code, verifier))
	status, second := postToken(t, s, exchangeForm(id, code, verifier))
	if status != http.StatusBadRequest || second.Error != "invalid_grant" {
		t.Fatalf("second exchange: %d %+v", status, second)
	}
	if _, err := s.Verify(context.Background(), first.AccessToken); err == nil {
		t.Error("access token from the first exchange still works after code reuse")
	}
}

func TestCodeExchangeChecks(t *testing.T) {
	cases := map[string]func(f url.Values, otherClient string){
		"wrong verifier": func(f url.Values, _ string) { f.Set("code_verifier", strings.Repeat("a", 43)) },
		"short verifier": func(f url.Values, _ string) { f.Set("code_verifier", "short") },
		"wrong redirect": func(f url.Values, _ string) { f.Set("redirect_uri", "https://claude.ai/other") },
		"unknown code":   func(f url.Values, _ string) { f.Set("code", "nope") },
		"another client": func(f url.Values, other string) { f.Set("client_id", other) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			s, _ := newTestAuthServer(t)
			id := registerPublicClient(t, s)
			other := registerPublicClient(t, s)
			verifier, challenge := pkcePair(t)
			form := exchangeForm(id, signIn(t, s, id, challenge), verifier)
			mutate(form, other)
			status, out := postToken(t, s, form)
			if status != http.StatusBadRequest || out.Error != "invalid_grant" {
				t.Errorf("status = %d, body = %+v; want 400 invalid_grant", status, out)
			}
		})
	}
}

func TestCodeExpiresAfterSixtySeconds(t *testing.T) {
	s, clock := newTestAuthServer(t)
	id := registerPublicClient(t, s)
	verifier, challenge := pkcePair(t)
	code := signIn(t, s, id, challenge)
	clock.advance(codeLifetime)
	if status, out := postToken(t, s, exchangeForm(id, code, verifier)); status != http.StatusBadRequest || out.Error != "invalid_grant" {
		t.Errorf("status = %d, body = %+v", status, out)
	}
}

func TestCodeExchangeWithoutRedirectURI(t *testing.T) {
	s, _ := newTestAuthServer(t)
	id := registerPublicClient(t, s)
	verifier, challenge := pkcePair(t)
	form := exchangeForm(id, signIn(t, s, id, challenge), verifier)
	form.Del("redirect_uri")
	if status, out := postToken(t, s, form); status != http.StatusOK {
		t.Errorf("status = %d, body = %+v", status, out)
	}
}

func TestTokenRejectsWrongResource(t *testing.T) {
	s, _ := newTestAuthServer(t)
	id, tokens := connect(t, s)
	form := refreshForm(id, tokens.RefreshToken)
	form.Set("resource", "https://other.example.com")
	if status, out := postToken(t, s, form); status != http.StatusBadRequest || out.Error != "invalid_target" {
		t.Errorf("status = %d, body = %+v", status, out)
	}
}

func TestRefreshRotatesAndDetectsReuse(t *testing.T) {
	s, _ := newTestAuthServer(t)
	id, first := connect(t, s)

	status, second := postToken(t, s, refreshForm(id, first.RefreshToken))
	if status != http.StatusOK || second.RefreshToken == first.RefreshToken || second.AccessToken == "" {
		t.Fatalf("refresh: %d %+v", status, second)
	}
	if _, err := s.Verify(context.Background(), second.AccessToken); err != nil {
		t.Fatalf("refreshed access token: %v", err)
	}

	status, reused := postToken(t, s, refreshForm(id, first.RefreshToken))
	if status != http.StatusBadRequest || reused.Error != "invalid_grant" {
		t.Fatalf("reuse: %d %+v", status, reused)
	}
	if _, err := s.Verify(context.Background(), second.AccessToken); err == nil {
		t.Error("access token still works after refresh-token reuse")
	}
	if status, _ := postToken(t, s, refreshForm(id, second.RefreshToken)); status != http.StatusBadRequest {
		t.Error("current refresh token still works after reuse revoked the sign-in")
	}
}

func TestRefreshAfterNinetyDaysFails(t *testing.T) {
	s, clock := newTestAuthServer(t)
	id, tokens := connect(t, s)
	clock.advance(grantLifetime + 1)
	if status, out := postToken(t, s, refreshForm(id, tokens.RefreshToken)); status != http.StatusBadRequest || out.Error != "invalid_grant" {
		t.Errorf("status = %d, body = %+v", status, out)
	}
}

func TestRefreshWorksAfterRestart(t *testing.T) {
	dir, clock := t.TempDir(), newTestClock()
	s, err := New(Options{PublicURL: testPublicURL, Password: testPassword, StoreDir: dir, Now: clock.now})
	if err != nil {
		t.Fatal(err)
	}
	id, tokens := connect(t, s)

	restarted, err := New(Options{PublicURL: testPublicURL, Password: testPassword, StoreDir: dir, Now: clock.now})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Verify(context.Background(), tokens.AccessToken); err == nil {
		t.Error("access token survived a restart")
	}
	if status, out := postToken(t, restarted, refreshForm(id, tokens.RefreshToken)); status != http.StatusOK {
		t.Errorf("refresh after restart: %d %+v", status, out)
	}
}

func TestConfidentialClientAuthentication(t *testing.T) {
	s, _ := newTestAuthServer(t)
	_, reg := register(t, s, map[string]any{"redirect_uris": []string{testRedirect}, "token_endpoint_auth_method": "client_secret_basic"})
	id, secret := reg["client_id"].(string), reg["client_secret"].(string)
	verifier, challenge := pkcePair(t)
	code := signIn(t, s, id, challenge)

	form := exchangeForm(id, code, verifier)
	form.Del("client_id")
	wrong := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(form.Encode()))
	wrong.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	wrong.SetBasicAuth(id, "wrong-secret")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, wrong)
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "invalid_client") {
		t.Fatalf("wrong secret: %d %s", rec.Code, rec.Body)
	}

	right := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(form.Encode()))
	right.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	right.SetBasicAuth(id, secret)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, right)
	if rec.Code != http.StatusOK {
		t.Fatalf("basic auth: %d %s", rec.Code, rec.Body)
	}

	posted := refreshForm(id, "")
	var tokens tokenResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &tokens)
	posted.Set("refresh_token", tokens.RefreshToken)
	posted.Set("client_secret", secret)
	if status, out := postToken(t, s, posted); status != http.StatusOK {
		t.Errorf("client_secret_post: %d %+v", status, out)
	}
}

func TestTokenRequestErrors(t *testing.T) {
	s, _ := newTestAuthServer(t)
	id := registerPublicClient(t, s)
	if status, out := postToken(t, s, url.Values{"grant_type": {"refresh_token"}, "client_id": {"nope"}}); status != http.StatusUnauthorized || out.Error != "invalid_client" {
		t.Errorf("unknown client: %d %+v", status, out)
	}
	if status, out := postToken(t, s, url.Values{"grant_type": {"password"}, "client_id": {id}}); status != http.StatusBadRequest || out.Error != "unsupported_grant_type" {
		t.Errorf("password grant: %d %+v", status, out)
	}
	if status, out := postToken(t, s, refreshForm(id, "nope")); status != http.StatusBadRequest || out.Error != "invalid_grant" {
		t.Errorf("unknown refresh token: %d %+v", status, out)
	}
	rec := serve(s.Handler(), http.MethodPost, "/token", strings.NewReader("%zz"),
		http.Header{"Content-Type": {"application/x-www-form-urlencoded"}})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("unreadable form: %d", rec.Code)
	}
}

func TestTokenWithoutEntropy(t *testing.T) {
	s, _ := newTestAuthServer(t)
	id, tokens := connect(t, s)
	s.rand = failingReader{}
	if status, out := postToken(t, s, refreshForm(id, tokens.RefreshToken)); status != http.StatusInternalServerError || out.Error != "server_error" {
		t.Errorf("refresh: %d %+v", status, out)
	}
	s.rand = rand.Reader
	if status, _ := postToken(t, s, refreshForm(id, tokens.RefreshToken)); status != http.StatusOK {
		t.Error("a failed refresh used up the refresh token")
	}
}

func FuzzTokenEndpoint(f *testing.F) {
	f.Add("grant_type=authorization_code&code=x&code_verifier=y&client_id=z")
	f.Add("grant_type=refresh_token&refresh_token=x&client_id=z")
	f.Add("%zz")
	s, _ := newTestAuthServer(f)
	h := s.Handler()
	f.Fuzz(func(t *testing.T, body string) {
		rec := serve(h, http.MethodPost, "/token", strings.NewReader(body),
			http.Header{"Content-Type": {"application/x-www-form-urlencoded"}})
		switch rec.Code {
		case http.StatusBadRequest, http.StatusUnauthorized:
		default:
			t.Fatalf("body %q: status %d, want 400 or 401", body, rec.Code)
		}
	})
}

func TestFailedExchangeLeavesTheCodeUsable(t *testing.T) {
	s, _ := newTestAuthServer(t)
	id := registerPublicClient(t, s)
	verifier, challenge := pkcePair(t)
	code := signIn(t, s, id, challenge)
	bad := exchangeForm(id, code, strings.Repeat("a", 43))
	if status, _ := postToken(t, s, bad); status != http.StatusBadRequest {
		t.Fatalf("wrong verifier: %d", status)
	}
	if status, out := postToken(t, s, exchangeForm(id, code, verifier)); status != http.StatusOK {
		t.Errorf("valid exchange after a failed one: %d %+v", status, out)
	}
}

func TestCodeReuseWithoutThePKCEVerifierDoesNotRevoke(t *testing.T) {
	s, _ := newTestAuthServer(t)
	id := registerPublicClient(t, s)
	other := registerPublicClient(t, s)
	verifier, challenge := pkcePair(t)
	code := signIn(t, s, id, challenge)
	_, first := postToken(t, s, exchangeForm(id, code, verifier))
	postToken(t, s, exchangeForm(id, code, strings.Repeat("a", 43)))
	postToken(t, s, exchangeForm(other, code, verifier))
	if _, err := s.Verify(context.Background(), first.AccessToken); err != nil {
		t.Errorf("a leaked code without the verifier revoked the sign-in: %v", err)
	}
}

func TestCodeClaimedTwiceIsMarkedReused(t *testing.T) {
	s, _ := newTestAuthServer(t)
	id := registerPublicClient(t, s)
	_, challenge := pkcePair(t)
	signIn(t, s, id, challenge)
	var pc *pendingCode
	for _, p := range s.codes {
		pc = p
	}
	if _, claimed := s.claimCode(pc, "g1"); !claimed {
		t.Fatal("first claim failed")
	}
	if s.codeReused(pc) {
		t.Fatal("code marked reused after one claim")
	}
	if prior, claimed := s.claimCode(pc, "g2"); claimed || prior != "g1" {
		t.Fatalf("second claim = %q, %v", prior, claimed)
	}
	if !s.codeReused(pc) {
		t.Error("the first request cannot learn that the code was reused")
	}
	s.releaseCode(pc)
	if _, claimed := s.claimCode(pc, "g3"); !claimed {
		t.Error("released code cannot be claimed")
	}
}

func TestReuseFailsClosedWhenTheSaveFails(t *testing.T) {
	dir, clock := t.TempDir(), newTestClock()
	var logs bytes.Buffer
	s, err := New(Options{PublicURL: testPublicURL, Password: testPassword, StoreDir: dir, Now: clock.now,
		Audit: slog.New(slog.NewTextHandler(&logs, nil))})
	if err != nil {
		t.Fatal(err)
	}
	id, first := connect(t, s)
	_, second := postToken(t, s, refreshForm(id, first.RefreshToken))
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	status, out := postToken(t, s, refreshForm(id, first.RefreshToken))
	if status != http.StatusBadRequest || out.Error != "invalid_grant" {
		t.Fatalf("reuse with a read-only store: %d %+v", status, out)
	}
	if _, err := s.Verify(context.Background(), second.AccessToken); err == nil {
		t.Error("access token still works after reuse with a failed save")
	}
	if status, _ := postToken(t, s, refreshForm(id, second.RefreshToken)); status != http.StatusBadRequest {
		t.Error("current refresh token still works after reuse with a failed save")
	}
	if !strings.Contains(logs.String(), "auth store save failed") {
		t.Errorf("save error not logged: %s", logs.String())
	}
}
