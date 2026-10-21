package semantic

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultBinary and DefaultModel are where the Docker image installs the
	// embedding server and its model.
	DefaultBinary = "llama-server"
	DefaultModel  = "/opt/crystal-cove/nomic-embed-text-v1.5.Q8_0.gguf"

	initialBackoff = time.Second
	maxBackoff     = time.Minute
	stableRun      = time.Minute
)

// Engine runs llama-server with the embedding model on a loopback port and
// restarts it when it exits.
type Engine struct {
	binary, model, port string
	log                 *slog.Logger
	sleep               func(ctx context.Context, d time.Duration)
	stableRun           time.Duration
}

// NewEngine checks that the server binary and model exist and reserves a
// free loopback port for the server.
func NewEngine(binary, model string, log *slog.Logger) (*Engine, error) {
	path, err := exec.LookPath(binary)
	if err != nil {
		return nil, fmt.Errorf("semantic search needs the %s embedding server: %w", DefaultBinary, err)
	}
	if _, err := os.Stat(model); err != nil {
		return nil, fmt.Errorf("semantic search needs its embedding model: %w", err)
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("finding a port for the embedding server: %w", err)
	}
	_, port, _ := net.SplitHostPort(l.Addr().String())
	_ = l.Close()
	return &Engine{binary: path, model: model, port: port, log: log, sleep: sleepCtx, stableRun: stableRun}, nil
}

// URL is the base URL the embedding server listens on.
func (e *Engine) URL() string { return "http://127.0.0.1:" + e.port }

// Run keeps the embedding server running until ctx ends.
func (e *Engine) Run(ctx context.Context) {
	backoff := initialBackoff
	for ctx.Err() == nil {
		e.log.Info("starting embedding server", "model", e.model)
		start := time.Now()
		out := &tailWriter{log: e.log, keep: 20}
		cmd := exec.CommandContext(ctx, e.binary, // #nosec G204 -- fixed binary, arguments passed without a shell
			"-m", e.model, "--embedding", "--host", "127.0.0.1", "--port", e.port,
			"-c", "2048", "-b", "2048", "-ub", "2048", "-np", "1")
		cmd.Stdout = out
		cmd.Stderr = out
		err := cmd.Run()
		if ctx.Err() != nil {
			return
		}
		if time.Since(start) >= e.stableRun {
			backoff = initialBackoff
		}
		e.log.Warn("embedding server exited, restarting", "error", err, "backoff", backoff.String(), "output", out.tail())
		e.sleep(ctx, backoff)
		backoff = min(backoff*2, maxBackoff)
	}
}

// tailWriter logs a child's output at debug level and keeps its last lines
// to report when the child exits.
type tailWriter struct {
	log     *slog.Logger
	keep    int
	mu      sync.Mutex
	partial []byte
	lines   []string
}

func (w *tailWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.partial = append(w.partial, p...)
	for {
		i := bytes.IndexByte(w.partial, '\n')
		if i < 0 {
			return len(p), nil
		}
		line := string(w.partial[:i])
		w.partial = w.partial[i+1:]
		w.log.Debug("embedding server", "output", line)
		w.lines = append(w.lines, line)
		if len(w.lines) > w.keep {
			w.lines = w.lines[1:]
		}
	}
}

func (w *tailWriter) tail() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return strings.Join(w.lines, "\n")
}
