package vault

import (
	"fmt"
	"io/fs"
	"sort"
	"time"
)

// NoteExtension is the file extension a vault's notes carry.
const NoteExtension = ".md"

// Notes returns every note's vault-relative path, in path order. Hidden
// directories such as .obsidian and .trash are skipped, as they are for
// listing and search.
func (v *Vault) Notes() ([]string, error) {
	var paths []string
	err := v.walkNotes(func(rel string, _ fs.DirEntry) error {
		paths = append(paths, rel)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing notes: %w", err)
	}
	sort.Strings(paths)
	return paths, nil
}

// RecentNotes returns notes ordered by modification time, newest first.
// A limit of zero returns them all, and a zero since includes every note.
// Agents use this to pick up where work left off, which a path-ordered
// listing cannot answer.
func (v *Vault) RecentNotes(limit int, since time.Time) ([]Entry, error) {
	entries := []Entry{}
	err := v.walkNotes(func(rel string, d fs.DirEntry) error {
		e, err := entryAt(rel, d)
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

// ReadAll returns a note's full content. Read pages for MCP clients;
// this is for code that has to parse a whole note.
func (v *Vault) ReadAll(rel string) ([]byte, error) {
	root, clean, err := v.open(rel)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	data, err := root.ReadFile(clean)
	if err != nil {
		return nil, fmt.Errorf("reading %q: %w", rel, err)
	}
	return data, nil
}

// WriteAll replaces a note's content.
func (v *Vault) WriteAll(rel string, data []byte) error {
	root, clean, err := v.open(rel)
	if err != nil {
		return err
	}
	defer root.Close()
	return writeAtomic(root, clean, data)
}
