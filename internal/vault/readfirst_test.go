package vault

import (
	"strings"
	"testing"
)

func TestReadFirst(t *testing.T) {
	v := newTestVault(t)
	mustWrite(t, v, "long.md", strings.Repeat("é", ReadPageSize+100))
	mustWrite(t, v, "short.md", "hello")
	mustWrite(t, v, "empty.md", "")

	for _, tc := range []struct {
		path      string
		limit     int
		content   int
		truncated bool
		next      int
	}{
		{"long.md", ReadPageSize * 2, ReadPageSize, true, ReadPageSize},
		{"long.md", 300, 300, true, 300},
		{"long.md", 0, 0, true, 0},
		{"short.md", 300, 5, false, -1},
		{"short.md", 0, 0, true, 0},
		{"empty.md", 0, 0, false, -1},
	} {
		got, err := v.ReadFirst(tc.path, tc.limit)
		if err != nil {
			t.Fatalf("%+v: %v", tc, err)
		}
		if len([]rune(got.Content)) != tc.content || got.Truncated != tc.truncated || got.NextOffset != tc.next || got.Version == "" {
			t.Errorf("%+v: got %d characters, truncated %v, next %d, version %q", tc, len([]rune(got.Content)), got.Truncated, got.NextOffset, got.Version)
		}
	}
	if _, err := v.ReadFirst("missing.md", 10); err == nil {
		t.Error("read a missing note")
	}
}

func TestReadFirstFuncSizesThePageFromTheContent(t *testing.T) {
	v := newTestVault(t)
	mustWrite(t, v, "a.md", "head|rest of the note")

	got, err := v.ReadFirstFunc("a.md", func(data []byte) int { return strings.IndexByte(string(data), '|') + 1 })
	if err != nil {
		t.Fatal(err)
	}
	if got.Content != "head|" || got.NextOffset != 5 {
		t.Errorf("got %q, next %d", got.Content, got.NextOffset)
	}
	if _, err := v.ReadFirstFunc("missing.md", func([]byte) int { return 1 }); err == nil {
		t.Error("read a missing note")
	}
}
