package semantic

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func fakeServer(t *testing.T, script string) (binary, model string) {
	t.Helper()
	dir := t.TempDir()
	binary = filepath.Join(dir, "llama-server")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	model = filepath.Join(dir, "model.gguf")
	if err := os.WriteFile(model, []byte("gguf"), 0o644); err != nil {
		t.Fatal(err)
	}
	return binary, model
}

func TestNewEngineChecksItsFiles(t *testing.T) {
	binary, model := fakeServer(t, "exit 0")
	if _, err := NewEngine(filepath.Join(t.TempDir(), "missing"), model, quietLog()); err == nil || !strings.Contains(err.Error(), "llama-server") {
		t.Errorf("missing binary: err = %v", err)
	}
	if _, err := NewEngine(binary, filepath.Join(t.TempDir(), "missing.gguf"), quietLog()); err == nil || !strings.Contains(err.Error(), "model") {
		t.Errorf("missing model: err = %v", err)
	}
	e, err := NewEngine(binary, model, quietLog())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(e.URL(), "http://127.0.0.1:") || strings.HasSuffix(e.URL(), ":0") {
		t.Errorf("URL = %q, want a loopback port", e.URL())
	}
}

func TestEngineRunsWithArgumentsAndRestarts(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	binary, model := fakeServer(t, `echo "$@" > `+argsFile+`; echo "loading model"; echo "failed: bad model" >&2; exit 1`)
	var logs syncBuffer
	e, err := NewEngine(binary, model, slog.New(slog.NewTextHandler(&logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	var sleeps []time.Duration
	e.sleep = func(_ context.Context, d time.Duration) {
		sleeps = append(sleeps, d)
		if len(sleeps) == 3 {
			cancel()
		}
	}
	e.Run(ctx)

	if want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}; !slices.Equal(sleeps, want) {
		t.Errorf("backoff = %v, want %v", sleeps, want)
	}
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	port := strings.TrimPrefix(e.URL(), "http://127.0.0.1:")
	for _, want := range []string{"--embedding", "-m " + model, "--host 127.0.0.1", "--port " + port} {
		if !strings.Contains(string(args), want) {
			t.Errorf("args %q lack %q", args, want)
		}
	}
	if out := logs.String(); !strings.Contains(out, "failed: bad model") || !strings.Contains(out, "embedding server exited") {
		t.Errorf("log lacks the exit and its output:\n%s", out)
	}
}

func TestEngineResetsBackoffAfterStableRun(t *testing.T) {
	binary, model := fakeServer(t, "exit 1")
	e, err := NewEngine(binary, model, quietLog())
	if err != nil {
		t.Fatal(err)
	}
	e.stableRun = 0
	ctx, cancel := context.WithCancel(context.Background())
	var sleeps []time.Duration
	e.sleep = func(_ context.Context, d time.Duration) {
		sleeps = append(sleeps, d)
		if len(sleeps) == 2 {
			cancel()
		}
	}
	e.Run(ctx)
	if want := []time.Duration{time.Second, time.Second}; !slices.Equal(sleeps, want) {
		t.Errorf("backoff = %v, want %v", sleeps, want)
	}
}

func TestEngineStopsChildOnCancel(t *testing.T) {
	binary, model := fakeServer(t, "exec sleep 30")
	e, err := NewEngine(binary, model, quietLog())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		e.Run(ctx)
		close(done)
	}()
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func TestTailWriterKeepsLastLines(t *testing.T) {
	w := &tailWriter{log: quietLog(), keep: 2}
	w.Write([]byte("one\ntwo\nthr"))
	w.Write([]byte("ee\n"))
	if got := w.tail(); got != "two\nthree" {
		t.Errorf("tail = %q", got)
	}
}

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}
