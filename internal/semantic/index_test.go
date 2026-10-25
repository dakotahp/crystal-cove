package semantic

import (
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dakotahp/crystal-cove/internal/vault"
)

// fakeEmbedder turns text into a bag-of-words vector, so texts that share
// words score close together.
type fakeEmbedder struct {
	mu        sync.Mutex
	inputs    []string
	calls     int
	failAfter int
	notReady  int
	onCall    func(call int)
}

func (f *fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.onCall != nil {
		f.onCall(f.calls)
	}
	if f.failAfter > 0 && f.calls > f.failAfter {
		return nil, errors.New("embedding server gone")
	}
	f.inputs = append(f.inputs, texts...)
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = bagOfWords(t)
	}
	return out, nil
}

func (f *fakeEmbedder) Ready(context.Context) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.notReady > 0 {
		f.notReady--
		return false
	}
	return true
}

func (f *fakeEmbedder) seen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.inputs)
}

func bagOfWords(text string) []float32 {
	text = strings.TrimPrefix(strings.TrimPrefix(text, documentPrefix), queryPrefix)
	v := make([]float32, 64)
	for _, w := range strings.Fields(strings.ToLower(text)) {
		h := fnv.New32a()
		h.Write([]byte(strings.Trim(w, ".,:#>*")))
		v[h.Sum32()%64]++
	}
	v[63] += 0.01
	normalize(v)
	return v
}

func newTestIndex(t *testing.T, notes map[string]string) (*Index, *fakeEmbedder, *vault.Vault) {
	t.Helper()
	dir := t.TempDir()
	for name, body := range notes {
		writeNote(t, dir, name, body)
	}
	v := vault.New("test", dir)
	f := &fakeEmbedder{}
	return NewIndex(v, f, filepath.Join(t.TempDir(), "index.gob"), quietLog()), f, v
}

