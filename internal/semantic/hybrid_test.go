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
		"Garden.md": "# Layout\nraised beds paths borders " + repeatWord("layout", 80) +
			"\n# Watering basics\nwatering is deep and slow " + repeatWord("slow", 80) + "\n",
		"Beds.md":   "# Beds\nbeds beds borders\n",
		"Budget.md": "# Money\nrent income\n",
	})
	if err := ix.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	results := searchFor(t, ix, "watering and beds", nil)
	garden, ok := resultFor(results, "Garden.md")
	if !ok {
		t.Fatalf("results = %+v, want Garden.md", results)
	}
	if !slices.Equal(garden.HeadingPath, []string{"Watering basics"}) || !strings.HasPrefix(garden.Text, "watering is") {
		t.Errorf("garden = %+v, want the watering section", garden)
	}
	if !slices.Equal(garden.MatchedBy, []string{MatchMeaning}) {
		t.Errorf("matched_by = %q, want meaning only: no section holds both key words", garden.MatchedBy)
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
		"Reading/Project Kestrel.md": "a field study of small falcons\n",
		"Birds.md":                   "project kestrel project kestrel birds\n",
		"Other.md":                   "rent\n",
	})
	if err := ix.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	results := searchFor(t, ix, "Project Kestrel", nil)
	if results[0].Path != "Reading/Project Kestrel.md" || !slices.Contains(results[0].MatchedBy, MatchName) {
		t.Errorf("results = %+v, want the note named Project Kestrel first, matched by name", results)
	}
	if birds, _ := resultFor(results, "Birds.md"); slices.Contains(birds.MatchedBy, MatchName) {
		t.Errorf("Birds.md matched by name: %+v", birds)
	}
}

func TestSearchQuotedPhrase(t *testing.T) {
	ix, _, _ := newTestIndex(t, map[string]string{
		"a.md": "# Frames\nwhy a cold frame helps seedlings\n",
		"b.md": "frame is cold and frame is cold\n",
		"c.md": "rent\n",
	})
	if err := ix.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	results := searchFor(t, ix, `"Cold Frame" in spring`, nil)
	if results[0].Path != "a.md" || !slices.Contains(results[0].MatchedBy, MatchPhrase) {
		t.Errorf("results = %+v, want a.md first, matched by phrase", results)
	}
	if b, _ := resultFor(results, "b.md"); slices.Contains(b.MatchedBy, MatchPhrase) {
		t.Errorf("b.md lacks the phrase but matched it: %+v", b)
	}
}

func TestSearchFindsAPhraseInAHeading(t *testing.T) {
	ix, _, _ := newTestIndex(t, map[string]string{
		"Log.md": "# Week 2\n## Project Kestrel\nfield notes on falcons\n",
		"b.md":   "kestrel seen near the project site\n",
		"c.md":   "rent\n",
	})
	if err := ix.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	results := searchFor(t, ix, `"project kestrel"`, nil)
	if results[0].Path != "Log.md" || !slices.Contains(results[0].MatchedBy, MatchPhrase) {
		t.Errorf("results = %+v, want Log.md first, matched by phrase in its heading", results)
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

func TestSearchStopsAtTheClosestByMeaningPlusRescuedNotes(t *testing.T) {
	notes := map[string]string{}
	for i := range meaningListSize + 5 {
		notes[fmt.Sprintf("n%02d.md", i)] = fmt.Sprintf("garden soil note %d\n", i)
	}
	notes["zz.md"] = "zebra\n"
	ix, _, _ := newTestIndex(t, notes)
	ix.embed = &blindEmbedder{blind: "zebra"}
	if err := ix.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	results := searchFor(t, ix, "zebra garden soil", nil)
	if len(results) != meaningListSize+1 || results[0].Path != "zz.md" {
		t.Fatalf("%d results, first %s; want the rescued zebra note and the %d closest", len(results), results[0].Path, meaningListSize)
	}
	for _, r := range results[1:] {
		if !slices.Equal(r.MatchedBy, []string{MatchMeaning}) {
			t.Errorf("%+v: want the rest matched by meaning", r)
		}
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
	if partial, _ := resultFor(results, "partial.md"); slices.Contains(partial.MatchedBy, MatchKeyword) {
		t.Errorf("partial.md = %+v, want no keyword label for some of the key words", partial)
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

func TestSearchPointsAtTheJoinedSectionHoldingTheMatch(t *testing.T) {
	var chapter strings.Builder
	chapter.WriteString("# Letters 8\n")
	for i := 1; i <= 8; i++ {
		text := fmt.Sprintf("verse %d says little of note here.", i)
		if i == 6 {
			text = "all things work together for good."
		}
		fmt.Fprintf(&chapter, "###### %d\n%s\n", i, text)
	}
	ix, _, _ := newTestIndex(t, map[string]string{
		"Letters 8.md": chapter.String(),
		"Budget.md":    "# Money\nbudget rent income\n",
		"Garden.md":    "# Beds\nsoil compost\n",
	})
	if err := ix.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"things work together for good", "things work together for good budget"} {
		r, ok := resultFor(searchFor(t, ix, query, nil), "Letters 8.md")
		if !ok {
			t.Fatalf("%q: Letters 8.md not found", query)
		}
		if !slices.Equal(r.HeadingPath, []string{"Letters 8", "6"}) || r.Text != "all things work together for good." {
			t.Errorf("%q: got %q %q, want verse 6", query, r.HeadingPath, r.Text)
		}
	}
	r, _ := resultFor(searchFor(t, ix, "soil", nil), "Letters 8.md")
	if !slices.Equal(r.HeadingPath, []string{"Letters 8", "1"}) {
		t.Errorf("no shared words: got %q, want the chunk start", r.HeadingPath)
	}
}
