package semantic

import (
	"cmp"
	"context"
	"regexp"
	"slices"
	"strings"
)

const (
	// rrfK is reciprocal rank fusion's usual constant: it keeps the top
	// few places of one list from outweighing agreement between lists.
	rrfK = 60
	// meaningListSize is how many of the closest notes by meaning take part
	// in a search. Every note has some closeness, so without a cut every
	// note would count as found.
	meaningListSize = 50
	// pinnedScore puts a note named after the query first: no note can
	// score this much from the lists, whose shares are each below 1/rrfK.
	pinnedScore = 1
)

// The ways a search can find a note, as Result.MatchedBy reports them.
const (
	MatchName    = "name"
	MatchPhrase  = "phrase"
	MatchKeyword = "keyword"
	MatchMeaning = "meaning"
)

// Result is a note found by Search, with the section that best shows why.
type Result struct {
	Path        string
	HeadingPath []string
	Text        string
	MatchedBy   []string
}

var quotedPhrase = regexp.MustCompile(`"([^"]+)"`)

type candidate struct {
	result Result
	doc    int
	score  float64
	hit    *Hit
	// keywordChunk is the section that earned the keyword score.
	keywordChunk int
}

// Search finds notes by name, by quoted phrase, by keyword, and by meaning,
// and merges the four rankings by reciprocal rank fusion: a note scores
// 1/(rrfK+rank) for each list it is in. A keyword vote is scaled by how much
// of the query's word weight the note's best section holds, so a note that
// shares only a common word with a paraphrase does not outvote meaning. A note whose name holds every word
// of the query comes first. include, when not nil, limits which notes take
// part. Keyword results work as soon as the notes have been read, before any
// vector exists; meaning joins in once vectors do.
func (ix *Index) Search(ctx context.Context, query string, include func(string) bool) ([]Result, Status, error) {
	terms := ix.termSnapshot()
	if terms == nil {
		return nil, ix.currentStatus(), nil
	}
	found := map[string]*candidate{}
	add := func(doc int, how string, score float64) *candidate {
		p := terms.docs[doc].path
		c, ok := found[p]
		if !ok {
			c = &candidate{result: Result{Path: p}, doc: doc}
			found[p] = c
		}
		c.result.MatchedBy = append(c.result.MatchedBy, how)
		c.score += score
		return c
	}
	share := func(rank int) float64 { return 1 / float64(rrfK+rank+1) }

	keyTerms := terms.queryTerms(query)
	keyword := terms.rank(keyTerms, include)
	keywordRank := make(map[int]int, len(keyword))
	for i, s := range keyword {
		keywordRank[s.doc] = i
	}
	if want := tokens(quotedPhrase.ReplaceAllString(query, "$1")); len(want) > 0 {
		for i, d := range terms.docs {
			if (include == nil || include(d.path)) && holdsAll(tokens(noteTitle(d.path)), want) {
				add(i, MatchName, pinnedScore)
			}
		}
	}
	phrases := phrasesOf(query)
	if len(phrases) > 0 {
		var docs []int
		for i, d := range terms.docs {
			if (include == nil || include(d.path)) && phraseChunk(d, phrases) >= 0 {
				docs = append(docs, i)
			}
		}
		keywordOrder := func(doc int) int {
			if r, ok := keywordRank[doc]; ok {
				return r
			}
			return len(keyword)
		}
		slices.SortStableFunc(docs, func(a, b int) int { return cmp.Compare(keywordOrder(a), keywordOrder(b)) })
		for rank, doc := range docs {
			add(doc, MatchPhrase, share(rank))
		}
	}
	for rank, s := range keyword {
		add(s.doc, MatchKeyword, share(rank)*s.coverage).keywordChunk = s.chunk
	}
	hits, status, err := ix.searchMeaning(ctx, query)
	if err != nil {
		return nil, status, err
	}
	rank := 0
	for i := range hits {
		if rank == meaningListSize {
			break
		}
		doc, ok := terms.byPath[hits[i].Path]
		if !ok || (include != nil && !include(hits[i].Path)) {
			continue
		}
		add(doc, MatchMeaning, share(rank)).hit = &hits[i]
		rank++
	}

	ordered := make([]*candidate, 0, len(found))
	for _, c := range found {
		ordered = append(ordered, c)
	}
	slices.SortFunc(ordered, func(a, b *candidate) int {
		if c := cmp.Compare(b.score, a.score); c != 0 {
			return c
		}
		return strings.Compare(a.result.Path, b.result.Path)
	})
	results := make([]Result, len(ordered))
	for i, c := range ordered {
		results[i] = c.withSection(terms, phrases)
	}
	return results, status, nil
}

// withSection fills in the section that shows best why the note was found:
// the one holding a quoted phrase, else the one holding the rarest key
// words, else the one closest in meaning. A name match is always a keyword
// match too, because a note's name counts among its words.
func (c *candidate) withSection(terms *termIndex, phrases []string) Result {
	r := c.result
	d := terms.docs[c.doc]
	var chunk int
	switch {
	case slices.Contains(r.MatchedBy, MatchPhrase):
		chunk = phraseChunk(d, phrases)
	case slices.Contains(r.MatchedBy, MatchKeyword):
		chunk = c.keywordChunk
	default:
		r.HeadingPath, r.Text = c.hit.HeadingPath, c.hit.Text
		return r
	}
	if chunk >= 0 {
		r.HeadingPath, r.Text = d.chunks[chunk].HeadingPath, d.chunks[chunk].Text
	}
	return r
}

func phrasesOf(query string) []string {
	var out []string
	for _, m := range quotedPhrase.FindAllStringSubmatch(query, -1) {
		if p := strings.Join(tokens(m[1]), " "); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// phraseChunk returns the first chunk of d whose headings or text hold every
// phrase, compared word by word so case, punctuation and line breaks do not
// matter, or -1.
func phraseChunk(d termDoc, phrases []string) int {
	for i, c := range d.chunks {
		text := " " + strings.Join(tokens(strings.Join(c.HeadingPath, " ")+"\n"+c.Text), " ") + " "
		if !slices.ContainsFunc(phrases, func(p string) bool { return !strings.Contains(text, " "+p+" ") }) {
			return i
		}
	}
	return -1
}

func holdsAll(have, want []string) bool {
	for _, w := range want {
		if !slices.Contains(have, w) {
			return false
		}
	}
	return true
}
