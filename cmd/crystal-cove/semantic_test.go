package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dakotahp/crystal-cove/internal/vault"
)

func TestRunFailsWhenSemanticSearchCannotStart(t *testing.T) {
	installFakeOb(t, `case "$1" in sync) echo "Fully synced"; exec sleep 60;; *) exit 0;; esac`)
	t.Setenv("PATH", filepath.Dir(firstOnPath(t, "ob"))+string(os.PathListSeparator)+"/bin:/usr/bin")
	env := testEnv(t)
	env["MCP_SEMANTIC_SEARCH"] = "true"
	err := run(context.Background(), getenv(env), io.Discard, nil)
	if err == nil || !strings.Contains(err.Error(), "llama-server") {
		t.Errorf("err = %v, want a missing llama-server to stop the boot", err)
	}
}

func TestStartSemantic(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "llama-server")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nexec sleep 60\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	model := filepath.Join(dir, "model.gguf")
	if err := os.WriteFile(model, []byte("gguf"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	vaults := []*vault.Vault{vault.New("Work", t.TempDir()), vault.New("Personal", t.TempDir())}
	indexes, err := startSemantic(ctx, filepath.Join(dir, "semantic"), vaults, slog.New(slog.NewTextHandler(io.Discard, nil)), binary, model)
	if err != nil {
		t.Fatal(err)
	}
	if len(indexes) != 2 || indexes["Work"] == nil || indexes["Personal"] == nil {
		t.Errorf("indexes = %v", indexes)
	}
}

func firstOnPath(t *testing.T, name string) string {
	t.Helper()
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return filepath.Join(dir, name)
		}
	}
	t.Fatalf("%s not on PATH", name)
	return ""
}
