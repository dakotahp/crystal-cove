package semantic

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func subthemeNotes() map[string]string {
	notes := themeNotes(20)
	for p := range notes {
		if themeOf(p) == "Garden" {
			delete(notes, p)
		}
	}
	veg := []string{"tomato", "zucchini", "carrot", "lettuce", "pepper", "squash", "onion", "potato"}
	flower := []string{"rose", "tulip", "dahlia", "peony", "lily", "orchid", "daisy", "violet"}
	for i := range 10 {
		pick := func(vocab []string) string {
			return strings.Join([]string{vocab[i%8], vocab[(i+1)%8], vocab[(i+2)%8], vocab[(i+3)%8], vocab[(i+5)%8]}, " ")
		}
		shared := "soil watering compost garden "
		notes[fmt.Sprintf("Garden/v%02d.md", i)] = shared + pick(veg) + "\n"
		notes[fmt.Sprintf("Garden/f%02d.md", i)] = shared + pick(flower) + "\n"
	}
	return notes
}

func TestSubtopicsSplitATopic(t *testing.T) {
	ix, _, _ := newTestIndex(t, subthemeNotes())
	refresh(t, ix)
	m := topicsOf(t, ix, "", all)
	garden := gardenTopic(m)
	sub, _, err := ix.Subtopics(m.Build, garden.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sub.Build != m.Build || sub.Parent != garden.ID || len(sub.Topics) != 2 || sub.Notes != garden.Notes {
		t.Fatalf("sub = %+v", sub)
	}
	kinds := map[byte]bool{}
	for i, tp := range sub.Topics {
		if tp.ID != fmt.Sprintf("%s.%d", garden.ID, i+1) {
			t.Errorf("sub-topic id %q", tp.ID)
		}
		kind := tp.StartWith[0][len("Garden/")]
		for _, p := range tp.StartWith {
			if p[len("Garden/")] != kind {
				t.Errorf("sub-topic %s mixes %v", tp.ID, tp.StartWith)
			}
		}
		kinds[kind] = true
	}
	if !kinds['v'] || !kinds['f'] {
		t.Errorf("sub-topics did not split vegetables from flowers: %+v", sub.Topics)
	}
	deeper, _, err := ix.Subtopics(m.Build, sub.Topics[0].ID)
	if err != nil && !errors.Is(err, ErrTooSmallToSplit) {
		t.Fatalf("zooming deeper: %v", err)
	}
	for _, tp := range deeper.Topics {
		if !strings.HasPrefix(tp.ID, sub.Topics[0].ID+".") {
			t.Errorf("deeper id %q", tp.ID)
		}
	}
	again, _, err := ix.Subtopics(m.Build, garden.ID)
	if err != nil || !reflect.DeepEqual(again, sub) {
		t.Errorf("second zoom differs: %v %+v", err, again)
	}
}

func TestSubtopicsSurviveEdits(t *testing.T) {
	ix, _, v := newTestIndex(t, subthemeNotes())
	start := time.Now()
	ix.now = func() time.Time { return start }
	refresh(t, ix)
	m := topicsOf(t, ix, "", all)
	garden := gardenTopic(m)
	writeNote(t, v.Root(), "Garden/v00.md", "soil watering compost garden tomato changed\n")
	refresh(t, ix)
	if _, _, err := ix.Subtopics(m.Build, garden.ID); err != nil {
		t.Fatalf("zoom after an edit: %v", err)
	}
	ix.now = func() time.Time { return start.Add(31 * time.Minute) }
	if _, _, err := ix.Subtopics(m.Build, garden.ID); !errors.Is(err, ErrStaleBuild) {
		t.Errorf("zoom after 31 minutes: %v", err)
	}
}

func TestSubtopicsErrors(t *testing.T) {
	ix, _, _ := newTestIndex(t, themeNotes(3))
	refresh(t, ix)
	m := topicsOf(t, ix, "", all)
	if _, _, err := ix.Subtopics("nope", "1"); !errors.Is(err, ErrStaleBuild) {
		t.Errorf("unknown build: %v", err)
	}
	if _, _, err := ix.Subtopics(m.Build, "99"); err == nil || !strings.Contains(err.Error(), "1, 2") {
		t.Errorf("unknown topic: %v", err)
	}
	if _, _, err := ix.Subtopics(m.Build, "1.7"); err == nil || errors.Is(err, ErrStaleBuild) {
		t.Errorf("unknown sub-topic: %v", err)
	}
	if _, _, err := ix.Subtopics(m.Build, "1"); !errors.Is(err, ErrTooSmallToSplit) {
		t.Errorf("small topic: %v", err)
	}

	same := make([]entry, 9)
	members := make([]int, 9)
	for i := range same {
		same[i] = entry{path: fmt.Sprintf("S/s%d.md", i), chunk: Chunk{Text: "alpha"}, vec: axis(4, 0)}
		members[i] = i
	}
	ix.topics.mu.Lock()
	ix.topics.maps["alike"] = &builtMap{
		topicMap: TopicMap{Build: "alike", Topics: []Topic{{ID: "1"}}},
		created:  ix.now(),
		entries:  same,
		members:  [][]int{members},
	}
	ix.topics.mu.Unlock()
	if _, _, err := ix.Subtopics("alike", "1"); !errors.Is(err, ErrTooSmallToSplit) {
		t.Errorf("alike sections: %v", err)
	}
}

func TestTopicCacheEvictsOldest(t *testing.T) {
	ix, _, _ := newTestIndex(t, themeNotes(20))
	clock := time.Now()
	ix.now = func() time.Time { clock = clock.Add(time.Second); return clock }
	refresh(t, ix)
	var builds []string
	for i := range maxMaps + 1 {
		builds = append(builds, topicsOf(t, ix, fmt.Sprint(i), all).Build)
	}
	if _, _, err := ix.Subtopics(builds[0], "1"); !errors.Is(err, ErrStaleBuild) {
		t.Errorf("oldest build kept: %v", err)
	}
	if _, _, err := ix.Subtopics(builds[maxMaps], "1"); err != nil {
		t.Errorf("newest build: %v", err)
	}
}
