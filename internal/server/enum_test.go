package server

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestChoiceParametersAdvertiseTheirValues(t *testing.T) {
	s := newTestServer(t)
	want := map[string]map[string][]string{
		"grep_notes":   {"mode": {"auto", "words", "phrase", "regex"}, "output": {"lines", "files"}},
		"list_notes":   {"order": {"asc", "desc"}},
		"edit_section": {"mode": {"append", "prepend", "replace"}},
	}
	for _, tool := range listedTools(t, s) {
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
				Enum        []string `json:"enum"`
				Description string   `json:"description"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatal(err)
		}
		for param, values := range params {
			p := schema.Properties[param]
			if !slices.Equal(p.Enum, values) || p.Description == "" {
				t.Errorf("%s.%s: enum %q, description %q; want %q and the description kept", tool.Name, param, p.Enum, p.Description, values)
			}
		}
		delete(want, tool.Name)
	}
	if len(want) != 0 {
		t.Errorf("tools not listed: %v", want)
	}
}

func TestServerRejectsAValueOutsideTheChoices(t *testing.T) {
	s := rankServer(t, map[string]string{"Inbox/a.md": "bike\n"})
	session := testSession(t, s)

	msg := toolError(t, session, "grep_notes", map[string]any{"query": "bike", "mode": "fuzzy"})
	if !strings.Contains(msg, "mode") {
		t.Errorf("error %q should name the mode parameter", msg)
	}
}

func TestSearchAcceptsAutoByName(t *testing.T) {
	s := rankServer(t, map[string]string{"Inbox/a.md": "bike trail\n"})

	res := search1(t, s, grepNotesInput{Query: "bike trail", Mode: "auto"})
	if len(res.Files) != 1 {
		t.Errorf("Files = %+v, want mode auto to work like leaving mode out", res.Files)
	}
}

func TestPhraseHonoursCaseGlobAndContext(t *testing.T) {
	s := rankServer(t, map[string]string{
		"Inbox/a.md":   "before\nIdea [draft] 💡 (v2) — \"ok?\"\nafter\n",
		"Archive/b.md": "Idea [draft] 💡 (v2) — \"ok?\"\n",
		"Inbox/c.md":   "idea [draft] 💡 (v2) — \"ok?\"\n",
	})
	phrase := `Idea [draft] 💡 (v2) — "ok?"`

	res := search1(t, s, grepNotesInput{Query: phrase, Mode: "phrase", Glob: "Inbox/**", CaseSensitive: true, ContextLines: 1})
	if len(res.Files) != 1 || res.Files[0].Path != "Inbox/a.md" || len(res.Files[0].Lines) != 3 {
		t.Fatalf("Files = %+v, want only Inbox/a.md with its match and one line of context each side", res.Files)
	}

	res = search1(t, s, grepNotesInput{Query: phrase, Mode: "phrase", Glob: "Inbox/**"})
	if len(res.Files) != 2 {
		t.Errorf("Files = %+v, want both Inbox notes when case does not matter", res.Files)
	}
}
