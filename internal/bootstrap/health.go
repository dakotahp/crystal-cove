package bootstrap

import (
	"bytes"
	"sync"
	"time"
)

const (
	// obsidian-headless normally prints "Fully synced" every 30 seconds.
	// Two minutes tolerates several missed heartbeats before traffic is removed.
	syncReadyMaxAge = 2 * time.Minute
	// A process that stays alive but produces no heartbeat for five minutes is
	// wedged rather than merely reconnecting; restart just that sync child.
	defaultWatchdogAfter = 5 * time.Minute
	defaultWatchdogPoll  = 15 * time.Second
)

type syncState struct {
	running       bool
	startedAt     time.Time
	lastHeartbeat time.Time
}

// SyncReady reports whether every configured vault has a running sync child
// and has produced a recent "Fully synced" heartbeat.
func (b *Bootstrapper) SyncReady() bool {
	b.healthMu.RLock()
	defer b.healthMu.RUnlock()
	if len(b.cfg.Vaults) == 0 {
		return false
	}
	now := b.now()
	for _, vault := range b.cfg.Vaults {
		state := b.syncStates[vault.Name]
		if state == nil || !state.running || state.lastHeartbeat.IsZero() ||
			now.Sub(state.lastHeartbeat) > syncReadyMaxAge {
			return false
		}
	}
	return true
}

func (b *Bootstrapper) syncStarted(vault string) {
	b.healthMu.Lock()
	defer b.healthMu.Unlock()
	b.syncStates[vault] = &syncState{running: true, startedAt: b.now()}
}

func (b *Bootstrapper) syncHeartbeat(vault string) {
	b.healthMu.Lock()
	defer b.healthMu.Unlock()
	state := b.syncStates[vault]
	if state == nil {
		state = &syncState{running: true, startedAt: b.now()}
		b.syncStates[vault] = state
	}
	state.lastHeartbeat = b.now()
}

func (b *Bootstrapper) syncStopped(vault string) {
	b.healthMu.Lock()
	defer b.healthMu.Unlock()
	if state := b.syncStates[vault]; state != nil {
		state.running = false
	}
}

func (b *Bootstrapper) syncStale(vault string, maxAge time.Duration) bool {
	b.healthMu.RLock()
	defer b.healthMu.RUnlock()
	state := b.syncStates[vault]
	if state == nil || !state.running {
		return true
	}
	last := state.lastHeartbeat
	if last.IsZero() {
		last = state.startedAt
	}
	return b.now().Sub(last) > maxAge
}

// observedSyncWriter forwards ob output line by line while recognizing
// heartbeat lines, even when writes split a line across multiple chunks.
// ob prints "Fully synced" every 30 seconds, so a heartbeat is forwarded
// only when the line before it was something else.
type observedSyncWriter struct {
	b     *Bootstrapper
	vault string

	mu         sync.Mutex
	pending    []byte
	lastSynced bool
}

func (b *Bootstrapper) observedSyncOutput(vault string) *observedSyncWriter {
	return &observedSyncWriter{b: b, vault: vault}
}

func (w *observedSyncWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending = append(w.pending, p...)
	for {
		newline := bytes.IndexByte(w.pending, '\n')
		if newline < 0 {
			break
		}
		line := w.pending[:newline+1]
		w.pending = w.pending[newline+1:]
		if err := w.forward(line); err != nil {
			return len(p), err
		}
	}
	return len(p), nil
}

// flush forwards a final line that ob left without a newline.
func (w *observedSyncWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.pending) == 0 {
		return
	}
	_ = w.forward(append(w.pending, '\n'))
	w.pending = nil
}

func (w *observedSyncWriter) forward(line []byte) error {
	text := bytes.TrimSuffix(bytes.TrimSuffix(line, []byte{'\n'}), []byte{'\r'})
	synced := bytes.Equal(text, []byte("Fully synced"))
	if synced {
		w.b.syncHeartbeat(w.vault)
	}
	repeat := synced && w.lastSynced
	w.lastSynced = synced
	if repeat {
		return nil
	}
	w.b.outputMu.Lock()
	defer w.b.outputMu.Unlock()
	_, err := w.b.syncOutput.Write(line)
	return err
}
