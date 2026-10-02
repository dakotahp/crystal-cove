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
		s.audit.Warn("refresh token used twice: sign-in revoked", "client_id", c.ID)
		oauthError(w, http.StatusBadRequest, "invalid_grant", "the refresh token was already used; sign in again")
		return
	case errors.Is(err, errGrantExpired):
		s.audit.Info("sign-in expired", "client_id", c.ID)
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
