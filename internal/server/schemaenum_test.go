package server

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestChoiceParametersAdvertiseTheirValues(t *testing.T) {
	s := rankServer(t, map[string]string{"Inbox/a.md": "bike\n"})
	session := testSession(t, s)
	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]map[string][]string{
		"search_notes": {"mode": {"auto", "words", "phrase", "regex"}, "output": {"lines", "files"}},
		"list_notes":   {"order": {"asc", "desc"}},
		"edit_section": {"mode": {"append", "prepend", "replace"}},
	}
	for _, tool := range tools.Tools {
		params, ok := want[tool.Name]
		if !ok {
			continue
		}
		raw, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatal(err)
		}
		var schema struct {
			Properties map[string]struct {
				Enum []string `json:"enum"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatal(err)
		}
		for param, values := range params {
			if got := schema.Properties[param].Enum; !slices.Equal(got, values) {
				t.Errorf("%s.%s enum = %q, want %q", tool.Name, param, got, values)
			}
		}
		delete(want, tool.Name)
	}
	if len(want) > 0 {
		t.Errorf("tools not listed: %v", want)
	}
}

func TestAnUnknownChoiceIsRejectedWithTheParameters(t *testing.T) {
	s := rankServer(t, map[string]string{"Inbox/a.md": "bike\n"})
	session := testSession(t, s)

	msg := toolError(t, session, "search_notes", map[string]any{"query": "bike", "mode": "exact"})
	if !strings.Contains(msg, "mode") || !strings.Contains(msg, "search_notes takes parameters") {
		t.Errorf("error = %q, want the bad mode named with the tool's parameters", msg)
	}
}
