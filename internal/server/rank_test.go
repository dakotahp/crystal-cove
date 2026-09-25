package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dakotahp/crystal-cove/internal/search"
	"github.com/dakotahp/crystal-cove/internal/vault"
)

func rankServer(t *testing.T, notes map[string]string) *Server {
	t.Helper()
	v := vault.New("Personal", t.TempDir())
	for p, content := range notes {
		full := filepath.Join(v.Root(), filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return New([]*vault.Vault{v}, search.New("rg", nil), func() bool { return true })
}

func search1(t *testing.T, s *Server, in searchNotesInput) *search.Result {
	t.Helper()
	_, res, err := s.searchNotes(context.Background(), &mcp.CallToolRequest{}, in)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestRankingPutsTitleMatchesAboveBodyMatches(t *testing.T) {
	s := rankServer(t, map[string]string{
		"Inbox/daily.md":    "planning planning planning planning\n",
		"Areas/Planning.md": "a short note\n",
	})

	res := search1(t, s, searchNotesInput{Query: "planning"})
	if len(res.Files) < 2 {
		t.Fatalf("Files = %+v", res.Files)
	}
	if res.Files[0].Path != "Areas/Planning.md" {
		t.Errorf("Files[0] = %q, want the note named after the query", res.Files[0].Path)
	}
}

func TestRankingPrefersNotesCoveringMoreQueryWords(t *testing.T) {
	s := rankServer(t, map[string]string{
		"Inbox/one.md": "bike bike bike bike bike\n",
		"Inbox/two.md": "bike and maintenance\n",
	})

	res := search1(t, s, searchNotesInput{Query: "bike maintenance", Mode: "words"})
	if len(res.Files) == 0 {
		t.Fatal("no matches")
	}
	if res.Files[0].Path != "Inbox/two.md" {
		t.Errorf("Files[0] = %q, want the note holding both words", res.Files[0].Path)
	}
}

func TestRankingUsesMatchCountAsATieBreak(t *testing.T) {
	s := rankServer(t, map[string]string{
		"Inbox/few.md":  "bike\n",
		"Inbox/many.md": "bike\nbike\nbike\n",
	})

	res := search1(t, s, searchNotesInput{Query: "bike"})
	if len(res.Files) != 2 {
		t.Fatalf("Files = %+v", res.Files)
	}
	if res.Files[0].Path != "Inbox/many.md" {
		t.Errorf("Files[0] = %q, want the note with more matches first", res.Files[0].Path)
	}
}

func TestRankingIsStableForEqualScores(t *testing.T) {
	s := rankServer(t, map[string]string{
		"Inbox/b.md": "bike\n",
		"Inbox/a.md": "bike\n",
	})

	res := search1(t, s, searchNotesInput{Query: "bike"})
	if len(res.Files) != 2 || res.Files[0].Path != "Inbox/a.md" {
		t.Errorf("Files = %+v, want path order when scores tie", res.Files)
	}
}

func TestSearchAcceptsModeAndLineCap(t *testing.T) {
	s := rankServer(t, map[string]string{"Inbox/a.md": "bike\nbike\nbike\n"})

	res := search1(t, s, searchNotesInput{Query: "bike", MaxLinesPerNote: 1})
	if len(res.Files) != 1 || len(res.Files[0].Lines) != 1 {
		t.Errorf("Files = %+v, want one line kept", res.Files)
	}

	res = search1(t, s, searchNotesInput{Query: "bike bike", Mode: "regex"})
	if len(res.Files) != 0 {
		t.Errorf("Files = %+v, want the phrase treated as a regex", res.Files)
	}
}

func TestSearchRejectsAnUnknownMode(t *testing.T) {
	s := rankServer(t, map[string]string{"Inbox/a.md": "bike\n"})

	_, _, err := s.searchNotes(context.Background(), &mcp.CallToolRequest{}, searchNotesInput{Query: "bike", Mode: "fuzzy"})
	if err == nil {
		t.Error("searchNotes accepted an unknown mode")
	}
}

func TestRankingCountsEveryMatchNotJustTheLinesShown(t *testing.T) {
	s := rankServer(t, map[string]string{
		"Inbox/a.md": strings.Repeat("bike\n", search.DefaultLinesPerNote+1),
		"Inbox/b.md": strings.Repeat("bike\n", 4*search.DefaultLinesPerNote),
	})

	res := search1(t, s, searchNotesInput{Query: "bike"})
	if len(res.Files) != 2 || res.Files[0].Path != "Inbox/b.md" {
		t.Errorf("Files = %+v, want the note with more matches first", res.Files)
	}
}

func TestSearchRanksBeforeLimiting(t *testing.T) {
	notes := map[string]string{"Inbox/z.md": strings.Repeat("bike\n", 10)}
	for i := range 20 {
		notes[fmt.Sprintf("Inbox/a%02d.md", i)] = "bike\n"
	}
	s := rankServer(t, notes)

	res := search1(t, s, searchNotesInput{Query: "bike", MaxResults: 5})
	if len(res.Files) != 5 || !res.Truncated {
		t.Fatalf("got %d notes, Truncated=%v; want 5 and true", len(res.Files), res.Truncated)
	}
	if res.Files[0].Path != "Inbox/z.md" {
		t.Errorf("Files[0] = %q, want the best match even though it sorts last by path", res.Files[0].Path)
	}
}

func TestSearchTotalMatchesCountsTheNotesReturned(t *testing.T) {
	s := rankServer(t, map[string]string{
		"Inbox/a.md": "bike\n",
		"Inbox/b.md": strings.Repeat("bike\n", 3),
		"Inbox/c.md": strings.Repeat("bike\n", 7),
	})

	res := search1(t, s, searchNotesInput{Query: "bike", MaxResults: 2})
	if res.TotalMatches != 10 {
		t.Errorf("TotalMatches = %d, want 10 across the two notes returned", res.TotalMatches)
	}
}

func TestRankingKeepsTitleMatchesAboveAnyMatchCount(t *testing.T) {
	s := rankServer(t, map[string]string{
		"Inbox/busy.md": strings.Repeat("bike\n", 2000),
		"Areas/Bike.md": "a short note\n",
	})

	res := search1(t, s, searchNotesInput{Query: "bike"})
	if len(res.Files) != 2 || res.Files[0].Path != "Areas/Bike.md" {
		t.Errorf("Files = %+v, want the note named after the query first", res.Files)
	}
}
