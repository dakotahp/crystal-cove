package semantic

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
)

var themes = map[string][]string{
	"Garden": {"tomato", "basil", "compost", "soil", "watering", "seedling", "mulch", "harvest", "trowel", "pruning", "greenhouse", "fertilizer"},
	"Money":  {"budget", "savings", "income", "expenses", "rent", "mortgage", "invoice", "taxes", "pension", "dividend", "ledger", "payroll"},
	"Music":  {"guitar", "chord", "melody", "rhythm", "tempo", "piano", "drums", "lyrics", "chorus", "album", "concert", "harmony"},
}

func themeWords(theme string, i int) string {
	vocab := themes[theme]
	words := make([]string, 0, 10)
	for j := range 8 {
		words = append(words, vocab[(i*5+j)%len(vocab)])
	}
	return strings.Join(append(words, "2026", "the"), " ")
}

func longSection(theme string) string {
	return strings.Repeat(strings.Join(themes[theme], " ")+" ", 6)
}

func themeNotes(n int) map[string]string {
	notes := map[string]string{}
	for theme := range themes {
		for i := range n {
			notes[fmt.Sprintf("%s/%c%02d.md", theme, theme[0]+32, i)] = themeWords(theme, i) + "\n"
		}
	}
	return notes
}

func all(string) bool { return true }

