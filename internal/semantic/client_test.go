package semantic

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientEmbedNormalizesAndKeepsOrder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		var req embeddingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decoding request: %v", err)
		}
		if len(req.Input) != 2 || req.Input[0] != "a" {
			t.Errorf("input = %q", req.Input)
		}
		w.Write([]byte(`{"data":[{"index":1,"embedding":[0,2]},{"index":0,"embedding":[3,4]}]}`))
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL).Embed(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	want := [][]float32{{0.6, 0.8}, {0, 1}}
	for i := range want {
		for j := range want[i] {
			if math.Abs(float64(got[i][j]-want[i][j])) > 1e-6 {
				t.Fatalf("got %v, want %v", got, want)
			}
		}
	}
}

func TestClientEmbedErrors(t *testing.T) {
	cases := []struct {
		name, body, want string
		status           int
	}{
		{"status", `model loading`, "503", http.StatusServiceUnavailable},
		{"bad json", `{`, "decoding", http.StatusOK},
		{"wrong count", `{"data":[{"index":0,"embedding":[1]}]}`, "1 embeddings for 2 inputs", http.StatusOK},
		{"bad index", `{"data":[{"index":0,"embedding":[1]},{"index":5,"embedding":[1]}]}`, "index 5", http.StatusOK},
		{"zero vector", `{"data":[{"index":0,"embedding":[0]},{"index":1,"embedding":[1]}]}`, "empty", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			_, err := NewClient(srv.URL).Embed(context.Background(), []string{"a", "b"})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestClientEmbedUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	if _, err := NewClient(srv.URL).Embed(context.Background(), []string{"a"}); err == nil {
		t.Error("no error from a closed server")
	}
	if NewClient(srv.URL).Ready(context.Background()) {
		t.Error("closed server reported ready")
	}
}

func TestClientEmbedBadURL(t *testing.T) {
	if _, err := NewClient("http://bad host").Embed(context.Background(), []string{"a"}); err == nil {
		t.Error("no error for an invalid URL")
	}
	if NewClient("http://bad host").Ready(context.Background()) {
		t.Error("invalid URL reported ready")
	}
}

func TestClientReady(t *testing.T) {
	status := http.StatusServiceUnavailable
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(status)
	}))
	defer srv.Close()
	c := NewClient(srv.URL)
	if c.Ready(context.Background()) {
		t.Error("ready while loading")
	}
	status = http.StatusOK
	if !c.Ready(context.Background()) {
		t.Error("not ready once healthy")
	}
}
