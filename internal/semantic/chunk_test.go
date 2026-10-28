package semantic

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestChunkNote(t *testing.T) {
	long := strings.Repeat("word ", 100)
	cases := []struct {
		name, note string
		want       []Chunk
	}{
		{"empty", "", nil},
		{"headings only", "# A\n# B\n", nil},
		{"small sections join", "# A\none\n# B\ntwo\n", []Chunk{
			{HeadingPath: []string{"A"}, Text: "one\n\ntwo", pieces: []piece{
				{HeadingPath: []string{"A"}, Text: "one"},
				{HeadingPath: []string{"B"}, Text: "two"},
			}},
		}},
		{"nested heading path", "# A\n## B\n" + long + "\n", []Chunk{
			{HeadingPath: []string{"A", "B"}, Text: strings.TrimSpace(long)},
		}},
		{"large sections stay apart", "# A\n" + long + long + "\n# B\n" + long + long + "\n", []Chunk{
			{HeadingPath: []string{"A"}, Text: strings.TrimSpace(long + long)},
			{HeadingPath: []string{"B"}, Text: strings.TrimSpace(long + long)},
		}},
		{"lead text before headings", "intro\n# A\nbody\n", []Chunk{
			{Text: "intro\n\nbody", pieces: []piece{{Text: "intro"}, {HeadingPath: []string{"A"}, Text: "body"}}},
		}},
		{"links shown as text", "# A\n[[Ch 1|← Chapter 1]] | [[Book]]\n", []Chunk{
			{HeadingPath: []string{"A"}, Text: "← Chapter 1 | Book"},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := chunkNote([]byte(tc.note), nil)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("chunkNote(%q)\n got %#v\nwant %#v", tc.note, got, tc.want)
			}
		})
	}
}

func TestChunkNoteSplitsLongSections(t *testing.T) {
	para := strings.Repeat("a", 900)
	note := "# A\n" + para + "\n\n" + para + "\n\n" + para + "\n"
	got := chunkNote([]byte(note), nil)
	if len(got) != 3 {
		t.Fatalf("got %d chunks, want 3: %#v", len(got), got)
	}
	for _, c := range got {
		if !reflect.DeepEqual(c.HeadingPath, []string{"A"}) || c.Text != para {
			t.Errorf("chunk = %q / %d chars, want heading A and one paragraph", c.HeadingPath, len(c.Text))
		}
	}
}

func TestChunkNoteCutsAnOverlongParagraphOnRuneBoundaries(t *testing.T) {
	note := strings.Repeat("é", maxChunkChars)
	got := chunkNote([]byte(note), nil)
	if len(got) < 2 {
		t.Fatalf("got %d chunks, want the paragraph cut", len(got))
	}
	for _, c := range got {
		if len(c.Text) > maxChunkChars || !utf8.ValidString(c.Text) {
			t.Errorf("chunk of %d bytes, valid UTF-8 %v", len(c.Text), utf8.ValidString(c.Text))
		}
	}
}

func TestChunkNoteDropsBoilerplate(t *testing.T) {
	note := "> **Quote of the Day**\nBe kind.\n\n# Log\n> **Quote of the Day**\nreal entry\n"
	boiler := map[uint64]bool{lineKey("> **Quote of the Day**"): true}
	got := chunkNote([]byte(note), boiler)
	want := []Chunk{{Text: "Be kind.\n\nreal entry", pieces: []piece{{Text: "Be kind."}, {HeadingPath: []string{"Log"}, Text: "real entry"}}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestBoilerplate(t *testing.T) {
	notes := make([][]byte, 0, 12)
	for i := range 12 {
		body := "## Template heading line\n> template\nunique " + strings.Repeat("x", i) + "\n"
		if i < 3 {
			body += "shared by few\n"
		}
		notes = append(notes, []byte(body))
	}
	var lines lineCounts
	for _, n := range notes {
		lines.add(n)
	}
	boiler := lines.boilerplate(len(notes))
	if !boiler[lineKey("> template")] || !boiler[lineKey("## Template heading line")] {
		t.Errorf("lines in every note are not boilerplate: %v", boiler)
	}
	if boiler[lineKey("shared by few")] || boiler[lineKey("unique x")] {
		t.Errorf("rare lines marked boilerplate")
	}
}

func TestBoilerplateThresholdGrowsWithVault(t *testing.T) {
	if got := boilerplateThreshold(100); got != minBoilerplateNotes {
		t.Errorf("small vault threshold = %d, want %d", got, minBoilerplateNotes)
	}
	if got := boilerplateThreshold(5000); got != 50 {
		t.Errorf("5000-note threshold = %d, want 50", got)
	}
}

func TestLineKeyIgnoresSurroundingSpace(t *testing.T) {
	if lineKey("  text \t") != lineKey("text") {
		t.Error("surrounding space changed the key")
	}
}
