package vault

import (
	"fmt"
	"io/fs"
	"path"
	"strings"
)

// walk calls fn for every visible entry under the vault-relative directory
// base, excluding base itself, with slash-separated vault-relative paths. It
// reads through the vault's os.Root, and skips hidden entries such as
// .obsidian and .trash and symlinks that lead outside the vault, so a walk
// never offers a path the other operations would refuse. fn may return
// fs.SkipDir for a directory it does not want entered.
func (v *Vault) walk(base string, fn func(rel string, d fs.DirEntry) error) error {
	root, err := v.openRoot()
	if err != nil {
		return err
	}
	defer root.Close()
	return fs.WalkDir(root.FS(), base, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == base {
			if !d.IsDir() {
				return fmt.Errorf("%s is not a directory", base)
			}
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			if _, err := root.Stat(p); err != nil {
				return nil
			}
		}
		return fn(p, d)
	})
}

// walkNotes calls fn for every note walk finds in the whole vault.
func (v *Vault) walkNotes(fn func(rel string, d fs.DirEntry) error) error {
	return v.walk(".", func(rel string, d fs.DirEntry) error {
		if d.IsDir() || !strings.EqualFold(path.Ext(rel), NoteExtension) {
			return nil
		}
		return fn(rel, d)
	})
}
