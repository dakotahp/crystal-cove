package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// guardedVault returns a vault holding Inbox/a.md, last changed by another
// device editedAgo before the fake clock's start, with the edit guard on.
// Sleeping advances the fake clock and runs onSleep, if set.
func guardedVault(t *testing.T, editedAgo time.Duration, onSleep func(now time.Time)) (*Vault, *[]time.Duration) {
	t.Helper()
	v := newNotesVault(t, "Inbox/a.md")
	now := time.Now()
	touch(t, v, now.Add(-editedAgo))
	slept := &[]time.Duration{}
	v.now = func() time.Time { return now }
	v.sleep = func(d time.Duration) {
		*slept = append(*slept, d)
		now = now.Add(d)
		if onSleep != nil {
			onSleep(now)
		}
	}
	v.SetEditGuard(EditGuard{Quiet: 10 * time.Second, MaxWait: 30 * time.Second})
	return v, slept
}

func touch(t *testing.T, v *Vault, at time.Time) {
	t.Helper()
	if err := os.Chtimes(filepath.Join(v.Root(), "Inbox", "a.md"), at, at); err != nil {
		t.Fatal(err)
	}
}

func TestWriteWaitsUntilAnOutsideEditSettles(t *testing.T) {
	v, slept := guardedVault(t, 2*time.Second, nil)

	if err := v.Append("Inbox/a.md", "agent\n"); err != nil {
		t.Fatal(err)
	}
	if len(*slept) != 1 || (*slept)[0] != 8*time.Second {
		t.Errorf("slept %v, want one 8s wait for the 10s quiet period to finish", *slept)
	}
	if data, _ := os.ReadFile(filepath.Join(v.Root(), "Inbox", "a.md")); string(data) != "body\nagent\n" {
		t.Errorf("note = %q, want the append written after the wait", data)
	}
}

func TestWriteGoesAheadAtOnceOnAQuietNote(t *testing.T) {
	v, slept := guardedVault(t, time.Minute, nil)

	if err := v.Append("Inbox/a.md", "agent\n"); err != nil {
		t.Fatal(err)
	}
	if len(*slept) != 0 {
		t.Errorf("slept %v, want no wait", *slept)
	}
}

func TestWriteDoesNotWaitAfterThisServersOwnWrite(t *testing.T) {
	v, slept := guardedVault(t, time.Minute, nil)
	v.now = time.Now

	if err := v.Append("Inbox/a.md", "first\n"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := v.Edit("Inbox/a.md", "", Replacement{Find: "first", Replace: "second"}); err != nil {
		t.Fatal(err)
	}
	if len(*slept) != 0 {
		t.Errorf("slept %v, want no wait after the server's own writes", *slept)
	}
}

func TestWriteDoesNotWaitAfterCreatingOrMovingANote(t *testing.T) {
	v, slept := guardedVault(t, time.Minute, nil)
	v.now = time.Now

	if err := v.Create("Inbox/new.md", "new\n"); err != nil {
		t.Fatal(err)
	}
	if err := v.Append("Inbox/new.md", "more\n"); err != nil {
		t.Fatal(err)
	}
	if err := v.Move("Inbox/new.md", "Inbox/moved.md"); err != nil {
		t.Fatal(err)
	}
	if err := v.Append("Inbox/moved.md", "after move\n"); err != nil {
		t.Fatal(err)
	}
	if len(*slept) != 0 {
		t.Errorf("slept %v, want no wait on notes only this server wrote", *slept)
	}
}

func TestWriteDoesNotWaitAfterMovingTheFolderOfANote(t *testing.T) {
	v, slept := guardedVault(t, time.Minute, nil)
	v.now = time.Now

	if err := v.Create("Inbox/Project/new.md", "new\n"); err != nil {
		t.Fatal(err)
	}
	if err := v.MoveFolder("Inbox/Project", "Archive/Project"); err != nil {
		t.Fatal(err)
	}
	if err := v.Append("Archive/Project/new.md", "after move\n"); err != nil {
		t.Fatal(err)
	}
	if len(*slept) != 0 {
		t.Errorf("slept %v, want no wait on a note only this server wrote", *slept)
	}
}

func TestWriteIsRefusedWhileANoteKeepsChangingElsewhere(t *testing.T) {
	var v *Vault
	typing := func(now time.Time) {
		target := filepath.Join(v.Root(), "Inbox", "a.md")
		if err := os.WriteFile(target, []byte("typed at "+now.String()+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		touch(t, v, now)
	}
	v, slept := guardedVault(t, 0, typing)

	err := v.Append("Inbox/a.md", "agent\n")
	if err == nil || !strings.Contains(err.Error(), "another device") {
		t.Fatalf("err = %v, want a refusal naming another device", err)
	}
	var total time.Duration
	for _, d := range *slept {
		total += d
	}
	if total > 30*time.Second {
		t.Errorf("waited %v in all, want at most the 30s limit", total)
	}
	if data, _ := os.ReadFile(filepath.Join(v.Root(), "Inbox", "a.md")); strings.Contains(string(data), "agent") {
		t.Errorf("note = %q, want nothing written", data)
	}
}

func TestWriteToAMissingNoteDoesNotWait(t *testing.T) {
	v, slept := guardedVault(t, 0, nil)

	if err := v.Append("Inbox/b.md", "new\n"); err != nil {
		t.Fatal(err)
	}
	if len(*slept) != 0 {
		t.Errorf("slept %v, want no wait for a note that does not exist yet", *slept)
	}
}
