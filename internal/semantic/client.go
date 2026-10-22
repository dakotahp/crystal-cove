package semantic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"
)

// Client asks an embedding server for vectors over its OpenAI-compatible
// API, as llama-server serves it.
type Client struct {
	url  string
	http *http.Client
}

// NewClient returns a Client for the server at baseURL.
func NewClient(baseURL string) *Client {
	return &Client{url: baseURL, http: &http.Client{Timeout: 5 * time.Minute}}
}

type embeddingRequest struct {
	Input []string `json:"input"`
}

type embeddingResponse struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

// Embed returns one unit-length vector per text, in the order given.
func (c *Client) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	body, err := json.Marshal(embeddingRequest{Input: texts})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("embedding request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("embedding server answered %d: %s", resp.StatusCode, bytes.TrimSpace(msg))
	}
	var out embeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decoding embeddings: %w", err)
	}
	if len(out.Data) != len(texts) {
		return nil, fmt.Errorf("embedding server returned %d embeddings for %d inputs", len(out.Data), len(texts))
	}
	vectors := make([][]float32, len(texts))
	for _, d := range out.Data {
		if d.Index < 0 || d.Index >= len(texts) {
			return nil, fmt.Errorf("embedding server returned index %d for %d inputs", d.Index, len(texts))
		}
		if err := normalize(d.Embedding); err != nil {
			return nil, err
		}
		vectors[d.Index] = d.Embedding
	}
	return vectors, nil
}

// Ready reports whether the server has loaded its model.
func (c *Client) Ready(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url+"/health", nil)
	if err != nil {
		return false
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func normalize(v []float32) error {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	if sum == 0 {
		return errors.New("embedding server returned an empty vector")
	}
	scale := float32(1 / math.Sqrt(sum))
	for i := range v {
		v[i] *= scale
	}
	return nil
}
