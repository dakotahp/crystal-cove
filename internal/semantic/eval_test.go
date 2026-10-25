package semantic

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/dakotahp/crystal-cove/internal/vault"
)

// evalFile is the file TestEval reads. Its queries name real notes, so it
// lives outside the repository.
type evalFile struct {
	// Vault is the vault folder to index.
	Vault string `json:"vault"`
	// Store is a saved index to start from, such as a copy of the server's
	// ~/.crystal-cove/semantic/<vault>.gob, so only changed notes are
	// embedded. Without it the whole vault is embedded first.
	Store string `json:"store"`
	// Embedder is the base URL of a running llama-server with the model.
	Embedder string      `json:"embedder"`
	Queries  []evalQuery `json:"queries"`
}

type evalQuery struct {
	Query   string   `json:"query"`
	Folders []string `json:"folders"`
	// Expect lists the notes a good answer holds. Empty means the vault has
	// no answer, and the report shows how the top results were found.
	Expect []string `json:"expect"`
}

const evalTop = 5

// TestEval compares meaning-only ranking with hybrid Search on labelled
// queries. It runs only when CRYSTAL_COVE_EVAL names an eval file:
//
//	CRYSTAL_COVE_EVAL=queries.json go test ./internal/semantic -run TestEval -v -timeout 2h
func TestEval(t *testing.T) {
	name := os.Getenv("CRYSTAL_COVE_EVAL")
	if name == "" {
		t.Skip("set CRYSTAL_COVE_EVAL to an eval file to run")
	}
	raw, err := os.ReadFile(name) // #nosec G304 -- a path the person running the test chose
	if err != nil {
		t.Fatal(err)
	}
	var file evalFile
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	store := file.Store
	if store == "" {
		store = t.TempDir() + "/eval.gob"
	}
	ix := NewIndex(vault.New("eval", file.Vault), NewClient(file.Embedder), store, quietLog())
	ctx := context.Background()
	ix.load()
	if err := ix.Refresh(ctx); err != nil {
		t.Fatal(err)
	}

	var meaningTop, hybridTop, labelled int
	var meaningRR, hybridRR float64
	for _, q := range file.Queries {
		include := func(p string) bool {
			return len(q.Folders) == 0 || slices.ContainsFunc(q.Folders, func(f string) bool {
				return strings.HasPrefix(p, strings.Trim(f, "/")+"/")
			})
		}
		hits, _, err := ix.searchMeaning(ctx, q.Query)
		if err != nil {
			t.Fatal(err)
		}
		var meaning []string
		for _, h := range hits {
			if include(h.Path) {
				meaning = append(meaning, h.Path)
			}
		}
		results, _, err := ix.Search(ctx, q.Query, include)
		if err != nil {
			t.Fatal(err)
		}
		hybrid := make([]string, len(results))
		for i, r := range results {
			hybrid[i] = r.Path
		}
		if os.Getenv("CRYSTAL_COVE_EVAL_DETAIL") != "" {
			for i, r := range results[:min(evalTop*2, len(results))] {
				t.Logf("    %2d %-24s %s", i+1, strings.Join(r.MatchedBy, "+"), r.Path)
			}
		}

		if len(q.Expect) == 0 {
			var how []string
			for _, r := range results[:min(evalTop, len(results))] {
				how = append(how, strings.Join(r.MatchedBy, "+"))
			}
			t.Logf("%-50q no answer expected; hybrid top found by: %s", q.Query, strings.Join(how, ", "))
			continue
		}
		labelled++
		m, h := firstRank(meaning, q.Expect), firstRank(hybrid, q.Expect)
		if m > 0 && m <= evalTop {
			meaningTop++
		}
		if h > 0 && h <= evalTop {
			hybridTop++
		}
		if m > 0 {
			meaningRR += 1 / float64(m)
		}
		if h > 0 {
			hybridRR += 1 / float64(h)
		}
		t.Logf("%-50q meaning rank %-4s hybrid rank %s", q.Query, rankText(m), rankText(h))
	}
	if labelled > 0 {
		n := float64(labelled)
		t.Logf("expected note in top %d: meaning %d/%d, hybrid %d/%d", evalTop, meaningTop, labelled, hybridTop, labelled)
		t.Logf("mean reciprocal rank: meaning %.3f, hybrid %.3f", meaningRR/n, hybridRR/n)
	}
}

// firstRank is the 1-based place of the first expected note, or 0.
func firstRank(ranked, expect []string) int {
	for i, p := range ranked {
		if slices.Contains(expect, p) {
			return i + 1
		}
	}
	return 0
}

func rankText(r int) string {
	if r == 0 {
		return "-"
	}
	return fmt.Sprint(r)
}
