package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCreateNoteReturnsAVersionForAFollowingEdit(t *testing.T) {
	s := rankServer(t, map[string]string{})
	session := testSession(t, s)

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "create_note", Arguments: map[string]any{
		"path": "Inbox/new.md", "content": "status: draft\n",
	}})
	if err != nil || res.IsError {
		t.Fatalf("create_note: %v %+v", err, res)
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var created struct {
		OK      bool   `json:"ok"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(raw, &created); err != nil {
		t.Fatal(err)
	}
	if !created.OK || created.Version == "" {
		t.Fatalf("create_note result = %s, want ok and a version", raw)
	}

	res, err = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "edit_note", Arguments: map[string]any{
		"path":    "Inbox/new.md",
		"version": created.Version,
		"edits":   []map[string]any{{"find": "draft", "replace": "done"}},
	}})
	if err != nil || res.IsError {
		t.Fatalf("edit_note with the create_note version: %v %+v", err, res)
	}
	data, err := os.ReadFile(filepath.Join(s.vaultList[0].Root(), "Inbox", "new.md"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != "status: done\n" {
		t.Errorf("note = %q, want the edit applied", got)
	}
}
