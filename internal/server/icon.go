package server

import (
	_ "embed"
	"encoding/base64"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

//go:embed logo.svg
var logoSVG []byte

// favicon.ico is logo.svg rendered at 16, 32 and 48 pixels with rsvg-convert
// and ImageMagick; render it again when the logo changes.
//
//go:embed favicon.ico
var faviconICO []byte

var serverIcon = mcp.Icon{
	Source:   "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(logoSVG),
	MIMEType: "image/svg+xml",
	Sizes:    []string{"any"},
}

func serveIcon(contentType string, body []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write(body)
	}
}
