package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setServerSecrets(t *testing.T) {
	t.Helper()
	t.Setenv("MCP_AUTH_TOKEN", "bearer-secret")
	t.Setenv("OBSIDIAN_PASSWORD", "account-secret")
	t.Setenv("OBSIDIAN_VAULT_PASSWORD", "vault-secret")
	t.Setenv("OBSIDIAN_VAULTS", "Work:vault-secret")
	t.Setenv("OBSIDIAN_AUTH_TOKEN", "sync-token")
}

func checkObEnv(t *testing.T, envLog string) {
	t.Helper()
	data, err := os.ReadFile(envLog)
	if err != nil {
		t.Fatal(err)
	}
	env := string(data)
	for _, name := range []string{"MCP_AUTH_TOKEN", "OBSIDIAN_PASSWORD", "OBSIDIAN_VAULT_PASSWORD", "OBSIDIAN_VAULTS"} {
		if strings.Contains(env, name+"=") {
			t.Errorf("ob saw %s in its environment", name)
		}
	}
	if !strings.Contains(env, "OBSIDIAN_AUTH_TOKEN=sync-token") {
		t.Error("ob lost OBSIDIAN_AUTH_TOKEN, which it reads from the environment")
	}
}

func TestObCommandsDoNotSeeServerSecrets(t *testing.T) {
	setServerSecrets(t)
	envLog := filepath.Join(t.TempDir(), "env.log")
	b, _ := newTestBootstrapper(t, "env >> "+envLog)

	if err := b.SetupVaults(context.Background()); err != nil {
		t.Fatal(err)
	}
	checkObEnv(t, envLog)
}

func TestContinuousSyncDoesNotSeeServerSecrets(t *testing.T) {
	setServerSecrets(t)
	envLog := filepath.Join(t.TempDir(), "env.log")
	b, _ := newTestBootstrapper(t, "env >> "+envLog+"; exit 1")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b.sleep = func(context.Context, time.Duration) { cancel() }

	b.SyncContinuously(ctx)
	checkObEnv(t, envLog)
}
