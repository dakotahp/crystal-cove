package server

import (
	"context"
	"encoding/json"
	"errors"
	"hash/fnv"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dakotahp/crystal-cove/internal/search"
	"github.com/dakotahp/crystal-cove/internal/semantic"
	"github.com/dakotahp/crystal-cove/internal/vault"
)

// wordsEmbedder is a stand-in model: texts that share words get close
// vectors.
type wordsEmbedder struct {
	mu   sync.Mutex
	fail bool
}

func (w *wordsEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.fail {
		return nil, errors.New("model unavailable")
	}
	out := make([][]float32, len(texts))
	for i, text := range texts {
		_, text, _ = strings.Cut(text, ": ")
		v := make([]float32, 64)
		for _, word := range strings.Fields(strings.ToLower(text)) {
			h := fnv.New32a()
			h.Write([]byte(strings.Trim(word, ".,#[]")))
			v[h.Sum32()%64]++
		}
		v[63] += 0.01
		var sum float64
		for _, x := range v {
			sum += float64(x * x)
		}
		for j := range v {
			v[j] /= float32(math.Sqrt(sum))
		}
		out[i] = v
	}
	return out, nil
}

func (w *wordsEmbedder) Ready(context.Context) bool { return true }

func newSemanticServer(t *testing.T) (*Server, *wordsEmbedder) {
	t.Helper()
	v := vault.New("Garden", t.TempDir())
	write(t, v, "Tomatoes.md", "# Growing\ntomatoes need watering and rich soil compost\n")
	write(t, v, "Beds/Seedlings.md", "# Spring\nseedlings tomatoes soil watering trays [[Tomatoes]]\n")
	write(t, v, "Beds/Herbs.md", "# Basil\nbasil soil watering sunlight\n")
	write(t, v, "Budget.md", "# Money\nsavings income expenses rent\n")
	write(t, v, "Archive/Old.md", "tomatoes watering soil from last year\n")
	write(t, v, "Links.md", "see [[Tomatoes]] for more\n")
	embedder := &wordsEmbedder{}
	ix := semantic.NewIndex(v, embedder, filepath.Join(t.TempDir(), "garden.gob"), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := ix.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	s := New([]*vault.Vault{v}, search.New("rg", nil), func() bool { return true })
	s.SetSemantic(map[string]*semantic.Index{"Garden": ix})
	return s, embedder
}

func TestSearchNotes(t *testing.T) {
	s, _ := newSemanticServer(t)
	_, out, err := s.searchNotes(context.Background(), nil, searchNotesInput{Query: "rich compost growing"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Total != 6 || len(out.Notes) != 6 || out.NextOffset != -1 {
		t.Fatalf("total %d, %d notes, next %d", out.Total, len(out.Notes), out.NextOffset)
	}
	if !out.Index.Complete || out.Index.NotesTotal != 6 {
		t.Errorf("index = %+v", out.Index)
	}
	top := out.Notes[0]
	if top.Path != "Tomatoes.md" || !slices.Equal(top.HeadingPath, []string{"Growing"}) || !strings.Contains(top.Excerpt, "compost") {
		t.Errorf("top = %+v", top)
	}
	if !slices.Contains(top.MatchedBy, "keyword") || !slices.Contains(top.MatchedBy, "meaning") {
		t.Errorf("top matched_by = %q, want keyword and meaning", top.MatchedBy)
	}
	if out.Notes[1].Path == "Budget.md" {
		t.Errorf("unrelated note ranked near the top: %+v", out.Notes)
	}

	_, out, err = s.searchNotes(context.Background(), nil, searchNotesInput{
		Query:        "tomatoes",
		folderFilter: folderFilter{ExcludeFolders: []string{"Archive"}},
		Limit:        2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Total != 5 || len(out.Notes) != 2 || out.NextOffset != 2 {
		t.Errorf("filtered page: total %d, %d notes, next %d", out.Total, len(out.Notes), out.NextOffset)
	}
}

func TestSearchNotesErrors(t *testing.T) {
	s, embedder := newSemanticServer(t)
	ctx := context.Background()
	if _, _, err := s.searchNotes(ctx, nil, searchNotesInput{Vault: "Nope", Query: "x"}); err == nil {
		t.Error("unknown vault accepted")
	}
	if _, _, err := s.searchNotes(ctx, nil, searchNotesInput{Query: "  "}); err == nil || !strings.Contains(err.Error(), "query") {
		t.Errorf("empty query: err = %v", err)
	}
	if _, _, err := s.searchNotes(ctx, nil, searchNotesInput{Query: "x", Offset: 99}); err == nil {
		t.Error("offset past the end accepted")
	}
	embedder.fail = true
	if _, _, err := s.searchNotes(ctx, nil, searchNotesInput{Query: "x"}); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Errorf("model failure: err = %v", err)
	}
}

func TestSearchNotesWhileBuilding(t *testing.T) {
	v := vault.New("Garden", t.TempDir())
	write(t, v, "a.md", "text\n")
	ix := semantic.NewIndex(v, &wordsEmbedder{}, filepath.Join(t.TempDir(), "g.gob"), slog.New(slog.NewTextHandler(io.Discard, nil)))
	s := New([]*vault.Vault{v}, search.New("rg", nil), func() bool { return true })
	s.SetSemantic(map[string]*semantic.Index{"Garden": ix})
	_, _, err := s.searchNotes(context.Background(), nil, searchNotesInput{Query: "text"})
	if err == nil || !strings.Contains(err.Error(), "still being built") || !strings.Contains(err.Error(), "grep_notes") {
		t.Errorf("err = %v, want a hint that the index is building", err)
	}
	_, _, err = s.relatedNotes(context.Background(), nil, relatedNotesInput{Path: "a.md"})
	if err == nil || !strings.Contains(err.Error(), "still being built") {
		t.Errorf("related err = %v, want a hint that the index is building", err)
	}
}

func TestRelatedNotes(t *testing.T) {
	s, _ := newSemanticServer(t)
	ctx := context.Background()
	_, out, err := s.relatedNotes(ctx, nil, relatedNotesInput{Path: "Tomatoes.md"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Path != "Tomatoes.md" || out.Total != 5 || !out.Index.Complete {
		t.Fatalf("out = %+v", out)
	}
	linked := map[string]bool{}
	for _, n := range out.Notes {
		if n.Path == "Tomatoes.md" {
			t.Error("note listed as related to itself")
		}
		linked[n.Path] = n.Linked
	}
	if !linked["Beds/Seedlings.md"] || !linked["Links.md"] || linked["Archive/Old.md"] || linked["Budget.md"] {
		t.Errorf("linked = %v, want only the notes linking to Tomatoes", linked)
	}
	if out.Notes[0].Path != "Archive/Old.md" && out.Notes[0].Path != "Beds/Seedlings.md" {
		t.Errorf("closest = %+v", out.Notes[0])
	}
	if !slices.Equal(out.Notes[0].FromHeadingPath, []string{"Growing"}) {
		t.Errorf("from_heading_path = %q", out.Notes[0].FromHeadingPath)
	}

	_, out, err = s.relatedNotes(ctx, nil, relatedNotesInput{Path: "Tomatoes.md", Unlinked: true, folderFilter: folderFilter{Folders: []string{"Beds", "Archive"}}})
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, n := range out.Notes {
		paths = append(paths, n.Path)
	}
	slices.Sort(paths)
	if !slices.Equal(paths, []string{"Archive/Old.md", "Beds/Herbs.md"}) {
		t.Errorf("unlinked in folders = %v", paths)
	}
}

func TestRelatedNotesLinkedFromTheNote(t *testing.T) {
	s, _ := newSemanticServer(t)
	_, out, err := s.relatedNotes(context.Background(), nil, relatedNotesInput{Path: "Beds/Seedlings.md"})
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range out.Notes {
		if n.Path == "Tomatoes.md" && !n.Linked {
			t.Error("note the given note links to is not marked linked")
		}
	}
}

func TestRelatedNotesErrors(t *testing.T) {
	s, _ := newSemanticServer(t)
	ctx := context.Background()
	cases := map[string]relatedNotesInput{
		"not a note":   {Path: "image.png"},
		"hidden":       {Path: ".obsidian/x.md"},
		"missing":      {Path: "Nope.md"},
		"bad vault":    {Vault: "Nope", Path: "Tomatoes.md"},
		"past the end": {Path: "Tomatoes.md", Offset: 50},
	}
	for name, in := range cases {
		if _, _, err := s.relatedNotes(ctx, nil, in); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	write(t, s.vaultList[0], "Empty.md", "")
	if _, _, err := s.relatedNotes(ctx, nil, relatedNotesInput{Path: "Empty.md"}); err == nil || !strings.Contains(err.Error(), "not in the semantic index") {
		t.Errorf("unindexed note: err = %v", err)
	}
}

func TestExcerpt(t *testing.T) {
	if got := excerpt("one\n\ntwo   three"); got != "one two three" {
		t.Errorf("excerpt = %q", got)
	}
	long := strings.Repeat("é", excerptChars+10)
	got := excerpt(long)
	if !strings.HasSuffix(got, "…") || len([]rune(got)) != excerptChars+1 {
		t.Errorf("long excerpt has %d runes", len([]rune(got)))
	}
}

func toolDescriptions(t *testing.T, s *Server) map[string]string {
	t.Helper()
	ts := httptest.NewServer(s.Handler(AuthConfig{StaticToken: "secret"}))
	t.Cleanup(ts.Close)
	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   ts.URL,
		HTTPClient: &http.Client{Transport: authTransport{token: "secret"}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, tool := range tools.Tools {
		out[tool.Name] = tool.Description
	}
	return out
}

func TestSemanticToolsOnlyWhenEnabled(t *testing.T) {
	off := toolDescriptions(t, newTestServer(t))
	if _, ok := off["search_notes"]; ok {
		t.Error("search_notes registered without an index")
	}
	if strings.Contains(off["grep_notes"], "search_notes") || strings.Contains(off["get_backlinks"], "related_notes") {
		t.Error("descriptions point at tools that are not registered")
	}

	s, _ := newSemanticServer(t)
	s.SetPolicy(Policy{ReadOnly: true})
	on := toolDescriptions(t, s)
	for _, name := range []string{"search_notes", "related_notes"} {
		if on[name] == "" {
			t.Errorf("%s missing from a read-only server", name)
		}
	}
	if !strings.Contains(on["grep_notes"], "search_notes") || !strings.Contains(on["get_backlinks"], "related_notes") {
		t.Error("existing tools do not point at the semantic tools")
	}
	if !strings.Contains(on["search_notes"], "grep_notes") || !strings.Contains(on["related_notes"], "get_backlinks") {
		t.Error("semantic tools do not point back at the exact tools")
	}
}

func TestSearchNotesOverHTTP(t *testing.T) {
	s, _ := newSemanticServer(t)
	ts := httptest.NewServer(s.Handler(AuthConfig{StaticToken: "secret"}))
	defer ts.Close()
	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   ts.URL,
		HTTPClient: &http.Client{Transport: authTransport{token: "secret"}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "search_notes", Arguments: map[string]any{"query": "tomatoes", "limit": 1}})
	if err != nil || res.IsError {
		t.Fatalf("call: %v %+v", err, res)
	}
	var out SearchNotesResult
	raw, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Notes) != 1 || out.Notes[0].Path == "" || out.Index.NotesTotal != 6 {
		t.Errorf("out = %+v", out)
	}
}
