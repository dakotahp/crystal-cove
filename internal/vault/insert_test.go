package vault

import "testing"

func TestAppending(t *testing.T) {
	cases := []struct {
		existing, content string
		inline            bool
		want              string
	}{
		{"one", "two", false, "one\ntwo"},
		{"one\n", "two\n", false, "one\ntwo\n"},
		{"one\r\ntwo", "three", false, "one\r\ntwo\r\nthree"},
		{"", "two", false, "two"},
		{"one", "", false, "one"},
		{"- [ ] task", " (soon)", true, "- [ ] task (soon)"},
		{"one\n", "two", true, "one\ntwo"},
	}
	for _, c := range cases {
		got, err := Appending(c.content, c.inline)([]byte(c.existing))
		if err != nil || string(got) != c.want {
			t.Errorf("Appending(%q, %v) to %q = %q, %v; want %q", c.content, c.inline, c.existing, got, err, c.want)
		}
	}
}

func TestPrepending(t *testing.T) {
	cases := []struct {
		existing, content string
		inline            bool
		want              string
	}{
		{"body\n", "first", false, "first\nbody\n"},
		{"---\ntags: [a]\n---\nbody\n", "first", false, "---\ntags: [a]\n---\nfirst\nbody\n"},
		{"---\r\ntags: [a]\r\n---\r\nbody\r\n", "first", false, "---\r\ntags: [a]\r\n---\r\nfirst\r\nbody\r\n"},
		{"---\n---\nbody", "first", false, "---\n---\nfirst\nbody"},
		{"---\ntags: [a]\n---", "first", false, "---\ntags: [a]\n---\nfirst\n"},
		{"---\nnot closed\nbody", "first", false, "first\n---\nnot closed\nbody"},
		{"--- \nbody", "first", false, "first\n--- \nbody"},
		{"", "first", false, "first\n"},
		{"body", "", false, "body"},
		{"Title: body", "Note ", true, "Note Title: body"},
	}
	for _, c := range cases {
		got, err := Prepending(c.content, c.inline)([]byte(c.existing))
		if err != nil || string(got) != c.want {
			t.Errorf("Prepending(%q, %v) to %q = %q, %v; want %q", c.content, c.inline, c.existing, got, err, c.want)
		}
	}
}
