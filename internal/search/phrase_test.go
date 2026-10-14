package search

import (
	"context"
	"slices"
	"testing"
)

func phraseVault(t *testing.T) string {
	return wordsVault(t, map[string]string{
		"exact.md":     "- Dropped \"one line for tomorrow\" (again?) — everywhere\n",
		"scattered.md": "one line\nfor\ntomorrow\n",
		"reordered.md": "tomorrow, one line for\n",
	})
}

func TestPhraseModeMatchesLiteralTextWithPunctuation(t *testing.T) {
	s := requireRipgrep(t)
	root := phraseVault(t)

	res, err := s.Search(context.Background(), root, Options{Query: `line for tomorrow" (again?) — every`, Mode: ModePhrase})
	if err != nil {
		t.Fatal(err)
	}
	if got := paths(res); !slices.Equal(got, []string{"exact.md"}) {
		t.Errorf("matched %q, want only the note holding the exact text", got)
	}
}

func TestPhraseModeIgnoresCaseByDefault(t *testing.T) {
	s := requireRipgrep(t)
	root := phraseVault(t)

	res, err := s.Search(context.Background(), root, Options{Query: "ONE LINE FOR TOMORROW", Mode: ModePhrase})
	if err != nil {
		t.Fatal(err)
	}
	if got := paths(res); !slices.Equal(got, []string{"exact.md"}) {
		t.Errorf("matched %q, want the phrase found regardless of case", got)
	}
}

func TestQuotedQueryIsAPhrase(t *testing.T) {
	s := requireRipgrep(t)
	root := phraseVault(t)

	res, err := s.Search(context.Background(), root, Options{Query: `"one line for tomorrow"`})
	if err != nil {
		t.Fatal(err)
	}
	if got := paths(res); !slices.Equal(got, []string{"exact.md"}) {
		t.Errorf("matched %q, want a quoted query read as one phrase", got)
	}
}

func TestQueryPhrase(t *testing.T) {
	cases := []struct {
		opts   Options
		phrase string
		ok     bool
	}{
		{Options{Query: "a (b)?", Mode: ModePhrase}, "a (b)?", true},
		{Options{Query: `"a b"`}, "a b", true},
		{Options{Query: `  "a? b"  `}, "a? b", true},
		{Options{Query: `"a b"`, Mode: ModeWords}, "", false},
		{Options{Query: `"a b"`, Mode: ModeRegex}, "", false},
		{Options{Query: `""`}, "", false},
		{Options{Query: `"a b`}, "", false},
		{Options{Query: "a b"}, "", false},
	}
	for _, c := range cases {
		phrase, ok := QueryPhrase(c.opts)
		if phrase != c.phrase || ok != c.ok {
			t.Errorf("QueryPhrase(%+v) = %q, %v; want %q, %v", c.opts, phrase, ok, c.phrase, c.ok)
		}
	}
}

func TestPhraseQueriesHaveNoWords(t *testing.T) {
	for _, opts := range []Options{
		{Query: "one line for tomorrow", Mode: ModePhrase},
		{Query: `"one line for tomorrow"`},
	} {
		if words := QueryWords(opts); words != nil {
			t.Errorf("QueryWords(%+v) = %q, want none for a phrase", opts, words)
		}
	}
}
