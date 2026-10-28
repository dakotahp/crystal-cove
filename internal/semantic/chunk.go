// Package semantic finds notes by meaning and by key words. It splits notes
// into chunks at their headings, turns each chunk into a vector with a local
// embedding model, and compares vectors to answer a query or to find related
// notes. Search merges that ranking with keyword, phrase and name rankings.
package semantic

import (
	"bytes"
	"hash/fnv"
	"strings"
	"unicode/utf8"

	"github.com/dakotahp/crystal-cove/internal/notes"
	"github.com/dakotahp/crystal-cove/internal/vault"
)

const (
	// maxChunkChars keeps a chunk well inside the model's context window.
	maxChunkChars = 1500
	// minChunkChars is the size below which a chunk joins its neighbor, so
	// a note with a heading on every line is not split into fragments.
	minChunkChars = 400
	// minBoilerplateNotes is the fewest notes a line must appear in before
	// it counts as template text.
	minBoilerplateNotes = 10
)

// Chunk is a stretch of one note that is embedded as one vector.
type Chunk struct {
	// HeadingPath is the heading hierarchy the chunk starts under, empty
	// for text before the note's first heading.
	HeadingPath []string
	Text        string
	// pieces are the sections joined into Text, when there is more than
	// one, so a search can point at the one that matched.
	pieces []piece
}

type piece struct {
	HeadingPath []string
	Text        string
}

// chunkNote splits a note at its headings, drops lines in boilerplate, shows
// links as their text, cuts sections longer than maxChunkChars at paragraph
// breaks, and joins pieces shorter than minChunkChars with their neighbor.
func chunkNote(source []byte, boilerplate map[uint64]bool) []Chunk {
	var pieces []piece
	for _, p := range vault.Passages(source) {
		for _, text := range splitText(notes.LinkText(stripLines(p.Text, boilerplate))) {
			pieces = append(pieces, piece{HeadingPath: p.HeadingPath, Text: text})
		}
	}
	var out []Chunk
	for _, p := range pieces {
		if n := len(out); n > 0 {
			last := &out[n-1]
			small := len(last.Text) < minChunkChars || len(p.Text) < minChunkChars
			if small && len(last.Text)+2+len(p.Text) <= maxChunkChars {
				if last.pieces == nil {
					last.pieces = []piece{{HeadingPath: last.HeadingPath, Text: last.Text}}
				}
				last.pieces = append(last.pieces, p)
				last.Text += "\n\n" + p.Text
				continue
			}
		}
		out = append(out, Chunk{HeadingPath: p.HeadingPath, Text: p.Text})
	}
	return out
}

func stripLines(text string, boilerplate map[uint64]bool) string {
	if len(boilerplate) == 0 {
		return text
	}
	lines := strings.SplitAfter(text, "\n")
	kept := lines[:0]
	for _, line := range lines {
		if !boilerplate[lineKey(line)] {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "")
}

// splitText cuts text into pieces of at most maxChunkChars, at paragraph
// breaks where it can, and leaves out empty pieces.
func splitText(text string) []string {
	var out []string
	current := ""
	flush := func() {
		if t := strings.TrimSpace(current); t != "" {
			out = append(out, t)
		}
		current = ""
	}
	for _, para := range strings.Split(text, "\n\n") {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		if current != "" && len(current)+2+len(para) > maxChunkChars {
			flush()
		}
		for len(para) > maxChunkChars {
			flush()
			cut := maxChunkChars
			for !utf8.RuneStart(para[cut]) {
				cut--
			}
			out = append(out, para[:cut])
			para = para[cut:]
		}
		if current != "" {
			current += "\n\n"
		}
		current += para
	}
	flush()
	return out
}

func lineKey(line string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(strings.TrimSpace(line)))
	return h.Sum64()
}

// lineCounts counts, for each distinct line, how many notes hold it.
type lineCounts map[uint64]int

func (c *lineCounts) add(source []byte) {
	if *c == nil {
		*c = lineCounts{}
	}
	seen := map[uint64]bool{}
	for line := range bytes.SplitSeq(source, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		key := lineKey(string(line))
		if !seen[key] {
			seen[key] = true
			(*c)[key]++
		}
	}
}

// boilerplate returns the lines that appear in enough of a vault's notes to
// be template text rather than writing, such as a daily-note header.
func (c lineCounts) boilerplate(notes int) map[uint64]bool {
	threshold := boilerplateThreshold(notes)
	out := map[uint64]bool{}
	for key, n := range c {
		if n >= threshold {
			out[key] = true
		}
	}
	return out
}

func boilerplateThreshold(notes int) int {
	return max(minBoilerplateNotes, notes/100)
}
