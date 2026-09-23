package vault

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// writeAtomic replaces the file at abs with data in one step: it writes a
// temporary file beside it and renames that over the target. A reader, such
// as the sync client watching this folder, therefore sees either the old
// note or the new one, never a half-written file, and an interrupted write
// leaves the original intact.
func writeAtomic(abs string, data []byte) error {
	dir := filepath.Dir(abs)
	perm := fs.FileMode(0o644)
	if info, err := os.Stat(abs); err == nil {
		perm = info.Mode().Perm()
	}

	tmp, err := os.CreateTemp(dir, ".tmp-"+filepath.Base(abs)+"-*")
	if err != nil {
		return fmt.Errorf("creating a temporary file beside %q: %w", abs, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("writing %q: %w", abs, err)
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return fmt.Errorf("setting permissions on %q: %w", abs, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("writing %q: %w", abs, err)
	}
	if err := os.Rename(tmpName, abs); err != nil {
		return fmt.Errorf("replacing %q: %w", abs, err)
	}
	return nil
}

// RecentNotes returns notes ordered by modification time, newest first.
// A limit of zero returns them all, and a zero since includes every note.
// Agents use this to pick up where work left off, which a path-ordered
// listing cannot answer.
func (v *Vault) RecentNotes(limit int, since time.Time) ([]Entry, error) {
	entries := []Entry{}
	err := filepath.WalkDir(v.root, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if p == v.root {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || !strings.EqualFold(filepath.Ext(d.Name()), NoteExtension) {
			return nil
		}
		e, err := newEntry(v.root, p, d)
		if err != nil {
			return err
		}
		if !since.IsZero() && e.Modified.Before(since) {
			return nil
		}
		entries = append(entries, e)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing recent notes: %w", err)
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Modified.Equal(entries[j].Modified) {
			return entries[i].Path < entries[j].Path
		}
		return entries[i].Modified.After(entries[j].Modified)
	})
	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}
	return entries, nil
}
