# Built-in OAuth Sign-in Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let claude.ai and other MCP clients connect to Crystal Cove with only `MCP_PUBLIC_URL` and an owner password: no identity provider, no shim, no client ID or secret to paste.

**Architecture:** A new package `internal/authserver` is a small OAuth 2.1 authorization server: RFC 8414 metadata, RFC 7591 dynamic registration, a server-rendered password page at `/authorize`, and `/token` with PKCE, rotating refresh tokens, and reuse detection. Grants and clients persist in `~/.crystal-cove/auth.json`; codes and access tokens live in memory. `internal/server` mounts its handler and adds its `Verify` to the bearer check, and `cmd/crystal-cove` builds it when `MCP_OWNER_PASSWORD` is set.

**Tech Stack:** Go 1.25, standard library (`net/http`, `html/template`, `crypto/*`), `github.com/modelcontextprotocol/go-sdk` v1.8.0 (`auth`, `oauthex`, `mcp`).

**Spec:** `docs/superpowers/specs/2026-10-02-built-in-oauth-design.md`

## Global Constraints

- `MCP_OWNER_PASSWORD` is at least 16 characters.
- `MCP_OWNER_PASSWORD` requires `MCP_PUBLIC_URL`, and cannot be set together with `OAUTH_ISSUER`.
- Authorization code: 60 seconds, single use. Access token: 1 hour. Grant: 90 days from sign-in.
- All tokens are 32 random bytes, base64url without padding. Only SHA-256 hashes are stored or compared.
- At most 50 registered clients.
- Wrong-password lock: after 5 consecutive failures, 1 minute, doubling per further failure, at most 1 hour.
- Store file `<HOME>/.crystal-cove/auth.json`, mode `0600`, folder `0700`, written by temp file and rename.
- PKCE `S256` is required. Redirect URIs are `https`, or `http` on a loopback host, with no fragment.
- No token, code, secret, or password is ever logged.
- No panics in library code. Errors wrapped with `%w` and enough context to act on.
- Tests: real filesystems (`t.TempDir()`), `httptest`, the real MCP client, no mocking libraries, injected clock.
- Run `gofmt -w` on every Go file you write; the code blocks here are not guaranteed to be gofmt-aligned.
- Before every commit: `gofmt -l .` empty, `go vet ./...`, and the tests of the package you changed. Total coverage stays at 95% or more (checked in Task 10).
- No code comments unless the code looks wrong but is right, or a constraint forces its shape.

## Review Focus

- **The browser blocks the return to claude.ai after a correct password.** Chrome applies CSP `form-action` to the redirect that follows a form post. Expected: the page allows the redirect URI's origin. Test in Task 5.
- **The client sends `resource` with a trailing slash** (`https://host/` instead of `https://host`). Expected: accepted. Tests in Task 5 and Task 6.
- **The server restarts while a client is connected.** Expected: the old access token stops working, and the refresh token still works. Test in Task 6.
- **An authorization code is used twice.** Expected: the second use fails, and the access token from the first use stops working at once. Test in Task 6.
- **A client registers without `token_endpoint_auth_method`.** Expected: RFC 7591 default `client_secret_basic`, and a secret is returned. Test in Task 4.

---

## File Structure

| File | Responsibility |
| --- | --- |
| `internal/config/config.go` | Parse `MCP_OWNER_PASSWORD`, standalone `MCP_PUBLIC_URL`, `AuthStoreDir` |
| `internal/authserver/token.go` | Random tokens and hashes |
| `internal/authserver/store.go` | `auth.json`: clients, grants, password fingerprint, rotation |
| `internal/authserver/limiter.go` | Wrong-password lock |
| `internal/authserver/authserver.go` | `Server`, `Options`, `New`, `Handler`, `Verify`, metadata, shared helpers |
| `internal/authserver/register.go` | `POST /register` |
| `internal/authserver/authorize.go` | `GET`/`POST /authorize` |
| `internal/authserver/page.go` | Sign-in page template and headers |
| `internal/authserver/tokenendpoint.go` | `POST /token` |
| `internal/server/http.go` | Mount the handler, publish resource metadata, verify built-in tokens |
| `cmd/crystal-cove/main.go` | Build the auth server from config |
| Docs | README, `docs/oauth.md`, `docs/configuration.md`, `.env.example`, `docs/SECURITY.md`, `CLAUDE.md`, CI fuzz line |

---

### Task 1: Configuration

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `Config.OwnerPassword string`, `Config.AuthStoreDir string`, `Config.PublicURL` now also set without `OAUTH_ISSUER`, constant `MinOwnerPasswordLength = 16`.

- [ ] **Step 1: Write the failing tests**

Add to `internal/config/config_test.go`:

```go
const testOwnerPassword = "correct-horse-battery"

func TestLoadOwnerPassword(t *testing.T) {
	m := validEnv()
	delete(m, "MCP_AUTH_TOKEN")
	m["MCP_OWNER_PASSWORD"] = testOwnerPassword
	m["MCP_PUBLIC_URL"] = "https://vault.example.com/"
	cfg, err := Load(env(m), rand.Reader)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.OwnerPassword != testOwnerPassword {
		t.Errorf("OwnerPassword = %q", cfg.OwnerPassword)
	}
	if cfg.PublicURL != "https://vault.example.com" {
		t.Errorf("PublicURL = %q, want the URL without its trailing slash", cfg.PublicURL)
	}
}

func TestLoadAuthStoreDirFollowsHomeNotVaultsDir(t *testing.T) {
	m := validEnv()
	m["VAULTS_DIR"] = "/data/vaults"
	cfg, err := Load(env(m), rand.Reader)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AuthStoreDir != "/home/test/.crystal-cove" {
		t.Errorf("AuthStoreDir = %q, want /home/test/.crystal-cove", cfg.AuthStoreDir)
	}
}

func TestLoadOwnerPasswordValidation(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(map[string]string)
		wantErr string
	}{
		{"too short", func(m map[string]string) {
			m["MCP_OWNER_PASSWORD"] = "short"
			m["MCP_PUBLIC_URL"] = "https://vault.example.com"
		}, "at least 16"},
		{"without public url", func(m map[string]string) {
			m["MCP_OWNER_PASSWORD"] = testOwnerPassword
		}, "MCP_PUBLIC_URL must be set"},
		{"with an issuer", func(m map[string]string) {
			m["MCP_OWNER_PASSWORD"] = testOwnerPassword
			m["MCP_PUBLIC_URL"] = "https://vault.example.com"
			m["OAUTH_ISSUER"] = "https://idp.example.com"
			m["OAUTH_AUDIENCE"] = "crystal-cove"
		}, "cannot both be set"},
		{"plain-http public url off loopback", func(m map[string]string) {
			m["MCP_OWNER_PASSWORD"] = testOwnerPassword
			m["MCP_PUBLIC_URL"] = "http://vault.example.com"
		}, "MCP_PUBLIC_URL must use https"},
		{"public url that is not a url", func(m map[string]string) {
			m["MCP_OWNER_PASSWORD"] = testOwnerPassword
			m["MCP_PUBLIC_URL"] = "vault.example.com"
		}, "MCP_PUBLIC_URL must be an http(s) URL"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := validEnv()
			c.mutate(m)
			_, err := Load(env(m), rand.Reader)
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("err = %v, want mention of %q", err, c.wantErr)
			}
		})
	}
}

func TestLoadOwnerPasswordAllowsPlainHTTPOnLoopback(t *testing.T) {
	m := validEnv()
	m["MCP_OWNER_PASSWORD"] = testOwnerPassword
	m["MCP_PUBLIC_URL"] = "http://127.0.0.1:8080"
	if _, err := Load(env(m), rand.Reader); err != nil {
		t.Fatalf("Load: %v", err)
	}
}
```

In `TestLoadOAuthValidation`, change the expected text of the `"no auth at all"` case from `"MCP_AUTH_TOKEN or OAUTH_ISSUER"` to `"MCP_OWNER_PASSWORD"`.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/config/`
Expected: build failure, `cfg.OwnerPassword undefined` and `cfg.AuthStoreDir undefined`.

- [ ] **Step 3: Implement**

In `internal/config/config.go`, add below `MinAuthTokenLength`:

```go
// MinOwnerPasswordLength is the shortest MCP_OWNER_PASSWORD accepted. The
// sign-in page faces the internet; the wrong-password lock slows guessing,
// but only a long password makes it hopeless.
const MinOwnerPasswordLength = 16
```

Add to `Config`, after `PublicURL`:

```go
	// OwnerPassword turns on the built-in OAuth sign-in. The owner types it
	// on the sign-in page when an MCP client connects.
	OwnerPassword string
	// AuthStoreDir holds the built-in sign-in's registered clients and
	// grants. It sits beside the vaults, never inside one.
	AuthStoreDir string
```

Update the `PublicURL` field comment to:

```go
	// PublicURL is this server's canonical external URL, used as the
	// protected-resource identifier in OAuth metadata and as the built-in
	// sign-in's issuer. Required with OAuth or OwnerPassword.
```

In `Load`, replace this block:

```go
	cfg.OAuth = oauth
	cfg.PublicURL = publicURL
	if cfg.AuthToken == "" && cfg.OAuth == nil {
		return nil, errors.New("MCP_AUTH_TOKEN or OAUTH_ISSUER must be set: the MCP endpoint is bearer-token protected")
	}
```

with:

```go
	cfg.OAuth = oauth
	cfg.PublicURL = publicURL
	if cfg.PublicURL == "" {
		cfg.PublicURL = strings.TrimSuffix(strings.TrimSpace(getenv("MCP_PUBLIC_URL")), "/")
	}
	cfg.OwnerPassword = getenv("MCP_OWNER_PASSWORD")
	if err := checkOwnerPassword(cfg); err != nil {
		return nil, err
	}
	if cfg.AuthToken == "" && cfg.OAuth == nil && cfg.OwnerPassword == "" {
		return nil, errors.New("MCP_AUTH_TOKEN, OAUTH_ISSUER, or MCP_OWNER_PASSWORD must be set: the MCP endpoint is bearer-token protected")
	}
