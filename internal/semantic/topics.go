package semantic

import (
	"cmp"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	mapTTL        = 30 * time.Minute
	groupDims     = 256
	maxMaps       = 16
	keyWordCount  = 5
	folderCount   = 3
	startCount    = 3
	closeCount    = 2
	minWordRunes  = 3
	commonSection = 0.5
)

// ErrTooFewSections means the scope holds too few indexed sections to group.
var ErrTooFewSections = errors.New("too few indexed notes in scope to group into topics")

// ErrStaleBuild means a zoom named a build that is unknown or has expired.
var ErrStaleBuild = errors.New("topic map build is unknown or older than 30 minutes")

// ErrTooSmallToSplit means a topic holds too few sections to split further.
var ErrTooSmallToSplit = errors.New("topic is too small to split")

// FolderCount is how many of a topic's notes sit in one folder.
type FolderCount struct {
	// Folder is the notes' parent folder, "/" for the vault root.
	Folder string
	Notes  int
}

// Topic is one group of sections close in meaning, described by its notes.
type Topic struct {
	// ID is "1", "2", ... at the top level and "4.1", "4.2" inside topic 4.
	ID        string
	Notes     int
	KeyWords  []string
	Folders   []FolderCount
	StartWith []string
	// CloseTo holds the IDs of the nearest topics at the same level.
	CloseTo []string
}

// TopicMap is one level of topics for a scope of a vault.
type TopicMap struct {
	Build string
	// Parent is the topic these are sub-topics of, "" at the top level.
	Parent string
	Topics []Topic
	// Notes counts notes with at least one section in scope, and
	// OutsideNotes those of them with no section in any topic.
	Notes        int
	OutsideNotes int
}

type topicCache struct {
	mu   sync.Mutex
	maps map[string]*builtMap
}

// builtMap is a cached TopicMap with what a zoom needs: the sections it was
// built from, so a later refresh cannot change it, and which sections each
// topic holds, in the order of topicMap.Topics.
type builtMap struct {
	topicMap TopicMap
	created  time.Time
	entries  []entry
	members  [][]int
	children map[string]*builtMap
}

// Topics groups the indexed sections whose note include accepts into
// topics. scope names what include selects, so each scope gets its own
// build. A map is built once per build and kept until mapTTL has passed
// since it was last handed out.
func (ix *Index) Topics(scope string, include func(string) bool) (TopicMap, Status, error) {
	entries, status := ix.snapshot()
	var kept []entry
	for _, e := range entries {
		if include(e.path) {
			kept = append(kept, e)
		}
	}
	if len(kept) < minGroupSections {
		return TopicMap{}, status, ErrTooFewSections
	}
	build := buildID(scope, kept)
	ix.topics.mu.Lock()
	defer ix.topics.mu.Unlock()
	ix.expireMaps()
	if b, ok := ix.topics.maps[build]; ok {
		b.created = ix.now()
		return b.topicMap, status, nil
	}
	b := buildMap(kept, build, "")
	if len(b.topicMap.Topics) == 0 {
		return TopicMap{}, status, ErrTooFewSections
	}
	b.created = ix.now()
	if ix.topics.maps == nil {
		ix.topics.maps = map[string]*builtMap{}
	}
	ix.topics.maps[build] = b
	ix.evictMaps()
	return b.topicMap, status, nil
}

// Subtopics splits one topic of a cached build into sub-topics. It answers
// from the sections the build was made from, so an edit since then does not
// change the answer; a build that has expired or was never made gives
// ErrStaleBuild.
func (ix *Index) Subtopics(build, topic string) (TopicMap, Status, error) {
	status := ix.currentStatus()
	ix.topics.mu.Lock()
	defer ix.topics.mu.Unlock()
	ix.expireMaps()
	node, ok := ix.topics.maps[build]
	if !ok {
		return TopicMap{}, status, fmt.Errorf("build %s: %w", build, ErrStaleBuild)
	}
	node.created = ix.now()
	parts := strings.Split(topic, ".")
	for depth := range parts {
		id := strings.Join(parts[:depth+1], ".")
		i := slices.IndexFunc(node.topicMap.Topics, func(t Topic) bool { return t.ID == id })
		if i < 0 {
			return TopicMap{}, status, fmt.Errorf("topic %s is not in build %s; its topics are %s", id, build, topicIDs(node.topicMap.Topics))
		}
		child, err := node.zoom(i)
		if err != nil {
			return TopicMap{}, status, fmt.Errorf("topic %s in build %s: %w", id, build, err)
		}
		node = child
	}
	return node.topicMap, status, nil
}

