package vault

import (
	"reflect"
	"testing"
)

func TestPassages(t *testing.T) {
	cases := []struct {
		name, note string
		want       []Passage
	}{
		{"empty", "", nil},
		{"no headings", "just text\n", []Passage{{Text: "just text\n"}}},
		{"frontmatter only", "---\ntags: [a]\n---\n", nil},
		{"frontmatter skipped", "---\ntitle: x\n---\nintro\n# One\nbody\n", []Passage{
			{Text: "intro\n"},
			{HeadingPath: []string{"One"}, Text: "body\n"},
		}},
		{"nested keeps own text", "# Project\nlead\n## Tasks\none\n## Done\ntwo\n", []Passage{
			{HeadingPath: []string{"Project"}, Text: "lead\n"},
			{HeadingPath: []string{"Project", "Tasks"}, Text: "one\n"},
			{HeadingPath: []string{"Project", "Done"}, Text: "two\n"},
		}},
		{"heading without text", "# Empty\n# Full\ntext", []Passage{
			{HeadingPath: []string{"Empty"}, Text: ""},
			{HeadingPath: []string{"Full"}, Text: "text"},
		}},
		{"code fence is not a heading", "# Real\n```\n# fake\n```\n", []Passage{
			{HeadingPath: []string{"Real"}, Text: "```\n# fake\n```\n"},
		}},
		{"setext child", "Title\n=====\nintro\n\nSub\n---\nbody\n", []Passage{
			{HeadingPath: []string{"Title"}, Text: "intro\n\n"},
			{HeadingPath: []string{"Title", "Sub"}, Text: "body\n"},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Passages([]byte(tc.note))
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Passages(%q)\n got %#v\nwant %#v", tc.note, got, tc.want)
			}
		})
	}
}
