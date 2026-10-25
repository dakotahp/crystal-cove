package semantic

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
)

func searchFor(t *testing.T, ix *Index, query string, include func(string) bool) []Result {
	t.Helper()
	results, _, err := ix.Search(context.Background(), query, include)
	if err != nil {
		t.Fatal(err)
	}
	return results
}

func resultFor(results []Result, p string) (Result, bool) {
	i := slices.IndexFunc(results, func(r Result) bool { return r.Path == p })
	if i < 0 {
		return Result{}, false
	}
	return results[i], true
}

func TestSearchPicksTheSectionHoldingTheKeyWords(t *testing.T) {
	ix, _, _ := newTestIndex(t, map[string]string{
		"Lecture.md": "# Styles\ntransactional transformational leaders " + repeatWord("styles", 80) +
			"\n# Importance of trust\ntrust is willingness to be vulnerable " + repeatWord("vulnerable", 80) + "\n",
		"Teams.md":  "# Teams\nteams teams leaders\n",
		"Budget.md": "# Money\nrent income\n",
	})
	if err := ix.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	results := searchFor(t, ix, "trust and teams", nil)
	lecture, ok := resultFor(results, "Lecture.md")
	if !ok {
		t.Fatalf("results = %+v, want Lecture.md", results)
	}
	if !slices.Equal(lecture.HeadingPath, []string{"Importance of trust"}) || !strings.HasPrefix(lecture.Text, "trust is") {
		t.Errorf("lecture = %+v, want the trust section", lecture)
	}
	if !slices.Contains(lecture.MatchedBy, MatchKeyword) || !slices.Contains(lecture.MatchedBy, MatchMeaning) {
		t.Errorf("matched_by = %q, want keyword and meaning", lecture.MatchedBy)
	}
	if results[len(results)-1].Path != "Budget.md" {
		t.Errorf("results = %+v, want the budget note last", results)
	}
	if budget, _ := resultFor(results, "Budget.md"); slices.Contains(budget.MatchedBy, MatchKeyword) {
		t.Errorf("budget matched by keyword: %+v", budget)
	}
}

func TestSearchRanksNotesFoundBothWaysFirst(t *testing.T) {
	ix, _, _ := newTestIndex(t, map[string]string{
		"a.md": "compost soil watering\n",
		"b.md": "soil\n",
		"c.md": "rent income savings\n",
		"d.md": "holiday travel\n",
	})
	if err := ix.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	results := searchFor(t, ix, "compost watering", nil)
	if results[0].Path != "a.md" {
		t.Errorf("results = %+v, want a.md first", results)
	}
}

func TestSearchPinsANoteNamedAfterTheQuery(t *testing.T) {
	ix, _, _ := newTestIndex(t, map[string]string{
		"Reading/Project Aristotle.md": "google studied what makes groups work\n",
		"Teams.md":                     "project aristotle project aristotle teams\n",
		"Other.md":                     "rent\n",
	})
	if err := ix.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	results := searchFor(t, ix, "Project Aristotle", nil)
	if results[0].Path != "Reading/Project Aristotle.md" || !slices.Contains(results[0].MatchedBy, MatchName) {
		t.Errorf("results = %+v, want the note named Project Aristotle first, matched by name", results)
	}
	if teams, _ := resultFor(results, "Teams.md"); slices.Contains(teams.MatchedBy, MatchName) {
		t.Errorf("Teams.md matched by name: %+v", teams)
	}
}

func TestSearchQuotedPhrase(t *testing.T) {
	ix, _, _ := newTestIndex(t, map[string]string{
		"a.md": "# Safety\nwhy psychological safety matters\n",
		"b.md": "safety is psychological and safety is psychological\n",
		"c.md": "rent\n",
	})
	if err := ix.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	results := searchFor(t, ix, `"Psychological Safety" at work`, nil)
	if results[0].Path != "a.md" || !slices.Contains(results[0].MatchedBy, MatchPhrase) {
		t.Errorf("results = %+v, want a.md first, matched by phrase", results)
	}
	if b, _ := resultFor(results, "b.md"); slices.Contains(b.MatchedBy, MatchPhrase) {
		t.Errorf("b.md lacks the phrase but matched it: %+v", b)
	}
}

