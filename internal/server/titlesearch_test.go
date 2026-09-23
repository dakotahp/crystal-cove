package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/andyjmorgan/obsidian-hosted-mcp/internal/search"
	"github.com/andyjmorgan/obsidian-hosted-mcp/internal/vault"
)

func titleSearchServer(t *testing.T, paths ...string) *Server {
	t.Helper()
	v := vault.New("Personal", t.TempDir())
	for _, p := range paths {
		full := filepath.Join(v.Root(), filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("## Stores\n- Fred Meyer\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return New([]*vault.Vault{v}, search.New("rg", nil), func() bool { return true })
}

func searchFor(t *testing.T, s *Server, query string) *search.Result {
	t.Helper()
	_, res, err := s.searchNotes(context.Background(), &mcp.CallToolRequest{}, searchNotesInput{
		Vault: "Personal",
		Query: query,
	})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestSearchFindsNoteByTitleWhenBodyDoesNotMatch(t *testing.T) {
	s := titleSearchServer(t, "2_Areas/Meal Planning/Meal Planning.md")

	res := searchFor(t, s, "Meal Planning")
	if len(res.Files) != 1 {
		t.Fatalf("Files = %+v, want the note matched by title", res.Files)
	}
	f := res.Files[0]
	if f.Path != "2_Areas/Meal Planning/Meal Planning.md" || !f.TitleMatch {
		t.Errorf("Files[0] = %+v, want the note flagged as a title match", f)
	}
	if f.Lines == nil {
		t.Error("Files[0].Lines = nil, want an empty list so it marshals as []")
	}
}

func TestSearchPutsTitleMatchesFirst(t *testing.T) {
	s := titleSearchServer(t, "0_Inbox/groceries.md", "2_Areas/Stores.md")

	res := searchFor(t, s, "Stores")
	if len(res.Files) < 2 {
		t.Fatalf("Files = %+v, want both the title and the body match", res.Files)
	}
	if got := res.Files[0].Path; got != "2_Areas/Stores.md" {
		t.Errorf("Files[0].Path = %q, want the title match first", got)
	}
	if res.Files[0].TitleMatch == false {
		t.Error("Files[0].TitleMatch = false")
	}
}

func TestSearchKeepsBodyLinesOnATitleMatch(t *testing.T) {
	s := titleSearchServer(t, "2_Areas/Stores.md")

	res := searchFor(t, s, "Stores")
	if len(res.Files) != 1 {
		t.Fatalf("Files = %+v, want one file", res.Files)
	}
	if !res.Files[0].TitleMatch || len(res.Files[0].Lines) == 0 {
		t.Errorf("Files[0] = %+v, want both the title flag and its matching lines", res.Files[0])
	}
}

func TestSearchWithoutTitleMatchIsUnchanged(t *testing.T) {
	s := titleSearchServer(t, "0_Inbox/groceries.md")

	res := searchFor(t, s, "Fred Meyer")
	if len(res.Files) != 1 || res.Files[0].TitleMatch {
		t.Errorf("Files = %+v, want a plain content match", res.Files)
	}
}
