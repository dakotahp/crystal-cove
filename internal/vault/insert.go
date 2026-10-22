package vault

import (
	"bytes"
	"slices"
)

// Appending returns a change, for Update or Upsert, that adds content at the
// end of a note. Unless inline is set, content starts on a new line: when
// the note does not end with a line break, one is added first, in the note's
// own line-ending style.
func Appending(content string, inline bool) func(old []byte) ([]byte, error) {
	return func(old []byte) ([]byte, error) {
		sep := ""
		if !inline && len(old) > 0 && content != "" && old[len(old)-1] != '\n' {
			sep = lineBreak(old)
		}
		return slices.Concat(old, []byte(sep), []byte(content)), nil
	}
}

// Prepending returns a change that adds content at the start of a note's
// body, below its frontmatter. Unless inline is set, a line break follows
// content, so the old first line keeps its own line.
func Prepending(content string, inline bool) func(old []byte) ([]byte, error) {
	return func(old []byte) ([]byte, error) {
		if content == "" {
			return old, nil
		}
		at := bodyStart(old)
		lead := ""
		if at == len(old) && at > 0 && old[at-1] != '\n' {
			lead = lineBreak(old)
		}
		tail := ""
		if !inline {
			tail = lineBreak(old)
		}
		return slices.Concat(old[:at], []byte(lead+content+tail), old[at:]), nil
	}
}

// bodyStart returns the byte offset just past a note's frontmatter, or 0
// when it has none. Frontmatter opens with a --- line and closes at the next
// --- line, as in Obsidian; either line may end in \r\n.
func bodyStart(data []byte) int {
	lines := bytes.SplitAfter(data, []byte("\n"))
	if len(lines) < 2 || !isFence(lines[0]) {
		return 0
	}
	at := len(lines[0])
	for _, line := range lines[1:] {
		at += len(line)
		if isFence(line) {
			return at
		}
	}
	return 0
}

func isFence(line []byte) bool {
	return string(bytes.TrimRight(line, "\r\n")) == "---"
}

func lineBreak(note []byte) string {
	if bytes.Contains(note, []byte("\r\n")) {
		return "\r\n"
	}
	return "\n"
}
