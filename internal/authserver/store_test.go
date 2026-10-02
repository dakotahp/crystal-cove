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

func TestRotateRestoresStateOnWriteFailure(t *testing.T) {
	dir, clock := t.TempDir(), newTestClock()
	st := openTestStore(t, dir, "owner-password-1", clock)
	if err := st.createGrant(&grant{ID: "g1", ClientID: "c1", CreatedAt: clock.now(), RefreshHash: "r1"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	if _, err := st.rotate("c1", "r1", "r2"); err == nil {
		t.Error("rotate succeeded in a read-only folder")
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := st.rotate("c1", "r1", "r3"); err != nil {
		t.Errorf("rotate with original token after restore: %v", err)
	}
}

func TestAddClientRestoresStateOnWriteFailure(t *testing.T) {
	dir, clock := t.TempDir(), newTestClock()
	st := openTestStore(t, dir, "owner-password-1", clock)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	if err := st.addClient(&client{ID: "c1", AuthMethod: "none"}); err == nil {
		t.Error("addClient succeeded in a read-only folder")
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, ok := st.client("c1"); ok {
		t.Error("client was restored after failed addClient")
	}
}
