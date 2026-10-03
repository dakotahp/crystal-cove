package authserver

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

const testRedirect = "https://claude.ai/api/mcp/auth_callback"

func registerPublicClient(t *testing.T, s *Server) string {
	t.Helper()
	code, out := register(t, s, map[string]any{
		"client_name":                "Claude",
		"redirect_uris":              []string{testRedirect},
		"token_endpoint_auth_method": "none",
	})
	if code != http.StatusCreated {
		t.Fatalf("register: %d %v", code, out)
	}
	return out["client_id"].(string)
}

func pkcePair(t *testing.T) (verifier, challenge string) {
	t.Helper()
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:])
}

func authorizeForm(clientID, challenge string) url.Values {
	return url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {testRedirect},
		"state":                 {"xyz"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"resource":              {testPublicURL + "/"},
	}
}

func postForm(s *Server, path string, form url.Values) *httptest.ResponseRecorder {
	return serve(s.Handler(), http.MethodPost, path, strings.NewReader(form.Encode()),
		http.Header{"Content-Type": {"application/x-www-form-urlencoded"}})
}

func signIn(t *testing.T, s *Server, clientID, challenge string) string {
	t.Helper()
	form := authorizeForm(clientID, challenge)
	form.Set("password", testPassword)
	rec := postForm(s, "/authorize", form)
	if rec.Code != http.StatusFound {
		t.Fatalf("sign-in status = %d, body = %s", rec.Code, rec.Body)
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	return loc.Query().Get("code")
}

func TestAuthorizePageShowsClientAndReturnHost(t *testing.T) {
	s, _ := newTestAuthServer(t)
	id := registerPublicClient(t, s)
	_, challenge := pkcePair(t)
	rec := serve(s.Handler(), http.MethodGet, "/authorize?"+authorizeForm(id, challenge).Encode(), nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	for _, want := range []string{"<strong>Claude</strong> wants access", "<strong>claude.ai</strong>", `name="code_challenge"`, `type="password"`} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	h := rec.Header()
	if csp := h.Get("Content-Security-Policy"); !strings.Contains(csp, "form-action 'self' https://claude.ai;") ||
		!strings.Contains(csp, "frame-ancestors 'none'") || !strings.Contains(csp, "default-src 'none'") {
		t.Errorf("Content-Security-Policy = %q", csp)
	}
	for k, v := range map[string]string{"X-Frame-Options": "DENY", "Referrer-Policy": "no-referrer", "Cache-Control": "no-store"} {
		if h.Get(k) != v {
			t.Errorf("%s = %q, want %q", k, h.Get(k), v)
		}
	}
}

func TestAuthorizePageEscapesClientName(t *testing.T) {
	s, _ := newTestAuthServer(t)
	code, out := register(t, s, map[string]any{
		"client_name": "<script>alert(1)</script>", "redirect_uris": []string{testRedirect}, "token_endpoint_auth_method": "none",
	})
	if code != http.StatusCreated {
		t.Fatal(out)
	}
	_, challenge := pkcePair(t)
	rec := serve(s.Handler(), http.MethodGet, "/authorize?"+authorizeForm(out["client_id"].(string), challenge).Encode(), nil, nil)
	if strings.Contains(rec.Body.String(), "<script>") {
		t.Error("client name was not escaped")
	}
}

func TestAuthorizePageNamesUnnamedClient(t *testing.T) {
	s, _ := newTestAuthServer(t)
	_, out := register(t, s, map[string]any{"redirect_uris": []string{testRedirect}, "token_endpoint_auth_method": "none"})
	_, challenge := pkcePair(t)
	rec := serve(s.Handler(), http.MethodGet, "/authorize?"+authorizeForm(out["client_id"].(string), challenge).Encode(), nil, nil)
	if !strings.Contains(rec.Body.String(), "Unnamed client") {
		t.Error("page does not say Unnamed client")
	}
}

func TestAuthorizeUntrustedRedirectShowsErrorPage(t *testing.T) {
	s, _ := newTestAuthServer(t)
	id := registerPublicClient(t, s)
	_, challenge := pkcePair(t)
	for name, mutate := range map[string]func(url.Values){
		"unknown client":        func(f url.Values) { f.Set("client_id", "nope") },
		"unregistered redirect": func(f url.Values) { f.Set("redirect_uri", "https://evil.example.com/cb") },
	} {
		t.Run(name, func(t *testing.T) {
			form := authorizeForm(id, challenge)
			mutate(form)
			rec := serve(s.Handler(), http.MethodGet, "/authorize?"+form.Encode(), nil, nil)
			if rec.Code != http.StatusBadRequest || rec.Header().Get("Location") != "" {
				t.Errorf("status = %d, Location = %q; want a 400 page and no redirect", rec.Code, rec.Header().Get("Location"))
			}
		})
	}
}

func TestAuthorizeBadParametersShowAnErrorPage(t *testing.T) {
	s, _ := newTestAuthServer(t)
	id := registerPublicClient(t, s)
	_, challenge := pkcePair(t)
	cases := map[string]struct {
		mutate func(url.Values)
		want   string
	}{
		"response type":   {func(f url.Values) { f.Set("response_type", "token") }, "response_type must be code"},
		"no challenge":    {func(f url.Values) { f.Del("code_challenge") }, "code_challenge_method=S256 is required"},
		"plain challenge": {func(f url.Values) { f.Set("code_challenge_method", "plain") }, "code_challenge_method=S256 is required"},
		"wrong resource":  {func(f url.Values) { f.Set("resource", "https://other.example.com") }, "resource must be " + testPublicURL},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			form := authorizeForm(id, challenge)
			c.mutate(form)
			get := serve(s.Handler(), http.MethodGet, "/authorize?"+form.Encode(), nil, nil)
			form.Set("password", testPassword)
			post := postForm(s, "/authorize", form)
			for method, rec := range map[string]*httptest.ResponseRecorder{"GET": get, "POST": post} {
				if rec.Code != http.StatusBadRequest || rec.Header().Get("Location") != "" ||
					!strings.Contains(rec.Body.String(), c.want) {
					t.Errorf("%s: status = %d, Location = %q; want a 400 page saying %q and no redirect",
						method, rec.Code, rec.Header().Get("Location"), c.want)
				}
			}
		})
	}
}