func TestSearchFindsAPhraseInAHeading(t *testing.T) {
	ix, _, _ := newTestIndex(t, map[string]string{
		"Journal.md": "# Week 2\n## Project Aristotle\ngoogle studied teams\n",
		"b.md":       "aristotle wrote about project ethics\n",
		"c.md":       "rent\n",
	})
	if err := ix.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	results := searchFor(t, ix, `"project aristotle"`, nil)
	if results[0].Path != "Journal.md" || !slices.Contains(results[0].MatchedBy, MatchPhrase) {
		t.Errorf("results = %+v, want Journal.md first, matched by phrase in its heading", results)
	}
}

func TestSearchOrdersPhraseMatchesByKeywordRank(t *testing.T) {
	ix, _, _ := newTestIndex(t, map[string]string{
		"a.md": "the lean canvas\n",
		"b.md": "the lean canvas lean canvas startup startup\n",
		"c.md": "the rent\n",
		"d.md": "the holiday\n",
	})
	if err := ix.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	results := searchFor(t, ix, `"lean canvas" startup`, nil)
	if len(results) < 2 || results[0].Path != "b.md" || results[1].Path != "a.md" {
		t.Errorf("results = %+v, want b.md (phrase and more key words) before a.md", results)
	}
	for _, r := range results[:2] {
		if !slices.Contains(r.MatchedBy, MatchPhrase) {
			t.Errorf("%s not matched by phrase: %+v", r.Path, r)
		}
	}
}

func TestSearchKeepsPhraseMatchesMadeOfCommonWords(t *testing.T) {
	ix, _, _ := newTestIndex(t, map[string]string{
		"a.md": "# Plan\nmake the plan\n",
		"b.md": "the plan\n",
		"c.md": "the rent\n",
		"d.md": "the plan for a holiday\n",
	})
	if err := ix.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	results := searchFor(t, ix, `"the plan" rent`, nil)
	for _, p := range []string{"a.md", "b.md", "d.md"} {
		r, ok := resultFor(results, p)
		if !ok || !slices.Contains(r.MatchedBy, MatchPhrase) || !strings.Contains(r.Text, "the plan") {
			t.Errorf("%s: %+v, want a phrase match showing the phrase", p, r)
		}
	}
}

func TestSearchLabelsOnlyTheClosestAsMeaningMatches(t *testing.T) {
	notes := map[string]string{}
	for i := range meaningListSize + 5 {
		notes[fmt.Sprintf("n%02d.md", i)] = fmt.Sprintf("garden soil note %d\n", i)
	}
	ix, _, _ := newTestIndex(t, notes)
	if err := ix.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	results := searchFor(t, ix, "compost", nil)
	labelled := 0
	for _, r := range results {
		if slices.Contains(r.MatchedBy, MatchMeaning) {
			labelled++
		}
	}
	if len(results) != meaningListSize+5 || labelled != meaningListSize {
		t.Errorf("%d results, %d labelled meaning; want every note, the %d closest labelled", len(results), labelled, meaningListSize)
	}
}

// blindEmbedder cannot see one word, so a note can match it by keyword
// while meaning ranks it as unrelated.
type blindEmbedder struct {
	fakeEmbedder
	blind string
}

func (b *blindEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	seen := make([]string, len(texts))
	for i, t := range texts {
		seen[i] = strings.ReplaceAll(t, b.blind, "")
	}
	return b.fakeEmbedder.Embed(ctx, seen)
}

func TestSearchRescuesAFewNotesHoldingEveryKeyWord(t *testing.T) {
	ix, _, _ := newTestIndex(t, map[string]string{
		"z1.md":    "zebra zebra stripes\n",
		"z2.md":    "zebra stripes savanna\n",
		"z3.md":    "zebra stripes savanna grass\n",
		"z4.md":    "a zebra among many other animals in a long list of savanna grass herds\n",
		"apple.md": "stripes stripes stripes\n",
		"pear.md":  "pear\n",
	})
	ix.embed = &blindEmbedder{blind: "zebra"}
	if err := ix.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	results := searchFor(t, ix, "zebra stripes", nil)
	for _, r := range results[:rescueLimit] {
		if r.Path == "z4.md" || r.Path == "apple.md" || !slices.Contains(r.MatchedBy, MatchKeyword) {
			t.Errorf("rescued %+v, want the notes holding both words with the best keyword scores", r)
		}
	}
	if results[rescueLimit].Path != "apple.md" {
		t.Errorf("results = %+v, want meaning order after the rescued notes", results)
	}
}

