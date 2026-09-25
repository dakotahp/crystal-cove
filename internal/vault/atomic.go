package vault

import (
	"crypto/rand"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// createAtomic writes a new file at rel in one step, as replaceIfUnchanged
// does, but fails with fs.ErrExist when rel already exists. It links the
// finished temporary file into place: unlike a rename, a link refuses to
// replace a file, even one that appeared after the caller last looked.
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