func TestAuthorizeWithoutRedirectURIUsesTheOnlyRegisteredOne(t *testing.T) {
	s, _ := newTestAuthServer(t)
	id := registerPublicClient(t, s)
	_, challenge := pkcePair(t)
	form := authorizeForm(id, challenge)
	form.Del("redirect_uri")
	rec := serve(s.Handler(), http.MethodGet, "/authorize?"+form.Encode(), nil, nil)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}
}

func TestSignInRedirectsWithCode(t *testing.T) {
	s, clock := newTestAuthServer(t)
	id := registerPublicClient(t, s)
	_, challenge := pkcePair(t)
	form := authorizeForm(id, challenge)
	form.Set("password", testPassword)
	rec := postForm(s, "/authorize", form)
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	loc, _ := url.Parse(rec.Header().Get("Location"))
	q := loc.Query()
	if !strings.HasPrefix(loc.String(), testRedirect+"?") || q.Get("code") == "" || q.Get("state") != "xyz" || q.Get("iss") != testPublicURL {
		t.Fatalf("Location = %q", loc)
	}
	pc := s.codes[hashToken(q.Get("code"))]
	if pc == nil || pc.clientID != id || pc.redirectURI != testRedirect || pc.challenge != challenge ||
		!pc.expires.Equal(clock.now().Add(codeLifetime)) {
		t.Errorf("pending code = %+v", pc)
	}
}

func TestSignInWithoutStateOmitsIt(t *testing.T) {
	s, _ := newTestAuthServer(t)
	id := registerPublicClient(t, s)
	_, challenge := pkcePair(t)
	form := authorizeForm(id, challenge)
	form.Del("state")
	form.Set("password", testPassword)
	rec := postForm(s, "/authorize", form)
	if strings.Contains(rec.Header().Get("Location"), "state=") {
		t.Errorf("Location = %q, want no state", rec.Header().Get("Location"))
	}
}

func TestWrongPasswordIsLoggedAndLocksTheForm(t *testing.T) {
	s, clock := newTestAuthServer(t)
	var logs bytes.Buffer
	s.audit = slog.New(slog.NewTextHandler(&logs, nil))
	id := registerPublicClient(t, s)
	_, challenge := pkcePair(t)
	form := authorizeForm(id, challenge)
	form.Set("password", "wrong-password-guess")
	for range 5 {
		if rec := postForm(s, "/authorize", form); rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "Wrong password") {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
		}
	}
	if !strings.Contains(logs.String(), "wrong password") || strings.Contains(logs.String(), "wrong-password-guess") {
		t.Errorf("logs = %q, want the failure logged without the password", logs.String())
	}

	form.Set("password", testPassword)
	rec := postForm(s, "/authorize", form)
	if rec.Code != http.StatusTooManyRequests || !strings.Contains(rec.Body.String(), "Try again in 1m0s") {
		t.Fatalf("locked form: status = %d, body = %s", rec.Code, rec.Body)
	}
	clock.advance(time.Minute)
	if rec := postForm(s, "/authorize", form); rec.Code != http.StatusFound {
		t.Errorf("after the lock: status = %d", rec.Code)
	}
}

func TestSignInPostValidatesParametersAgain(t *testing.T) {
	s, _ := newTestAuthServer(t)
	id := registerPublicClient(t, s)
	_, challenge := pkcePair(t)
	form := authorizeForm(id, challenge)
	form.Set("redirect_uri", "https://evil.example.com/cb")
	form.Set("password", testPassword)
	if rec := postForm(s, "/authorize", form); rec.Code != http.StatusBadRequest || rec.Header().Get("Location") != "" {
		t.Errorf("status = %d, Location = %q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestSignInRejectsUnreadableForm(t *testing.T) {
	s, _ := newTestAuthServer(t)
	rec := serve(s.Handler(), http.MethodPost, "/authorize", strings.NewReader("%zz"),
		http.Header{"Content-Type": {"application/x-www-form-urlencoded"}})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d", rec.Code)
	}
}

func TestSignInWithoutEntropy(t *testing.T) {
	s, _ := newTestAuthServer(t)
	id := registerPublicClient(t, s)
	s.rand = failingReader{}
	_, challenge := pkcePair(t)
	form := authorizeForm(id, challenge)
	form.Set("password", testPassword)
	if rec := postForm(s, "/authorize", form); rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d", rec.Code)
	}
}

func TestConcurrentWrongPasswordsStopAtTheLock(t *testing.T) {
	s, _ := newTestAuthServer(t)
	id := registerPublicClient(t, s)
	_, challenge := pkcePair(t)
	form := authorizeForm(id, challenge)
	form.Set("password", "wrong-password-guess")

	const n = 20
	codes := make(chan int, n)
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes <- postForm(s, "/authorize", form).Code
		}()
	}
	wg.Wait()
	close(codes)
	counts := map[int]int{}
	for c := range codes {
		counts[c]++
	}
	if counts[http.StatusUnauthorized] != 5 || counts[http.StatusTooManyRequests] != n-5 {
		t.Errorf("status counts = %v, want 5 x 401 and %d x 429", counts, n-5)
	}
}
