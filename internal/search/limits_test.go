package search

import (
	"context"
	"fmt"
	"testing"
)

func TestMaxResultsCountsNotesNotLines(t *testing.T) {
	s := requireRipgrep(t)
	notes := map[string]string{}
	for i := range 5 {
		notes[fmt.Sprintf("n%d.md", i)] = "bike\nbike\nbike\nbike\n"
	}
	root := wordsVault(t, notes)

	res, err := s.Search(context.Background(), root, Options{Query: "bike", MaxResults: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Files) != 2 {
		t.Errorf("files = %d, want 2 notes", len(res.Files))
	}
	if !res.Truncated {
		t.Error("Truncated = false, want true when notes were left out")
	}
}

func TestDefaultCapsLinesPerNote(t *testing.T) {
	s := requireRipgrep(t)
	body := ""
	for range 20 {
		body += "bike\n"
	}
	root := wordsVault(t, map[string]string{"a.md": body})

	res, err := s.Search(context.Background(), root, Options{Query: "bike"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Files) != 1 {
		t.Fatalf("files = %d", len(res.Files))
	}
	if got := len(res.Files[0].Lines); got != DefaultLinesPerNote {
		t.Errorf("lines = %d, want the default cap of %d", got, DefaultLinesPerNote)
	}
	if res.Files[0].TotalMatches != 20 {
		t.Errorf("TotalMatches = %d, want every match counted", res.Files[0].TotalMatches)
	}
}

func TestLineCapCanBeLifted(t *testing.T) {
	s := requireRipgrep(t)
	body := ""
	for range 20 {
		body += "bike\n"
	}
	root := wordsVault(t, map[string]string{"a.md": body})

	res, err := s.Search(context.Background(), root, Options{Query: "bike", MaxLinesPerFile: -1})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(res.Files[0].Lines); got != 20 {
		t.Errorf("lines = %d, want every line when the cap is lifted", got)
	}
}
