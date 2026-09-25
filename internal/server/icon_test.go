package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dakotahp/crystal-cove/internal/search"
	"github.com/dakotahp/crystal-cove/internal/vault"
)

func TestServerIconReachesClientOnInitialize(t *testing.T) {
	srv := New([]*vault.Vault{vault.New("Personal", t.TempDir())}, search.New("rg", nil), func() bool { return true })
	ts := httptest.NewServer(srv.Handler(AuthConfig{StaticToken: "secret"}))
	defer ts.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "icon-test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:   ts.URL,
		HTTPClient: &http.Client{Transport: authTransport{token: "secret"}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	icons := session.InitializeResult().ServerInfo.Icons
	if len(icons) != 1 {
		t.Fatalf("icons = %+v, want one", icons)
	}
	icon := icons[0]
	if icon.MIMEType != "image/svg+xml" || len(icon.Sizes) != 1 || icon.Sizes[0] != "any" {
		t.Errorf("icon = %+v, want image/svg+xml sized any", icon)
	}
	const prefix = "data:image/svg+xml;base64,"
	encoded, ok := strings.CutPrefix(icon.Source, prefix)
	if !ok {
		t.Fatalf("src = %.40q, want prefix %q", icon.Source, prefix)
	}
	got, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("../../docs/images/logo.svg")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Error("icon differs from docs/images/logo.svg; copy the logo into internal/server/logo.svg")
	}
}
