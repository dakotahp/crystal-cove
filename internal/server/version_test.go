package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestEditNoteRefusesAnEditBuiltOnAnOldRead(t *testing.T) {
	s, v := metaServer(t, map[string]string{"Inbox/a.md": "- [ ] ship\n"})
	ctx := context.Background()
	req := &mcp.CallToolRequest{}

	_, read, err := s.readNote(ctx, req, readNoteInput{Path: "Inbox/a.md"})
	if err != nil {
		t.Fatal(err)
	}
	_, edited, err := s.editNote(ctx, req, editNoteInput{Path: "Inbox/a.md", Find: "- [ ]", Replace: "- [x]", Version: read.Version})
	if err != nil {
		t.Fatal(err)
	}

	synced := "- [x] ship\n- [ ] added on the phone\n"
	if err := os.WriteFile(filepath.Join(v.Root(), "Inbox", "a.md"), []byte(synced), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err = s.editNote(ctx, req, editNoteInput{Path: "Inbox/a.md", Find: "ship", Replace: "shipped", Version: edited.Version})
	if err == nil || !strings.Contains(err.Error(), "changed since") {
		t.Fatalf("err = %v, want the stale edit refused", err)
	}
	if got := readFile(t, v, "Inbox/a.md"); got != synced {
		t.Errorf("note = %q, want the synced text kept", got)
	}
}
