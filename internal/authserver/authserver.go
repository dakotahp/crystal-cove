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

type Options struct {
	PublicURL string
	Password  string
	StoreDir  string
	Audit     *slog.Logger
	Now       func() time.Time
	Rand      io.Reader
}

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

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", s.metadata)
	mux.HandleFunc("POST /register", s.register)
	mux.HandleFunc("GET /authorize", s.authorizePage)
	mux.HandleFunc("POST /authorize", s.authorizeSubmit)
	mux.HandleFunc("POST /token", s.token)
	return mux
}

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

func (s *Server) token(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
