package server

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func propertyNames(list *PropertyList) []string {
	names := make([]string, 0, len(list.Properties))
	for _, property := range list.Properties {
		names = append(names, property.Name)
	}
	return names
}

func TestListPropertiesCountsAndOrdersByUse(t *testing.T) {
	s, _ := metaServer(t, map[string]string{
		"a.md": "---\nstatus: active\npriority: 1\nsummary: x\n---\n\nbody\n",
		"b.md": "---\nstatus: done\nauthor: me\n---\n\nbody\n",
		"c.md": "---\nstatus:\n---\n\nbody\n",
		"d.md": "no frontmatter here\n",
	})

	_, res, err := s.listProperties(context.Background(), &mcp.CallToolRequest{}, listPropertiesInput{Vault: "Personal"})
	if err != nil {
		t.Fatal(err)
	}
	want := []PropertyCount{{"status", 3}, {"author", 1}, {"priority", 1}, {"summary", 1}}
	if !slices.Equal(res.Properties, want) {
		t.Errorf("Properties = %+v, want %+v", res.Properties, want)
	}
	if res.NotesScanned != 4 || res.Total != 4 || res.NextOffset != -1 {
		t.Errorf("scanned %d, total %d, next %d; want 4, 4, -1", res.NotesScanned, res.Total, res.NextOffset)
	}
}

func TestListPropertiesIgnoresInlineHashtags(t *testing.T) {
	s, _ := metaServer(t, map[string]string{"a.md": "---\nstatus: active\n---\n\nbody #idea\n"})

	_, res, err := s.listProperties(context.Background(), &mcp.CallToolRequest{}, listPropertiesInput{})
	if err != nil {
		t.Fatal(err)
	}
	if got := propertyNames(res); !slices.Equal(got, []string{"status"}) {
		t.Errorf("Properties = %v, want only the field the frontmatter holds", got)
	}
}

func TestListPropertiesNamesWorkInFindNotes(t *testing.T) {
	s, _ := metaServer(t, map[string]string{
		"lower.md": "---\nstatus: active\n---\n\nbody\n",
		"upper.md": "---\nStatus: active\n---\n\nbody\n",
	})
	ctx := context.Background()

	_, res, err := s.listProperties(ctx, &mcp.CallToolRequest{}, listPropertiesInput{})
	if err != nil {
		t.Fatal(err)
	}
	if got := propertyNames(res); !slices.Equal(got, []string{"Status", "status"}) {
		t.Fatalf("Properties = %v, want both spellings kept apart", got)
	}
	for _, property := range res.Properties {
		_, found, err := s.findNotes(ctx, &mcp.CallToolRequest{}, findNotesInput{Key: property.Name})
		if err != nil {
			t.Fatal(err)
		}
		if found.Total != property.Count {
			t.Errorf("find_notes on %q found %d notes, list_properties counted %d", property.Name, found.Total, property.Count)
		}
	}
}

func TestListPropertiesLimitReturnsTheMostUsed(t *testing.T) {
	s, _ := metaServer(t, map[string]string{
		"a.md": "---\ncommon: 1\nmiddle: 1\nrare: 1\n---\n",
		"b.md": "---\ncommon: 1\nmiddle: 1\n---\n",
		"c.md": "---\ncommon: 1\n---\n",
	})
	ctx := context.Background()

	_, res, err := s.listProperties(ctx, &mcp.CallToolRequest{}, listPropertiesInput{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if got := propertyNames(res); !slices.Equal(got, []string{"common", "middle"}) {
		t.Errorf("first page = %v, want common then middle", got)
	}
	if res.Total != 3 || res.NextOffset != 2 {
		t.Errorf("total %d, next %d; want 3 and 2", res.Total, res.NextOffset)
	}

	_, rest, err := s.listProperties(ctx, &mcp.CallToolRequest{}, listPropertiesInput{Limit: 2, Offset: res.NextOffset})
	if err != nil {
		t.Fatal(err)
	}
	if got := propertyNames(rest); !slices.Equal(got, []string{"rare"}) || rest.NextOffset != -1 {
		t.Errorf("second page = %+v, want only rare and no next page", rest)
	}
}

func TestListPropertiesPagesByDefault(t *testing.T) {
	notes := map[string]string{}
	for i := range DefaultListLimit + 5 {
		notes[fmt.Sprintf("n%04d.md", i)] = fmt.Sprintf("---\nfield%04d: 1\n---\n", i)
	}
	s, _ := metaServer(t, notes)

	_, res, err := s.listProperties(context.Background(), &mcp.CallToolRequest{}, listPropertiesInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Properties) != DefaultListLimit || res.Total != DefaultListLimit+5 || res.NextOffset != DefaultListLimit {
		t.Errorf("%d properties, total %d, next %d; want %d, %d, %d",
			len(res.Properties), res.Total, res.NextOffset, DefaultListLimit, DefaultListLimit+5, DefaultListLimit)
	}
}

func TestListPropertiesRejectsAnOffsetOutOfRange(t *testing.T) {
	s, _ := metaServer(t, map[string]string{"a.md": "---\nstatus: active\n---\n"})

	for _, offset := range []int{-1, 2} {
		if _, _, err := s.listProperties(context.Background(), &mcp.CallToolRequest{}, listPropertiesInput{Offset: offset}); err == nil {
			t.Errorf("offset %d accepted", offset)
		}
	}
}

func TestListPropertiesSkipsNotesWithBrokenYAML(t *testing.T) {
	s, _ := metaServer(t, map[string]string{
		"good.md": "---\nstatus: active\n---\n",
		"bad.md":  "---\ntags: [unclosed\n---\n\nbody\n",
	})

	_, res, err := s.listProperties(context.Background(), &mcp.CallToolRequest{}, listPropertiesInput{})
	if err != nil {
		t.Fatalf("one malformed note failed the whole scan: %v", err)
	}
	if got := propertyNames(res); res.NotesScanned != 2 || !slices.Equal(got, []string{"status"}) {
		t.Errorf("res = %+v, want both notes read and the good one's field", res)
	}
}

func TestListPropertiesRejectsAnUnknownVault(t *testing.T) {
	s, _ := metaServer(t, map[string]string{"a.md": "---\nstatus: active\n---\n"})

	if _, _, err := s.listProperties(context.Background(), &mcp.CallToolRequest{}, listPropertiesInput{Vault: "Missing"}); err == nil {
		t.Error("listProperties accepted an unknown vault")
	}
}
