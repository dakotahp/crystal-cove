package semantic

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestTopicsShortHeadingDailyNotesDoNotLead(t *testing.T) {
	notes := themeNotes(20)
	for i := range 20 {
		notes[fmt.Sprintf("Daily/D%02d.md", i)] = "# Garden\n" + themeWords("Garden", i) + "\n# Money\n" + themeWords("Money", i) + "\n"
	}
	ix, _, _ := newTestIndex(t, notes)
	refresh(t, ix)
	for _, tp := range topicsOf(t, ix, "", all).Topics {
		for _, p := range tp.StartWith {
			if strings.HasPrefix(p, "Daily/") {
				t.Errorf("topic %s starts with %s: %v", tp.ID, p, tp.StartWith)
			}
		}
	}
}

func TestTopicsMixedDailyNotesKeepRealTopics(t *testing.T) {
	notes := themeNotes(20)
	for i := range 60 {
		notes[fmt.Sprintf("Daily/D%02d.md", i)] = "# Garden\n" + themeWords("Garden", i) + "\n# Money\n" + themeWords("Money", i) + "\n# Music\n" + themeWords("Music", i) + "\n"
	}
	ix, _, _ := newTestIndex(t, notes)
	refresh(t, ix)
	m := topicsOf(t, ix, "", all)
	for theme := range themes {
		found := false
		for _, tp := range m.Topics {
			if tp.Folders[0].Folder == theme && tp.Folders[0].Notes >= 18 {
				found = true
			}
		}
		if !found {
			t.Errorf("no topic led by %s: %+v", theme, m.Topics)
		}
	}
}

func TestTopicsNoGroupsIsAnError(t *testing.T) {
	ix, _, _ := newTestIndex(t, map[string]string{
		"One.md": "# A\n" + longSection("Garden") + "\n# B\n" + longSection("Money") + "\n# C\n" + longSection("Music") + "\n# D\n" + longSection("Garden") + "\n",
	})
	refresh(t, ix)
	if m, _, err := ix.Topics("", all); !errors.Is(err, ErrTooFewSections) {
		t.Errorf("one note: %v %+v", err, m)
	}
}

func TestKeyWordsInATinyTopicUseSingleNoteWords(t *testing.T) {
	ix, _, _ := newTestIndex(t, map[string]string{
		"A.md": "tomatoes compost\n",
		"B.md": "seedlings basil\n",
		"C.md": "mulch trowel\n",
		"D.md": "pruning harvest\n",
	})
	refresh(t, ix)
	for _, tp := range topicsOf(t, ix, "", all).Topics {
		if len(tp.KeyWords) == 0 {
			t.Errorf("topic %s has no key words", tp.ID)
		}
	}
}

func TestGroupVectorKeepsAPrefix(t *testing.T) {
	long := make([]float32, groupDims+10)
	long[0], long[groupDims+5] = 3, 4
	if short := groupVector(long); len(short) != groupDims || short[0] != 1 {
		t.Errorf("short = %v", short[:2])
	}
	small := []float32{0.6, 0.8}
	if got := groupVector(small); &got[0] != &small[0] {
		t.Error("a short vector was copied")
	}
	empty := make([]float32, groupDims+1)
	empty[groupDims] = 1
	if got := groupVector(empty); len(got) != groupDims+1 {
		t.Error("a vector with an empty prefix was cut")
	}
}

func TestTopicsHitExtendsTheBuild(t *testing.T) {
	ix, _, _ := newTestIndex(t, themeNotes(20))
	start := time.Now()
	ix.now = func() time.Time { return start }
	refresh(t, ix)
	m := topicsOf(t, ix, "", all)
	ix.now = func() time.Time { return start.Add(29 * time.Minute) }
	topicsOf(t, ix, "", all)
	ix.now = func() time.Time { return start.Add(31 * time.Minute) }
	if _, _, err := ix.Subtopics(m.Build, "1"); errors.Is(err, ErrStaleBuild) {
		t.Errorf("build handed out at minute 29 expired at minute 31: %v", err)
	}
}
