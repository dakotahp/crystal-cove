package semantic

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/gob"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/dakotahp/crystal-cove/internal/vault"
)

const (
	// The model was trained to tell documents from queries by these prefixes.
	documentPrefix = "search_document: "
	queryPrefix    = "search_query: "
	// ModelName identifies the vectors in a saved index; a different model's
	// vectors cannot be compared, so a saved index for another is rebuilt.
	ModelName = "nomic-embed-text-v1.5"

	batchSize       = 16
	publishEvery    = 512
	readyPoll       = 2 * time.Second
	refreshInterval = 5 * time.Minute
	retryDelay      = 15 * time.Second
)

// Embedder turns texts into unit-length vectors.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Ready(ctx context.Context) bool
}

// Status says how much of a vault the index covers.
type Status struct {
	NotesIndexed int  `json:"notes_indexed"`
	NotesTotal   int  `json:"notes_total"`
	Complete     bool `json:"complete" jsonschema:"false while notes are still being indexed, so results may miss them"`
}

// Hit is a note's best-matching chunk.
type Hit struct {
	Path        string
	HeadingPath []string
	Text        string
	Score       float32
	pieces      []piece
}

// Match is a note related to another, with the pair of chunks that matched.
type Match struct {
	Hit
	// FromHeadingPath is the section of the note asked about that matched.
	FromHeadingPath []string
}

// Index keeps one vault's chunk vectors in memory and up to date.
type Index struct {
	vault *vault.Vault
	embed Embedder
	store string
	log   *slog.Logger
	sleep func(ctx context.Context, d time.Duration)

	// cache maps a document's hash to its vector. Only the goroutine
	// running Run touches it.
	cache map[string][]float32

	mu      sync.RWMutex
	entries []entry
	status  Status
	terms   *termIndex
}

type entry struct {
	path  string
	chunk Chunk
	vec   []float32
}

type noteChunks struct {
	path   string
	chunks []Chunk
	keys   []string
}

type storeFile struct {
	Model   string
	Vectors map[string][]float32
}

// NewIndex returns an Index over v that saves its vectors at store.
func NewIndex(v *vault.Vault, e Embedder, store string, log *slog.Logger) *Index {
	return &Index{vault: v, embed: e, store: store, log: log.With("vault", v.Name()), sleep: sleepCtx, cache: map[string][]float32{}}
}

// StorePath returns where a vault's index is saved inside dir.
func StorePath(dir, vaultName string) string {
	return filepath.Join(dir, url.PathEscape(vaultName)+".gob")
}

func sleepCtx(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// Run loads the saved index, then keeps it in step with the vault until ctx
// ends, embedding only chunks it has not seen before.
func (ix *Index) Run(ctx context.Context) {
	ix.load()
	for ctx.Err() == nil {
		if !ix.embed.Ready(ctx) {
			ix.sleep(ctx, readyPoll)
			continue
		}
		delay := refreshInterval
		start := time.Now()
		if err := ix.Refresh(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			ix.log.Warn("semantic index refresh failed", "error", err, "retry", retryDelay.String())
			delay = retryDelay
		} else {
			s := ix.currentStatus()
			ix.log.Debug("semantic index refreshed", "notes", s.NotesTotal, "took", time.Since(start).String())
		}
		ix.sleep(ctx, delay)
	}
}

// Refresh brings the index in step with the vault once, embedding only
// chunks it has not seen before and dropping vectors no note uses.
func (ix *Index) Refresh(ctx context.Context) error {
	paths, err := ix.vault.Notes()
	if err != nil {
		return err
	}
	var counts lineCounts
	sources := make([][]byte, len(paths))
	for i, p := range paths {
		if sources[i], err = ix.vault.ReadAll(p); err == nil {
			counts.add(sources[i])
		}
	}
	boilerplate := counts.boilerplate(len(paths))

	var notes []noteChunks
	var queue []string
	queued := map[string]string{}
	for i, p := range paths {
		if sources[i] == nil {
			continue
		}
		n := noteChunks{path: p, chunks: chunkNote(sources[i], boilerplate)}
		title := strings.TrimSuffix(path.Base(p), path.Ext(p))
		for _, c := range n.chunks {
			doc := documentText(title, c)
			key := docKey(doc)
			n.keys = append(n.keys, key)
			if _, ok := ix.cache[key]; !ok && queued[key] == "" {
				queued[key] = doc
				queue = append(queue, key)
			}
		}
		notes = append(notes, n)
	}
	terms := newTermIndex(notes)
	ix.mu.Lock()
	ix.terms = terms
	ix.mu.Unlock()

	if len(queue) > 0 {
		ix.log.Info("embedding notes for semantic search", "chunks", len(queue))
	}
	sincePublish := 0
	for start := 0; start < len(queue); start += batchSize {
		batch := queue[start:min(start+batchSize, len(queue))]
		texts := make([]string, len(batch))
		for i, key := range batch {
			texts[i] = queued[key]
		}
		vectors, err := ix.embed.Embed(ctx, texts)
		if err != nil {
			ix.publish(notes)
			return errors.Join(fmt.Errorf("embedding chunks: %w", err), ix.save())
		}
		for i, key := range batch {
			ix.cache[key] = vectors[i]
		}
		if sincePublish += len(batch); sincePublish >= publishEvery {
			ix.publish(notes)
			if err := ix.save(); err != nil {
				return err
			}
			sincePublish = 0
		}
	}

	used := map[string]bool{}
	for _, n := range notes {
		for _, key := range n.keys {
			used[key] = true
		}
	}
	pruned := false
	for key := range ix.cache {
		if !used[key] {
			delete(ix.cache, key)
			pruned = true
		}
	}
	ix.publish(notes)
	if len(queue) == 0 && !pruned {
		return nil
	}
	return ix.save()
}

func documentText(title string, c Chunk) string {
	heading := ""
	if len(c.HeadingPath) > 0 {
		heading = strings.Join(c.HeadingPath, " > ") + "\n"
	}
	return documentPrefix + title + "\n" + heading + c.Text
}

func docKey(doc string) string {
	sum := sha256.Sum256([]byte(doc))
	return string(sum[:])
}

// publish makes every embedded chunk searchable and updates the status.
func (ix *Index) publish(notes []noteChunks) {
	var entries []entry
	status := Status{NotesTotal: len(notes)}
	for _, n := range notes {
		complete := true
		for i, key := range n.keys {
			vec, ok := ix.cache[key]
			if !ok {
				complete = false
				continue
			}
			entries = append(entries, entry{path: n.path, chunk: n.chunks[i], vec: vec})
		}
		if complete {
			status.NotesIndexed++
		}
	}
	status.Complete = status.NotesIndexed == status.NotesTotal
	ix.mu.Lock()
	ix.entries, ix.status = entries, status
	ix.mu.Unlock()
}

func (ix *Index) snapshot() ([]entry, Status) {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return ix.entries, ix.status
}

func (ix *Index) termSnapshot() *termIndex {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return ix.terms
}

func (ix *Index) currentStatus() Status {
	_, s := ix.snapshot()
	return s
}

func (ix *Index) load() {
	f, err := os.Open(ix.store) // #nosec G304 -- the path comes from configuration, not from a request
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil {
		ix.log.Warn("cannot read the saved semantic index, rebuilding it", "error", err)
		return
	}
	defer f.Close()
	var saved storeFile
	if err := gob.NewDecoder(f).Decode(&saved); err != nil {
		ix.log.Warn("saved semantic index is damaged, rebuilding it", "error", err)
		return
	}
	if saved.Model != ModelName {
		ix.log.Info("saved semantic index is for another model, rebuilding it", "model", saved.Model)
		return
	}
	ix.cache = saved.Vectors
}

// save writes the vectors to a temporary file and renames it into place, so
// a crash mid-write leaves the previous index intact.
func (ix *Index) save() error {
	dir := filepath.Dir(ix.store)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("saving the semantic index: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".semantic-*.gob")
	if err != nil {
		return fmt.Errorf("saving the semantic index: %w", err)
	}
	defer os.Remove(tmp.Name())
	err = gob.NewEncoder(tmp).Encode(storeFile{Model: ModelName, Vectors: ix.cache})
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), ix.store)
	}
	if err != nil {
		return fmt.Errorf("saving the semantic index: %w", err)
	}
	return nil
}

