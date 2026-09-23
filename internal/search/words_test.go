package search

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func wordsVault(t *testing.T, notes map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for p, content := range notes {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func requireRipgrep(t *testing.T) *Searcher {
	t.Helper()
	if _, err := os.Stat("/opt/homebrew/bin/rg"); err != nil {
		if _, err := os.Stat("/usr/bin/rg"); err != nil {
			t.Skip("ripgrep is not installed")
		}
	}
	return New("rg", nil)
}

func paths(res *Result) []string {
	var out []string
	for _, f := range res.Files {
		out = append(out, f.Path)
	}
	return out
}

func TestWordsModeIgnoresWordOrder(t *testing.T) {
	s := requireRipgrep(t)
	root := wordsVault(t, map[string]string{
		"a.md": "weekly planning for the bike\n",
		"b.md": "planning only\n",
	})

	for _, query := range []string{"bike planning", "planning bike"} {
		res, err := s.Search(context.Background(), root, Options{Query: query})
		if err != nil {
			t.Fatal(err)
		}
		if got := paths(res); len(got) != 1 || got[0] != "a.md" {
			t.Errorf("query %q matched %q, want only the note holding both words", query, got)
		}
	}
}

func TestWordsModeNeedsEveryWord(t *testing.T) {
	s := requireRipgrep(t)
	root := wordsVault(t, map[string]string{
		"a.md": "bike maintenance notes\n",
		"b.md": "bike only\n",
	})

	res, err := s.Search(context.Background(), root, Options{Query: "bike maintenance"})
	if err != nil {
		t.Fatal(err)
	}
	if got := paths(res); len(got) != 1 || got[0] != "a.md" {
		t.Errorf("matched %q, want only the note holding both words", got)
	}
}

func TestWordsModeMatchesAcrossLines(t *testing.T) {
	s := requireRipgrep(t)
	root := wordsVault(t, map[string]string{"a.md": "bike\n\nmaintenance\n"})

	res, err := s.Search(context.Background(), root, Options{Query: "bike maintenance"})
	if err != nil {
		t.Fatal(err)
	}
	if got := paths(res); len(got) != 1 {
		t.Errorf("matched %q, want the note whose words sit on separate lines", got)
	}
}

func TestRegexQueriesStayRegex(t *testing.T) {
	s := requireRipgrep(t)
	root := wordsVault(t, map[string]string{
		"a.md": "---\nagent-context: vault\n---\n\nbody\n",
		"b.md": "mentions agent-context in passing\n",
	})

	res, err := s.Search(context.Background(), root, Options{Query: "^agent-context:"})
	if err != nil {
		t.Fatal(err)
	}
	if got := paths(res); len(got) != 1 || got[0] != "a.md" {
		t.Errorf("matched %q, want the anchored regex to hold", got)
	}
}

func TestRegexModeForcesASingleWordQuery(t *testing.T) {
	s := requireRipgrep(t)
	root := wordsVault(t, map[string]string{"a.md": "bike maintenance\n"})

	res, err := s.Search(context.Background(), root, Options{Query: "bike maintenance", Mode: ModeRegex})
	if err != nil {
		t.Fatal(err)
	}
	if got := paths(res); len(got) != 1 {
		t.Errorf("matched %q, want the phrase to match as written", got)
	}

	res, err = s.Search(context.Background(), root, Options{Query: "maintenance bike", Mode: ModeRegex})
	if err != nil {
		t.Fatal(err)
	}
	if got := paths(res); len(got) != 0 {
		t.Errorf("matched %q, want no match for the reversed phrase in regex mode", got)
	}
}

func TestWordsModeCanBeForcedOnARegexLookingQuery(t *testing.T) {
	s := requireRipgrep(t)
	root := wordsVault(t, map[string]string{"a.md": "costs $5 and $7\n"})

	res, err := s.Search(context.Background(), root, Options{Query: "$5 $7", Mode: ModeWords})
	if err != nil {
		t.Fatal(err)
	}
	if got := paths(res); len(got) != 1 {
		t.Errorf("matched %q, want the literal words found", got)
	}
}

func TestMaxLinesPerNoteCapsOneNote(t *testing.T) {
	s := requireRipgrep(t)
	root := wordsVault(t, map[string]string{
		"a.md": "bike\nbike\nbike\nbike\nbike\n",
		"b.md": "bike\n",
	})

	res, err := s.Search(context.Background(), root, Options{Query: "bike", MaxLinesPerFile: 2})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range res.Files {
		if len(f.Lines) > 2 {
			t.Errorf("%s returned %d lines, want at most 2", f.Path, len(f.Lines))
		}
	}
	if len(res.Files) != 2 {
		t.Errorf("files = %q, want both notes kept", paths(res))
	}
}
