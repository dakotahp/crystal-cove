package search

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestLongLinesAreClippedAroundTheMatch(t *testing.T) {
	s := requireRipgrep(t)
	line := strings.Repeat("é filler ", 300) + "needle" + strings.Repeat(" tail", 300)
	root := wordsVault(t, map[string]string{"long.md": line + "\nshort needle\n"})

	res, err := s.Search(context.Background(), root, Options{Query: "needle"})
	if err != nil {
		t.Fatal(err)
	}
	got := res.Files[0].Lines
	if len(got) != 2 {
		t.Fatalf("Lines = %+v, want two", got)
	}
	long := got[0]
	if !long.TextTruncated || !strings.Contains(long.Text, "needle") ||
		!strings.HasPrefix(long.Text, "…") || !strings.HasSuffix(long.Text, "…") {
		t.Errorf("long line = %q (truncated %v), want a clipped window holding the match", long.Text, long.TextTruncated)
	}
	if n := utf8.RuneCountInString(long.Text); n > MaxLineChars+2 {
		t.Errorf("long line has %d characters, want at most %d plus the markers", n, MaxLineChars)
	}
	if short := got[1]; short.TextTruncated || short.Text != "short needle" {
		t.Errorf("short line = %+v, want it whole", short)
	}
}

func TestClipLine(t *testing.T) {
	long := strings.Repeat("x", MaxLineChars*2)
	cases := []struct {
		name      string
		text      string
		at        int
		prefix    bool
		suffix    bool
		truncated bool
	}{
		{"fits", "short", 0, false, false, false},
		{"match at start", long, 0, false, true, true},
		{"match at end", long, len(long) - 1, true, false, true},
		{"match in middle", long, len(long) / 2, true, true, true},
		{"offset past the end", long, len(long) + 10, true, false, true},
	}
	for _, c := range cases {
		got, truncated := clipLine(c.text, c.at)
		if truncated != c.truncated || strings.HasPrefix(got, "…") != c.prefix || strings.HasSuffix(got, "…") != c.suffix {
			t.Errorf("%s: clipLine = %q, %v", c.name, got, truncated)
		}
	}
}
