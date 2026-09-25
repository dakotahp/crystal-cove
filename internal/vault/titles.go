package vault

import (
	"fmt"
	"io/fs"
	"path"
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

	return v.matchNames(re.MatchString, limit)
}

// MatchTitleWords returns notes whose name holds every word, in any order,
// which is what a multi-word search means. Go's regexp engine has no
// lookahead, so this cannot be expressed as one pattern.
func (v *Vault) MatchTitleWords(words []string, caseSensitive bool, limit int) ([]string, bool, error) {
	prepared := make([]string, len(words))
	for i, w := range words {
		if caseSensitive {
			prepared[i] = w
		} else {
			prepared[i] = strings.ToLower(w)
		}
	}
	holdsAll := func(name string) bool {
		if !caseSensitive {
			name = strings.ToLower(name)
		}
		for _, w := range prepared {
			if !strings.Contains(name, w) {
				return false
			}
		}
		return true
	}
	return v.matchNames(holdsAll, limit)
}

func (v *Vault) matchNames(matches func(name string) bool, limit int) ([]string, bool, error) {
	var paths []string
	err := v.walkNotes(func(rel string, d fs.DirEntry) error {
		if matches(strings.TrimSuffix(d.Name(), path.Ext(d.Name()))) {
			paths = append(paths, rel)
		}
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