// searchMeaning returns every indexed note ranked by how close its best chunk is
// to the query in meaning. It returns no hits, and embeds nothing, while
// the index is empty.
func (ix *Index) searchMeaning(ctx context.Context, query string) ([]Hit, Status, error) {
	entries, status := ix.snapshot()
	if len(entries) == 0 {
		return nil, status, nil
	}
	vectors, err := ix.embed.Embed(ctx, []string{queryPrefix + query})
	if err != nil {
		return nil, status, fmt.Errorf("embedding the query: %w", err)
	}
	q := vectors[0]
	best := map[string]Hit{}
	for _, e := range entries {
		score := dot(e.vec, q)
		if h, ok := best[e.path]; !ok || score > h.Score {
			best[e.path] = Hit{Path: e.path, HeadingPath: e.chunk.HeadingPath, Text: e.chunk.Text, Score: score, pieces: e.chunk.pieces}
		}
	}
	hits := make([]Hit, 0, len(best))
	for _, h := range best {
		hits = append(hits, h)
	}
	slices.SortFunc(hits, func(a, b Hit) int { return byScore(a, b) })
	return hits, status, nil
}

// Related returns every other indexed note ranked by the closest pair of
// chunks between it and the note at rel, so a long note matches on its
// most similar section rather than on an average of all of them.
func (ix *Index) Related(rel string) ([]Match, Status, error) {
	entries, status := ix.snapshot()
	var own []entry
	for _, e := range entries {
		if e.path == rel {
			own = append(own, e)
		}
	}
	if len(own) == 0 {
		return nil, status, fmt.Errorf("%s is not in the semantic index: it has no text to compare, or it is new and not indexed yet", rel)
	}
	best := map[string]Match{}
	for _, e := range entries {
		if e.path == rel {
			continue
		}
		for _, o := range own {
			score := dot(e.vec, o.vec)
			if m, ok := best[e.path]; !ok || score > m.Score {
				best[e.path] = Match{
					Hit:             Hit{Path: e.path, HeadingPath: e.chunk.HeadingPath, Text: e.chunk.Text, Score: score},
					FromHeadingPath: o.chunk.HeadingPath,
				}
			}
		}
	}
	matches := make([]Match, 0, len(best))
	for _, m := range best {
		matches = append(matches, m)
	}
	slices.SortFunc(matches, func(a, b Match) int { return byScore(a.Hit, b.Hit) })
	return matches, status, nil
}

func byScore(a, b Hit) int {
	if c := cmp.Compare(b.Score, a.Score); c != 0 {
		return c
	}
	return strings.Compare(a.Path, b.Path)
}

func dot(a, b []float32) float32 {
	var s float32
	for i := range min(len(a), len(b)) {
		s += a[i] * b[i]
	}
	return s
}
