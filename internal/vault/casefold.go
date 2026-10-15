package vault

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// checkCase refuses rel when one of its parts matches an existing file or
// folder in everything but letter case. The server's filesystem tells such
// names apart, but macOS, Windows, iOS and Android treat them as one, so
// sync merges them on those devices and one note's text is lost. except is
// a path allowed to match, so a note can be renamed to change only its case.
func checkCase(root *os.Root, rel, except string) error {
	rel, except = filepath.ToSlash(rel), filepath.ToSlash(except)
	dir := "."
	for part := range strings.SplitSeq(rel, "/") {
		entries, err := fs.ReadDir(root.FS(), dir)
		if err != nil {
			// A part that is not a folder: the write that follows reports it.
			return nil
		}
		exact := false
		for _, e := range entries {
			exact = exact || e.Name() == part
		}
		if exact {
			dir = path.Join(dir, part)
			continue
		}
		for _, e := range entries {
			existing := path.Join(dir, e.Name())
			if strings.EqualFold(e.Name(), part) && existing != except {
				return fmt.Errorf("%q differs only in letter case from %q, which already exists: Mac, Windows and phone file systems treat the two names as one, so sync would merge them and lose one note's text; use %q or choose another name", rel, existing, existing)
			}
		}
		return nil
	}
	return nil
}