func (b *builtMap) zoom(i int) (*builtMap, error) {
	id := b.topicMap.Topics[i].ID
	if child, ok := b.children[id]; ok {
		return child, nil
	}
	if len(b.members[i]) < 2*minGroupSections {
		return nil, ErrTooSmallToSplit
	}
	sections := make([]entry, len(b.members[i]))
	for j, s := range b.members[i] {
		sections[j] = b.entries[s]
	}
	child := buildMap(sections, b.topicMap.Build, id)
	if len(child.topicMap.Topics) < 2 {
		return nil, ErrTooSmallToSplit
	}
	child.created = b.created
	if b.children == nil {
		b.children = map[string]*builtMap{}
	}
	b.children[id] = child
	return child, nil
}

func topicIDs(topics []Topic) string {
	ids := make([]string, len(topics))
	for i, t := range topics {
		ids[i] = t.ID
	}
	return strings.Join(ids, ", ")
}

func (ix *Index) expireMaps() {
	now := ix.now()
	for id, b := range ix.topics.maps {
		if now.Sub(b.created) >= mapTTL {
			delete(ix.topics.maps, id)
		}
	}
}

func (ix *Index) evictMaps() {
	for len(ix.topics.maps) > maxMaps {
		oldest := ""
		for id, b := range ix.topics.maps {
			if oldest == "" || b.created.Before(ix.topics.maps[oldest].created) {
				oldest = id
			}
		}
		delete(ix.topics.maps, oldest)
	}
}

// groupVector keeps the first groupDims numbers of a vector, renormalized.
// nomic-embed-text is trained so that a prefix of its vector still carries
// the meaning, and grouping on the shorter vector is about three times
// faster.
func groupVector(v []float32) []float32 {
	if len(v) <= groupDims {
		return v
	}
	short := slices.Clone(v[:groupDims])
	if normalize(short) != nil {
		return v
	}
	return short
}

func groupSeed(build, topic string) uint64 {
	sum := sha256.Sum256([]byte(build + "/" + topic))
	return binary.BigEndian.Uint64(sum[:8])
}

func buildMap(sections []entry, build, parent string) *builtMap {
	vecs := make([][]float32, len(sections))
	for i, e := range sections {
		vecs[i] = groupVector(e.vec)
	}
	g := group(vecs, topicCount(len(sections)), groupSeed(build, parent))
	topics, members := describe(sections, g, parent)
	notes := map[string]bool{}
	inside := map[string]bool{}
	for _, e := range sections {
		notes[e.path] = true
	}
	for _, m := range members {
		for _, s := range m {
			inside[sections[s].path] = true
		}
	}
	return &builtMap{
		topicMap: TopicMap{Build: build, Parent: parent, Topics: topics, Notes: len(notes), OutsideNotes: len(notes) - len(inside)},
		entries:  sections,
		members:  members,
	}
}

type draft struct {
	topic   Topic
	center  []float32
	members []int
}

// describe turns a grouping into topics, dropping groups with fewer than
// minTopicNotes notes, and returns each topic's sections in topic order.
func describe(sections []entry, g grouping, parent string) ([]Topic, [][]int) {
	perNote := map[string]int{}
	for _, e := range sections {
		perNote[e.path]++
	}
	byGroup := make([][]int, len(g.centers))
	for i, c := range g.assign {
		if c >= 0 {
			byGroup[c] = append(byGroup[c], i)
		}
	}
	var drafts []*draft
	for c, members := range byGroup {
		if len(notesOf(sections, members)) >= minTopicNotes {
			drafts = append(drafts, &draft{center: g.centers[c], members: members})
		}
	}
	words := keyWords(sections, drafts)
	for i, d := range drafts {
		d.topic = Topic{
			Notes:     len(notesOf(sections, d.members)),
			KeyWords:  words[i],
			Folders:   folders(sections, d.members),
			StartWith: startWith(sections, d.members, g.margin, perNote),
		}
	}
	slices.SortStableFunc(drafts, func(a, b *draft) int {
		if c := cmp.Compare(b.topic.Notes, a.topic.Notes); c != 0 {
			return c
		}
		return slices.Compare(a.topic.KeyWords, b.topic.KeyWords)
	})
	for i, d := range drafts {
		d.topic.ID = strconv.Itoa(i + 1)
		if parent != "" {
			d.topic.ID = parent + "." + d.topic.ID
		}
	}
	topics := make([]Topic, len(drafts))
	members := make([][]int, len(drafts))
	for i, d := range drafts {
		d.topic.CloseTo = closeTo(drafts, i)
		topics[i], members[i] = d.topic, d.members
	}
	return topics, members
}

func notesOf(sections []entry, members []int) map[string]int {
	notes := map[string]int{}
	for _, s := range members {
		notes[sections[s].path]++
	}
	return notes
}

