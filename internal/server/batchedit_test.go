package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestEditNoteAppliesSeveralEditsOverMCP(t *testing.T) {
	s := rankServer(t, map[string]string{"Inbox/a.md": "| row | old |\n\nend\n"})
	session := testSession(t, s)

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "edit_note", Arguments: map[string]any{
		"path": "Inbox/a.md",
		"edits": []map[string]any{
			{"find": "| row | old |", "replace": "| row | new |"},
			{"find": "end\n", "replace": "end\n\nadded\n"},
		},
	}})
	if err != nil || res.IsError {
		t.Fatalf("edit_note: %v %+v", err, res)
	}
	data, err := os.ReadFile(filepath.Join(s.vaultList[0].Root(), "Inbox", "a.md"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != "| row | new |\n\nend\n\nadded\n" {
		t.Errorf("note = %q, want both edits applied", got)
	}
}

func TestEditNoteNamesTheFailingEditOverMCP(t *testing.T) {
	s := rankServer(t, map[string]string{"Inbox/a.md": "alpha\n"})
	session := testSession(t, s)

	msg := toolError(t, session, "edit_note", map[string]any{
		"path": "Inbox/a.md",
		"edits": []map[string]any{
			{"find": "alpha", "replace": "beta"},
			{"find": "gamma", "replace": "delta"},
		},
	})
	if !strings.Contains(msg, "edit 2 of 2") {
		t.Errorf("error %q should name the failing edit", msg)
	}
}