```

Replace the `VaultsDir` block:

```go
	cfg.VaultsDir = getenv("VAULTS_DIR")
	if cfg.VaultsDir == "" {
		home := getenv("HOME")
		if home == "" {
			home = "/"
		}
		cfg.VaultsDir = filepath.Join(home, "vaults")
	}
```

with:

```go
	home := getenv("HOME")
	if home == "" {
		home = "/"
	}
	cfg.VaultsDir = getenv("VAULTS_DIR")
	if cfg.VaultsDir == "" {
		cfg.VaultsDir = filepath.Join(home, "vaults")
	}
	cfg.AuthStoreDir = filepath.Join(home, ".crystal-cove")
```

Replace `checkIssuerURL` with a general check, and update its one caller in `parseOAuth` from `checkIssuerURL(issuer)` to `checkHTTPSURL("OAUTH_ISSUER", issuer)`:

```go
// checkHTTPSURL requires an https URL. MCP clients are sent to it to sign
// in, so plain http is allowed only on a loopback host, where a local
// server commonly runs without TLS.
func checkHTTPSURL(name, raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return fmt.Errorf("%s must be an http(s) URL, got %q", name, raw)
	}
	if u.Scheme == "http" && !isLoopback(u.Hostname()) {
		return fmt.Errorf("%s must use https, got %q: plain http is allowed only on a loopback host", name, raw)
	}
	return nil
}

// checkOwnerPassword validates the built-in sign-in settings. Only one
// place to sign in is advertised, so it excludes an external provider.
func checkOwnerPassword(cfg *Config) error {
	if cfg.OwnerPassword == "" {
		return nil
	}
	if len(cfg.OwnerPassword) < MinOwnerPasswordLength {
		return fmt.Errorf("MCP_OWNER_PASSWORD must be at least %d characters, got %d",
			MinOwnerPasswordLength, len(cfg.OwnerPassword))
	}
	if cfg.OAuth != nil {
		return errors.New("MCP_OWNER_PASSWORD and OAUTH_ISSUER cannot both be set: use the built-in sign-in or an external identity provider")
	}
	if cfg.PublicURL == "" {
		return errors.New("MCP_PUBLIC_URL must be set when MCP_OWNER_PASSWORD is: MCP clients sign in at that address")
	}
	return checkHTTPSURL("MCP_PUBLIC_URL", cfg.PublicURL)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./internal/config/ && go test ./internal/config/`
Expected: no gofmt output, `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/config
git commit -m "feat: parse MCP_OWNER_PASSWORD for the built-in sign-in"
```

---

### Task 2: Tokens and the auth store

**Files:**
- Create: `internal/authserver/token.go`, `internal/authserver/store.go`
- Test: `internal/authserver/store_test.go`

**Interfaces:**
- Produces (package-internal, used by Tasks 4 to 6):
  - `newToken(r io.Reader) (string, error)`, `hashToken(token string) string`
  - `type client struct { ID, Name string; RedirectURIs []string; AuthMethod, SecretHash string; CreatedAt time.Time }`
  - `type grant struct { ID, ClientID string; CreatedAt time.Time; RefreshHash string; UsedHashes []string }`
  - `openStore(dir, password string, now func() time.Time, rnd io.Reader) (st *store, passwordChanged bool, err error)`
  - `(*store).addClient(c *client) error` (returns `errStoreFull`)
  - `(*store).client(id string) (*client, bool)`
  - `(*store).createGrant(g *grant) error`
  - `(*store).hasGrant(id string) bool`
  - `(*store).revokeGrant(id string) error`
  - `(*store).rotate(clientID, oldHash, newHash string) (grant, error)` (returns `errUnknownRefresh`, `errRefreshReused`, `errGrantExpired`)
  - constants `maxClients = 50`, `grantLifetime = 90 * 24 * time.Hour`

- [ ] **Step 1: Write the failing tests**

Create `internal/authserver/store_test.go`:

```go
package authserver

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func newTestClock() *testClock {
	return &testClock{t: time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)}
}

func (c *testClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *testClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("no entropy") }

