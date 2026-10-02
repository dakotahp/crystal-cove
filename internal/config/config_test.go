package config

import (
	"crypto/rand"
	"errors"
	"slices"
	"strings"
	"testing"
)

const (
	testToken         = "0123456789abcdef0123456789abcdef"
	testOwnerPassword = "correct-horse-battery"
)

func env(m map[string]string) Getenv {
	return func(key string) string { return m[key] }
}

func validEnv() map[string]string {
	return map[string]string{
		"OBSIDIAN_EMAIL":    "user@example.com",
		"OBSIDIAN_PASSWORD": "secret",
		"OBSIDIAN_VAULTS":   "Notes",
		"MCP_AUTH_TOKEN":    testToken,
		"HOME":              "/home/test",
	}
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(env(validEnv()), rand.Reader)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Email != "user@example.com" || cfg.Password != "secret" || cfg.AuthToken != testToken {
		t.Errorf("credentials not loaded: %+v", cfg)
	}
	if cfg.Port != DefaultPort {
		t.Errorf("Port = %d, want %d", cfg.Port, DefaultPort)
	}
	if cfg.VaultsDir != "/home/test/vaults" {
		t.Errorf("VaultsDir = %q, want /home/test/vaults", cfg.VaultsDir)
	}
	if !strings.HasPrefix(cfg.DeviceName, deviceNamePrefix) || len(cfg.DeviceName) != len(deviceNamePrefix)+8 {
		t.Errorf("DeviceName = %q, want %s + 8 hex chars", cfg.DeviceName, deviceNamePrefix)
	}
	if len(cfg.Vaults) != 1 || cfg.Vaults[0] != (Vault{Name: "Notes"}) {
		t.Errorf("Vaults = %+v", cfg.Vaults)
	}
}

func TestLoadExplicitValues(t *testing.T) {
	m := validEnv()
	m["OBSIDIAN_DEVICE_NAME"] = "my-device"
	m["VAULTS_DIR"] = "/data/vaults"
	m["PORT"] = "9000"
	cfg, err := Load(env(m), rand.Reader)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DeviceName != "my-device" {
		t.Errorf("DeviceName = %q", cfg.DeviceName)
	}
	if cfg.VaultsDir != "/data/vaults" {
		t.Errorf("VaultsDir = %q", cfg.VaultsDir)
	}
	if cfg.Port != 9000 {
		t.Errorf("Port = %d", cfg.Port)
	}
}

func TestLoadNoHomeFallsBackToRoot(t *testing.T) {
	m := validEnv()
	delete(m, "HOME")
	cfg, err := Load(env(m), rand.Reader)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.VaultsDir != "/vaults" {
		t.Errorf("VaultsDir = %q, want /vaults", cfg.VaultsDir)
	}
}

func TestLoadObsidianAuthTokenReplacesAccountCredentials(t *testing.T) {
	m := validEnv()
	delete(m, "OBSIDIAN_EMAIL")
	delete(m, "OBSIDIAN_PASSWORD")
	m["OBSIDIAN_AUTH_TOKEN"] = "sync-token"
	cfg, err := Load(env(m), rand.Reader)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ObsidianAuthToken != "sync-token" {
		t.Errorf("ObsidianAuthToken = %q", cfg.ObsidianAuthToken)
	}
}

func TestLoadRequiresCredentialsOrObsidianAuthToken(t *testing.T) {
	m := validEnv()
	delete(m, "OBSIDIAN_PASSWORD")
	_, err := Load(env(m), rand.Reader)
	if err == nil || !strings.Contains(err.Error(), "OBSIDIAN_AUTH_TOKEN") {
		t.Errorf("err = %v, want mention of OBSIDIAN_AUTH_TOKEN", err)
	}
}

func TestLoadMissingRequired(t *testing.T) {
	for _, key := range []string{"OBSIDIAN_EMAIL", "OBSIDIAN_PASSWORD", "MCP_AUTH_TOKEN", "OBSIDIAN_VAULTS"} {
		t.Run(key, func(t *testing.T) {
			m := validEnv()
			delete(m, key)
			if _, err := Load(env(m), rand.Reader); err == nil {
				t.Errorf("Load succeeded without %s", key)
			}
		})
	}
}

