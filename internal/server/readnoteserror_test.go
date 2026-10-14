package server

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestReadNotesFailureCarriesNoNoteFields(t *testing.T) {
	s, _ := metaServer(t, map[string]string{"Inbox/empty.md": ""})
	session := testSession(t, s)

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "read_notes", Arguments: map[string]any{
		"paths": []string{"Inbox/missing.md", "Inbox/empty.md"},
	}})
	if err != nil || res.IsError {
		t.Fatalf("read_notes: %v %+v", err, res)
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Notes []map[string]any `json:"notes"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	failed, empty := out.Notes[0], out.Notes[1]
	for _, key := range []string{"content", "version", "offset", "total_characters", "truncated", "next_offset"} {
		if _, ok := failed[key]; ok {
			t.Errorf("failed note carries %q, which could pass for an empty note: %v", key, failed)
		}
	}
	if failed["error"] == nil || failed["path"] != "Inbox/missing.md" {
		t.Errorf("failed note = %v, want its path and error", failed)
	}
	if empty["content"] != "" || empty["version"] == "" || empty["error"] != nil {
		t.Errorf("empty note = %v, want empty content with a version and no error", empty)
	}
}