func writeNote(t *testing.T, root, name, body string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

var gardenNotes = map[string]string{
	"Garden.md":          "# Plants\ntomatoes basil watering soil compost\n",
	"Notes/Seedlings.md": "# Spring\nseedlings tomatoes soil trays watering\n",
	"Budget.md":          "# Money\nsavings income expenses rent budget\n",
}

func TestIndexSearchRanksByMeaning(t *testing.T) {
	ix, f, _ := newTestIndex(t, gardenNotes)
	if err := ix.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	hits, status, err := ix.searchMeaning(context.Background(), "watering tomatoes")
	if err != nil {
		t.Fatal(err)
	}
	if want := (Status{NotesIndexed: 3, NotesTotal: 3, Complete: true}); status != want {
		t.Errorf("status = %+v, want %+v", status, want)
	}
	if len(hits) != 3 || hits[2].Path != "Budget.md" {
		t.Fatalf("hits = %+v, want the budget note last", hits)
	}
	if hits[0].Path != "Garden.md" || !reflect.DeepEqual(hits[0].HeadingPath, []string{"Plants"}) {
		t.Errorf("best hit = %+v, want Garden.md under Plants", hits[0])
	}
	if hits[0].Score < hits[1].Score || hits[1].Score < hits[2].Score {
		t.Errorf("hits not ordered by score: %+v", hits)
	}
	if !strings.Contains(hits[0].Text, "compost") {
		t.Errorf("hit text = %q", hits[0].Text)
	}
	inputs := f.seen()
	if !slices.Contains(inputs, documentPrefix+"Garden\nPlants\ntomatoes basil watering soil compost") {
		t.Errorf("documents lack the prefix, title, or heading: %q", inputs)
	}
	if inputs[len(inputs)-1] != queryPrefix+"watering tomatoes" {
		t.Errorf("query sent as %q", inputs[len(inputs)-1])
	}
}

func TestIndexSearchBeforeAnyRefresh(t *testing.T) {
	ix, f, _ := newTestIndex(t, gardenNotes)
	hits, status, err := ix.searchMeaning(context.Background(), "tomatoes")
	if err != nil || hits != nil || status.Complete {
		t.Errorf("Search on an empty index = %v, %+v, %v", hits, status, err)
	}
	if f.calls != 0 {
		t.Errorf("embedded a query with nothing to compare it to")
	}
}

func TestIndexSearchQueryError(t *testing.T) {
	ix, f, _ := newTestIndex(t, gardenNotes)
	if err := ix.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.failAfter = f.calls
	if _, _, err := ix.searchMeaning(context.Background(), "tomatoes"); err == nil || !strings.Contains(err.Error(), "query") {
		t.Errorf("err = %v, want a query embedding error", err)
	}
}

func TestIndexRelated(t *testing.T) {
	ix, _, _ := newTestIndex(t, gardenNotes)
	if err := ix.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	matches, status, err := ix.Related("Garden.md")
	if err != nil {
		t.Fatal(err)
	}
	if !status.Complete {
		t.Errorf("status = %+v", status)
	}
	if len(matches) != 2 || matches[0].Path != "Notes/Seedlings.md" || matches[1].Path != "Budget.md" {
		t.Fatalf("matches = %+v, want Seedlings then Budget", matches)
	}
	m := matches[0]
	if !reflect.DeepEqual(m.HeadingPath, []string{"Spring"}) || !reflect.DeepEqual(m.FromHeadingPath, []string{"Plants"}) {
		t.Errorf("match sections = %q from %q", m.HeadingPath, m.FromHeadingPath)
	}
	if _, _, err := ix.Related("Missing.md"); err == nil || !strings.Contains(err.Error(), "Missing.md") {
		t.Errorf("err = %v, want one naming the note", err)
	}
}

func TestIndexRelatedPicksBestSectionPair(t *testing.T) {
	ix, _, _ := newTestIndex(t, map[string]string{
		"Mixed.md": "# Food\n" + strings.Repeat("recipes cooking dinner ", 30) + "\n# Garden\n" + strings.Repeat("tomatoes soil watering ", 30) + "\n",
		"Plot.md":  "# Beds\ntomatoes soil watering compost\n",
		"Diet.md":  "# Meals\nrecipes dinner cooking lunch\n",
	})
	if err := ix.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	matches, _, err := ix.Related("Mixed.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range matches {
		want := map[string]string{"Plot.md": "Garden", "Diet.md": "Food"}[m.Path]
		if !reflect.DeepEqual(m.FromHeadingPath, []string{want}) {
			t.Errorf("%s matched from %q, want %q", m.Path, m.FromHeadingPath, want)
		}
	}
}

func TestIndexRefreshEmbedsOnlyChanges(t *testing.T) {
	ix, f, v := newTestIndex(t, gardenNotes)
	ctx := context.Background()
	if err := ix.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	first := len(f.seen())
	if err := ix.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if got := len(f.seen()); got != first {
		t.Errorf("unchanged vault embedded %d more texts", got-first)
	}

	writeNote(t, v.Root(), "Budget.md", "# Money\nsavings pension retirement\n")
	if err := os.Remove(filepath.Join(v.Root(), "Garden.md")); err != nil {
		t.Fatal(err)
	}
	if err := ix.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if got := f.seen()[first:]; len(got) != 1 || !strings.Contains(got[0], "pension") {
		t.Errorf("after one edit embedded %q", got)
	}
	if len(ix.cache) != 2 {
		t.Errorf("cache holds %d vectors, want the deleted note's dropped", len(ix.cache))
	}
	hits, status, _ := ix.searchMeaning(ctx, "tomatoes")
	if status.NotesTotal != 2 || len(hits) != 2 {
		t.Errorf("after delete: %d hits, status %+v", len(hits), status)
	}
}

func TestIndexIgnoresBoilerplate(t *testing.T) {
	notes := map[string]string{}
	for i := range 12 {
		notes[fmt.Sprintf("Daily/%02d.md", i)] = "> **Quote of the Day**\n# Log\nentry number " + strings.Repeat("x", i) + "\n"
	}
	ix, f, _ := newTestIndex(t, notes)
	if err := ix.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, in := range f.seen() {
		if strings.Contains(in, "Quote of the Day") {
			t.Fatalf("template line embedded: %q", in)
		}
	}
}

func TestIndexSkipsEmptyNotesButCountsThem(t *testing.T) {
	ix, _, _ := newTestIndex(t, map[string]string{"Empty.md": "---\ntags: [x]\n---\n", "Full.md": "words here\n"})
	if err := ix.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, status, _ := ix.searchMeaning(context.Background(), "words")
	if want := (Status{NotesIndexed: 2, NotesTotal: 2, Complete: true}); status != want {
		t.Errorf("status = %+v, want %+v", status, want)
	}
}

func TestIndexPersistsAcrossRestarts(t *testing.T) {
	ix, _, v := newTestIndex(t, gardenNotes)
	if err := ix.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	f2 := &fakeEmbedder{}
	again := NewIndex(v, f2, ix.store, quietLog())
	again.load()
	if err := again.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(f2.seen()) != 0 {
		t.Errorf("restart re-embedded %d texts", len(f2.seen()))
	}
	if info, err := os.Stat(ix.store); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("store file: %v, %v", info, err)
	}
}

