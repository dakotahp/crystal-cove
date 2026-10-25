package semantic

import (
	"cmp"
	"context"
	"regexp"
	"slices"
	"strings"
)

const (
	// meaningListSize is how many of the closest notes by meaning a search
	// returns. Every note has some closeness, so without a cut every note in
	// the vault would be a result.
	meaningListSize = 50
	// rescueLimit is how many notes holding every key word of the query move
	// above the meaning ranking. More would push close paraphrase matches
	// off the first page.
	rescueLimit = 3
	// fullCoverage allows for rounding in a section's share of the query.
	fullCoverage = 1 - 1e-9
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

type found struct {
	name, phrase, closest bool
	// rescued is set for a note moved above the meaning ranking.
	rescued bool
	keyword *scoredDoc
	hit     *Hit
}

// Search ranks notes by meaning and lets key words rescue the clear hits
// that meaning misses. Notes whose name holds every word of the query come
// first, then notes holding a quoted phrase, then the rescueLimit notes with
// the best keyword scores among those with a section holding every key word;
// each group is in meaning order, and the meaningListSize closest notes
// follow in meaning order. A note matching only some key words keeps its
// meaning place, because a paraphrase shares ordinary words with many
// unrelated notes. Before any vector exists, key words alone set the order.
// include, when not nil, limits which notes take part.
func (ix *Index) Search(ctx context.Context, query string, include func(string) bool) ([]Result, Status, error) {
	terms := ix.termSnapshot()
	if terms == nil {
		return nil, ix.currentStatus(), nil
	}
	included := func(doc int) bool { return include == nil || include(terms.docs[doc].path) }
	notes := map[int]*found{}
	get := func(doc int) *found {
		f, ok := notes[doc]
		if !ok {
			f = &found{}
			notes[doc] = f
		}
		return f
	}

	keyword := terms.rank(terms.queryTerms(query), include)
	keywordRank := make(map[int]int, len(keyword))
	for i := range keyword {
		keywordRank[keyword[i].doc] = i
		get(keyword[i].doc).keyword = &keyword[i]
	}
	hits, status, err := ix.searchMeaning(ctx, query)
	if err != nil {
		return nil, status, err
	}
	var meaning []int
	meaningRank := map[int]int{}
	for i := range hits {
		doc, ok := terms.byPath[hits[i].Path]
		if !ok || !included(doc) {
			continue
		}
		f := get(doc)
		f.hit = &hits[i]
		f.closest = len(meaning) < meaningListSize
		meaningRank[doc] = len(meaning)
		meaning = append(meaning, doc)
	}
	// Notes moved up together each hold what moved them, so meaning orders
	// them, then key words for notes not yet embedded.
	place := func(doc int) int {
		if r, ok := meaningRank[doc]; ok {
			return r
		}
		return len(meaning) + keywordRank[doc]
	}
	byMeaning := func(a, b int) int { return cmp.Compare(place(a), place(b)) }

	var order []int
	rescue := func(docs []int, mark func(*found)) {
		slices.SortStableFunc(docs, byMeaning)
		for _, doc := range docs {
			f := get(doc)
			mark(f)
			if !f.rescued {
				f.rescued = true
				order = append(order, doc)
			}
		}
	}
	var named []int
	if want := tokens(quotedPhrase.ReplaceAllString(query, "$1")); len(want) > 0 {
		for i, d := range terms.docs {
			if included(i) && holdsAll(tokens(noteTitle(d.path)), want) {
				named = append(named, i)
			}
		}
	}
	rescue(named, func(f *found) { f.name = true })
	phrases := phrasesOf(query)
	var quoted []int
	if len(phrases) > 0 {
		for i, d := range terms.docs {
			if included(i) && phraseChunk(d, phrases) >= 0 {
				quoted = append(quoted, i)
			}
		}
	}
	rescue(quoted, func(f *found) { f.phrase = true })
	var covering []int
	for _, s := range keyword {
		if len(covering) == rescueLimit {
			break
		}
		if s.coverage >= fullCoverage && !get(s.doc).rescued {
			covering = append(covering, s.doc)
		}
	}
	rescue(covering, func(*found) {})
	for _, doc := range meaning[:min(meaningListSize, len(meaning))] {
		if !get(doc).rescued {
			order = append(order, doc)
		}
	}
	for _, s := range keyword {
		if get(s.doc).hit == nil && !get(s.doc).rescued {
			order = append(order, s.doc)
		}
	}

	results := make([]Result, len(order))
	for i, doc := range order {
		results[i] = notes[doc].result(terms.docs[doc], phrases)
	}
	return results, status, nil
}

// result reports how the note was found and the section that shows it best:
// the one holding a quoted phrase; else, for a note placed by its key words,
// the section that earned its keyword score; else the closest in meaning.
func (f *found) result(d termDoc, phrases []string) Result {
	r := Result{Path: d.path}
	for _, m := range []struct {
		ok  bool
		how string
	}{{f.name, MatchName}, {f.phrase, MatchPhrase}, {f.keyword != nil, MatchKeyword}, {f.closest, MatchMeaning}} {
		if m.ok {
			r.MatchedBy = append(r.MatchedBy, m.how)
		}
	}
	chunk := -1
	switch {
	case f.phrase:
		chunk = phraseChunk(d, phrases)
	case f.keyword != nil && (f.rescued || f.hit == nil):
		chunk = f.keyword.chunk
	case f.hit != nil:
		r.HeadingPath, r.Text = f.hit.HeadingPath, f.hit.Text
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
