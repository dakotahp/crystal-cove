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
	body, err := json.Marshal(&resp)
	if err != nil {
		s.serverError(w, err)
		return
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		s.serverError(w, err)
		return
	}
	if resp.ClientSecret != "" {
		fields["client_secret_expires_at"] = json.RawMessage("0")
	}
	writeJSON(w, http.StatusCreated, fields)
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
