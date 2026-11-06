package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dakotahp/crystal-cove/internal/search"
	"github.com/dakotahp/crystal-cove/internal/semantic"
	"github.com/dakotahp/crystal-cove/internal/vault"
)

func newTopicServer(t *testing.T) *Server {
	t.Helper()
	v := vault.New("Topics", t.TempDir())
	vocab := map[string][]string{
		"Money": {"budget", "savings", "income", "expenses", "rent", "mortgage", "invoice", "taxes"},
		"Music": {"guitar", "chord", "melody", "rhythm", "tempo", "piano", "drums", "lyrics"},
		"Veg":   {"tomato", "zucchini", "carrot", "lettuce", "pepper", "squash", "onion", "potato"},
		"Bloom": {"rose", "tulip", "dahlia", "peony", "lily", "orchid", "daisy", "violet"},
	}
	for theme, words := range vocab {
		folder, prefix := theme, ""
		if theme == "Veg" || theme == "Bloom" {
			folder, prefix = "Garden", "soil watering compost garden "
		}
		for i := range 10 {
			body := prefix + strings.Join([]string{words[i%8], words[(i+1)%8], words[(i+2)%8], words[(i+3)%8], words[(i+5)%8]}, " ")
			write(t, v, fmt.Sprintf("%s/%s%02d.md", folder, strings.ToLower(theme[:1]), i), body+"\n")
		}
	}
	ix := semantic.NewIndex(v, &wordsEmbedder{}, filepath.Join(t.TempDir(), "t.gob"), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := ix.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	s := New([]*vault.Vault{v}, search.New("rg", nil), func() bool { return true })
	s.SetSemantic(map[string]*semantic.Index{"Topics": ix})
	return s
}

func TestTopicMap(t *testing.T) {
	s, _ := newSemanticServer(t)
	_, out, err := s.topicMap(context.Background(), nil, topicMapInput{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Build == "" || len(out.Topics) == 0 || out.Notes == 0 || out.Index.NotesTotal != 6 || out.Topic != "" {
		t.Fatalf("out = %+v", out)
	}
	for _, tp := range out.Topics {
		if len(tp.KeyWords) == 0 || len(tp.StartWith) == 0 || len(tp.Folders) == 0 || tp.CloseTo == nil {
			t.Errorf("topic = %+v", tp)
		}
	}
}

func TestTopicMapZoom(t *testing.T) {
	s := newTopicServer(t)
	ctx := context.Background()
	_, top, err := s.topicMap(ctx, nil, topicMapInput{})
	if err != nil {
		t.Fatal(err)
	}
	var garden TopicEntry
	for _, tp := range top.Topics {
		if tp.Folders[0].Folder == "Garden" {
			garden = tp
		}
	}
	if garden.ID == "" {
		t.Fatalf("no garden topic in %+v", top.Topics)
	}
	_, sub, err := s.topicMap(ctx, nil, topicMapInput{Topic: garden.ID, Build: top.Build})
	if err != nil {
		t.Fatal(err)
	}
	if sub.Topic != garden.ID || sub.Build != top.Build || len(sub.Topics) < 2 {
		t.Fatalf("sub = %+v", sub)
	}
	for _, tp := range sub.Topics {
		if !strings.HasPrefix(tp.ID, garden.ID+".") {
			t.Errorf("sub-topic id %q", tp.ID)
		}
	}
	_, scoped, err := s.topicMap(ctx, nil, topicMapInput{folderFilter: folderFilter{Folders: []string{"/Garden/"}}})
	if err != nil || scoped.Build == top.Build || scoped.Notes != 20 {
		t.Errorf("scoped = %v %+v", err, scoped)
	}
}

func TestTopicMapErrors(t *testing.T) {
	s := newTopicServer(t)
	ctx := context.Background()
	_, top, err := s.topicMap(ctx, nil, topicMapInput{})
	if err != nil {
		t.Fatal(err)
	}
	small := ""
	for _, tp := range top.Topics {
		if tp.Notes < 8 {
			small = tp.ID
		}
	}
	for name, c := range map[string]struct {
		in   topicMapInput
		want []string
	}{
		"topic without build": {topicMapInput{Topic: "1"}, []string{"build"}},
		"build without topic": {topicMapInput{Build: top.Build}, []string{"topic"}},
		"stale build":         {topicMapInput{Topic: "1", Build: "000000000000"}, []string{"call topic_map again without topic"}},
		"empty folder":        {topicMapInput{folderFilter: folderFilter{Folders: []string{"Nowhere"}}}, []string{"too few indexed notes", "widen folders"}},
		"unknown vault":       {topicMapInput{Vault: "Nope"}, []string{"Nope"}},
	} {
		_, _, err := s.topicMap(ctx, nil, c.in)
		for _, w := range c.want {
			if err == nil || !strings.Contains(err.Error(), w) {
				t.Errorf("%s: err = %v, want %q", name, err, w)
			}
		}
	}
	if small != "" {
		_, _, err := s.topicMap(ctx, nil, topicMapInput{Topic: small, Build: top.Build})
		if err == nil || !strings.Contains(err.Error(), "too small to split") || !strings.Contains(err.Error(), "search_notes") {
			t.Errorf("small topic: %v", err)
		}
	}
	semS, _ := newSemanticServer(t)
	_, semTop, err := semS.topicMap(ctx, nil, topicMapInput{})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = semS.topicMap(ctx, nil, topicMapInput{Topic: semTop.Topics[0].ID, Build: semTop.Build})
	if err == nil || !strings.Contains(err.Error(), "too small to split") || !strings.Contains(err.Error(), "search_notes") {
		t.Errorf("small vault topic: %v", err)
	}
}

func TestTopicMapWhileBuilding(t *testing.T) {
	v := vault.New("Garden", t.TempDir())
	write(t, v, "A.md", "tomatoes soil\n")
	ix := semantic.NewIndex(v, &wordsEmbedder{}, filepath.Join(t.TempDir(), "g.gob"), slog.New(slog.NewTextHandler(io.Discard, nil)))
	s := New([]*vault.Vault{v}, search.New("rg", nil), func() bool { return true })
	s.SetSemantic(map[string]*semantic.Index{"Garden": ix})
	if _, _, err := s.topicMap(context.Background(), nil, topicMapInput{}); err == nil || !strings.Contains(err.Error(), "still being built") {
		t.Errorf("err = %v", err)
	}
}

func TestTopicMapOverHTTP(t *testing.T) {
	s := newTopicServer(t)
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
	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "topic_map", Arguments: map[string]any{}})
	if err != nil || res.IsError {
		t.Fatalf("call: %v %+v", err, res)
	}
	var out TopicMapResult
	raw, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.Build == "" || len(out.Topics) == 0 || out.Topics[0].KeyWords == nil {
		t.Errorf("out = %+v", out)
	}
}
