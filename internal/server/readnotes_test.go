package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dakotahp/crystal-cove/internal/vault"
)

func readNotesOverMCP(t *testing.T, s *Server, paths ...string) ReadNotes {
	t.Helper()
	session := testSession(t, s)
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "read_notes", Arguments: map[string]any{"paths": paths}})
	if err != nil || res.IsError {
		t.Fatalf("read_notes: %v %+v", err, res)
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var out ReadNotes
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestReadNotesReturnsEachNoteInOrder(t *testing.T) {
	s, _ := metaServer(t, map[string]string{
		"Inbox/a.md": "alpha\n",
		"Inbox/b.md": "beta\n",
	})

	out := readNotesOverMCP(t, s, "Inbox/b.md", "Inbox/a.md")
	if len(out.Notes) != 2 || out.Notes[0].Path != "Inbox/b.md" || out.Notes[0].Content != "beta\n" || out.Notes[1].Content != "alpha\n" {
		t.Fatalf("Notes = %+v, want both notes in the order asked", out.Notes)
	}
	for _, n := range out.Notes {
		if n.Version == "" || n.Truncated || n.NextOffset != -1 || n.Error != "" {
			t.Errorf("%s: %+v, want a whole note with its version", n.Path, n)
		}
	}
}

func TestReadNotesPagesALongNote(t *testing.T) {
	s, _ := metaServer(t, map[string]string{"Inbox/long.md": strings.Repeat("x", vault.ReadPageSize+10)})

	out := readNotesOverMCP(t, s, "Inbox/long.md")
	n := out.Notes[0]
	if len(n.Content) != vault.ReadPageSize || !n.Truncated || n.NextOffset != vault.ReadPageSize {
		t.Errorf("got %d characters, truncated %v, next %d; want one page and where to continue", len(n.Content), n.Truncated, n.NextOffset)
	}
}

func TestReadNotesStopsAtTheBudgetWithoutDroppingNotes(t *testing.T) {
	notes := map[string]string{}
	var paths []string
	count := MaxReadNotesChars/vault.ReadPageSize + 2
	for i := range count {
		p := fmt.Sprintf("Inbox/%02d.md", i)
		notes[p] = strings.Repeat("x", vault.ReadPageSize)
		paths = append(paths, p)
	}
	s, _ := metaServer(t, notes)

	out := readNotesOverMCP(t, s, paths...)
	if len(out.Notes) != count {
		t.Fatalf("got %d notes, want every note asked for", len(out.Notes))
	}
	total := 0
	for _, n := range out.Notes {
		total += len(n.Content)
	}
	last := out.Notes[count-1]
	if total != MaxReadNotesChars || last.Content != "" || !last.Truncated || last.NextOffset != 0 || last.Version == "" {
		t.Errorf("read %d characters, last note %+v; want the budget spent and the rest pointing at offset 0", total, last)
	}
	if !out.BudgetExhausted {
		t.Error("budget_exhausted not set")
	}
}

func TestReadNotesReportsBadPathsPerNote(t *testing.T) {
	s, _ := metaServer(t, map[string]string{"Inbox/a.md": "alpha\n", "Inbox/b.txt": "text\n"})

	out := readNotesOverMCP(t, s, "Inbox/missing.md", "Inbox/b.txt", "Inbox/a.md")
	if len(out.Notes) != 3 || out.Notes[2].Content != "alpha\n" {
		t.Fatalf("Notes = %+v, want the good note read despite the others", out.Notes)
	}
	if !strings.Contains(out.Notes[0].Error, "not_found") || out.Notes[1].Error == "" || out.Notes[2].Error != "" {
		t.Errorf("Notes = %+v, want an error on each bad path only", out.Notes)
	}
}

func TestReadNotesLeavesTheContentOutOfAFailedNote(t *testing.T) {
	s, _ := metaServer(t, map[string]string{"Inbox/empty.md": ""})
	session := testSession(t, s)
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "read_notes", Arguments: map[string]any{"paths": []string{"Inbox/missing.md", "Inbox/empty.md"}}})
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
	if want := []string{"path", "error"}; len(out.Notes[0]) != len(want) || out.Notes[0]["error"] == nil {
		t.Errorf("failed note = %v, want only %v", out.Notes[0], want)
	}
	if _, ok := out.Notes[1]["content"]; !ok {
		t.Errorf("empty note = %v, want its empty content reported", out.Notes[1])
	}
}

func TestReadNotesRejectsTooFewOrTooManyPaths(t *testing.T) {
	s, _ := metaServer(t, map[string]string{"Inbox/a.md": "alpha\n"})
	req := &mcp.CallToolRequest{}

	if _, _, err := s.readNotes(context.Background(), req, readNotesInput{}); err == nil {
		t.Error("read_notes accepted no paths")
	}
	many := make([]string, MaxReadNotes+1)
	for i := range many {
		many[i] = "Inbox/a.md"
	}
	if _, _, err := s.readNotes(context.Background(), req, readNotesInput{Paths: many}); err == nil || !strings.Contains(err.Error(), fmt.Sprint(MaxReadNotes)) {
		t.Errorf("err = %v, want the path limit named", err)
	}
	if _, _, err := s.readNotes(context.Background(), req, readNotesInput{Vault: "Nope", Paths: []string{"Inbox/a.md"}}); err == nil {
		t.Error("read_notes accepted an unknown vault")
	}
}
