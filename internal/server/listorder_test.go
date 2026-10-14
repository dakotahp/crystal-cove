package server

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func sessionLogs() map[string]string {
	return map[string]string{
		"Logs/2026-10-01-09_00-a.md": "a\n",
		"Logs/2026-10-12-21_33-b.md": "b\n",
		"Logs/2026-10-13-16_32-c.md": "c\n",
		"Logs/2026-09-30-08_15-d.md": "d\n",
	}
}

func listedPaths(out listNotesOutput) []string {
	var paths []string
	for _, e := range out.Entries {
		paths = append(paths, e.Path)
	}
	return paths
}

func TestListNotesNewestNamesFirst(t *testing.T) {
	s, _ := metaServer(t, sessionLogs())

	_, out, err := s.listNotes(context.Background(), &mcp.CallToolRequest{}, listNotesInput{Dir: "Logs", Order: "desc", Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Logs/2026-10-13-16_32-c.md", "Logs/2026-10-12-21_33-b.md", "Logs/2026-10-01-09_00-a.md"}
	if got := listedPaths(out); !slices.Equal(got, want) || out.NextOffset != 3 || out.Total != 4 {
		t.Errorf("got %q next %d total %d, want %q, 3, 4", got, out.NextOffset, out.Total, want)
	}

	_, rest, err := s.listNotes(context.Background(), &mcp.CallToolRequest{}, listNotesInput{Dir: "Logs", Order: "desc", Offset: out.NextOffset})
	if err != nil {
		t.Fatal(err)
	}
	if got := listedPaths(rest); !slices.Equal(got, []string{"Logs/2026-09-30-08_15-d.md"}) {
		t.Errorf("second page = %q, want the oldest log", got)
	}
}

func TestListNotesAscendingIsTheDefault(t *testing.T) {
	s, _ := metaServer(t, sessionLogs())

	for _, order := range []string{"", "asc"} {
		_, out, err := s.listNotes(context.Background(), &mcp.CallToolRequest{}, listNotesInput{Dir: "Logs", Order: order})
		if err != nil {
			t.Fatal(err)
		}
		if got := listedPaths(out); !slices.IsSorted(got) || len(got) != 4 {
			t.Errorf("order %q: got %q, want path order", order, got)
		}
	}
}

func TestListNotesRejectsAnUnknownOrder(t *testing.T) {
	s, _ := metaServer(t, sessionLogs())

	_, _, err := s.listNotes(context.Background(), &mcp.CallToolRequest{}, listNotesInput{Dir: "Logs", Order: "newest"})
	if err == nil || !strings.Contains(err.Error(), "desc") {
		t.Errorf("err = %v, want one naming the orders", err)
	}
}
