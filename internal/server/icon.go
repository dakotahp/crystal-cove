package server

import (
	_ "embed"
	"encoding/base64"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

//go:embed logo.svg
var logoSVG []byte

var serverIcon = mcp.Icon{
	Source:   "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(logoSVG),
	MIMEType: "image/svg+xml",
	Sizes:    []string{"any"},
}