func TestSearchKeepsMeaningOrderForPartialKeywordMatches(t *testing.T) {
	ix, _, _ := newTestIndex(t, map[string]string{
		"close.md":   "apple orchard harvest\n",
		"partial.md": "zebra crossing\n",
		"other.md":   "rent\n",
		"more.md":    "travel\n",
	})
	ix.embed = &blindEmbedder{blind: "zebra"}
	if err := ix.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	results := searchFor(t, ix, "apple zebra", nil)
	if results[0].Path != "close.md" {
		t.Errorf("results = %+v, want the closest by meaning first", results)
	}
	if partial, _ := resultFor(results, "partial.md"); !slices.Contains(partial.MatchedBy, MatchKeyword) {
		t.Errorf("partial.md = %+v, want it labelled keyword", partial)
	}
}

func TestSearchHonoursInclude(t *testing.T) {
	ix, _, _ := newTestIndex(t, gardenNotes)
	if err := ix.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	results := searchFor(t, ix, "tomatoes", func(p string) bool { return strings.HasPrefix(p, "Notes/") })
	if len(results) != 1 || results[0].Path != "Notes/Seedlings.md" {
		t.Errorf("results = %+v, want only Notes/Seedlings.md", results)
	}
}

func TestSearchUsesKeywordsBeforeAnyVectorExists(t *testing.T) {
	ix, _, _ := newTestIndex(t, gardenNotes)
	ix.embed = alwaysFail{}
	if err := ix.Refresh(context.Background()); err == nil {
		t.Fatal("refresh with a failing model succeeded")
	}
	results, status, err := ix.Search(context.Background(), "tomatoes", nil)
	if err != nil {
		t.Fatal(err)
	}
	if status.Complete {
		t.Errorf("status = %+v, want incomplete", status)
	}
	if len(results) != 2 {
		t.Fatalf("results = %+v, want the two notes holding tomatoes", results)
	}
	for _, r := range results {
		if !slices.Equal(r.MatchedBy, []string{MatchKeyword}) {
			t.Errorf("result %+v: want keyword only", r)
		}
	}
}

func TestSearchListsEveryKeywordMatchBeforeAnyVectorExists(t *testing.T) {
	notes := map[string]string{"other.md": "rent\n", "more.md": "travel\n", "else.md": "budget\n", "last.md": "holiday\n"}
	for i := range rescueLimit + 2 {
		notes[fmt.Sprintf("z%d.md", i)] = "zebra\n"
	}
	ix, _, _ := newTestIndex(t, notes)
	ix.embed = alwaysFail{}
	if err := ix.Refresh(context.Background()); err == nil {
		t.Fatal("refresh with a failing model succeeded")
	}
	if results := searchFor(t, ix, "zebra", nil); len(results) != rescueLimit+2 {
		t.Errorf("%d results, want all %d notes holding zebra", len(results), rescueLimit+2)
	}
}

type alwaysFail struct{}

func (alwaysFail) Embed(context.Context, []string) ([][]float32, error) {
	return nil, context.DeadlineExceeded
}

func (alwaysFail) Ready(context.Context) bool { return true }

func TestSearchBeforeAnyRefresh(t *testing.T) {
	ix, f, _ := newTestIndex(t, gardenNotes)
	results, _, err := ix.Search(context.Background(), "tomatoes", nil)
	if err != nil || len(results) != 0 || f.calls != 0 {
		t.Errorf("results %+v, err %v, model calls %d: want nothing and no embedding", results, err, f.calls)
	}
}

func TestSearchReportsAQueryEmbeddingFailure(t *testing.T) {
	ix, f, _ := newTestIndex(t, gardenNotes)
	if err := ix.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.failAfter = f.calls
	if _, _, err := ix.Search(context.Background(), "tomatoes", nil); err == nil || !strings.Contains(err.Error(), "query") {
		t.Errorf("err = %v, want the query embedding failure", err)
	}
}

func TestSearchNameOnlyMatchWithoutText(t *testing.T) {
	ix, _, _ := newTestIndex(t, map[string]string{
		"Inbox.md": "",
		"a.md":     "rent\n",
	})
	if err := ix.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	results := searchFor(t, ix, "inbox", nil)
	top := results[0]
	if top.Path != "Inbox.md" || !slices.Contains(top.MatchedBy, MatchName) || top.Text != "" || len(top.HeadingPath) != 0 {
		t.Errorf("results = %+v, want the empty Inbox note first, by name, with no section", results)
	}
}
