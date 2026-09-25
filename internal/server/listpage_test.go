package server

import (
	"context"
	"fmt"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func manyNotes(n int) map[string]string {
	notes := map[string]string{}
	for i := range n {
		notes[fmt.Sprintf("Inbox/n%04d.md", i)] = "body\n"
	}
	return notes
}

func TestListNotesPagesLongListings(t *testing.T) {
	s, _ := metaServer(t, manyNotes(DefaultListLimit+50))
	ctx := context.Background()

	_, first, err := s.listNotes(ctx, &mcp.CallToolRequest{}, listNotesInput{Dir: "Inbox"})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Entries) != DefaultListLimit || first.Total != DefaultListLimit+50 || first.NextOffset != DefaultListLimit {
		t.Fatalf("first page: %d entries, total %d, next %d; want %d, %d, %d",
			len(first.Entries), first.Total, first.NextOffset, DefaultListLimit, DefaultListLimit+50, DefaultListLimit)
	}

	_, second, err := s.listNotes(ctx, &mcp.CallToolRequest{}, listNotesInput{Dir: "Inbox", Offset: first.NextOffset})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Entries) != 50 || second.NextOffset != -1 {
		t.Fatalf("second page: %d entries, next %d; want 50 and -1", len(second.Entries), second.NextOffset)
	}
	if second.Entries[0].Path != fmt.Sprintf("Inbox/n%04d.md", DefaultListLimit) {
		t.Errorf("second page starts at %q", second.Entries[0].Path)
	}
}

func TestListNotesHonoursALimitUpToTheCeiling(t *testing.T) {
	s, _ := metaServer(t, manyNotes(MaxListLimit+1))
	ctx := context.Background()

	_, out, err := s.listNotes(ctx, &mcp.CallToolRequest{}, listNotesInput{Dir: "Inbox", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Entries) != 10 || out.NextOffset != 10 {
		t.Errorf("limit 10: %d entries, next %d", len(out.Entries), out.NextOffset)
	}

	_, out, err = s.listNotes(ctx, &mcp.CallToolRequest{}, listNotesInput{Dir: "Inbox", Limit: 100000})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Entries) != MaxListLimit || out.NextOffset != MaxListLimit {
		t.Errorf("huge limit: %d entries, next %d; want the ceiling of %d", len(out.Entries), out.NextOffset, MaxListLimit)
	}
}

func TestListNotesRejectsAnOffsetOutOfRange(t *testing.T) {
	s, _ := metaServer(t, manyNotes(3))
	ctx := context.Background()

	for _, offset := range []int{-1, 4} {
		if _, _, err := s.listNotes(ctx, &mcp.CallToolRequest{}, listNotesInput{Dir: "Inbox", Offset: offset}); err == nil {
			t.Errorf("offset %d accepted", offset)
		}
	}
	_, out, err := s.listNotes(ctx, &mcp.CallToolRequest{}, listNotesInput{Dir: "Inbox", Offset: 3})
	if err != nil || len(out.Entries) != 0 || out.NextOffset != -1 {
		t.Errorf("offset at the end = %+v, %v; want an empty last page", out, err)
	}
}