func TestIndexLoadIgnoresBadStores(t *testing.T) {
	cases := map[string]func(t *testing.T, path string){
		"missing": func(*testing.T, string) {},
		"corrupt": func(t *testing.T, p string) {
			if err := os.WriteFile(p, []byte("not gob"), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"other model": func(t *testing.T, p string) {
			f, err := os.Create(p)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			if err := gob.NewEncoder(f).Encode(storeFile{Model: "other", Vectors: map[string][]float32{"k": {1}}}); err != nil {
				t.Fatal(err)
			}
		},
		"unreadable": func(t *testing.T, p string) {
			if err := os.WriteFile(p, []byte("x"), 0); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			ix, _, _ := newTestIndex(t, nil)
			setup(t, ix.store)
			ix.load()
			if len(ix.cache) != 0 {
				t.Errorf("loaded %d vectors", len(ix.cache))
			}
		})
	}
}

func TestIndexRefreshErrors(t *testing.T) {
	t.Run("vault missing", func(t *testing.T) {
		ix := NewIndex(vault.New("gone", filepath.Join(t.TempDir(), "nope")), &fakeEmbedder{}, filepath.Join(t.TempDir(), "s"), quietLog())
		if err := ix.Refresh(context.Background()); err == nil {
			t.Error("no error for a missing vault")
		}
	})
	t.Run("store not writable", func(t *testing.T) {
		ix, _, _ := newTestIndex(t, gardenNotes)
		blocker := filepath.Join(t.TempDir(), "file")
		writeNote(t, filepath.Dir(blocker), "file", "")
		ix.store = filepath.Join(blocker, "index.gob")
		if err := ix.Refresh(context.Background()); err == nil {
			t.Error("no error when the store cannot be written")
		}
	})
}

func TestIndexSkipsUnreadableNotes(t *testing.T) {
	ix, _, v := newTestIndex(t, gardenNotes)
	locked := filepath.Join(v.Root(), "Budget.md")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o644) })
	if _, err := os.ReadFile(locked); err == nil {
		t.Skip("running with permission to read any file")
	}
	if err := ix.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, status, _ := ix.searchMeaning(context.Background(), "x"); status.NotesTotal != 2 {
		t.Errorf("status = %+v, want the unreadable note left out", status)
	}
}

func TestIndexEmbedFailureKeepsProgress(t *testing.T) {
	notes := map[string]string{}
	for i := range batchSize * 3 {
		notes[fmt.Sprintf("n%03d.md", i)] = fmt.Sprintf("note number %d about topic %d\n", i, i%5)
	}
	ix, f, _ := newTestIndex(t, notes)
	f.failAfter = 1
	err := ix.Refresh(context.Background())
	if err == nil {
		t.Fatal("no error when the embedding server fails")
	}
	_, status, _ := ix.searchMeaning(context.Background(), "")
	if status.NotesIndexed != batchSize || status.NotesTotal != batchSize*3 || status.Complete {
		t.Errorf("status = %+v, want the first batch kept", status)
	}
	if info, err := os.Stat(ix.store); err != nil || info.Size() == 0 {
		t.Errorf("progress not saved: %v", err)
	}
}

func TestIndexPublishesDuringLongBuilds(t *testing.T) {
	notes := map[string]string{}
	for i := range publishEvery + batchSize {
		notes[fmt.Sprintf("n%04d.md", i)] = fmt.Sprintf("note %d\n", i)
	}
	ix, f, _ := newTestIndex(t, notes)
	var during Status
	f.onCall = func(call int) {
		if call == publishEvery/batchSize+1 {
			during = ix.currentStatus()
		}
	}
	if err := ix.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if during.NotesIndexed != publishEvery || during.Complete {
		t.Errorf("status during the build = %+v, want %d notes published", during, publishEvery)
	}
}

func TestIndexRun(t *testing.T) {
	ix, f, _ := newTestIndex(t, gardenNotes)
	f.notReady = 1
	ctx, cancel := context.WithCancel(context.Background())
	var sleeps []time.Duration
	ix.sleep = func(_ context.Context, d time.Duration) {
		sleeps = append(sleeps, d)
		if len(sleeps) == 2 {
			f.failAfter = f.calls
			writeNote(t, ix.vault.Root(), "New.md", "fresh words\n")
		}
		if len(sleeps) == 3 {
			cancel()
		}
	}
	ix.Run(ctx)
	if want := []time.Duration{readyPoll, refreshInterval, retryDelay}; !slices.Equal(sleeps, want) {
		t.Errorf("sleeps = %v, want %v", sleeps, want)
	}
	if _, status, _ := ix.searchMeaning(context.Background(), ""); status.NotesIndexed != 3 {
		t.Errorf("status = %+v", status)
	}
}

func TestIndexRunStopsWhenCancelledDuringRefresh(t *testing.T) {
	ix, f, _ := newTestIndex(t, gardenNotes)
	ctx, cancel := context.WithCancel(context.Background())
	ix.embed = cancelOnEmbed{f, cancel}
	ix.sleep = func(context.Context, time.Duration) { t.Error("slept after cancel") }
	ix.Run(ctx)
}

type cancelOnEmbed struct {
	*fakeEmbedder
	cancel func()
}

func (c cancelOnEmbed) Embed(context.Context, []string) ([][]float32, error) {
	c.cancel()
	return nil, context.Canceled
}

func TestStorePathForVault(t *testing.T) {
	if got := StorePath("/data", "Work/Notes"); got != filepath.Join("/data", "Work%2FNotes.gob") {
		t.Errorf("StorePath = %q", got)
	}
}
