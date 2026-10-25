package semantic

import (
	"cmp"
	"math"
	"path"
	"slices"
	"strings"
	"unicode"
)

// BM25's usual constants: k1 sets how quickly repeats of a word stop adding
// score, and b how strongly a long note is marked down for its length.
const (
	bm25K1 = 1.2
	bm25B  = 0.75
)

// termIndex holds each note's word counts for keyword ranking. It is built
// whole on every refresh and never changed after, so readers need no lock.
type termIndex struct {
	docs   []termDoc
	byPath map[string]int
	df     map[string]int
	avgLen float64
}

type termDoc struct {
	path   string
	chunks []Chunk
	tf     map[string]int
	length int
}

type scoredDoc struct {
	doc   int
	score float64
}

// tokens lowercases text and splits it into runs of letters and digits.
// It needs no word list, so it works for any language written with spaces.
func tokens(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

func newTermIndex(notes []noteChunks) *termIndex {
	ti := &termIndex{byPath: map[string]int{}, df: map[string]int{}}
	total := 0
	for _, n := range notes {
		d := termDoc{path: n.path, chunks: n.chunks, tf: map[string]int{}}
		words := tokens(noteTitle(n.path))
		for _, c := range n.chunks {
			words = append(words, tokens(c.Text)...)
		}
		for _, w := range words {
			d.tf[w]++
		}
		for w := range d.tf {
			ti.df[w]++
		}
		d.length = len(words)
		total += d.length
		ti.byPath[d.path] = len(ti.docs)
		ti.docs = append(ti.docs, d)
	}
	if len(ti.docs) > 0 {
		ti.avgLen = float64(total) / float64(len(ti.docs))
	}
	return ti
}

func noteTitle(p string) string {
	return strings.TrimSuffix(path.Base(p), path.Ext(p))
}

func (ti *termIndex) idf(term string) float64 {
	n, df := float64(len(ti.docs)), float64(ti.df[term])
	return math.Log(1 + (n-df+0.5)/(df+0.5))
}

// queryTerms returns the distinct words of query, without the words more
// than half the notes hold, which say little about any one note. When no
// word in between is left, it keeps them all.
func (ti *termIndex) queryTerms(query string) []string {
	var all, rare []string
	for _, w := range tokens(query) {
		if slices.Contains(all, w) {
			continue
		}
		all = append(all, w)
		if df := ti.df[w]; df > 0 && 2*df <= len(ti.docs) {
			rare = append(rare, w)
		}
	}
	if len(rare) == 0 {
		return all
	}
	return rare
}

// rank scores every note include accepts by BM25 over terms and returns the
// notes holding at least one term, best first.
func (ti *termIndex) rank(terms []string, include func(string) bool) []scoredDoc {
	var out []scoredDoc
	for i, d := range ti.docs {
		if include != nil && !include(d.path) {
			continue
		}
		var score float64
		norm := bm25K1 * (1 - bm25B + bm25B*float64(d.length)/ti.avgLen)
		for _, t := range terms {
			if tf := float64(d.tf[t]); tf > 0 {
				score += ti.idf(t) * tf * (bm25K1 + 1) / (tf + norm)
			}
		}
		if score > 0 {
			out = append(out, scoredDoc{doc: i, score: score})
		}
	}
	slices.SortFunc(out, func(a, b scoredDoc) int {
		if c := cmp.Compare(b.score, a.score); c != 0 {
			return c
		}
		return strings.Compare(ti.docs[a.doc].path, ti.docs[b.doc].path)
	})
	return out
}

// bestChunk returns the index of the note's chunk holding the rarest of
// terms, the first chunk when none holds any, or -1 for a note with no
// chunks.
func (ti *termIndex) bestChunk(doc int, terms []string) int {
	chunks := ti.docs[doc].chunks
	if len(chunks) == 0 {
		return -1
	}
	best, bestScore := 0, 0.0
	for i, c := range chunks {
		words := tokens(c.Text)
		var score float64
		for _, t := range terms {
			if slices.Contains(words, t) {
				score += ti.idf(t)
			}
		}
		if score > bestScore {
			best, bestScore = i, score
		}
	}
	return best
}
