package vault

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// NoteExtension is the file extension a vault's notes carry.
const NoteExtension = ".md"

// Notes returns every note's vault-relative path, in path order. Hidden
// directories such as .obsidian and .trash are skipped, as they are for
// listing and search.
func (v *Vault) Notes() ([]string, error) {
	var paths []string
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
		rel, err := filepath.Rel(v.root, p)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing notes: %w", err)
	}
	sort.Strings(paths)
	return paths, nil
}

// ReadAll returns a note's full content. Read pages for MCP clients;
// this is for code that has to parse a whole note.
func (v *Vault) ReadAll(rel string) ([]byte, error) {
	abs, err := v.resolve(rel)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("reading %q: %w", rel, err)
	}
	return data, nil
}

// WriteAll replaces a note's content.
func (v *Vault) WriteAll(rel string, data []byte) error {
	abs, err := v.resolve(rel)
	if err != nil {
		return err
	}
	return writeAtomic(abs, data)
}
