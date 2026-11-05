package server

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func outlineOver(t *testing.T, session *mcp.ClientSession, args map[string]any) getOutlineOutput {
	t.Helper()
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_outline", Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("get_outline: %+v", res.Content[0])
	}
	data, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var out getOutlineOutput
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestOutlineHeadingPathOpensTheSection(t *testing.T) {
	s, _ := metaServer(t, map[string]string{
		"plan.md": "---\ntitle: x\n---\n# Plan\n## Tasks\nship it\n## Log\n",
	})
	session := testSession(t, s)

	out := outlineOver(t, session, map[string]any{"path": "plan.md"})
	if out.Total != 3 || out.NextOffset != -1 || len(out.Headings) != 3 {
		t.Fatalf("outline = %+v", out)
	}
	tasks := out.Headings[1]
	if tasks.Level != 2 || !slices.Equal(tasks.HeadingPath, []string{"Plan", "Tasks"}) {
		t.Fatalf("second heading = %+v", tasks)
	}

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_section",
		Arguments: map[string]any{"path": "plan.md", "heading_path": tasks.HeadingPath},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError || !strings.Contains(res.Content[0].(*mcp.TextContent).Text, "ship it") {
		t.Fatalf("get_section = %+v", res.Content[0])
	}
}

func TestOutlinePages(t *testing.T) {
	var note strings.Builder
	for i := range 5 {
		fmt.Fprintf(&note, "# H%d\n", i)
	}
	s, _ := metaServer(t, map[string]string{"long.md": note.String()})
	session := testSession(t, s)

	first := outlineOver(t, session, map[string]any{"path": "long.md", "limit": 2})
	if first.Total != 5 || first.NextOffset != 2 || len(first.Headings) != 2 {
		t.Fatalf("first page = %+v", first)
	}
	last := outlineOver(t, session, map[string]any{"path": "long.md", "offset": 4, "limit": 2})
	if last.NextOffset != -1 || len(last.Headings) != 1 || !slices.Equal(last.Headings[0].HeadingPath, []string{"H4"}) {
		t.Fatalf("last page = %+v", last)
	}

	got := toolError(t, session, "get_outline", map[string]any{"path": "long.md", "offset": 9})
	if !strings.Contains(got, "out of range") {
		t.Errorf("error = %q", got)
	}
}

func TestOutlineOfANoteWithoutHeadingsReturnsAnEmptyList(t *testing.T) {
	s, _ := metaServer(t, map[string]string{"plain.md": "just text\n"})

	out := outlineOver(t, testSession(t, s), map[string]any{"path": "plain.md"})
	if out.Headings == nil || len(out.Headings) != 0 || out.Total != 0 || out.NextOffset != -1 {
		t.Errorf("outline = %+v", out)
	}
}

func TestOutlineOfAMissingNoteFails(t *testing.T) {
	s, _ := metaServer(t, map[string]string{"plain.md": "x\n"})

	got := toolError(t, testSession(t, s), "get_outline", map[string]any{"path": "missing.md"})
	if !strings.Contains(got, "missing.md") {
		t.Errorf("error = %q", got)
	}
}
