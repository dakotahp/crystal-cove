package notes

import "testing"

func changeTags(t *testing.T, doc string, add, remove []string) string {
	t.Helper()
	got, err := ChangeTags([]byte(doc), add, remove)
	if err != nil {
		t.Fatal(err)
	}
	return string(got)
}

func TestChangeTagsMergesIntoAFlowList(t *testing.T) {
	doc := "---\ntitle: Plan\ntags: [a, b]\nstatus: active\n---\n\nBody #inline\n"

	got := changeTags(t, doc, []string{"b", "#c"}, nil)
	want := "---\ntitle: Plan\ntags: [a, b, c]\nstatus: active\n---\n\nBody #inline\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestChangeTagsMergesIntoABlockList(t *testing.T) {
	doc := "---\ntags:\n  - a\n  - b\n---\nbody\n"

	got := changeTags(t, doc, []string{"c", "A"}, []string{"b"})
	want := "---\ntags:\n  - a\n  - c\n---\nbody\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestChangeTagsTurnsAStringIntoAList(t *testing.T) {
	for _, doc := range []string{"---\ntags: a, b\n---\nbody\n", "---\ntags: a b\n---\nbody\n", "---\ntags: \"#a, b\"\n---\nbody\n"} {
		got := changeTags(t, doc, []string{"c"}, nil)
		want := "---\ntags:\n  - a\n  - b\n  - c\n---\nbody\n"
		if got != want {
			t.Errorf("from %q got %q, want %q", doc, got, want)
		}
	}
}

func TestChangeTagsCreatesTheField(t *testing.T) {
	cases := map[string]string{
		"# No block\n":               "---\ntags:\n  - new\n---\n# No block\n",
		"---\ntitle: x\n---\nbody\n": "---\ntitle: x\ntags:\n  - new\n---\nbody\n",
		"---\ntags:\n---\nbody\n":    "---\ntags:\n  - new\n---\nbody\n",
		"---\ntags: []\n---\nbody\n": "---\ntags: [new]\n---\nbody\n",
	}
	for doc, want := range cases {
		if got := changeTags(t, doc, []string{"new"}, nil); got != want {
			t.Errorf("from %q got %q, want %q", doc, got, want)
		}
	}
}

func TestChangeTagsDropsTheFieldWhenTheLastTagGoes(t *testing.T) {
	got := changeTags(t, "---\ntitle: x\ntags: [a]\n---\nbody\n", nil, []string{"#a"})
	if want := "---\ntitle: x\n---\nbody\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestChangeTagsLeavesTheFileAloneWhenNothingChanges(t *testing.T) {
	doc := "---\ntags:   [a,b]   # mine\n---\nbody\n"
	if got := changeTags(t, doc, []string{"a", ""}, []string{"zzz"}); got != doc {
		t.Errorf("got %q, want the file byte for byte", got)
	}
	if got := changeTags(t, "plain\n", nil, []string{"a"}); got != "plain\n" {
		t.Errorf("got %q, want the file unchanged", got)
	}
}

func TestChangeTagsRejectsBrokenYAMLAndOddTags(t *testing.T) {
	if _, err := ChangeTags([]byte("---\ntags: [unclosed\n---\n"), []string{"a"}, nil); err == nil {
		t.Error("ChangeTags accepted malformed frontmatter")
	}
	if _, err := ChangeTags([]byte("---\ntags:\n  key: value\n---\n"), []string{"a"}, nil); err == nil {
		t.Error("ChangeTags accepted a tags field that is a mapping")
	}
}
