package server

import (
	"context"
	"fmt"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestListTagsLimitReturnsTheMostUsed(t *testing.T) {
	s, _ := metaServer(t, map[string]string{
		"a.md": tagged("common", "middle", "rare"),
		"b.md": tagged("common", "middle"),
		"c.md": tagged("common"),
	})

	_, res, err := s.listTags(context.Background(), &mcp.CallToolRequest{}, listTagsInput{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tags) != 2 || res.Tags[0].Tag != "common" || res.Tags[1].Tag != "middle" {
		t.Errorf("Tags = %+v, want common then middle", res.Tags)
	}
	if res.Total != 3 || res.NextOffset != 2 {
		t.Errorf("total %d, next %d; want 3 and 2", res.Total, res.NextOffset)
	}

	_, rest, err := s.listTags(context.Background(), &mcp.CallToolRequest{}, listTagsInput{Limit: 2, Offset: res.NextOffset})
	if err != nil {
		t.Fatal(err)
	}
	if len(rest.Tags) != 1 || rest.Tags[0].Tag != "rare" || rest.NextOffset != -1 {
		t.Errorf("second page = %+v, want only rare and no next page", rest)
	}
}

func TestListTagsPagesByDefault(t *testing.T) {
	notes := map[string]string{}
	for i := range DefaultListLimit + 5 {
		notes[fmt.Sprintf("n%04d.md", i)] = tagged(fmt.Sprintf("tag%04d", i))
	}
	s, _ := metaServer(t, notes)

	_, res, err := s.listTags(context.Background(), &mcp.CallToolRequest{}, listTagsInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tags) != DefaultListLimit || res.Total != DefaultListLimit+5 || res.NextOffset != DefaultListLimit {
		t.Errorf("%d tags, total %d, next %d; want %d, %d, %d",
			len(res.Tags), res.Total, res.NextOffset, DefaultListLimit, DefaultListLimit+5, DefaultListLimit)
	}
}

func TestListTagsRejectsAnOffsetOutOfRange(t *testing.T) {
	s, _ := metaServer(t, map[string]string{"a.md": tagged("one")})

	for _, offset := range []int{-1, 2} {
		if _, _, err := s.listTags(context.Background(), &mcp.CallToolRequest{}, listTagsInput{Offset: offset}); err == nil {
			t.Errorf("offset %d accepted", offset)
		}
	}
}
