package vault

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// updateAttempts is how many times Update reads and changes a note that
// keeps changing underneath it before giving up.
const updateAttempts = 3

// errChangedMeanwhile reports that the note on disk no longer holds what was
// read, so writing now would discard someone else's change.
var errChangedMeanwhile = errors.New("note changed while it was being written")

// Update reads the note at rel, passes its content to change, and puts what
// change returns in its place. The sync client can write the note at any
// moment, so just before the replacement Update checks that the note still
// holds what it read. When it does not, Update reads it again and change runs
// on the new content, so the other edit is kept rather than overwritten.
// It returns the content now in the note; a change that alters nothing
// writes nothing. A missing note is an error.
func (v *Vault) Update(rel string, change func(old []byte) ([]byte, error)) ([]byte, error) {
	return v.update(rel, false, change)
}

// update is Update, and when missingOK is set a missing note reaches change
// as empty content and is created, never replacing a note that appears
// meanwhile.
func (v *Vault) update(rel string, missingOK bool, change func(old []byte) ([]byte, error)) ([]byte, error) {
	root, clean, err := v.open(rel)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	for range updateAttempts {
		old, err := root.ReadFile(clean)
		missing := errors.Is(err, fs.ErrNotExist)
		if err != nil && !(missing && missingOK) {
			return nil, fmt.Errorf("reading %q: %w", rel, err)
		}
		updated, err := change(old)
		if err != nil {
			return nil, err
		}
		if !missing && bytes.Equal(updated, old) {
			return old, nil
		}
		if missing {
			err = createAtomic(root, clean, updated)
			if errors.Is(err, fs.ErrExist) {
				err = errChangedMeanwhile
			}
		} else {
			err = replaceIfUnchanged(root, clean, old, updated)
		}
		if errors.Is(err, errChangedMeanwhile) {
			continue
		}
		if err != nil {
			return nil, err
		}
		return updated, nil
	}
	return nil, fmt.Errorf("note %q kept changing while it was being written, most likely from sync: try again", rel)
}

// replaceIfUnchanged puts data at rel in one step, but only when rel still
// holds old; otherwise it returns errChangedMeanwhile. It writes a temporary
// file beside rel and renames that over it, so a reader such as the sync
// client sees the old note or the new one, never a half-written file, and an
// interrupted write leaves the original intact. The check runs after the
// temporary file is written, right before the rename, which leaves the
// smallest gap a plain filesystem allows.
func replaceIfUnchanged(root *os.Root, rel string, old, data []byte) error {
	perm := fs.FileMode(0o644)
	if info, err := root.Stat(rel); err == nil {
		perm = info.Mode().Perm()
	}
	return writeThroughTemp(root, rel, data, perm, func(tmpName string) error {
		current, err := root.ReadFile(rel)
		if err != nil || !bytes.Equal(current, old) {
			return errChangedMeanwhile
		}
		if err := root.Rename(tmpName, rel); err != nil {
			return fmt.Errorf("replacing %q: %w", rel, err)
		}
		return nil
	})
}
