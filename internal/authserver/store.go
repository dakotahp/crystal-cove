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

func (s *store) deepCopyData() storeData {
	cp := s.data
	cp.Clients = make([]*client, len(s.data.Clients))
	for i, c := range s.data.Clients {
		cc := *c
		cc.RedirectURIs = slices.Clone(c.RedirectURIs)
		cp.Clients[i] = &cc
	}
	cp.Grants = make([]*grant, len(s.data.Grants))
	for i, g := range s.data.Grants {
		gg := *g
		gg.UsedHashes = slices.Clone(g.UsedHashes)
		cp.Grants[i] = &gg
	}
	return cp
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
	saved := s.deepCopyData()
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
	if err := s.saveLocked(); err != nil {
		s.data = saved
		return err
	}
	return nil
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
	saved := s.deepCopyData()
	s.data.Grants = append(s.data.Grants, g)
	if err := s.saveLocked(); err != nil {
		s.data = saved
		return err
	}
	return nil
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
	saved := s.deepCopyData()
	s.data.Grants = slices.DeleteFunc(s.data.Grants, func(g *grant) bool { return g.ID == id })
	if len(s.data.Grants) == before {
		return nil
	}
	if err := s.saveLocked(); err != nil {
		s.data = saved
		return err
	}
	return nil
}

// rotate replaces the refresh token oldHash of clientID's grant with
// newHash. A token that was already rotated away revokes its grant, and a
// grant past its lifetime is deleted. Both deletions stay in memory when the
// save fails: failing open would leave a revoked sign-in usable.
func (s *store) rotate(clientID, oldHash, newHash string) (grant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, g := range s.data.Grants {
		switch {
		case slices.Contains(g.UsedHashes, oldHash):
			s.data.Grants = slices.Delete(s.data.Grants, i, i+1)
			if saveErr := s.saveLocked(); saveErr != nil {
				return *g, errors.Join(errRefreshReused, saveErr)
			}
			return *g, errRefreshReused
		case g.RefreshHash != oldHash || g.ClientID != clientID:
			continue
		case s.expired(g):
			s.data.Grants = slices.Delete(s.data.Grants, i, i+1)
			if saveErr := s.saveLocked(); saveErr != nil {
				return *g, errors.Join(errGrantExpired, saveErr)
			}
			return *g, errGrantExpired
		}
		saved := s.deepCopyData()
		g.UsedHashes = append(g.UsedHashes, oldHash)
		g.RefreshHash = newHash
		if err := s.saveLocked(); err != nil {
			s.data = saved
			return grant{}, err
		}
		return *g, nil
	}
	return grant{}, errUnknownRefresh
}
