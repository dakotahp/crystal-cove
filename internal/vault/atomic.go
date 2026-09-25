package vault

import (
	"crypto/rand"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// writeAtomic replaces the file at rel inside root with data in one step:
// it writes a temporary file beside it and renames that over the target. A
// reader, such as the sync client watching this folder, therefore sees
// either the old note or the new one, never a half-written file, and an
// interrupted write leaves the original intact.
func writeAtomic(root *os.Root, rel string, data []byte) error {
	perm := fs.FileMode(0o644)
	if info, err := root.Stat(rel); err == nil {
		perm = info.Mode().Perm()
	}
	return writeThroughTemp(root, rel, data, perm, func(tmpName string) error {
		if err := root.Rename(tmpName, rel); err != nil {
			return fmt.Errorf("replacing %q: %w", rel, err)
		}
		return nil
	})
}

// createAtomic writes a new file at rel in one step, as writeAtomic does, but
// fails with fs.ErrExist when rel already exists. It links the finished
// temporary file into place: unlike a rename, a link refuses to replace a
// file, even one that appeared after the caller last looked.
func createAtomic(root *os.Root, rel string, data []byte) error {
	return writeThroughTemp(root, rel, data, 0o644, func(tmpName string) error {
		return root.Link(tmpName, rel)
	})
}

// writeThroughTemp writes data to a temporary file beside rel with mode perm,
// then calls place to move it to rel. The temporary file is always removed.
func writeThroughTemp(root *os.Root, rel string, data []byte, perm fs.FileMode, place func(tmpName string) error) error {
	tmpName := filepath.Join(filepath.Dir(rel), ".tmp-"+filepath.Base(rel)+"-"+rand.Text())
	tmp, err := root.OpenFile(tmpName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("creating a temporary file beside %q: %w", rel, err)
	}
	defer root.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing %q: %w", rel, err)
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("setting permissions on %q: %w", rel, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("writing %q: %w", rel, err)
	}
	return place(tmpName)
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
