package server

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
)

// metadataPath is where RFC 9728 protected-resource metadata is served
// when OIDC delegation is enabled.
const metadataPath = "/.well-known/oauth-protected-resource"

// OIDCAuth enables delegating bearer-token validation to a third-party
// OpenID Connect identity provider.
type OIDCAuth struct {
	// Verify validates a provider-issued token (see internal/oidcauth).
	Verify func(ctx context.Context, token string) (*auth.TokenInfo, error)
	// Issuer is advertised to MCP clients as the authorization server.
	Issuer string
	// Scopes are advertised as scopes_supported.
	Scopes []string
	// PublicURL is this server's canonical external URL — the protected
	// resource identifier.
	PublicURL string
}

// AuthConfig selects how MCP requests are authenticated: a static bearer
// token (API key), a third-party OIDC provider, or both side by side.
type AuthConfig struct {
	StaticToken string
	OIDC        *OIDCAuth
}

// Handler returns the HTTP handler: process health at /health, sync-aware
// readiness at /ready, RFC 9728 protected-resource metadata when OIDC is
// enabled, and the bearer-protected MCP endpoint everywhere else.
func (s *Server) Handler(authCfg AuthConfig) http.Handler {
	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return s.MCPServer()
	}, nil)
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})
	mux.HandleFunc("/ready", func(w http.ResponseWriter, _ *http.Request) {
		if s.syncReady == nil || !s.syncReady() {
			http.Error(w, "sync not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})
	opts := &auth.RequireBearerTokenOptions{}
	if authCfg.OIDC != nil {
		opts.ResourceMetadataURL = authCfg.OIDC.PublicURL + metadataPath
		mux.Handle(metadataPath, auth.ProtectedResourceMetadataHandler(&oauthex.ProtectedResourceMetadata{
			Resource:               authCfg.OIDC.PublicURL,
			AuthorizationServers:   []string{authCfg.OIDC.Issuer},
			ScopesSupported:        authCfg.OIDC.Scopes,
			BearerMethodsSupported: []string{"header"},
		}))
	}
	mux.Handle("/", auth.RequireBearerToken(s.logRejections(verifyToken(authCfg)), opts)(mcpHandler))
	return mux
}

// logRejections records each presented token that verify refuses, without
// the token itself, so guessing shows up in the audit log.
func (s *Server) logRejections(verify auth.TokenVerifier) auth.TokenVerifier {
	if s.audit == nil {
		return verify
	}
	return func(ctx context.Context, token string, r *http.Request) (*auth.TokenInfo, error) {
		info, err := verify(ctx, token, r)
		if err != nil {
			s.audit.Warn("rejected bearer token", "remote_addr", r.RemoteAddr, "reason", err.Error())
		}
		return info, err
	}
}

// verifyToken accepts the static token (constant-time compare) when one is
// configured, then falls back to the OIDC verifier when one is configured.
func verifyToken(cfg AuthConfig) auth.TokenVerifier {
	return func(ctx context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		if cfg.StaticToken != "" &&
			subtle.ConstantTimeCompare([]byte(token), []byte(cfg.StaticToken)) == 1 {
			// Static keys do not expire; the middleware requires a bound.
			return &auth.TokenInfo{Expiration: time.Now().Add(time.Hour)}, nil
		}
		if cfg.OIDC != nil {
			return cfg.OIDC.Verify(ctx, token)
		}
		return nil, fmt.Errorf("%w: unknown bearer token", auth.ErrInvalidToken)
	}
}
