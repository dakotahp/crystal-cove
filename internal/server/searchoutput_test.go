package server

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestFilesOutputLeavesOutLinesButKeepsCounts(t *testing.T) {
	s := rankServer(t, map[string]string{
		"Inbox/a.md": "bike\nbike\nbike\n",
		"Bike.md":    "nothing here\n",
	})

	res := search1(t, s, searchNotesInput{Query: "bike", Output: "files", ContextLines: 2})
	if len(res.Files) != 2 || res.TotalMatches != 3 {
		t.Fatalf("Files = %+v, TotalMatches = %d; want both notes and every match counted", res.Files, res.TotalMatches)
	}
	if !res.Files[0].TitleMatch || res.Files[1].TotalMatches != 3 {
		t.Errorf("Files = %+v, want the name match flagged and the body count kept", res.Files)
	}
	for _, f := range res.Files {
		if f.Lines == nil || len(f.Lines) != 0 {
			t.Errorf("%s: Lines = %+v, want an empty list", f.Path, f.Lines)
		}
	}
}

func TestLinesOutputIsTheDefault(t *testing.T) {
	s := rankServer(t, map[string]string{"Inbox/a.md": "bike\n"})

	for _, output := range []string{"", "lines"} {
		res := search1(t, s, searchNotesInput{Query: "bike", Output: output})
		if len(res.Files) != 1 || len(res.Files[0].Lines) != 1 {
			t.Errorf("output %q: Files = %+v, want the matching line", output, res.Files)
		}
	}
}

func TestSearchRejectsAnUnknownOutput(t *testing.T) {
	s := rankServer(t, map[string]string{"Inbox/a.md": "bike\n"})

	_, _, err := s.searchNotes(context.Background(), &mcp.CallToolRequest{}, searchNotesInput{Query: "bike", Output: "counts"})
	if err == nil || !strings.Contains(err.Error(), "files") {
		t.Errorf("err = %v, want one naming the outputs", err)
	}
}

func TestZeroLinesPerNoteIsRejectedNotDefaulted(t *testing.T) {
	s := rankServer(t, map[string]string{"Inbox/a.md": "bike\n"})
	session := testSession(t, s)

	msg := toolError(t, session, "search_notes", map[string]any{"query": "bike", "max_lines_per_note": 0})
	if !strings.Contains(msg, "max_lines_per_note") || !strings.Contains(msg, "output") {
		t.Errorf("error = %q, want one pointing at output files", msg)
	}
}

func TestLinesPerNoteDefaultsWhenLeftOut(t *testing.T) {
	s := rankServer(t, map[string]string{"Inbox/a.md": strings.Repeat("bike\n", 8)})

	if res := search1(t, s, searchNotesInput{Query: "bike"}); len(res.Files[0].Lines) != 5 {
		t.Errorf("Lines = %d, want the default 5", len(res.Files[0].Lines))
	}
	every := -1
	if res := search1(t, s, searchNotesInput{Query: "bike", MaxLinesPerNote: &every}); len(res.Files[0].Lines) != 8 {
		t.Errorf("Lines = %d, want all 8 for -1", len(res.Files[0].Lines))
	}
}
