package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dakotahp/crystal-cove/internal/search"
	"github.com/dakotahp/crystal-cove/internal/vault"
)

func TestFaviconsNeedNoToken(t *testing.T) {
	srv := New([]*vault.Vault{vault.New("Personal", t.TempDir())}, search.New("rg", nil), func() bool { return true })
	h := srv.Handler(AuthConfig{StaticToken: "secret"})

	for _, tc := range []struct {
		path, contentType string
		check             func([]byte) bool
	}{
		{"/favicon.ico", "image/x-icon", func(b []byte) bool { return bytes.HasPrefix(b, []byte{0, 0, 1, 0}) }},
		{"/favicon.svg", "image/svg+xml", func(b []byte) bool { return bytes.Equal(b, logoSVG) }},
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200", tc.path, rec.Code)
			continue
		}
		if got := rec.Header().Get("Content-Type"); got != tc.contentType {
			t.Errorf("%s: Content-Type = %q, want %q", tc.path, got, tc.contentType)
		}
		if rec.Header().Get("Cache-Control") == "" {
			t.Errorf("%s: no Cache-Control", tc.path)
		}
		if !tc.check(rec.Body.Bytes()) {
			t.Errorf("%s: unexpected body", tc.path)
		}
	}
}

func TestFaviconRejectsWrites(t *testing.T) {
	srv := New([]*vault.Vault{vault.New("Personal", t.TempDir())}, search.New("rg", nil), func() bool { return true })
	rec := httptest.NewRecorder()
	srv.Handler(AuthConfig{StaticToken: "secret"}).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/favicon.ico", nil))
	if rec.Code == http.StatusOK {
		t.Errorf("POST /favicon.ico status = 200, want a refusal")
	}
}
