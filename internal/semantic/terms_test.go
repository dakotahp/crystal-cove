package semantic

import (
	"slices"
	"testing"
)

func TestTokens(t *testing.T) {
	got := tokens("Trust, Teams & PSYCHOLOGICAL-safety in 2024: café naïve")
	want := []string{"trust", "teams", "psychological", "safety", "in", "2024", "café", "naïve"}
	if !slices.Equal(got, want) {
		t.Errorf("tokens = %q, want %q", got, want)
	}
	if got := tokens("  ...  "); len(got) != 0 {
		t.Errorf("punctuation only: %q", got)
	}
}

func termNotes(notes map[string][]Chunk) []noteChunks {
	var out []noteChunks
	for p, chunks := range notes {
		out = append(out, noteChunks{path: p, chunks: chunks})
	}
	slices.SortFunc(out, func(a, b noteChunks) int {
		if a.path < b.path {
			return -1
		}
		return 1
	})
	return out
}

func rankedPaths(ti *termIndex, query []string) []string {
	var out []string
	for _, s := range ti.rank(query, nil) {
		out = append(out, ti.docs[s.doc].path)
	}
	return out
}

func TestTermIndexRanksRareWordsAboveCommonOnes(t *testing.T) {
	ti := newTermIndex(termNotes(map[string][]Chunk{
		"a.md": {{Text: "the team met and the team planned"}},
		"b.md": {{Text: "the trust in a team"}},
		"c.md": {{Text: "the weather today"}},
		"d.md": {{Text: "the budget"}},
	}))
	got := rankedPaths(ti, []string{"trust", "team"})
	if len(got) != 2 || got[0] != "b.md" {
		t.Errorf("ranked = %q, want b.md first: trust is rarer than team", got)
	}
}

func TestTermIndexSaturatesRepeatsAndNormalizesLength(t *testing.T) {
	ti := newTermIndex(termNotes(map[string][]Chunk{
		"short.md":  {{Text: "compost soil"}},
		"long.md":   {{Text: "compost " + repeatWord("filler", 200)}},
		"thrice.md": {{Text: repeatWord("compost", 3) + repeatWord("filler", 200)}},
		"spammy.md": {{Text: repeatWord("compost", 30) + repeatWord("filler", 200)}},
		"other.md":  {{Text: "rent"}},
	}))
	score := map[string]float64{}
	for _, s := range ti.rank([]string{"compost"}, nil) {
		score[ti.docs[s.doc].path] = s.score
	}
	if len(score) != 4 || score["short.md"] <= score["long.md"] {
		t.Errorf("scores = %v, want the short note above the long one with the same single match", score)
	}
	if score["spammy.md"] > 1.6*score["thrice.md"] {
		t.Errorf("scores = %v: ten times the repeats should add far less than ten times the score", score)
	}
}

func repeatWord(w string, n int) string {
	out := ""
	for range n {
		out += w + " "
	}
	return out
}

func TestTermIndexCountsTheNoteName(t *testing.T) {
	ti := newTermIndex(termNotes(map[string][]Chunk{
		"Projects/Aristotle.md": {{Text: "google studied teams"}},
		"b.md":                  {{Text: "unrelated"}},
	}))
	if got := rankedPaths(ti, []string{"aristotle"}); !slices.Equal(got, []string{"Projects/Aristotle.md"}) {
		t.Errorf("ranked = %q, want the note named Aristotle", got)
	}
}

func TestTermIndexRankHonoursInclude(t *testing.T) {
	ti := newTermIndex(termNotes(map[string][]Chunk{
		"in/a.md":  {{Text: "compost"}},
		"out/b.md": {{Text: "compost compost"}},
	}))
	got := ti.rank([]string{"compost"}, func(p string) bool { return p == "in/a.md" })
	if len(got) != 1 || ti.docs[got[0].doc].path != "in/a.md" {
		t.Errorf("rank = %+v, want only in/a.md", got)
	}
}

func TestQueryTermsDropWordsMostNotesHold(t *testing.T) {
	ti := newTermIndex(termNotes(map[string][]Chunk{
		"a.md": {{Text: "the trust"}},
		"b.md": {{Text: "the team"}},
		"c.md": {{Text: "the plan"}},
	}))
	if got := ti.queryTerms("the trust the"); !slices.Equal(got, []string{"trust"}) {
		t.Errorf("queryTerms = %q, want [trust]", got)
	}
	if got := ti.queryTerms("the"); !slices.Equal(got, []string{"the"}) {
		t.Errorf("a query of only common words keeps them: %q", got)
	}
	if got := ti.queryTerms("the absent"); !slices.Equal(got, []string{"the", "absent"}) {
		t.Errorf("a word in no note is not rare, so the common word stays: %q", got)
	}
	if got := ti.queryTerms("..."); len(got) != 0 {
		t.Errorf("no words: %q", got)
	}
}

func TestBestChunkPrefersTheSectionHoldingRareTerms(t *testing.T) {
	ti := newTermIndex(termNotes(map[string][]Chunk{
		"lecture.md": {
			{HeadingPath: []string{"Leadership styles"}, Text: "transactional and transformational leaders and teams"},
			{HeadingPath: []string{"Importance of trust"}, Text: "trust is willingness to be vulnerable"},
		},
		"b.md": {{Text: "teams teams"}},
		"c.md": {{Text: "teams"}},
	}))
	lecture := slices.IndexFunc(ti.docs, func(d termDoc) bool { return d.path == "lecture.md" })
	if got := ti.bestChunk(lecture, []string{"trust", "teams"}); got != 1 {
		t.Errorf("bestChunk = %d, want the trust section", got)
	}
	if got := ti.bestChunk(lecture, []string{"absent"}); got != 0 {
		t.Errorf("no term present: bestChunk = %d, want the first section", got)
	}
}

func TestEmptyTermIndex(t *testing.T) {
	if got := newTermIndex(nil).rank([]string{"x"}, nil); len(got) != 0 {
		t.Errorf("rank on empty index = %+v", got)
	}
	ti := newTermIndex(termNotes(map[string][]Chunk{"empty.md": nil}))
	if got := ti.bestChunk(0, []string{"empty"}); got != -1 {
		t.Errorf("bestChunk on a note without chunks = %d", got)
	}
}