func refresh(t *testing.T, ix *Index) {
	t.Helper()
	if err := ix.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func topicsOf(t *testing.T, ix *Index, scope string, include func(string) bool) TopicMap {
	t.Helper()
	m, _, err := ix.Topics(scope, include)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func themeOf(p string) string { return strings.SplitN(p, "/", 2)[0] }

// topicNotes lists the notes in each topic of a cached build, by topic ID.
func topicNotes(t *testing.T, ix *Index, build string) map[string][]string {
	t.Helper()
	ix.topics.mu.Lock()
	defer ix.topics.mu.Unlock()
	b, ok := ix.topics.maps[build]
	if !ok {
		t.Fatalf("build %s not cached", build)
	}
	out := map[string][]string{}
	for i, members := range b.members {
		for _, s := range members {
			p := b.entries[s].path
			if !slices.Contains(out[b.topicMap.Topics[i].ID], p) {
				out[b.topicMap.Topics[i].ID] = append(out[b.topicMap.Topics[i].ID], p)
			}
		}
	}
	return out
}

func gardenTopic(m TopicMap) Topic {
	for _, tp := range m.Topics {
		if tp.Folders[0].Folder == "Garden" {
			return tp
		}
	}
	return Topic{}
}

func TestTopicsGroupsByTheme(t *testing.T) {
	ix, _, _ := newTestIndex(t, themeNotes(20))
	refresh(t, ix)
	m := topicsOf(t, ix, "", all)
	if len(m.Topics) != 3 || m.Notes != 60 || m.OutsideNotes != 0 || m.Build == "" || m.Parent != "" {
		t.Fatalf("map = %+v", m)
	}
	for i, tp := range m.Topics {
		if tp.ID != fmt.Sprint(i+1) {
			t.Errorf("topic %d has id %q", i, tp.ID)
		}
		if i > 0 && tp.Notes > m.Topics[i-1].Notes {
			t.Error("topics not sorted by notes")
		}
		theme := tp.Folders[0].Folder
		if themes[theme] == nil || tp.Folders[0].Notes != tp.Notes {
			t.Errorf("topic %s folders = %+v", tp.ID, tp.Folders)
		}
		if len(tp.StartWith) != 3 {
			t.Errorf("topic %s start_with = %v", tp.ID, tp.StartWith)
		}
		for _, p := range tp.StartWith {
			if themeOf(p) != theme {
				t.Errorf("topic %s starts with %s", tp.ID, p)
			}
		}
		if len(tp.KeyWords) != 5 {
			t.Errorf("topic %s key words = %v", tp.ID, tp.KeyWords)
		}
		for _, w := range tp.KeyWords {
			if !slices.Contains(themes[theme], w) {
				t.Errorf("topic %s key word %q is not a %s word", tp.ID, w, theme)
			}
		}
		if len(tp.CloseTo) != 2 || slices.Contains(tp.CloseTo, tp.ID) {
			t.Errorf("topic %s close_to = %v", tp.ID, tp.CloseTo)
		}
	}
}

func TestTopicsMixedNoteJoinsEachTopic(t *testing.T) {
	notes := themeNotes(20)
	notes["Log.md"] = "# Garden\n" + longSection("Garden") + "\n# Money\n" + longSection("Money") + "\n"
	ix, _, _ := newTestIndex(t, notes)
	refresh(t, ix)
	m := topicsOf(t, ix, "", all)
	in := 0
	for _, paths := range topicNotes(t, ix, m.Build) {
		if slices.Contains(paths, "Log.md") {
			in++
		}
	}
	if in != 2 {
		t.Errorf("Log.md is in %d topics, want 2", in)
	}
}

func TestTopicsLeavesOutliers(t *testing.T) {
	notes := themeNotes(20)
	notes["Odd.md"] = "zebra quantum lantern velvet glacier orbit canyon whistle marble falcon ember tundra prism saddle kettle mosaic harbor nectar quiver ripple sonnet tapestry umbra vortex walnut yonder zephyr anvil bramble cobalt\n"
	ix, _, _ := newTestIndex(t, notes)
	refresh(t, ix)
	m := topicsOf(t, ix, "", all)
	if m.OutsideNotes != 1 || m.Notes != 61 {
		t.Errorf("notes %d, outside %d", m.Notes, m.OutsideNotes)
	}
	for id, paths := range topicNotes(t, ix, m.Build) {
		if slices.Contains(paths, "Odd.md") {
			t.Errorf("Odd.md is in topic %s", id)
		}
	}
}

func TestTopicsStartWithPrefersFocusedNotes(t *testing.T) {
	notes := themeNotes(20)
	for i := range 5 {
		notes[fmt.Sprintf("Daily/D%d.md", i)] = "# Garden\n" + longSection("Garden") + "\n# Money\n" + longSection("Money") + "\n# Music\n" + longSection("Music") + "\n"
	}
	ix, _, _ := newTestIndex(t, notes)
	refresh(t, ix)
	m := topicsOf(t, ix, "", all)
	for _, tp := range m.Topics {
		if tp.Folders[0].Folder == "Daily" {
			t.Errorf("topic %s led by Daily: %+v", tp.ID, tp.Folders)
		}
		for _, p := range tp.StartWith {
			if strings.HasPrefix(p, "Daily/") {
				t.Errorf("topic %s starts with %s", tp.ID, p)
			}
		}
	}
}

func TestTopicsStableAcrossRefreshes(t *testing.T) {
	ix, _, v := newTestIndex(t, themeNotes(20))
	refresh(t, ix)
	first := topicsOf(t, ix, "", all)
	refresh(t, ix)
	if again := topicsOf(t, ix, "", all); !reflect.DeepEqual(first, again) {
		t.Errorf("unchanged vault gave a new map:\n%+v\n%+v", first, again)
	}
	writeNote(t, v.Root(), "Garden/g00.md", "tomato basil compost soil changed\n")
	refresh(t, ix)
	if changed := topicsOf(t, ix, "", all); changed.Build == first.Build {
		t.Error("edited vault kept its build")
	}
}

func TestTopicsScope(t *testing.T) {
	ix, _, _ := newTestIndex(t, themeNotes(20))
	refresh(t, ix)
	whole := topicsOf(t, ix, "", all)
	garden := topicsOf(t, ix, "+Garden", func(p string) bool { return strings.HasPrefix(p, "Garden/") })
	if garden.Build == whole.Build || garden.Notes != 20 {
		t.Fatalf("garden = %+v", garden)
	}
	for _, tp := range garden.Topics {
		for _, f := range tp.Folders {
			if f.Folder != "Garden" {
				t.Errorf("folder %s in a Garden-only map", f.Folder)
			}
		}
	}
}

func TestTopicsTooFewSections(t *testing.T) {
	ix, _, _ := newTestIndex(t, map[string]string{"A.md": "one\n", "B.md": "two\n", "C.md": "three\n"})
	if _, status, err := ix.Topics("", all); !errors.Is(err, ErrTooFewSections) || status.Complete {
		t.Errorf("before refresh: %v %+v", err, status)
	}
	refresh(t, ix)
	if _, _, err := ix.Topics("", all); !errors.Is(err, ErrTooFewSections) {
		t.Errorf("err = %v", err)
	}
}

func TestTopicsConcurrent(t *testing.T) {
	ix, _, _ := newTestIndex(t, themeNotes(20))
	refresh(t, ix)
	builds := make([]string, 8)
	var wg sync.WaitGroup
	for i := range builds {
		wg.Go(func() {
			m, _, err := ix.Topics("", all)
			if err != nil {
				t.Error(err)
			}
			builds[i] = m.Build
		})
	}
	wg.Wait()
	for _, b := range builds {
		if b != builds[0] || b == "" {
			t.Errorf("builds = %v", builds)
		}
	}
}

func TestKeyWordsNeedTwoNotes(t *testing.T) {
	notes := themeNotes(20)
	notes["Garden/g00.md"] = themeWords("Garden", 0) + " zucchini zucchini zucchini\n"
	ix, _, _ := newTestIndex(t, notes)
	refresh(t, ix)
	m := topicsOf(t, ix, "", all)
	if tp := gardenTopic(m); slices.Contains(tp.KeyWords, "zucchini") || tp.ID == "" {
		t.Errorf("garden topic = %+v", tp)
	}
}
