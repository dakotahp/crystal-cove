package server

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/dakotahp/crystal-cove/internal/vault"
)

// inTrash reports whether a vault-relative path lies inside the trash, where
// Vault.Delete always deletes outright.
func inTrash(p string) bool {
	_, ok := outOfTrash(p)
	return ok
}

// outOfTrash returns where a note inside the trash goes back to when no
// destination is given: its path inside the trash, taken from the vault
// root. delete_note keeps a note's folder inside the trash, so that is where
// it was deleted from; a note trashed flat, as Obsidian itself does, goes
// back to the vault root. The second value is false when p is not inside
// the trash.
func outOfTrash(p string) (string, bool) {
	return strings.CutPrefix(path.Clean(filepath.ToSlash(p)), vault.TrashDir+"/")
}

// requireNote rejects a path that is not a Markdown note, any hidden file,
// and any path inside a hidden folder other than the vault's trash. The path
// sandbox stops escapes from the vault but allows every file inside it, so
// without this a caller could rewrite a stylesheet or Obsidian's own config.
// The trash stays reachable because delete and restore work through it.
func requireNote(path string) error {
	if !strings.EqualFold(filepath.Ext(path), vault.NoteExtension) {
		return fmt.Errorf("path %q is not a %s note: these tools work on notes only", path, vault.NoteExtension)
	}
	if !inHiddenFolder(filepath.ToSlash(path)) {
		return nil
	}
	if strings.HasPrefix(filepath.Base(path), ".") {
		return fmt.Errorf("path %q is a hidden file: these tools work on notes only", path)
	}
	return fmt.Errorf("path %q is inside a hidden folder: these tools work on notes only", path)
}

// requireVisibleDir rejects a directory inside a hidden folder other than
// the trash, so list_notes cannot enumerate .obsidian or other dotfolders.
func requireVisibleDir(dir string) error {
	if clean := path.Clean(filepath.ToSlash(dir)); clean != "." && inHiddenFolder(clean) {
		return fmt.Errorf("directory %q is a hidden folder: only notes and the %s folder can be listed", dir, vault.TrashDir)
	}
	return nil
}

func inHiddenFolder(slashPath string) bool {
	for _, part := range strings.Split(slashPath, "/") {
		if strings.HasPrefix(part, ".") && part != vault.TrashDir {
			return true
		}
	}
	return false
}

// requireWritableNote applies requireNote and also refuses the synced
// instructions note. Its text reaches every later session as server
// instructions, so a note that talks an assistant into rewriting it would
// steer every client from then on. It stays editable from Obsidian.
func requireWritableNote(path string) error {
	if err := requireNote(path); err != nil {
		return err
	}
	if strings.EqualFold(filepath.ToSlash(filepath.Clean(path)), SyncedInstructionsFile) {
		return fmt.Errorf("%s holds this server's instructions to every assistant, so tools cannot change it: edit it in Obsidian instead",
			SyncedInstructionsFile)
	}
	return nil
}

// note returns the named vault once path passes requireNote.
func (s *Server) note(vaultName, path string) (*vault.Vault, error) {
	v, err := s.vault(vaultName)
	if err != nil {
		return nil, err
	}
	if err := requireNote(path); err != nil {
		return nil, err
	}
	return v, nil
}

// writableNote returns the named vault once path passes requireWritableNote.
func (s *Server) writableNote(vaultName, path string) (*vault.Vault, error) {
	v, err := s.vault(vaultName)
	if err != nil {
		return nil, err
	}
	if err := requireWritableNote(path); err != nil {
		return nil, err
	}
	return v, nil
}
