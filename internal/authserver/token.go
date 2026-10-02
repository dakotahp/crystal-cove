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
