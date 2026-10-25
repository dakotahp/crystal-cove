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
// score, and b how strongly a long section is marked down for its length.
const (
	bm25K1 = 1.2
	bm25B  = 0.75
)

// termIndex scores key words per section, not per note: a long note with
// one section on the query would otherwise be marked down for the length
// of everything else in it. A note scores as its best section. The index is
// built whole on every refresh and never changed after, so readers need no
// lock.
type termIndex struct {
	docs     []termDoc
	byPath   map[string]int
	chunkDoc []int
	chunkLen []int
	postings map[string][]posting
	avgLen   float64
}

type termDoc struct {
	path   string
	chunks []Chunk
	// first is the index of the note's first section across the vault.
	first int
}

type posting struct {
	chunk int
	tf    int
}

// scoredDoc is a note's keyword score and the section, within the note,
// that earned it; chunk is -1 for a note matched by name with no text.
// coverage is the share of the query's word weight (summed idf) that the
// section holds: 1 when it holds every term, small when it holds only the
// commonest.
type scoredDoc struct {
	doc      int
	score    float64
	chunk    int
	coverage float64
}

// tokens lowercases text and splits it into runs of letters and digits.
// It needs no word list, so it works for any language written with spaces.
func tokens(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// newTermIndex counts every section's words and its headings' words, with
// the note's name counted in each section so a name match reaches the whole
// note. A note with no
// text gets one empty section, so it can still be found by name.
func newTermIndex(notes []noteChunks) *termIndex {
	ti := &termIndex{byPath: map[string]int{}, postings: map[string][]posting{}}
	total := 0
	for _, n := range notes {
		d := termDoc{path: n.path, chunks: n.chunks, first: len(ti.chunkDoc)}
		title := tokens(noteTitle(n.path))
		texts := make([]string, max(1, len(n.chunks)))
		for i, c := range n.chunks {
			texts[i] = strings.Join(c.HeadingPath, " ") + " " + c.Text
		}
		for _, text := range texts {
			chunk := len(ti.chunkDoc)
			words := append(slices.Clone(title), tokens(text)...)
			tf := map[string]int{}
			for _, w := range words {
				tf[w]++
			}
			for w, n := range tf {
				ti.postings[w] = append(ti.postings[w], posting{chunk: chunk, tf: n})
			}
			ti.chunkDoc = append(ti.chunkDoc, len(ti.docs))
			ti.chunkLen = append(ti.chunkLen, len(words))
			total += len(words)
		}
		ti.byPath[d.path] = len(ti.docs)
		ti.docs = append(ti.docs, d)
	}
	if len(ti.chunkLen) > 0 {
		ti.avgLen = float64(total) / float64(len(ti.chunkLen))
	}
	return ti
}

func noteTitle(p string) string {
	return strings.TrimSuffix(path.Base(p), path.Ext(p))
}

func (ti *termIndex) idf(term string) float64 {
	n, df := float64(len(ti.chunkLen)), float64(len(ti.postings[term]))
	return math.Log(1 + (n-df+0.5)/(df+0.5))
}

// queryTerms returns the distinct words of query, without the words more
// than half the sections hold, which say little about any one note. When no
// word in between is left, it keeps them all.
func (ti *termIndex) queryTerms(query string) []string {
	var all, rare []string
	for _, w := range tokens(query) {
		if slices.Contains(all, w) {
			continue
		}
		all = append(all, w)
		if df := len(ti.postings[w]); df > 0 && 2*df <= len(ti.chunkLen) {
			rare = append(rare, w)
		}
	}
	if len(rare) == 0 {
		return all
	}
	return rare
}

// rank scores every section by BM25 over terms and returns each note that
// include accepts and that holds at least one term, scored by its best
// section, best first.
func (ti *termIndex) rank(terms []string, include func(string) bool) []scoredDoc {
	scores := map[int]float64{}
	covered := map[int]float64{}
	var weight float64
	for _, t := range terms {
		idf := ti.idf(t)
		weight += idf
		for _, p := range ti.postings[t] {
			norm := bm25K1 * (1 - bm25B + bm25B*float64(ti.chunkLen[p.chunk])/ti.avgLen)
			tf := float64(p.tf)
			scores[p.chunk] += idf * tf * (bm25K1 + 1) / (tf + norm)
			covered[p.chunk] += idf
		}
	}
	best := map[int]scoredDoc{}
	for chunk, score := range scores {
		doc := ti.chunkDoc[chunk]
		if b, ok := best[doc]; ok && (b.score > score || b.score == score && b.chunk < chunk-ti.docs[doc].first) {
			continue
		}
		best[doc] = scoredDoc{doc: doc, score: score, chunk: chunk - ti.docs[doc].first, coverage: covered[chunk] / weight}
	}
	out := make([]scoredDoc, 0, len(best))
	for _, s := range best {
		if include != nil && !include(ti.docs[s.doc].path) {
			continue
		}
		if s.chunk >= len(ti.docs[s.doc].chunks) {
			s.chunk = -1
		}
		out = append(out, s)
	}
	slices.SortFunc(out, func(a, b scoredDoc) int {
		if c := cmp.Compare(b.score, a.score); c != 0 {
			return c
		}
		return strings.Compare(ti.docs[a.doc].path, ti.docs[b.doc].path)
	})
	return out
}
