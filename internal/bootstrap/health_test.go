package bootstrap

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dakotahp/crystal-cove/internal/config"
)

func TestSyncReadyRequiresFreshHeartbeatFromEveryVault(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	b := New(testConfig(t), slog.New(slog.NewTextHandler(io.Discard, nil)))
	b.now = func() time.Time { return now }

	if b.SyncReady() {
		t.Fatal("SyncReady = true before sync processes started")
	}
	for _, vault := range b.cfg.Vaults {
		b.syncStarted(vault.Name)
		b.syncHeartbeat(vault.Name)
	}
	if !b.SyncReady() {
		t.Fatal("SyncReady = false with fresh heartbeats for every vault")
	}

	now = now.Add(syncReadyMaxAge + time.Second)
	if b.SyncReady() {
		t.Fatal("SyncReady = true with stale heartbeats")
	}

	b.syncHeartbeat(b.cfg.Vaults[0].Name)
	if b.SyncReady() {
		t.Fatal("SyncReady = true while another vault remains stale")
	}
	b.syncStopped(b.cfg.Vaults[1].Name)
	if b.SyncReady() {
		t.Fatal("SyncReady = true while a vault sync process is stopped")
	}
}

func TestSyncOutputRecognizesChunkedHeartbeatAndPreservesOutput(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	b := New(&config.Config{Vaults: []config.Vault{{Name: "Notes"}}}, slog.Default())
	b.now = func() time.Time { return now }
	var output bytes.Buffer
	b.syncOutput = &output
	b.syncStarted("Notes")

	w := b.observedSyncOutput("Notes")
	for _, chunk := range []string{"Connecting...\nFully ", "synced\r\n"} {
		if _, err := w.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
	}
	if !b.SyncReady() {
		t.Fatal("SyncReady = false after chunked Fully synced line")
	}
	if got := output.String(); got != "Connecting...\nFully synced\r\n" {
		t.Errorf("forwarded output = %q", got)
	}
}

func TestSuperviseVaultRestartsRunningProcessWithStaleHeartbeat(t *testing.T) {
	b, argsLog := newTestBootstrapper(t, `case "$1" in sync) exec sleep 60;; *) exit 0;; esac`)
	b.cfg.Vaults = b.cfg.Vaults[:1]
	// The watchdog has to outlast the fake ob script's startup. Below that, the
	// child is killed before its shell logs the call, and the assertion sees no
	// invocations at all. Process start is slower on macOS than on CI Linux.
	b.watchdogAfter = 500 * time.Millisecond
	b.watchdogPoll = 25 * time.Millisecond
	b.stableRun = 0

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var restarts atomic.Int32
	b.sleep = func(context.Context, time.Duration) {
		if restarts.Add(1) >= 2 {
			cancel()
		}
	}
	b.superviseVault(ctx, b.cfg.Vaults[0])

	calls := loggedCalls(t, argsLog)
	if got := strings.Count(strings.Join(calls, "\n"), "sync --continuous"); got < 2 {
		t.Fatalf("sync invocations = %d, want at least 2; calls: %v", got, calls)
	}
}

func TestSyncOutputShowsRepeatedHeartbeatOnce(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	b := New(&config.Config{Vaults: []config.Vault{{Name: "Notes"}}}, slog.Default())
	b.now = func() time.Time { return now }
	var output bytes.Buffer
	b.syncOutput = &output
	b.syncStarted("Notes")

	w := b.observedSyncOutput("Notes")
	if _, err := w.Write([]byte("Fully synced\nFully synced\nDownloaded a.md\nFully synced\n")); err != nil {
		t.Fatal(err)
	}
	now = now.Add(syncReadyMaxAge)
	if _, err := w.Write([]byte("Fully synced\n")); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Second)
	if !b.SyncReady() {
		t.Error("SyncReady = false: a heartbeat left out of the output was not recorded")
	}
	if got, want := output.String(), "Fully synced\nDownloaded a.md\nFully synced\n"; got != want {
		t.Errorf("forwarded output = %q, want %q", got, want)
	}
}

func TestSyncOutputFlushForwardsUnfinishedLine(t *testing.T) {
	b := New(&config.Config{Vaults: []config.Vault{{Name: "Notes"}}}, slog.Default())
	var output bytes.Buffer
	b.syncOutput = &output

	w := b.observedSyncOutput("Notes")
	if _, err := w.Write([]byte("Error: connection lost")); err != nil {
		t.Fatal(err)
	}
	w.flush()
	if got := output.String(); got != "Error: connection lost\n" {
		t.Errorf("forwarded output = %q", got)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestSyncOutputReportsForwardError(t *testing.T) {
	b := New(&config.Config{Vaults: []config.Vault{{Name: "Notes"}}}, slog.Default())
	b.syncOutput = failingWriter{}

	if _, err := b.observedSyncOutput("Notes").Write([]byte("Downloaded a.md\n")); err == nil {
		t.Error("Write succeeded although the output failed")
	}
}
