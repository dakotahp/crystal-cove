package semantic

import (
	"fmt"
	"math"
	"path"
	"slices"
	"strings"
	"testing"
)

func TestKeyWordsSkipWordsCommonAcrossTheVault(t *testing.T) {
	notes := themeNotes(20)
	for p, body := range notes {
		if themeOf(p) == "Garden" {
			notes[p] = strings.TrimSpace(body) + " you you you your\n"
		}
	}
	for i := range 200 {
		notes[fmt.Sprintf("Journal/j%03d.md", i)] = fmt.Sprintf("you your was have this today entry%d\n", i)
	}
	ix, _, _ := newTestIndex(t, notes)
	refresh(t, ix)
	m := topicsOf(t, ix, "-Journal", func(p string) bool { return !strings.HasPrefix(p, "Journal/") })
	garden := gardenTopic(m)
	if garden.ID == "" {
		t.Fatalf("no garden topic: %+v", m.Topics)
	}
	for _, w := range garden.KeyWords {
		if w == "you" || w == "your" {
			t.Errorf("garden key words %v hold a word common across the vault", garden.KeyWords)
		}
	}
}

func TestKeyWordsSkipWordsInFewNotesOfATopic(t *testing.T) {
	notes := themeNotes(20)
	names := []string{"bryce", "claire", "morgan", "priya", "tobias", "wendell"}
	for i := range 18 {
		p := fmt.Sprintf("Garden/g%02d.md", i)
		name := names[i/3]
		notes[p] = strings.TrimSpace(notes[p]) + strings.Repeat(" "+name, 4) + "\n"
	}
	for i := range 100 {
		g := themes["Garden"]
		notes[fmt.Sprintf("Misc/x%03d.md", i)] = fmt.Sprintf("%s %s misc%d\n", g[i%12], g[(i+5)%12], i)
	}
	ix, _, _ := newTestIndex(t, notes)
	refresh(t, ix)
	garden := gardenTopic(topicsOf(t, ix, "-Misc", func(p string) bool { return !strings.HasPrefix(p, "Misc/") }))
	if garden.ID == "" {
		t.Fatal("no garden topic")
	}
	for _, w := range garden.KeyWords {
		if slices.Contains(names, w) {
			t.Errorf("garden key words %v hold a name found in 3 of 20 notes", garden.KeyWords)
		}
	}
}

func TestKeyWordsNeedTwiceTheVaultRate(t *testing.T) {
	notes := themeNotes(20)
	for p, body := range notes {
		if themeOf(p) == "Garden" {
			notes[p] = strings.TrimSpace(body) + strings.Repeat(" someone", 6) + "\n"
		}
	}
	for i := range 200 {
		notes[fmt.Sprintf("Journal/j%03d.md", i)] = fmt.Sprintf("someone today entry%d note\n", i)
	}
	ix, _, _ := newTestIndex(t, notes)
	refresh(t, ix)
	garden := gardenTopic(topicsOf(t, ix, "-Journal", func(p string) bool { return !strings.HasPrefix(p, "Journal/") }))
	if garden.ID == "" || slices.Contains(garden.KeyWords, "someone") {
		t.Errorf("garden key words %v hold a word under twice its vault rate", garden.KeyWords)
	}
}

func TestKeyWordsSkipLinks(t *testing.T) {
	notes := themeNotes(20)
	for i := range 10 {
		p := fmt.Sprintf("Garden/g%02d.md", i)
		notes[p] = strings.TrimSpace(notes[p]) + fmt.Sprintf(" see https://www.seedvendor.com/p/%d and www.seedvendor.com\n", i)
	}
	ix, _, _ := newTestIndex(t, notes)
	refresh(t, ix)
	garden := gardenTopic(topicsOf(t, ix, "", all))
	for _, w := range garden.KeyWords {
		if slices.Contains([]string{"https", "www", "seedvendor", "com"}, w) {
			t.Errorf("garden key words %v hold a piece of a link", garden.KeyWords)
		}
	}
}

func TestStartWithSkipsAnEditedCopy(t *testing.T) {
	v2 := []float32{0.94, float32(math.Sqrt(1 - 0.94*0.94)), 0}
	sections := []entry{
		{path: "Letters/Letter.md", vec: []float32{1, 0, 0}},
		{path: "Letters/Letter v2.md", vec: v2},
		{path: "Letters/Other.md", vec: []float32{0.89, 0, float32(math.Sqrt(1 - 0.89*0.89))}},
	}
	got := startWith(sections, []int{0, 1, 2}, []float32{0.9, 0.8, 0.7}, map[string]int{"Letters/Letter.md": 1, "Letters/Letter v2.md": 1, "Letters/Other.md": 1})
	if want := []string{"Letters/Letter.md", "Letters/Other.md"}; !slices.Equal(got, want) {
		t.Errorf("start_with = %v, want %v", got, want)
	}
}

func TestStartWithSkipsNearCopies(t *testing.T) {
	notes := themeNotes(20)
	best := strings.Repeat(strings.Join(themes["Garden"], " ")+" ", 2)
	notes["Garden/Cover.md"] = best + "\n"
	notes["Garden/Cover v2.md"] = best + "revised\n"
	ix, _, _ := newTestIndex(t, notes)
	refresh(t, ix)
	garden := gardenTopic(topicsOf(t, ix, "", all))
	copies := 0
	for _, p := range garden.StartWith {
		if strings.HasPrefix(path.Base(p), "Cover") {
			copies++
		}
	}
	if copies != 1 {
		t.Errorf("start_with holds %d copies of Cover: %v", copies, garden.StartWith)
	}
}

func TestStartWithListsEachNoteNameOnce(t *testing.T) {
	notes := themeNotes(20)
	best := strings.Repeat(strings.Join(themes["Garden"], " ")+" ", 2) + "\n"
	notes["Garden/Best.md"] = best
	notes["Archive/Best.md"] = best
	ix, _, _ := newTestIndex(t, notes)
	refresh(t, ix)
	garden := gardenTopic(topicsOf(t, ix, "", all))
	var names []string
	for _, p := range garden.StartWith {
		names = append(names, path.Base(p))
	}
	if !slices.Contains(names, "Best.md") {
		t.Fatalf("Best.md is not an entry note: %v", garden.StartWith)
	}
	slices.Sort(names)
	if len(slices.Compact(names)) != len(garden.StartWith) {
		t.Errorf("start_with repeats a note name: %v", garden.StartWith)
	}
}
