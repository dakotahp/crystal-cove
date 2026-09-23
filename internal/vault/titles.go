package vault

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// MatchTitles returns the vault-relative paths of notes whose name matches
// pattern, a regular expression applied to the file name without its
// extension. In Obsidian a note's name is usually its subject, and such a note
// often never repeats that subject in its body, so a content search alone
// misses the note a person would have opened by name.
//
// Matching ignores case unless caseSensitive is set. Hidden directories such
// as .obsidian and .trash are skipped, as they are for listing and content
// search. Results come back in path order. A limit of zero means no limit;
// otherwise the second return value reports whether matches were cut off.
func (v *Vault) MatchTitles(pattern string, caseSensitive bool, limit int) ([]string, bool, error) {
	if !caseSensitive {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, false, fmt.Errorf("invalid title pattern: %w", err)
	}

	var paths []string
	err = filepath.WalkDir(v.root, func(p string, d fs.DirEntry, walkErr error) error {
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
		if d.IsDir() {
			return nil
		}
		name := strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))
		if !re.MatchString(name) {
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
		return nil, false, fmt.Errorf("matching titles: %w", err)
	}

	sort.Strings(paths)
	if limit > 0 && len(paths) > limit {
		return paths[:limit], true, nil
	}
	return paths, false, nil
}
