package vault

import (
	"fmt"
	"os"
	"time"
)

// EditGuard holds back writes to a note that another device changed moments
// ago. Obsidian, on a device where the note is open and being typed in,
// saves its own copy over changes that reach it meanwhile, so a write that
// lands then is lost there even though it succeeded here.
type EditGuard struct {
	// Quiet is how long a note must go unchanged by other devices before it
	// is written. Zero turns the guard off.
	Quiet time.Duration
	// MaxWait is the longest a write waits for that before it is refused.
	MaxWait time.Duration
}

// SetEditGuard turns on the guard for this vault's writes.
func (v *Vault) SetEditGuard(g EditGuard) { v.guard = g }

// waitUntilQuiet returns once the note at clean has gone Quiet without a
// change from another device, or an error when that does not happen within
// MaxWait. A note holding what this server last wrote to it changed here,
// not elsewhere, so it never waits. The sync client stamps a note it
// downloads with the time it changed on the other device.
func (v *Vault) waitUntilQuiet(root *os.Root, clean, rel string) error {
	if v.guard.Quiet <= 0 {
		return nil
	}
	deadline := v.now().Add(v.guard.MaxWait)
	for {
		data, err := root.ReadFile(clean)
		if err != nil {
			return nil
		}
		info, err := root.Stat(clean)
		if err != nil || v.wroteLast(clean, data) {
			return nil
		}
		now := v.now()
		settled := info.ModTime().Add(v.guard.Quiet)
		if !settled.After(now) {
			return nil
		}
		if settled.After(deadline) {
			return fmt.Errorf("note %q is still changing on another device, most likely because someone is typing in it: that device would save over a write made now, so nothing was written; try again once the edits stop, or write to a different note", rel)
		}
		v.sleep(settled.Sub(now))
	}
}

// remember records data as what this server last wrote to the note at clean.
func (v *Vault) remember(clean string, data []byte) {
	v.writtenMu.Lock()
	defer v.writtenMu.Unlock()
	if v.written == nil {
		v.written = map[string]string{}
	}
	v.written[clean] = Version(data)
}

// remembered moves the record of what this server wrote from one path to
// another, for a note it moved.
func (v *Vault) remembered(from, to string) {
	v.writtenMu.Lock()
	defer v.writtenMu.Unlock()
	if version, ok := v.written[from]; ok {
		delete(v.written, from)
		v.written[to] = version
	}
}

func (v *Vault) wroteLast(clean string, data []byte) bool {
	v.writtenMu.Lock()
	defer v.writtenMu.Unlock()
	return v.written[clean] == Version(data)
}