func openTestStore(t *testing.T, dir, password string, clock *testClock) *store {
	t.Helper()
	st, _, err := openStore(dir, password, clock.now, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestNewTokenIsRandomAndHashIsStable(t *testing.T) {
	a, err := newToken(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := newToken(rand.Reader)
	if a == b || len(a) != 43 {
		t.Errorf("tokens %q and %q: want two different 43-character tokens", a, b)
	}
	if hashToken(a) != hashToken(a) || hashToken(a) == hashToken(b) || len(hashToken(a)) != 64 {
		t.Error("hashToken is not a stable SHA-256 hex digest")
	}
	if _, err := newToken(failingReader{}); err == nil {
		t.Error("newToken succeeded without entropy")
	}
}

func TestOpenStoreCreatesPrivateFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".crystal-cove")
	openTestStore(t, dir, "owner-password-1", newTestClock())
	info, err := os.Stat(filepath.Join(dir, "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("auth.json mode = %v, want 0600", info.Mode().Perm())
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Errorf("store folder mode = %v, want 0700", dirInfo.Mode().Perm())
	}
}

func TestStorePersistsClientsAndGrants(t *testing.T) {
	dir, clock := t.TempDir(), newTestClock()
	st := openTestStore(t, dir, "owner-password-1", clock)
	if err := st.addClient(&client{ID: "c1", Name: "Claude", RedirectURIs: []string{"https://claude.ai/cb"}, AuthMethod: "none"}); err != nil {
		t.Fatal(err)
	}
	if err := st.createGrant(&grant{ID: "g1", ClientID: "c1", CreatedAt: clock.now(), RefreshHash: "r1"}); err != nil {
		t.Fatal(err)
	}

	reopened, changed, err := openStore(dir, "owner-password-1", clock.now, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("passwordChanged = true for the same password")
	}
	if c, ok := reopened.client("c1"); !ok || c.Name != "Claude" {
		t.Errorf("client after reopen = %+v, %v", c, ok)
	}
	if !reopened.hasGrant("g1") {
		t.Error("grant lost on reopen")
	}
}

func TestPasswordChangeDeletesGrants(t *testing.T) {
	dir, clock := t.TempDir(), newTestClock()
	st := openTestStore(t, dir, "owner-password-1", clock)
	if err := st.addClient(&client{ID: "c1", AuthMethod: "none"}); err != nil {
		t.Fatal(err)
	}
	if err := st.createGrant(&grant{ID: "g1", ClientID: "c1", CreatedAt: clock.now(), RefreshHash: "r1"}); err != nil {
		t.Fatal(err)
	}

	reopened, changed, err := openStore(dir, "owner-password-2", clock.now, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Error("passwordChanged = false after a new password")
	}
	if reopened.hasGrant("g1") {
		t.Error("grant survived a password change")
	}
	if _, ok := reopened.client("c1"); !ok {
		t.Error("client registration was deleted with the grants")
	}
}

func TestRotateReplacesRefreshToken(t *testing.T) {
	clock := newTestClock()
	st := openTestStore(t, t.TempDir(), "owner-password-1", clock)
	if err := st.createGrant(&grant{ID: "g1", ClientID: "c1", CreatedAt: clock.now(), RefreshHash: "r1"}); err != nil {
		t.Fatal(err)
	}
	g, err := st.rotate("c1", "r1", "r2")
	if err != nil || g.ID != "g1" {
		t.Fatalf("rotate = %+v, %v", g, err)
	}
	if _, err := st.rotate("c1", "r2", "r3"); err != nil {
		t.Errorf("rotating the new token: %v", err)
	}
}

func TestRotateReuseRevokesGrant(t *testing.T) {
	clock := newTestClock()
	st := openTestStore(t, t.TempDir(), "owner-password-1", clock)
	if err := st.createGrant(&grant{ID: "g1", ClientID: "c1", CreatedAt: clock.now(), RefreshHash: "r1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.rotate("c1", "r1", "r2"); err != nil {
		t.Fatal(err)
	}
	g, err := st.rotate("c1", "r1", "r3")
	if !errors.Is(err, errRefreshReused) || g.ClientID != "c1" {
		t.Fatalf("reuse = %+v, %v; want errRefreshReused with the grant", g, err)
	}
	if st.hasGrant("g1") {
		t.Error("grant survived refresh-token reuse")
	}
	if _, err := st.rotate("c1", "r2", "r4"); !errors.Is(err, errUnknownRefresh) {
		t.Errorf("current token after reuse: err = %v, want errUnknownRefresh", err)
	}
}

func TestRotateRejectsOtherClientAndUnknownToken(t *testing.T) {
	clock := newTestClock()
	st := openTestStore(t, t.TempDir(), "owner-password-1", clock)
	if err := st.createGrant(&grant{ID: "g1", ClientID: "c1", CreatedAt: clock.now(), RefreshHash: "r1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.rotate("c2", "r1", "r2"); !errors.Is(err, errUnknownRefresh) {
		t.Errorf("other client: err = %v, want errUnknownRefresh", err)
	}
	if _, err := st.rotate("c1", "nope", "r2"); !errors.Is(err, errUnknownRefresh) {
		t.Errorf("unknown token: err = %v, want errUnknownRefresh", err)
	}
	if _, err := st.rotate("c1", "r1", "r2"); err != nil {
		t.Errorf("rejected attempts changed the grant: %v", err)
	}
}

func TestGrantExpiresAfterNinetyDays(t *testing.T) {
	clock := newTestClock()
	st := openTestStore(t, t.TempDir(), "owner-password-1", clock)
	if err := st.createGrant(&grant{ID: "g1", ClientID: "c1", CreatedAt: clock.now(), RefreshHash: "r1"}); err != nil {
		t.Fatal(err)
	}
	clock.advance(grantLifetime)
	if !st.hasGrant("g1") {
		t.Fatal("grant expired at exactly 90 days")
	}
	clock.advance(time.Second)
	if st.hasGrant("g1") {
		t.Error("hasGrant = true after 90 days")
	}
	if _, err := st.rotate("c1", "r1", "r2"); !errors.Is(err, errGrantExpired) {
		t.Errorf("rotate after 90 days: err = %v, want errGrantExpired", err)
	}
	if _, err := st.rotate("c1", "r1", "r2"); !errors.Is(err, errUnknownRefresh) {
		t.Errorf("expired grant was not deleted: err = %v", err)
	}
}

func TestRevokeGrant(t *testing.T) {
	clock := newTestClock()
	st := openTestStore(t, t.TempDir(), "owner-password-1", clock)
	if err := st.createGrant(&grant{ID: "g1", ClientID: "c1", CreatedAt: clock.now(), RefreshHash: "r1"}); err != nil {
		t.Fatal(err)
	}
	if err := st.revokeGrant("g1"); err != nil {
		t.Fatal(err)
	}
	if st.hasGrant("g1") {
		t.Error("grant survived revokeGrant")
	}
	if err := st.revokeGrant("missing"); err != nil {
		t.Errorf("revoking a missing grant: %v", err)
	}
}

func TestClientLimitEvictsOldestClientWithoutActiveGrant(t *testing.T) {
	clock := newTestClock()
	st := openTestStore(t, t.TempDir(), "owner-password-1", clock)
	for i := range maxClients {
		if err := st.addClient(&client{ID: fmt.Sprintf("c%d", i), AuthMethod: "none"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.createGrant(&grant{ID: "g0", ClientID: "c0", CreatedAt: clock.now(), RefreshHash: "r0"}); err != nil {
		t.Fatal(err)
	}
	if err := st.addClient(&client{ID: "new", AuthMethod: "none"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := st.client("c0"); !ok {
		t.Error("evicted a client with an active grant")
	}
	if _, ok := st.client("c1"); ok {
		t.Error("oldest client without a grant was kept")
	}
	if _, ok := st.client("new"); !ok {
		t.Error("new client was not added")
	}
}

func TestClientLimitRefusesWhenEveryClientIsSignedIn(t *testing.T) {
	clock := newTestClock()
	st := openTestStore(t, t.TempDir(), "owner-password-1", clock)
	for i := range maxClients {
		id := fmt.Sprintf("c%d", i)
		if err := st.addClient(&client{ID: id, AuthMethod: "none"}); err != nil {
			t.Fatal(err)
		}
		if err := st.createGrant(&grant{ID: "g" + id, ClientID: id, CreatedAt: clock.now(), RefreshHash: "r" + id}); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.addClient(&client{ID: "new", AuthMethod: "none"}); !errors.Is(err, errStoreFull) {
		t.Errorf("err = %v, want errStoreFull", err)
	}
}

func TestOpenStoreFailures(t *testing.T) {
	clock := newTestClock()

	corrupt := t.TempDir()
	if err := os.WriteFile(filepath.Join(corrupt, "auth.json"), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := openStore(corrupt, "owner-password-1", clock.now, rand.Reader); err == nil {
		t.Error("opened a corrupt store")
	}

	noKey := t.TempDir()
	if err := os.WriteFile(filepath.Join(noKey, "auth.json"), []byte(`{"fingerprint_key":"zz"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := openStore(noKey, "owner-password-1", clock.now, rand.Reader); err == nil {
		t.Error("opened a store without a valid key")
	}

	unreadable := t.TempDir()
	if err := os.Mkdir(filepath.Join(unreadable, "auth.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := openStore(unreadable, "owner-password-1", clock.now, rand.Reader); err == nil {
		t.Error("opened a store whose file is a directory")
	}

	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := openStore(filepath.Join(blocked, "store"), "owner-password-1", clock.now, rand.Reader); err == nil {
		t.Error("created a store under a file")
	}

	if _, _, err := openStore(t.TempDir(), "owner-password-1", clock.now, failingReader{}); err == nil {
		t.Error("created a store without entropy")
	}
}

func TestStoreReportsWriteFailure(t *testing.T) {
	dir, clock := t.TempDir(), newTestClock()
	st := openTestStore(t, dir, "owner-password-1", clock)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	if err := st.addClient(&client{ID: "c1", AuthMethod: "none"}); err == nil {
		t.Error("addClient succeeded in a read-only folder")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/authserver/`
Expected: build failure, `undefined: openStore` (and the other names).

- [ ] **Step 3: Implement**

Create `internal/authserver/token.go`:

```go
// Package authserver is a small OAuth 2.1 authorization server for one
// owner: dynamic client registration, a password sign-in page, and
// rotating refresh tokens. It lets MCP clients such as claude.ai connect
// without an external identity provider.
package authserver

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
)

func newToken(r io.Reader) (string, error) {
	b := make([]byte, 32)
	if _, err := io.ReadFull(r, b); err != nil {
		return "", fmt.Errorf("generating token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
```

Create `internal/authserver/store.go`:

```go
package authserver

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

const (
	maxClients    = 50
	grantLifetime = 90 * 24 * time.Hour
	storeFile     = "auth.json"
)

var (
	errStoreFull      = errors.New("client limit reached: every registered client has an active sign-in")
	errUnknownRefresh = errors.New("unknown refresh token")
	errRefreshReused  = errors.New("refresh token already used")
	errGrantExpired   = errors.New("sign-in expired")
)

type client struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	RedirectURIs []string  `json:"redirect_uris"`
	AuthMethod   string    `json:"auth_method"`
	SecretHash   string    `json:"secret_hash,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type grant struct {
	ID          string    `json:"id"`
	ClientID    string    `json:"client_id"`
	CreatedAt   time.Time `json:"created_at"`
	RefreshHash string    `json:"refresh_hash"`
	UsedHashes  []string  `json:"used_hashes,omitempty"`
}

type storeData struct {
	FingerprintKey      string    `json:"fingerprint_key"`
	PasswordFingerprint string    `json:"password_fingerprint"`
	Clients             []*client `json:"clients"`
	Grants              []*grant  `json:"grants"`
}

type store struct {
	path string
	now  func() time.Time

	mu   sync.Mutex
	data storeData
}

// openStore loads dir/auth.json, creating it on first use. When the owner
// password differs from the one the store last saw, every grant is deleted
// and passwordChanged is true.
func openStore(dir, password string, now func() time.Time, rnd io.Reader) (st *store, passwordChanged bool, err error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, false, fmt.Errorf("creating auth store folder %q: %w", dir, err)
	}
	st = &store{path: filepath.Join(dir, storeFile), now: now}
	raw, err := os.ReadFile(st.path) // #nosec G304 -- the path comes from configuration, not from a request
	switch {
	case errors.Is(err, fs.ErrNotExist):
		key := make([]byte, 32)
		if _, err := io.ReadFull(rnd, key); err != nil {
			return nil, false, fmt.Errorf("generating auth store key: %w", err)
		}
		st.data.FingerprintKey = hex.EncodeToString(key)
	case err != nil:
		return nil, false, fmt.Errorf("reading auth store %q: %w", st.path, err)
	default:
		if err := json.Unmarshal(raw, &st.data); err != nil {
			return nil, false, fmt.Errorf("parsing auth store %q: %w", st.path, err)
		}
	}
	key, err := hex.DecodeString(st.data.FingerprintKey)
	if err != nil || len(key) == 0 {
		return nil, false, fmt.Errorf("auth store %q has no valid fingerprint key: delete the file to start over", st.path)
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(password))
	fingerprint := hex.EncodeToString(mac.Sum(nil))
	if st.data.PasswordFingerprint != "" && st.data.PasswordFingerprint != fingerprint {
		passwordChanged = true
		st.data.Grants = nil
	}
	st.data.PasswordFingerprint = fingerprint
	st.pruneLocked()
	if err := st.saveLocked(); err != nil {
		return nil, false, err
	}
	return st, passwordChanged, nil
}

func (s *store) saveLocked() error {
	raw, err := json.MarshalIndent(&s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding auth store: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".auth-*.json")
	if err != nil {
		return fmt.Errorf("writing auth store: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing auth store: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("writing auth store: %w", err)
	}
	if err := os.Rename(tmp.Name(), s.path); err != nil {
		return fmt.Errorf("replacing auth store %q: %w", s.path, err)
	}
	return nil
}

func (s *store) expired(g *grant) bool {
	return s.now().Sub(g.CreatedAt) > grantLifetime
}

func (s *store) pruneLocked() {
	s.data.Grants = slices.DeleteFunc(s.data.Grants, s.expired)
}

func (s *store) addClient(c *client) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked()
	if len(s.data.Clients) >= maxClients {
		i := slices.IndexFunc(s.data.Clients, func(c *client) bool {
			return !slices.ContainsFunc(s.data.Grants, func(g *grant) bool { return g.ClientID == c.ID })
		})
		if i < 0 {
			return errStoreFull
		}
		s.data.Clients = slices.Delete(s.data.Clients, i, i+1)
	}
	s.data.Clients = append(s.data.Clients, c)
	return s.saveLocked()
}

func (s *store) client(id string) (*client, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.data.Clients {
		if c.ID == id {
			cp := *c
			return &cp, true
		}
	}
	return nil, false
}

func (s *store) createGrant(g *grant) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Grants = append(s.data.Grants, g)
	return s.saveLocked()
}

func (s *store) hasGrant(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.ContainsFunc(s.data.Grants, func(g *grant) bool { return g.ID == id && !s.expired(g) })
}

func (s *store) revokeGrant(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	before := len(s.data.Grants)
	s.data.Grants = slices.DeleteFunc(s.data.Grants, func(g *grant) bool { return g.ID == id })
	if len(s.data.Grants) == before {
		return nil
	}
	return s.saveLocked()
}

// rotate replaces the refresh token oldHash of clientID's grant with
// newHash. A token that was already rotated away revokes its grant, and a
// grant past its lifetime is deleted.
func (s *store) rotate(clientID, oldHash, newHash string) (grant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, g := range s.data.Grants {
		switch {
		case slices.Contains(g.UsedHashes, oldHash):
			s.data.Grants = slices.Delete(s.data.Grants, i, i+1)
			return *g, errors.Join(errRefreshReused, s.saveLocked())
		case g.RefreshHash != oldHash || g.ClientID != clientID:
			continue
		case s.expired(g):
			s.data.Grants = slices.Delete(s.data.Grants, i, i+1)
			return *g, errors.Join(errGrantExpired, s.saveLocked())
		}
		g.UsedHashes = append(g.UsedHashes, oldHash)
		g.RefreshHash = newHash
		return *g, s.saveLocked()
	}
	return grant{}, errUnknownRefresh
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./internal/authserver/ && go test ./internal/authserver/`
Expected: no gofmt output, `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/authserver
git commit -m "feat: add the built-in sign-in's persistent store"
```

---

### Task 3: Wrong-password lock

**Files:**
- Create: `internal/authserver/limiter.go`
- Test: `internal/authserver/limiter_test.go`

**Interfaces:**
- Produces: `type limiter struct { now func() time.Time; ... }`, `(*limiter).wait() time.Duration`, `(*limiter).fail()`, `(*limiter).succeed()`.

- [ ] **Step 1: Write the failing test**

Create `internal/authserver/limiter_test.go`:

```go
package authserver

import (
	"testing"
	"time"
)

func TestLimiterLocksAfterFiveFailuresAndDoubles(t *testing.T) {
	clock := newTestClock()
	l := &limiter{now: clock.now}
	for range 4 {
		l.fail()
	}
	if w := l.wait(); w != 0 {
		t.Fatalf("locked after 4 failures: %v", w)
	}

	want := []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute,
		16 * time.Minute, 32 * time.Minute, time.Hour, time.Hour}
	for i, d := range want {
		l.fail()
		if w := l.wait(); w != d {
			t.Fatalf("failure %d: wait = %v, want %v", i+5, w, d)
		}
		clock.advance(d)
		if w := l.wait(); w != 0 {
			t.Fatalf("still locked after the lock passed: %v", w)
		}
	}
}

func TestLimiterSuccessResets(t *testing.T) {
	clock := newTestClock()
	l := &limiter{now: clock.now}
	for range 5 {
		l.fail()
	}
	l.succeed()
	if w := l.wait(); w != 0 {
		t.Errorf("wait after success = %v", w)
	}
	l.fail()
	if w := l.wait(); w != 0 {
		t.Errorf("one failure after a reset locked the form: %v", w)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/authserver/ -run Limiter`
Expected: build failure, `undefined: limiter`.

- [ ] **Step 3: Implement**

Create `internal/authserver/limiter.go`:

```go
package authserver

import (
	"sync"
	"time"
)

const (
	freeAttempts = 5
	firstLock    = time.Minute
	maxLock      = time.Hour
)

// limiter locks the sign-in form after repeated wrong passwords. One
// counter covers the whole server, because there is one owner.
type limiter struct {
	now func() time.Time

	mu          sync.Mutex
	failures    int
	lockedUntil time.Time
}

func (l *limiter) wait() time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	return max(l.lockedUntil.Sub(l.now()), 0)
}

func (l *limiter) fail() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failures++
	if l.failures < freeAttempts {
		return
	}
	shift := min(l.failures-freeAttempts, 6)
	l.lockedUntil = l.now().Add(min(firstLock<<shift, maxLock))
}

func (l *limiter) succeed() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failures = 0
	l.lockedUntil = time.Time{}
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./internal/authserver/ && go test ./internal/authserver/`
Expected: `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/authserver
git commit -m "feat: lock the sign-in form after repeated wrong passwords"
```

---

### Task 4: Server, metadata, registration, and Verify

**Files:**
- Create: `internal/authserver/authserver.go`, `internal/authserver/register.go`
- Test: `internal/authserver/authserver_test.go`, `internal/authserver/register_test.go`

**Interfaces:**
- Consumes: Task 2 store and tokens, Task 3 limiter.
- Produces (exported, used by Tasks 8 and 9):
  - `type Options struct { PublicURL, Password, StoreDir string; Audit *slog.Logger; Now func() time.Time; Rand io.Reader }`
  - `func New(opts Options) (*Server, error)`
  - `func (s *Server) Handler() http.Handler` serving `GET /.well-known/oauth-authorization-server`, `POST /register`, `GET /authorize`, `POST /authorize`, `POST /token`
  - `func (s *Server) Verify(ctx context.Context, token string) (*auth.TokenInfo, error)`
- Produces (package-internal, used by Tasks 5 and 6): fields `codes map[string]*pendingCode`, `access map[string]accessToken`, type `pendingCode`, helpers `writeJSON`, `oauthError`, `(*Server).serverError`, `(*Server).pruneLocked`, `(*Server).matchesResource`, `isLoopback`.

Tasks 5 and 6 add `authorizePage`, `authorizeSubmit`, and `token`. So that this task compiles and its tests pass alone, this task adds them as stubs in `authserver.go` that answer `501 Not Implemented`. Tasks 5 and 6 delete each stub when they add the real method.

- [ ] **Step 1: Write the failing tests**

Create `internal/authserver/authserver_test.go`:

```go
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
```

Create `internal/authserver/register_test.go`:

```go
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/authserver/`
Expected: build failure, `undefined: New`.

- [ ] **Step 3: Implement**

Create `internal/authserver/authserver.go`:

```go
package authserver

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
)

const (
	codeLifetime   = time.Minute
	accessLifetime = time.Hour
)

// Options configures New.
type Options struct {
	// PublicURL is the issuer and the protected resource clients sign in for.
	PublicURL string
	// Password is the owner password typed on the sign-in page.
	Password string
	// StoreDir holds auth.json with registered clients and grants.
	StoreDir string
	// Audit receives sign-in events. Nil discards them.
	Audit *slog.Logger
	// Now and Rand default to time.Now and crypto/rand.Reader.
	Now  func() time.Time
	Rand io.Reader
}

// Server is the built-in authorization server.
type Server struct {
	publicURL    string
	passwordHash [sha256.Size]byte
	store        *store
	limiter      *limiter
	audit        *slog.Logger
	now          func() time.Time
	rand         io.Reader

	mu     sync.Mutex
	codes  map[string]*pendingCode
	access map[string]accessToken
}

type pendingCode struct {
	clientID    string
	redirectURI string
	challenge   string
	expires     time.Time
	grantID     string
}

type accessToken struct {
	grantID string
	expires time.Time
}

// New opens the store in opts.StoreDir and returns a ready server. A
// changed owner password signs every client out.
func New(opts Options) (*Server, error) {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	rnd := opts.Rand
	if rnd == nil {
		rnd = rand.Reader
	}
	audit := opts.Audit
	if audit == nil {
		audit = slog.New(slog.DiscardHandler)
	}
	st, passwordChanged, err := openStore(opts.StoreDir, opts.Password, now, rnd)
	if err != nil {
		return nil, err
	}
	if passwordChanged {
		audit.Warn("owner password changed: every client must sign in again")
	}
	return &Server{
		publicURL:    strings.TrimSuffix(opts.PublicURL, "/"),
		passwordHash: sha256.Sum256([]byte(opts.Password)),
		store:        st,
		limiter:      &limiter{now: now},
		audit:        audit,
		now:          now,
		rand:         rnd,
		codes:        make(map[string]*pendingCode),
		access:       make(map[string]accessToken),
	}, nil
}

// Handler serves the metadata, registration, sign-in, and token endpoints.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", s.metadata)
	mux.HandleFunc("POST /register", s.register)
	mux.HandleFunc("GET /authorize", s.authorizePage)
	mux.HandleFunc("POST /authorize", s.authorizeSubmit)
	mux.HandleFunc("POST /token", s.token)
	return mux
}

// Verify accepts an access token this server issued, while it is fresh
// and its grant is still active.
func (s *Server) Verify(_ context.Context, token string) (*auth.TokenInfo, error) {
	s.mu.Lock()
	at, ok := s.access[hashToken(token)]
	s.mu.Unlock()
	if !ok || !s.now().Before(at.expires) {
		return nil, fmt.Errorf("%w: unknown or expired access token", auth.ErrInvalidToken)
	}
	if !s.store.hasGrant(at.grantID) {
		return nil, fmt.Errorf("%w: the sign-in for this token was revoked", auth.ErrInvalidToken)
	}
	return &auth.TokenInfo{UserID: "owner", Expiration: at.expires}, nil
}

type serverMetadata struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	RegistrationEndpoint              string   `json:"registration_endpoint"`
	ScopesSupported                   []string `json:"scopes_supported"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	GrantTypesSupported               []string `json:"grant_types_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported"`
	IssParameterSupported             bool     `json:"authorization_response_iss_parameter_supported"`
}

var authMethods = []string{"none", "client_secret_post", "client_secret_basic"}

func (s *Server) metadata(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, serverMetadata{
		Issuer:                            s.publicURL,
		AuthorizationEndpoint:             s.publicURL + "/authorize",
		TokenEndpoint:                     s.publicURL + "/token",
		RegistrationEndpoint:              s.publicURL + "/register",
		ScopesSupported:                   []string{"offline_access"},
		ResponseTypesSupported:            []string{"code"},
		GrantTypesSupported:               []string{"authorization_code", "refresh_token"},
		TokenEndpointAuthMethodsSupported: authMethods,
		CodeChallengeMethodsSupported:     []string{"S256"},
		IssParameterSupported:             true,
	})
}

func (s *Server) matchesResource(resource string) bool {
	return strings.TrimSuffix(resource, "/") == s.publicURL
}

func (s *Server) pruneLocked() {
	now := s.now()
	maps.DeleteFunc(s.codes, func(_ string, c *pendingCode) bool { return !now.Before(c.expires) })
	maps.DeleteFunc(s.access, func(_ string, a accessToken) bool { return !now.Before(a.expires) })
}

func (s *Server) serverError(w http.ResponseWriter, err error) {
	s.audit.Error("sign-in server error", "error", err.Error())
	oauthError(w, http.StatusInternalServerError, "server_error", "internal error, see the server log")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func oauthError(w http.ResponseWriter, status int, code, description string) {
	writeJSON(w, status, map[string]string{"error": code, "error_description": description})
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *Server) authorizePage(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

func (s *Server) authorizeSubmit(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

func (s *Server) token(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
```

Create `internal/authserver/register.go`:

```go
package authserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/oauthex"
)

const maxBodyBytes = 64 << 10

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var meta oauthex.ClientRegistrationMetadata
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&meta); err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_client_metadata", "the body must be a JSON client registration")
		return
	}
	if len(meta.RedirectURIs) == 0 {
		oauthError(w, http.StatusBadRequest, "invalid_redirect_uri", "redirect_uris is required")
		return
	}
	for _, u := range meta.RedirectURIs {
		if err := checkRedirectURI(u); err != nil {
			oauthError(w, http.StatusBadRequest, "invalid_redirect_uri", err.Error())
			return
		}
	}
	if meta.TokenEndpointAuthMethod == "" {
		meta.TokenEndpointAuthMethod = "client_secret_basic"
	}
	if !slices.Contains(authMethods, meta.TokenEndpointAuthMethod) {
		oauthError(w, http.StatusBadRequest, "invalid_client_metadata",
			"token_endpoint_auth_method must be one of "+strings.Join(authMethods, ", "))
		return
	}
	for _, gt := range meta.GrantTypes {
		if gt != "authorization_code" && gt != "refresh_token" {
			oauthError(w, http.StatusBadRequest, "invalid_client_metadata",
				"grant_types may contain only authorization_code and refresh_token")
			return
		}
	}

	id, err := newToken(s.rand)
	if err != nil {
		s.serverError(w, err)
		return
	}
	c := &client{ID: id, Name: meta.ClientName, RedirectURIs: meta.RedirectURIs,
		AuthMethod: meta.TokenEndpointAuthMethod, CreatedAt: s.now()}
	resp := oauthex.ClientRegistrationResponse{ClientRegistrationMetadata: meta, ClientID: id, ClientIDIssuedAt: c.CreatedAt}
	if c.AuthMethod != "none" {
		secret, err := newToken(s.rand)
		if err != nil {
			s.serverError(w, err)
			return
		}
		c.SecretHash = hashToken(secret)
		resp.ClientSecret = secret
	}
	if err := s.store.addClient(c); err != nil {
		if errors.Is(err, errStoreFull) {
			s.audit.Warn("client registration refused", "reason", err.Error())
			oauthError(w, http.StatusServiceUnavailable, "temporarily_unavailable", err.Error())
			return
		}
		s.serverError(w, err)
		return
	}
	s.audit.Info("client registered", "client_id", id, "client_name", meta.ClientName)
	writeJSON(w, http.StatusCreated, &resp)
}

func checkRedirectURI(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() || u.Host == "" {
		return fmt.Errorf("redirect URI %q must be an absolute URL", raw)
	}
	if strings.Contains(raw, "#") {
		return fmt.Errorf("redirect URI %q must not contain a fragment", raw)
	}
	if u.Scheme != "https" && (u.Scheme != "http" || !isLoopback(u.Hostname())) {
		return fmt.Errorf("redirect URI %q must use https, or http on a loopback host", raw)
	}
	return nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./internal/authserver/ && go test ./internal/authserver/`
Expected: `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/authserver
git commit -m "feat: serve sign-in metadata and dynamic client registration"
```

---

### Task 5: Sign-in page

**Files:**
- Create: `internal/authserver/authorize.go`, `internal/authserver/page.go`
- Modify: `internal/authserver/authserver.go` (delete the `authorizePage` and `authorizeSubmit` stubs)
- Test: `internal/authserver/authorize_test.go`

**Interfaces:**
- Consumes: Task 4 `Server`, `store.client`, `limiter`, `pendingCode`, `pruneLocked`, `matchesResource`.
- Produces: `(*Server).authorizePage`, `(*Server).authorizeSubmit`. On success, a `pendingCode` is stored under `hashToken(code)` with `clientID`, `redirectURI`, `challenge`, `expires = now + codeLifetime`. Redirects carry `code`, `state` (when sent), and `iss = publicURL`.
- Produces test helpers used by Task 6: `registerPublicClient(t, s) string`, `pkcePair(t) (verifier, challenge string)`, `authorizeForm(clientID, challenge string) url.Values`, `signIn(t, s, clientID, challenge string) string` (returns the code).

- [ ] **Step 1: Write the failing tests**

Create `internal/authserver/authorize_test.go`:

```go
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
		"unknown client":     func(f url.Values) { f.Set("client_id", "nope") },
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

func TestAuthorizeBadParametersRedirectWithError(t *testing.T) {
	s, _ := newTestAuthServer(t)
	id := registerPublicClient(t, s)
	_, challenge := pkcePair(t)
	cases := map[string]struct {
		mutate func(url.Values)
		want   string
	}{
		"response type":    {func(f url.Values) { f.Set("response_type", "token") }, "unsupported_response_type"},
		"no challenge":     {func(f url.Values) { f.Del("code_challenge") }, "invalid_request"},
		"plain challenge":  {func(f url.Values) { f.Set("code_challenge_method", "plain") }, "invalid_request"},
		"wrong resource":   {func(f url.Values) { f.Set("resource", "https://other.example.com") }, "invalid_target"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			form := authorizeForm(id, challenge)
			c.mutate(form)
			rec := serve(s.Handler(), http.MethodGet, "/authorize?"+form.Encode(), nil, nil)
			loc, _ := url.Parse(rec.Header().Get("Location"))
			if rec.Code != http.StatusFound || loc.Query().Get("error") != c.want ||
				loc.Query().Get("state") != "xyz" || loc.Query().Get("iss") != testPublicURL {
				t.Errorf("status = %d, Location = %q; want a redirect with error=%s, state, and iss", rec.Code, loc, c.want)
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/authserver/ -run 'Authorize|SignIn|WrongPassword'`
Expected: FAIL. The stubs answer 501, so the status checks fail.

- [ ] **Step 3: Implement**

Delete the `authorizePage` and `authorizeSubmit` stubs from `authserver.go`.

Create `internal/authserver/page.go`:

```go
package authserver

import (
	"html/template"
	"net/http"
)

type pageData struct {
	ClientName string
	ReturnHost string
	Fields     map[string]string
	Message    string
	Error      string
}

var signInPage = template.Must(template.New("signin").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Sign in to Crystal Cove</title>
<style>
body{font-family:system-ui,sans-serif;max-width:26rem;margin:3rem auto;padding:0 1rem;color:#1f1d2b;background:#fff}
@media (prefers-color-scheme:dark){body{color:#ece9f5;background:#16141f}}
input,button{font:inherit;width:100%;box-sizing:border-box;padding:.6rem;margin:.4rem 0}
.message{color:#c4314b}
</style>
</head>
<body>
<h1>Crystal Cove</h1>
{{if .Error}}<p class="message">{{.Error}}</p>{{else}}
<p><strong>{{.ClientName}}</strong> wants access to your vault.</p>
<p>After you sign in, you return to <strong>{{.ReturnHost}}</strong>.</p>
{{with .Message}}<p class="message">{{.}}</p>{{end}}
<form method="post" action="/authorize">
{{range $name, $value := .Fields}}<input type="hidden" name="{{$name}}" value="{{$value}}">
{{end}}<label for="password">Owner password</label>
<input id="password" name="password" type="password" autocomplete="current-password" required autofocus>
<button type="submit">Allow access</button>
</form>{{end}}
</body>
</html>
`))

// renderPage writes the sign-in page. returnOrigin is allowed in
// form-action because browsers apply it to the redirect that follows the
// form post, which leaves for the client.
func renderPage(w http.ResponseWriter, status int, returnOrigin string, data pageData) {
	formAction := "'self'"
	if returnOrigin != "" {
		formAction += " " + returnOrigin
	}
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action "+formAction+"; frame-ancestors 'none'")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = signInPage.Execute(w, data)
}

func renderError(w http.ResponseWriter, status int, message string) {
	renderPage(w, status, "", pageData{Error: message})
}
```

Create `internal/authserver/authorize.go`:

```go
package authserver

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"time"
)

type authRequest struct {
	client      *client
	redirectURI string
	state       string
	challenge   string
}

type authError struct {
	code        string
	description string
	redirect    bool
}

var authorizeFields = []string{"response_type", "client_id", "redirect_uri", "state",
	"code_challenge", "code_challenge_method", "scope", "resource"}

// parseAuthRequest checks an authorization request. An error with redirect
// false must be shown as a page: the redirect URI is not trusted, so the
// server must not send the browser there.
func (s *Server) parseAuthRequest(form url.Values) (*authRequest, *authError) {
	c, ok := s.store.client(form.Get("client_id"))
	if !ok {
		return nil, &authError{"invalid_client", "This app is not registered with this server. Connect it again.", false}
	}
	redirectURI := form.Get("redirect_uri")
	if redirectURI == "" && len(c.RedirectURIs) == 1 {
		redirectURI = c.RedirectURIs[0]
	}
	if !slices.Contains(c.RedirectURIs, redirectURI) {
		return nil, &authError{"invalid_request", "The return address does not match the app's registration.", false}
	}
	req := &authRequest{client: c, redirectURI: redirectURI, state: form.Get("state"), challenge: form.Get("code_challenge")}
	switch {
	case form.Get("response_type") != "code":
		return req, &authError{"unsupported_response_type", "response_type must be code", true}
	case req.challenge == "" || form.Get("code_challenge_method") != "S256":
		return req, &authError{"invalid_request", "PKCE with code_challenge_method=S256 is required", true}
	case form.Get("resource") != "" && !s.matchesResource(form.Get("resource")):
		return req, &authError{"invalid_target", "resource must be " + s.publicURL, true}
	}
	return req, nil
}

func (s *Server) failAuthorize(w http.ResponseWriter, r *http.Request, req *authRequest, aerr *authError) {
	if !aerr.redirect {
		renderError(w, http.StatusBadRequest, aerr.description)
		return
	}
	s.redirect(w, r, req, url.Values{"error": {aerr.code}, "error_description": {aerr.description}})
}

// redirect sends the browser back to the client with params, the client's
// state, and this server's issuer.
func (s *Server) redirect(w http.ResponseWriter, r *http.Request, req *authRequest, params url.Values) {
	u, _ := url.Parse(req.redirectURI) // validated at registration
	q := u.Query()
	for k, vs := range params {
		q[k] = vs
	}
	if req.state != "" {
		q.Set("state", req.state)
	}
	q.Set("iss", s.publicURL)
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

func (s *Server) pageFor(req *authRequest, form url.Values) (string, pageData) {
	u, _ := url.Parse(req.redirectURI) // validated at registration
	data := pageData{ClientName: req.client.Name, ReturnHost: u.Host, Fields: make(map[string]string)}
	if data.ClientName == "" {
		data.ClientName = "Unnamed client"
	}
	for _, name := range authorizeFields {
		if v := form.Get(name); v != "" {
			data.Fields[name] = v
		}
	}
	data.Fields["redirect_uri"] = req.redirectURI
	return u.Scheme + "://" + u.Host, data
}

func (s *Server) authorizePage(w http.ResponseWriter, r *http.Request) {
	form := r.URL.Query()
	req, aerr := s.parseAuthRequest(form)
	if aerr != nil {
		s.failAuthorize(w, r, req, aerr)
		return
	}
	origin, data := s.pageFor(req, form)
	renderPage(w, http.StatusOK, origin, data)
}

func (s *Server) authorizeSubmit(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := r.ParseForm(); err != nil {
		renderError(w, http.StatusBadRequest, "The sign-in form could not be read.")
		return
	}
	form := r.PostForm
	req, aerr := s.parseAuthRequest(form)
	if aerr != nil {
		s.failAuthorize(w, r, req, aerr)
		return
	}
	origin, data := s.pageFor(req, form)
	if wait := s.limiter.wait(); wait > 0 {
		data.Message = fmt.Sprintf("Too many wrong passwords. Try again in %s.", wait.Round(time.Second))
		renderPage(w, http.StatusTooManyRequests, origin, data)
		return
	}
	sum := sha256.Sum256([]byte(form.Get("password")))
	if subtle.ConstantTimeCompare(sum[:], s.passwordHash[:]) != 1 {
		s.limiter.fail()
		s.audit.Warn("sign-in failed: wrong password", "client_id", req.client.ID, "remote_addr", r.RemoteAddr)
		data.Message = "Wrong password."
		renderPage(w, http.StatusUnauthorized, origin, data)
		return
	}
	s.limiter.succeed()

	code, err := newToken(s.rand)
	if err != nil {
		s.audit.Error("sign-in server error", "error", err.Error())
		renderError(w, http.StatusInternalServerError, "The server could not finish the sign-in. See the server log.")
		return
	}
	s.mu.Lock()
	s.pruneLocked()
	s.codes[hashToken(code)] = &pendingCode{
		clientID:    req.client.ID,
		redirectURI: req.redirectURI,
		challenge:   req.challenge,
		expires:     s.now().Add(codeLifetime),
	}
	s.mu.Unlock()
	s.audit.Info("sign-in succeeded", "client_id", req.client.ID, "client_name", req.client.Name)
	s.redirect(w, r, req, url.Values{"code": {code}})
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./internal/authserver/ && go test ./internal/authserver/`
Expected: `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/authserver
git commit -m "feat: add the owner-password sign-in page"
```

---

### Task 6: Token endpoint

**Files:**
- Create: `internal/authserver/tokenendpoint.go`
- Modify: `internal/authserver/authserver.go` (delete the `token` stub), `.github/workflows/ci.yml` (fuzz line)
- Test: `internal/authserver/token_test.go`

**Interfaces:**
- Consumes: Task 5 test helpers, `pendingCode`, `store.rotate`, `store.createGrant`, `store.revokeGrant`.
- Produces: `(*Server).token`. Successful responses are JSON `{"access_token", "token_type": "Bearer", "expires_in": 3600, "refresh_token"}`.

- [ ] **Step 1: Write the failing tests**

Create `internal/authserver/token_test.go`:

```go
package authserver

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
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
		"wrong verifier":      func(f url.Values, _ string) { f.Set("code_verifier", strings.Repeat("a", 43)) },
		"short verifier":      func(f url.Values, _ string) { f.Set("code_verifier", "short") },
		"wrong redirect":      func(f url.Values, _ string) { f.Set("redirect_uri", "https://claude.ai/other") },
		"unknown code":        func(f url.Values, _ string) { f.Set("code", "nope") },
		"another client":      func(f url.Values, other string) { f.Set("client_id", other) },
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/authserver/ -run 'Code|Refresh|Token|Confidential'`
Expected: FAIL. The stub answers 501 with a plain-text body, so `postToken` fails to parse JSON.

- [ ] **Step 3: Implement**

Delete the `token` stub from `authserver.go`.

Create `internal/authserver/tokenendpoint.go`:

```go
package authserver

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"time"
)

func (s *Server) token(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := r.ParseForm(); err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_request", "the body must be a form")
		return
	}
	c, ok := s.authenticateClient(r)
	if !ok {
		oauthError(w, http.StatusUnauthorized, "invalid_client", "unknown client or wrong client secret")
		return
	}
	if res := r.PostForm.Get("resource"); res != "" && !s.matchesResource(res) {
		oauthError(w, http.StatusBadRequest, "invalid_target", "resource must be "+s.publicURL)
		return
	}
	switch r.PostForm.Get("grant_type") {
	case "authorization_code":
		s.exchangeCode(w, r, c)
	case "refresh_token":
		s.refresh(w, r, c)
	default:
		oauthError(w, http.StatusBadRequest, "unsupported_grant_type", "grant_type must be authorization_code or refresh_token")
	}
}

func (s *Server) authenticateClient(r *http.Request) (*client, bool) {
	id, secret, basic := r.BasicAuth()
	if !basic {
		id, secret = r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
	}
	c, ok := s.store.client(id)
	if !ok {
		return nil, false
	}
	if c.AuthMethod == "none" {
		return c, true
	}
	return c, secret != "" && subtle.ConstantTimeCompare([]byte(hashToken(secret)), []byte(c.SecretHash)) == 1
}

// newTokens draws every token a response needs before any state changes,
// so running out of entropy never leaves a client without a usable token.
func (s *Server) newTokens(n int) ([]string, error) {
	tokens := make([]string, n)
	for i := range tokens {
		t, err := newToken(s.rand)
		if err != nil {
			return nil, err
		}
		tokens[i] = t
	}
	return tokens, nil
}

func (s *Server) exchangeCode(w http.ResponseWriter, r *http.Request, c *client) {
	form := r.PostForm
	tokens, err := s.newTokens(3)
	if err != nil {
		s.serverError(w, err)
		return
	}
	grantID, access, refresh := tokens[0], tokens[1], tokens[2]

	s.mu.Lock()
	pc, ok := s.codes[hashToken(form.Get("code"))]
	var reusedGrant string
	if ok {
		reusedGrant = pc.grantID
		if reusedGrant == "" {
			pc.grantID = grantID
		}
	}
	s.mu.Unlock()

	switch {
	case reusedGrant != "":
		if err := s.store.revokeGrant(reusedGrant); err != nil {
			s.serverError(w, err)
			return
		}
		s.audit.Warn("authorization code used twice: sign-in revoked", "client_id", c.ID)
		oauthError(w, http.StatusBadRequest, "invalid_grant", "the authorization code was already used")
		return
	case !ok || !s.now().Before(pc.expires):
		oauthError(w, http.StatusBadRequest, "invalid_grant", "unknown or expired authorization code")
		return
	case pc.clientID != c.ID:
		oauthError(w, http.StatusBadRequest, "invalid_grant", "the authorization code was issued to another client")
		return
	case form.Get("redirect_uri") != "" && form.Get("redirect_uri") != pc.redirectURI:
		oauthError(w, http.StatusBadRequest, "invalid_grant", "redirect_uri does not match the authorization request")
		return
	case !verifyPKCE(form.Get("code_verifier"), pc.challenge):
		oauthError(w, http.StatusBadRequest, "invalid_grant", "code_verifier does not match code_challenge")
		return
	}

	g := &grant{ID: grantID, ClientID: c.ID, CreatedAt: s.now(), RefreshHash: hashToken(refresh)}
	if err := s.store.createGrant(g); err != nil {
		s.serverError(w, err)
		return
	}
	s.issue(w, grantID, access, refresh)
}

func (s *Server) refresh(w http.ResponseWriter, r *http.Request, c *client) {
	tokens, err := s.newTokens(2)
	if err != nil {
		s.serverError(w, err)
		return
	}
	access, refresh := tokens[0], tokens[1]
	g, err := s.store.rotate(c.ID, hashToken(r.PostForm.Get("refresh_token")), hashToken(refresh))
	switch {
	case errors.Is(err, errRefreshReused):
		s.audit.Warn("refresh token used twice: sign-in revoked", "client_id", g.ClientID)
		oauthError(w, http.StatusBadRequest, "invalid_grant", "the refresh token was already used; sign in again")
		return
	case errors.Is(err, errGrantExpired):
		s.audit.Info("sign-in expired", "client_id", g.ClientID)
		oauthError(w, http.StatusBadRequest, "invalid_grant", "the sign-in expired; sign in again")
		return
	case errors.Is(err, errUnknownRefresh):
		oauthError(w, http.StatusBadRequest, "invalid_grant", "unknown refresh token")
		return
	case err != nil:
		s.serverError(w, err)
		return
	}
	s.audit.Info("token refreshed", "client_id", c.ID)
	s.issue(w, g.ID, access, refresh)
}

func (s *Server) issue(w http.ResponseWriter, grantID, access, refresh string) {
	s.mu.Lock()
	s.pruneLocked()
	s.access[hashToken(access)] = accessToken{grantID: grantID, expires: s.now().Add(accessLifetime)}
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token":  access,
		"token_type":    "Bearer",
		"expires_in":    int(accessLifetime / time.Second),
		"refresh_token": refresh,
	})
}

func verifyPKCE(verifier, challenge string) bool {
	if len(verifier) < 43 || len(verifier) > 128 {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	return subtle.ConstantTimeCompare([]byte(base64.RawURLEncoding.EncodeToString(sum[:])), []byte(challenge)) == 1
}
```

In `.github/workflows/ci.yml`, add this line after the `FuzzWritableNotesAreVisibleNotes` line in the `fuzz` job, with the same indentation:

```yaml
          go test ./internal/authserver/ -run '^$' -fuzz '^FuzzTokenEndpoint$' -fuzztime 20s
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./internal/authserver/ && go test ./internal/authserver/ && go test ./internal/authserver/ -run '^$' -fuzz '^FuzzTokenEndpoint$' -fuzztime 10s`
Expected: `ok` twice, fuzzing finds no failure.

- [ ] **Step 5: Commit**

```bash
git add internal/authserver .github/workflows/ci.yml
git commit -m "feat: issue and rotate tokens at the built-in token endpoint"
```

---

### Task 7: Coverage check for the new package

**Files:**
- Test: `internal/authserver/*_test.go` (only if coverage is short)

**Interfaces:** none.

- [ ] **Step 1: Measure**

Run: `go test ./internal/authserver/ -coverprofile=/tmp/authserver.out && go tool cover -func=/tmp/authserver.out | sort -k3 -n | head -15`
Expected: package coverage at 95% or more.

- [ ] **Step 2: Close gaps**

For each function under 90%, read the uncovered lines with `go tool cover -html=/tmp/authserver.out -o /tmp/authserver.html`. Add a test for each reachable branch in the matching `_test.go` file, in the style of the tests above. Branches that only an operating-system failure reaches (`tmp.Write`, `tmp.Close`, `os.Rename` in `saveLocked`) may stay uncovered. Do not add `//nolint` or build tags to skip code.

- [ ] **Step 3: Commit (only if tests were added)**

```bash
git add internal/authserver
git commit -m "test: cover the built-in sign-in's remaining branches"
```

---

### Task 8: Serve the built-in sign-in from the MCP server

**Files:**
- Modify: `internal/server/http.go`
- Test: `internal/server/builtin_auth_test.go`

**Interfaces:**
- Consumes: `authserver.New`, `authserver.Options`, `(*authserver.Server).Handler`, `(*authserver.Server).Verify` (Task 4).
- Produces: `type BuiltinAuth struct { Handler http.Handler; Verify func(ctx context.Context, token string) (*auth.TokenInfo, error); PublicURL string }` and `AuthConfig.Builtin *BuiltinAuth`.

- [ ] **Step 1: Write the failing tests**

Create `internal/server/builtin_auth_test.go`:

```go
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/server/ -run Builtin`
Expected: build failure, `unknown field Builtin in struct literal of type AuthConfig`.

- [ ] **Step 3: Implement**

In `internal/server/http.go`, add after `OIDCAuth`:

```go
// BuiltinAuth enables the built-in owner-password sign-in, served by this
// server at PublicURL.
type BuiltinAuth struct {
	// Handler serves the authorization server's endpoints.
	Handler http.Handler
	// Verify validates an access token the built-in sign-in issued.
	Verify func(ctx context.Context, token string) (*auth.TokenInfo, error)
	// PublicURL is both the protected resource and its authorization server.
	PublicURL string
}

// builtinPaths are the authorization server endpoints mounted on the mux.
var builtinPaths = []string{"/.well-known/oauth-authorization-server", "/register", "/authorize", "/token"}
```

Change `AuthConfig` to:

```go
// AuthConfig selects how MCP requests are authenticated: a static bearer
// token (API key), plus either a third-party OIDC provider or the built-in
// sign-in.
type AuthConfig struct {
	StaticToken string
	OIDC        *OIDCAuth
	Builtin     *BuiltinAuth
}
```

In `Handler`, after the `if authCfg.OIDC != nil { ... }` block, add:

```go
	if b := authCfg.Builtin; b != nil {
		opts.ResourceMetadataURL = b.PublicURL + metadataPath
		mux.Handle(metadataPath, auth.ProtectedResourceMetadataHandler(&oauthex.ProtectedResourceMetadata{
			Resource:               b.PublicURL,
			AuthorizationServers:   []string{b.PublicURL},
			BearerMethodsSupported: []string{"header"},
		}))
		for _, p := range builtinPaths {
			mux.Handle(p, b.Handler)
		}
	}
```

Update the `metadataPath` comment to say "when OIDC or the built-in sign-in is enabled", and the `Handler` doc comment to say "RFC 9728 protected-resource metadata and, with the built-in sign-in, its authorization endpoints".

In `verifyToken`, after the `if cfg.OIDC != nil { ... }` block, add:

```go
		if cfg.Builtin != nil {
			return cfg.Builtin.Verify(ctx, token)
		}
```

and update its doc comment to "then falls back to the OIDC or built-in verifier, whichever is configured".

- [ ] **Step 4: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./internal/server/ && go test ./internal/server/`
Expected: `ok`. If the SDK client fails to connect, read its error first: discovery, registration, and issuer checks each name the step that failed.

- [ ] **Step 5: Commit**

```bash
git add internal/server
git commit -m "feat: serve the built-in sign-in from the MCP endpoint"
```

---

### Task 9: Turn the built-in sign-in on from configuration

**Files:**
- Modify: `cmd/crystal-cove/main.go`
- Test: `cmd/crystal-cove/main_test.go`

**Interfaces:**
- Consumes: `config.Config.OwnerPassword`, `.AuthStoreDir`, `.PublicURL` (Task 1), `authserver.New` (Task 4), `server.BuiltinAuth` (Task 8).

- [ ] **Step 1: Write the failing tests**

Add to `cmd/crystal-cove/main_test.go` (add `"encoding/json"` to the imports):

```go
func builtinEnv(t *testing.T) map[string]string {
	t.Helper()
	env := testEnv(t)
	env["HOME"] = t.TempDir()
	env["MCP_OWNER_PASSWORD"] = "correct-horse-battery-staple"
	env["MCP_PUBLIC_URL"] = "http://127.0.0.1:8080"
	return env
}

func TestRunWithBuiltinSignIn(t *testing.T) {
	installFakeOb(t, `case "$1" in sync) echo "Fully synced"; exec sleep 60;; *) exit 0;; esac`)
	env := builtinEnv(t)
	logs := &lockedBuffer{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	addrCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() { errCh <- run(ctx, getenv(env), logs, func(addr string) { addrCh <- addr }) }()

	var addr string
	select {
	case addr = <-addrCh:
	case err := <-errCh:
		t.Fatalf("run exited early: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("server did not become ready")
	}

	res, err := http.Get(fmt.Sprintf("http://%s/.well-known/oauth-authorization-server", addr))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var meta struct {
		Issuer string `json:"issuer"`
	}
	if err := json.NewDecoder(res.Body).Decode(&meta); err != nil {
		t.Fatal(err)
	}
	if meta.Issuer != "http://127.0.0.1:8080" {
		t.Errorf("issuer = %q", meta.Issuer)
	}
	if _, err := os.Stat(filepath.Join(env["HOME"], ".crystal-cove", "auth.json")); err != nil {
		t.Errorf("auth store not created: %v", err)
	}
	if out := logs.String(); !strings.Contains(out, "built-in sign-in enabled") || strings.Contains(out, "correct-horse-battery-staple") {
		t.Errorf("logs = %q, want the sign-in announced without the password", out)
	}
}

func TestRunFailsWhenAuthStoreIsCorrupt(t *testing.T) {
	installFakeOb(t, `case "$1" in sync) echo "Fully synced"; exec sleep 60;; *) exit 0;; esac`)
	env := builtinEnv(t)
	dir := filepath.Join(env["HOME"], ".crystal-cove")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "auth.json"), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := run(context.Background(), getenv(env), io.Discard, nil)
	if err == nil || !strings.Contains(err.Error(), "starting the built-in sign-in") {
		t.Errorf("err = %v", err)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./cmd/crystal-cove/ -run 'Builtin|AuthStore'`
Expected: FAIL. The metadata request returns 401, and the corrupt store does not stop `run`.

- [ ] **Step 3: Implement**

In `cmd/crystal-cove/main.go`, add `"github.com/dakotahp/crystal-cove/internal/authserver"` to the imports. After the `if cfg.OAuth != nil { ... }` block, add:

```go
	if cfg.OwnerPassword != "" {
		signIn, err := authserver.New(authserver.Options{
			PublicURL: cfg.PublicURL,
			Password:  cfg.OwnerPassword,
			StoreDir:  cfg.AuthStoreDir,
			Audit:     logger.With("audit", true),
		})
		if err != nil {
			return fmt.Errorf("starting the built-in sign-in: %w", err)
		}
		authCfg.Builtin = &server.BuiltinAuth{Handler: signIn.Handler(), Verify: signIn.Verify, PublicURL: cfg.PublicURL}
		logger.Info("built-in sign-in enabled", "public_url", cfg.PublicURL, "store", cfg.AuthStoreDir)
	}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./... && go test ./cmd/crystal-cove/`
Expected: `ok`.

- [ ] **Step 5: Commit**

```bash
git add cmd/crystal-cove
git commit -m "feat: turn on the built-in sign-in with MCP_OWNER_PASSWORD"
```

---

### Task 10: Documentation and the full check

**Files:**
- Modify: `README.md`, `docs/oauth.md`, `docs/configuration.md`, `.env.example`, `docs/SECURITY.md`, `CLAUDE.md`

**Interfaces:** none.

- [ ] **Step 1: README remote steps**

In `README.md`, in the "Remote" section, replace step **3. Set up OAuth.** (its paragraph, the code block, and the sentence after it) with:

````markdown
**3. Turn on sign-in.** claude.ai and the Claude mobile app sign in with OAuth, and Crystal Cove has it built in. Add to `.env`:

```sh
MCP_PUBLIC_URL=https://obsidian.example.com
# The password you type when you connect an app. At least 16 characters.
MCP_OWNER_PASSWORD=
```

Your proxy sends every path to the container, with no token check of its own. For Caddy, that is all it needs:

```
obsidian.example.com {
	reverse_proxy localhost:8080
}
```

Keep an `MCP_AUTH_TOKEN` too if you also want to connect Claude Code or scripts with a plain token. To use an identity provider you already run instead, see [docs/oauth.md](docs/oauth.md).
````

In step **4**, delete the sentences about "a proxy that checks the token itself", and change the second `curl` to check `https://obsidian.example.com/ready` without the `Authorization` header.

In step **5**, replace the claude.ai bullet with:

```markdown
- **claude.ai / Claude Desktop / Claude mobile:** add a custom connector with URL `https://obsidian.example.com/`. Leave the client ID and secret empty. A Crystal Cove page opens: enter your owner password, and you return to Claude connected.
```

Run `node scripts/generate-toc.js` afterwards.

- [ ] **Step 2: `docs/oauth.md`**

Replace the whole file with:

````markdown
# OAuth

You need OAuth to use Crystal Cove from claude.ai in a browser, from the Claude mobile app, or from ChatGPT. Claude Code and most local MCP clients work with the static `MCP_AUTH_TOKEN` alone.

## Built-in sign-in

Set two values:

```sh
MCP_PUBLIC_URL=https://obsidian.example.com
MCP_OWNER_PASSWORD=<at least 16 characters>
```

Crystal Cove then is its own OAuth server. An app registers itself, opens a Crystal Cove page where you enter the owner password, and goes back connected. You paste no client ID or secret.

- **Staying connected:** an app gets an access token for one hour and renews it by itself. One sign-in lasts at most 90 days, then you sign in again.
- **Disconnecting every app:** change `MCP_OWNER_PASSWORD` and restart the container.
- **A stolen renewal token:** when an old one is used again, that app's sign-in ends at once.
- **Wrong passwords:** after five, the page locks for a minute, and the lock doubles with each further miss, up to an hour.
- **Storage:** `/home/obsidian/.crystal-cove/auth.json`, beside the vaults and never inside one, so Obsidian Sync never carries it. It holds only hashes of tokens and secrets.

The page shows which app asks and which site you return to. If either looks wrong, do not enter the password.

## Using an identity provider

If you already run Keycloak, Auth0, Entra ID, or another OIDC provider, Crystal Cove can accept its tokens instead. Use this or the built-in sign-in, not both.

Setting `OAUTH_ISSUER` turns the server into an OAuth 2.0 protected resource:

- Bearer JWTs from the issuer are validated: signature via JWKS, issuer, lifetime, and audience, with the `azp` fallback Keycloak uses for client tokens.
- RFC 9728 protected-resource metadata is served at `/.well-known/oauth-protected-resource`, and 401 responses carry a `resource_metadata` challenge. MCP clients use it to find your provider and run the flow themselves, including dynamic client registration if your provider allows it.
- The static `MCP_AUTH_TOKEN` keeps working alongside.

Minimal example:

```sh
OAUTH_ISSUER=https://auth.example.com/realms/myrealm
OAUTH_AUDIENCE=crystal-cove
MCP_PUBLIC_URL=https://obsidian.example.com
OAUTH_REQUIRED_ROLES=vault-owner
```

Your provider must issue tokens whose `aud` (or `azp`) contains `OAUTH_AUDIENCE`. In Keycloak, that is a client scope with an audience mapper, made a realm default so dynamically registered MCP clients pick it up automatically.

Set `OAUTH_REQUIRED_ROLES` to a role only you hold. Without it, anyone who can get a token for the audience from your provider can use every tool. With open sign-up, that means anyone.

The other OAuth variables are in [Configuration](configuration.md).
````

- [ ] **Step 3: Configuration, example file, security, and CLAUDE.md**

In `docs/configuration.md`, add this row after `MCP_AUTH_TOKEN`, and change the `MCP_AUTH_TOKEN` row's last sentence to "Optional when `OAUTH_ISSUER` or `MCP_OWNER_PASSWORD` is set; at least one of the three is required.":

```markdown
| `MCP_OWNER_PASSWORD` | no | Turns on the built-in OAuth sign-in, see [OAuth](oauth.md). At least 16 characters. Requires `MCP_PUBLIC_URL`; cannot be combined with `OAUTH_ISSUER`. Changing it signs every app out. |
```

and change the `MCP_PUBLIC_URL` row to:

```markdown
| `MCP_PUBLIC_URL` | with `OAUTH_ISSUER` or `MCP_OWNER_PASSWORD` | This server's canonical public URL: the protected-resource identifier, and the built-in sign-in's issuer. Must be `https`, except on a loopback host. |
```

In `.env.example`, after the `MCP_AUTH_TOKEN=` line, add:

```sh

# Unlocks this server for apps that sign in, such as claude.ai: the owner
# password you type on the sign-in page. At least 16 characters. Needs
# MCP_PUBLIC_URL. Changing it signs every app out.
#MCP_OWNER_PASSWORD=
#MCP_PUBLIC_URL=https://obsidian.example.com
```

and change the `MCP_AUTH_TOKEN` comment's last sentence to "Optional when OAUTH_ISSUER or MCP_OWNER_PASSWORD is configured." Also change the intro sentence "one unlocks this server for your AI clients (MCP_AUTH_TOKEN)" to "one unlocks this server for your AI clients (MCP_AUTH_TOKEN or MCP_OWNER_PASSWORD)".

In `docs/SECURITY.md`, find the section on authentication. Add a subsection:

```markdown
### Built-in sign-in

With `MCP_OWNER_PASSWORD`, the server issues its own OAuth tokens. Tokens are 32 random bytes, and only their SHA-256 hashes are stored, in `auth.json` beside the vaults with mode 0600. Authorization codes last 60 seconds and work once; a second use revokes the sign-in it created. Access tokens last one hour and live only in memory. Refresh tokens rotate on every use, a reused one revokes its sign-in, and every sign-in ends after 90 days. PKCE S256 is required, and redirect URIs must match a registered https (or loopback http) address exactly. The sign-in page cannot be framed, and after five wrong passwords it locks for one minute, doubling up to one hour. Changing the password deletes every sign-in at the next start.
```

In `CLAUDE.md`:
- Replace the MVP-boundary bullet that begins "Auth is a static bearer token" with: "Auth is a static bearer token (`MCP_AUTH_TOKEN`), plus either the built-in owner-password sign-in (`MCP_OWNER_PASSWORD` + `MCP_PUBLIC_URL`, `internal/authserver`) or an OIDC provider (`OAUTH_ISSUER` + `OAUTH_AUDIENCE` + `MCP_PUBLIC_URL`, validated in `internal/oidcauth`), never both. The server advertises RFC 9728 protected-resource metadata so MCP clients can bootstrap the OAuth flow."
- In the architecture block, add after `internal/oidcauth/`:

```
internal/authserver/   built-in OAuth server for one owner: RFC 8414
                       metadata, dynamic registration, a password sign-in
                       page, PKCE, rotating refresh tokens with reuse
                       detection; clients and grants in ~/.crystal-cove
```

- [ ] **Step 4: Full check**

Run:

```bash
gofmt -l .
node --test scripts/generate-toc.test.js
go vet ./...
go test ./... -coverprofile=coverage.out -covermode=atomic
go tool cover -func=coverage.out | tail -1
```

Expected: no gofmt output, TOC tests pass, vet clean, all packages `ok`, total coverage 95% or more. If the total is under 95%, return to Task 7.

- [ ] **Step 5: Commit**

```bash
git add README.md docs CLAUDE.md .env.example
git commit -m "docs: document the built-in sign-in"
```

---

## Manual check before merge

Not a code task. After the branch passes CI, deploy the branch image to the real server, replace the `auth.py` shim with `MCP_OWNER_PASSWORD`, simplify the Caddy block to a plain `reverse_proxy`, and connect claude.ai with empty client fields. Record in the PR which `token_endpoint_auth_method` claude.ai registered with (it appears in the `client registered` audit line's client record in `auth.json`).