// keyWords picks each topic's most typical words by class-based TF-IDF:
// how much of the topic's text a word makes up, weighted down by how often
// it appears in every topic together. Words held by more than half the
// sections come after every other word, and words found in only one note of
// the topic after those; words under minWordRunes and numbers are skipped.
// So no word list is needed, and a tiny topic still gets words.
func keyWords(sections []entry, drafts []*draft) [][]string {
	sectionWords := make([][]string, len(sections))
	held := map[string]int{}
	for i, e := range sections {
		seen := map[string]bool{}
		for _, w := range tokens(noteTitle(e.path) + " " + strings.Join(e.chunk.HeadingPath, " ") + " " + e.chunk.Text) {
			sectionWords[i] = append(sectionWords[i], w)
			if !seen[w] {
				seen[w] = true
				held[w]++
			}
		}
	}
	counts := make([]map[string]int, len(drafts))
	noteSets := make([]map[string]map[string]bool, len(drafts))
	totals := make([]int, len(drafts))
	global := map[string]int{}
	all := 0
	for i, d := range drafts {
		counts[i], noteSets[i] = map[string]int{}, map[string]map[string]bool{}
		for _, s := range d.members {
			for _, w := range sectionWords[s] {
				counts[i][w]++
				global[w]++
				totals[i]++
				all++
				if noteSets[i][w] == nil {
					noteSets[i][w] = map[string]bool{}
				}
				noteSets[i][w][sections[s].path] = true
			}
		}
	}
	avg := float64(all) / float64(max(1, len(drafts)))
	out := make([][]string, len(drafts))
	for i := range drafts {
		type scored struct {
			word  string
			tier  int
			score float64
		}
		var list []scored
		for w, n := range counts[i] {
			if !keyWordCandidate(w) {
				continue
			}
			tier := 0
			if float64(held[w]) > commonSection*float64(len(sections)) {
				tier = 1
			}
			if len(noteSets[i][w]) < 2 {
				tier = 2
			}
			score := float64(n) / float64(totals[i]) * math.Log(1+avg/float64(global[w]))
			list = append(list, scored{w, tier, score})
		}
		slices.SortFunc(list, func(a, b scored) int {
			if c := cmp.Compare(a.tier, b.tier); c != 0 {
				return c
			}
			if c := cmp.Compare(b.score, a.score); c != 0 {
				return c
			}
			return cmp.Compare(a.word, b.word)
		})
		for _, s := range list[:min(keyWordCount, len(list))] {
			out[i] = append(out[i], s.word)
		}
	}
	return out
}

func keyWordCandidate(w string) bool {
	if utf8.RuneCountInString(w) < minWordRunes {
		return false
	}
	for _, r := range w {
		if !unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func folders(sections []entry, members []int) []FolderCount {
	count := map[string]int{}
	for p := range notesOf(sections, members) {
		dir := path.Dir(p)
		if dir == "." {
			dir = "/"
		}
		count[dir]++
	}
	out := make([]FolderCount, 0, len(count))
	for f, n := range count {
		out = append(out, FolderCount{Folder: f, Notes: n})
	}
	slices.SortFunc(out, func(a, b FolderCount) int {
		if c := cmp.Compare(b.Notes, a.Notes); c != 0 {
			return c
		}
		return cmp.Compare(a.Folder, b.Folder)
	})
	return out[:min(folderCount, len(out))]
}

// startWith ranks a topic's notes by how much of each note is in the topic
// times how clearly those sections belong to it rather than to the next
// topic, so a note that only touches the topic, such as a daily note whose
// short headings were joined into one mixed section, does not lead it.
func startWith(sections []entry, members []int, margin []float32, perNote map[string]int) []string {
	sum := map[string]float64{}
	in := notesOf(sections, members)
	for _, s := range members {
		sum[sections[s].path] += float64(margin[s])
	}
	type scored struct {
		path  string
		score float64
	}
	list := make([]scored, 0, len(in))
	for p, n := range in {
		list = append(list, scored{p, float64(n) / float64(perNote[p]) * sum[p] / float64(n)})
	}
	slices.SortFunc(list, func(a, b scored) int {
		if c := cmp.Compare(b.score, a.score); c != 0 {
			return c
		}
		return cmp.Compare(a.path, b.path)
	})
	out := make([]string, 0, startCount)
	for _, s := range list[:min(startCount, len(list))] {
		out = append(out, s.path)
	}
	return out
}

func closeTo(drafts []*draft, i int) []string {
	type scored struct {
		id    string
		score float32
	}
	var list []scored
	for j, d := range drafts {
		if j != i {
			list = append(list, scored{d.topic.ID, dot(drafts[i].center, d.center)})
		}
	}
	slices.SortFunc(list, func(a, b scored) int {
		if c := cmp.Compare(b.score, a.score); c != 0 {
			return c
		}
		return cmp.Compare(a.id, b.id)
	})
	out := []string{}
	for _, s := range list[:min(closeCount, len(list))] {
		out = append(out, s.id)
	}
	return out
}