func TestLoadToolPolicy(t *testing.T) {
	cfg, err := Load(env(validEnv()), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ReadOnly || cfg.AllowPermanentDelete {
		t.Errorf("defaults: ReadOnly=%v AllowPermanentDelete=%v, want both false", cfg.ReadOnly, cfg.AllowPermanentDelete)
	}

	m := validEnv()
	m["MCP_READ_ONLY"] = "true"
	m["MCP_ALLOW_PERMANENT_DELETE"] = "1"
	cfg, err = Load(env(m), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.ReadOnly || !cfg.AllowPermanentDelete {
		t.Errorf("set: ReadOnly=%v AllowPermanentDelete=%v, want both true", cfg.ReadOnly, cfg.AllowPermanentDelete)
	}

	for _, key := range []string{"MCP_READ_ONLY", "MCP_ALLOW_PERMANENT_DELETE"} {
		m := validEnv()
		m[key] = "yes please"
		if _, err := Load(env(m), rand.Reader); err == nil || !strings.Contains(err.Error(), key) {
			t.Errorf("%s=%q: err = %v, want an error naming it", key, m[key], err)
		}
	}
}

func TestLoadRejectsShortAuthToken(t *testing.T) {
	m := validEnv()
	m["MCP_AUTH_TOKEN"] = "change-me"
	_, err := Load(env(m), rand.Reader)
	if err == nil || !strings.Contains(err.Error(), "at least 32 characters") {
		t.Fatalf("err = %v, want a minimum-length error", err)
	}
	if strings.Contains(err.Error(), "change-me") {
		t.Errorf("error %q echoes the token", err)
	}
}

func TestLoadInvalidPort(t *testing.T) {
	for _, port := range []string{"abc", "-1", "65536", "8080x"} {
		t.Run(port, func(t *testing.T) {
			m := validEnv()
			m["PORT"] = port
			if _, err := Load(env(m), rand.Reader); err == nil {
				t.Errorf("Load accepted PORT=%q", port)
			}
		})
	}
}

func TestLoadEphemeralPort(t *testing.T) {
	m := validEnv()
	m["PORT"] = "0"
	cfg, err := Load(env(m), rand.Reader)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != 0 {
		t.Errorf("Port = %d, want 0", cfg.Port)
	}
}

func TestParseVaults(t *testing.T) {
	tests := []struct {
		name   string
		list   string
		shared string
		want   []Vault
		errSub string
	}{
		{
			name: "single vault no password",
			list: "Notes",
			want: []Vault{{Name: "Notes"}},
		},
		{
			name: "per-vault passwords",
			list: "Work:pw1,Personal:pw2",
			want: []Vault{{Name: "Work", Password: "pw1"}, {Name: "Personal", Password: "pw2"}},
		},
		{
			name:   "shared password fallback",
			list:   "Work,Personal:own",
			shared: "shared",
			want:   []Vault{{Name: "Work", Password: "shared"}, {Name: "Personal", Password: "own"}},
		},
		{
			name: "password containing colons",
			list: "Notes:a:b:c",
			want: []Vault{{Name: "Notes", Password: "a:b:c"}},
		},
		{
			name: "whitespace and empty entries",
			list: " Notes , ,Ideas ",
			want: []Vault{{Name: "Notes"}, {Name: "Ideas"}},
		},
		{name: "empty list", list: "", errSub: "at least one vault"},
		{name: "only separators", list: " , ,", errSub: "at least one vault"},
		{name: "empty name with password", list: ":pw", errSub: "empty vault name"},
		{name: "duplicate name", list: "Notes,Notes", errSub: "more than once"},
		{name: "path separator in name", list: "a/b", errSub: "path separators"},
		{name: "backslash in name", list: `a\b`, errSub: "path separators"},
		{name: "dotdot name", list: "..", errSub: "path separators"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseVaults(tt.list, tt.shared)
			if tt.errSub != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errSub) {
					t.Fatalf("err = %v, want containing %q", err, tt.errSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseVaults: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("vault %d = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("no entropy") }

func TestLoadDeviceNameEntropyFailure(t *testing.T) {
	if _, err := Load(env(validEnv()), failingReader{}); err == nil {
		t.Error("Load succeeded with failing entropy source")
	}
}

func TestLoadOAuth(t *testing.T) {
	m := validEnv()
	m["OAUTH_ISSUER"] = "https://idp.example.com/realms/lab"
	m["OAUTH_AUDIENCE"] = "crystal-cove"
	m["MCP_PUBLIC_URL"] = "https://obsidian.example.com"
	cfg, err := Load(env(m), rand.Reader)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	o := cfg.OAuth
	if o == nil {
		t.Fatal("OAuth = nil, want configured")
	}
	if o.Issuer != "https://idp.example.com/realms/lab" || o.Audience != "crystal-cove" {
		t.Errorf("OAuth = %+v", o)
	}
	if o.InternalIssuer != o.Issuer {
		t.Errorf("InternalIssuer = %q, want issuer fallback", o.InternalIssuer)
	}
	if cfg.PublicURL != "https://obsidian.example.com" {
		t.Errorf("PublicURL = %q", cfg.PublicURL)
	}
	if want := []string{"openid", "profile", "email"}; !slices.Equal(o.Scopes, want) {
		t.Errorf("Scopes = %v, want default %v", o.Scopes, want)
	}
}

func TestLoadOAuthAllowsPlainHTTPOnLoopback(t *testing.T) {
	for _, issuer := range []string{"http://127.0.0.1:8080/realms/lab", "http://localhost:8080", "http://[::1]:9000"} {
		m := validEnv()
		m["OAUTH_ISSUER"] = issuer
		m["OAUTH_AUDIENCE"] = "crystal-cove"
		m["MCP_PUBLIC_URL"] = "http://127.0.0.1:8787"
		if _, err := Load(env(m), rand.Reader); err != nil {
			t.Errorf("Load with issuer %s: %v", issuer, err)
		}
	}
}

func TestLoadOAuthExplicit(t *testing.T) {
	m := validEnv()
	delete(m, "MCP_AUTH_TOKEN") // OAuth alone is a valid auth setup
	m["OAUTH_ISSUER"] = "https://idp.example.com/realms/lab"
	m["OAUTH_AUDIENCE"] = "crystal-cove"
	m["OAUTH_INTERNAL_ISSUER"] = "http://idp.cluster.local/realms/lab"
	m["OAUTH_SCOPES"] = "openid, obsidian-audience"
	m["MCP_PUBLIC_URL"] = "https://obsidian.example.com/"
	cfg, err := Load(env(m), rand.Reader)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AuthToken != "" {
		t.Errorf("AuthToken = %q, want empty", cfg.AuthToken)
	}
	if cfg.OAuth.InternalIssuer != "http://idp.cluster.local/realms/lab" {
		t.Errorf("InternalIssuer = %q", cfg.OAuth.InternalIssuer)
	}
	if want := []string{"openid", "obsidian-audience"}; !slices.Equal(cfg.OAuth.Scopes, want) {
		t.Errorf("Scopes = %v, want %v", cfg.OAuth.Scopes, want)
	}
	if cfg.PublicURL != "https://obsidian.example.com" {
		t.Errorf("PublicURL = %q, want trailing slash trimmed", cfg.PublicURL)
	}
}

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
		{"public url with a path", func(m map[string]string) {
			m["MCP_OWNER_PASSWORD"] = testOwnerPassword
			m["MCP_PUBLIC_URL"] = "https://vault.example.com/mcp"
		}, "must be the bare origin"},
		{"public url with a query", func(m map[string]string) {
			m["MCP_OWNER_PASSWORD"] = testOwnerPassword
			m["MCP_PUBLIC_URL"] = "https://vault.example.com?x=1"
		}, "must be the bare origin"},
		{"public url with a fragment", func(m map[string]string) {
			m["MCP_OWNER_PASSWORD"] = testOwnerPassword
			m["MCP_PUBLIC_URL"] = "https://vault.example.com#top"
		}, "must be the bare origin"},
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

func TestLoadOwnerPasswordAllowsTrailingSlashOrigin(t *testing.T) {
	m := validEnv()
	m["MCP_OWNER_PASSWORD"] = testOwnerPassword
	m["MCP_PUBLIC_URL"] = "https://vault.example.com/"
	if _, err := Load(env(m), rand.Reader); err != nil {
		t.Fatalf("Load: %v", err)
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

func TestLoadOAuthValidation(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(map[string]string)
		wantErr string
	}{
		{"no auth at all", func(m map[string]string) {
			delete(m, "MCP_AUTH_TOKEN")
		}, "MCP_OWNER_PASSWORD"},
		{"issuer without audience", func(m map[string]string) {
			m["OAUTH_ISSUER"] = "https://idp.example.com"
			m["MCP_PUBLIC_URL"] = "https://obsidian.example.com"
		}, "OAUTH_AUDIENCE"},
		{"issuer without public url", func(m map[string]string) {
			m["OAUTH_ISSUER"] = "https://idp.example.com"
			m["OAUTH_AUDIENCE"] = "crystal-cove"
		}, "MCP_PUBLIC_URL"},
		{"non-http issuer", func(m map[string]string) {
			m["OAUTH_ISSUER"] = "idp.example.com"
			m["OAUTH_AUDIENCE"] = "crystal-cove"
			m["MCP_PUBLIC_URL"] = "https://obsidian.example.com"
		}, "OAUTH_ISSUER"},
		{"plain-http issuer off loopback", func(m map[string]string) {
			m["OAUTH_ISSUER"] = "http://idp.example.com"
			m["OAUTH_AUDIENCE"] = "crystal-cove"
			m["MCP_PUBLIC_URL"] = "https://obsidian.example.com"
		}, "https"},
		{"audience without issuer", func(m map[string]string) {
			m["OAUTH_AUDIENCE"] = "crystal-cove"
		}, "OAUTH_ISSUER"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := validEnv()
			c.mutate(m)
			_, err := Load(env(m), rand.Reader)
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("err = %v, want mention of %s", err, c.wantErr)
			}
		})
	}
}
