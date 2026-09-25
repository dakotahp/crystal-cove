// Package vault provides sandboxed filesystem operations over a single
// synced Obsidian vault directory.
package vault

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
)

// ReadPageSize is the maximum number of characters returned by a single
// Read call. Longer notes are paged via the offset parameter.
const ReadPageSize = 10240

// TrashDir is the vault-relative directory soft-deleted notes move to. It
// matches Obsidian's own "move to vault trash" convention, so deletions
// sync and remain recoverable from any device.
const TrashDir = ".trash"

// Vault exposes file operations rooted at a synced vault directory. All
// paths are vault-relative; attempts to escape the root are rejected.
type Vault struct {
	name string
	root string
}

// New returns a Vault named name rooted at the absolute directory root.
func New(name, root string) *Vault {
	return &Vault{name: name, root: root}
}

// Name returns the vault's name.
func (v *Vault) Name() string { return v.name }

// Root returns the vault's root directory.
func (v *Vault) Root() string { return v.root }

// Entry describes a file or directory inside the vault.
type Entry struct {
	// Path is the vault-relative path, using forward slashes.
	Path string `json:"path"`
	// IsDir reports whether the entry is a directory.
	IsDir bool `json:"is_dir"`
	// Size is the file size in bytes; zero for directories.
	Size int64 `json:"size"`
	// Modified is when the entry last changed, in UTC.
	Modified time.Time `json:"modified"`
}

// ReadResult is one page of a note's content.
type ReadResult struct {
	// Content is up to ReadPageSize characters starting at Offset.
	Content string `json:"content"`
	// Offset is the character offset this page starts at.
	Offset int `json:"offset"`
	// TotalCharacters is the full length of the note in characters.
	TotalCharacters int `json:"total_characters"`
	// Truncated reports whether content remains after this page.
	Truncated bool `json:"truncated"`
	// NextOffset is the offset to pass to read the next page; -1 when the
	// note has been read to the end.
	NextOffset int `json:"next_offset"`
}

// resolve cleans a vault-relative path, rejecting empty, absolute, and
// root-escaping paths. It checks the text only; symlinks are caught by
// opening the path through the vault's os.Root.
func (v *Vault) resolve(rel string) (string, error) {
	if rel == "" {
		return "", errors.New("path must not be empty")
	}
	if path.IsAbs(rel) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("path %q must be vault-relative, not absolute", rel)
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes the vault root", rel)
	}
	return clean, nil
}

// open resolves rel and opens the vault directory as an os.Root, which
// refuses any path, symlinks included, that leads outside the vault. Every
// file operation goes through it. The caller closes the root.
func (v *Vault) open(rel string) (*os.Root, string, error) {
	clean, err := v.resolve(rel)
	if err != nil {
		return nil, "", err
	}
	root, err := v.openRoot()
	if err != nil {
		return nil, "", err
	}
	return root, clean, nil
}

func (v *Vault) openRoot() (*os.Root, error) {
	root, err := os.OpenRoot(v.root)
	if err != nil {
		return nil, fmt.Errorf("opening vault %q: %w", v.name, err)
	}
	return root, nil
}

// List returns entries under dir (vault root when dir is empty), skipping
// hidden files and directories such as .obsidian and .trash. Passing a
// hidden directory such as .trash as dir lists inside it explicitly. When
// recursive is true it descends into subdirectories.
func (v *Vault) List(dir string, recursive bool) ([]Entry, error) {
	base := "."
	if dir != "" {
		clean, err := v.resolve(dir)
		if err != nil {
			return nil, err
		}
		base = filepath.ToSlash(clean)
	}
	// entries starts non-nil so an empty listing marshals as [], not null.
	entries := []Entry{}
	err := v.walk(base, func(rel string, d fs.DirEntry) error {
		e, err := entryAt(rel, d)
		if err != nil {
			return err
		}
		entries = append(entries, e)
		if d.IsDir() && !recursive {
			return fs.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing %q: %w", dir, err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

// entryAt describes d, found at the slash-separated vault-relative path rel.
func entryAt(rel string, d fs.DirEntry) (Entry, error) {
	e := Entry{Path: rel, IsDir: d.IsDir()}
	info, err := d.Info()
	if err != nil {
		return Entry{}, err
	}
	e.Modified = info.ModTime().UTC()
	if !d.IsDir() {
		e.Size = info.Size()
	}
	return e, nil
}

// Read returns one page of up to ReadPageSize characters of the note at
// path, starting at the character offset.
func (v *Vault) Read(rel string, offset int) (*ReadResult, error) {
	data, err := v.ReadAll(rel)
	if err != nil {
		return nil, err
	}
	return page(string(data), offset, "note")
}

// page returns up to ReadPageSize characters of text starting at the
// character offset. what names the text in the out-of-range error.
func page(text string, offset int, what string) (*ReadResult, error) {
	runes := []rune(text)
	total := len(runes)
	if offset < 0 || offset > total {
		return nil, fmt.Errorf("offset %d is out of range: %s has %d characters", offset, what, total)
	}
	end := min(offset+ReadPageSize, total)
	res := &ReadResult{
		Content:         string(runes[offset:end]),
		Offset:          offset,
		TotalCharacters: total,
		Truncated:       end < total,
		NextOffset:      -1,
	}
	if res.Truncated {
		res.NextOffset = end
	}
	return res, nil
}

// Create writes a new note at path, creating parent directories as needed.
// It fails if the note already exists.
func (v *Vault) Create(rel, content string) error {
	root, clean, err := v.open(rel)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.MkdirAll(filepath.Dir(clean), 0o755); err != nil {
		return fmt.Errorf("creating parent directories for %q: %w", rel, err)
	}
	if err := createAtomic(root, clean, []byte(content)); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("note %q already exists: use append_note or edit_note to modify it", rel)
		}
		return fmt.Errorf("creating %q: %w", rel, err)
	}
	return nil
}

// Append appends content to the note at path, creating it (and parent
// directories) if it does not exist. Content starts on a new line: when the
// note does not end with a line break, one is added first, in the note's own
// line-ending style.
func (v *Vault) Append(rel, content string) error {
	root, clean, err := v.open(rel)
	if err != nil {
		return err
	}
	err = root.MkdirAll(filepath.Dir(clean), 0o755)
	root.Close()
	if err != nil {
		return fmt.Errorf("creating parent directories for %q: %w", rel, err)
	}
	_, err = v.update(rel, true, func(existing []byte) ([]byte, error) {
		sep := ""
		if len(existing) > 0 && content != "" && existing[len(existing)-1] != '\n' {
			sep = "\n"
			if bytes.Contains(existing, []byte("\r\n")) {
				sep = "\r\n"
			}
		}
		return slices.Concat(existing, []byte(sep), []byte(content)), nil
	})
	if err != nil {
		return fmt.Errorf("appending to %q: %w", rel, err)
	}
	return nil
}

// Edit replaces find with replace in the note at path and returns the number
// of replacements made. Unless replaceAll is true, find must occur exactly
// once.
func (v *Vault) Edit(rel, find, replace string, replaceAll bool) (int, error) {
	if find == "" {
		return 0, errors.New("find must not be empty")
	}
	var count int
	_, err := v.Update(rel, func(old []byte) ([]byte, error) {
		content := string(old)
		count = strings.Count(content, find)
		if count == 0 {
			return nil, fmt.Errorf("text not found in %q", rel)
		}
		if count > 1 && !replaceAll {
			return nil, fmt.Errorf("text occurs %d times in %q: provide more surrounding context to make it unique, or set replace_all", count, rel)
		}
		return []byte(strings.ReplaceAll(content, find, replace)), nil
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}

// Move renames a note from one vault-relative path to another, creating
// destination parent directories as needed. It fails if the destination
// already exists.
func (v *Vault) Move(from, to string) error {
	dst, err := v.resolve(to)
	if err != nil {
		return err
	}
	root, src, err := v.open(from)
	if err != nil {
		return err
	}
	defer root.Close()
	if _, err := root.Stat(src); err != nil {
		return fmt.Errorf("moving %q: %w", from, err)
	}
	if _, err := root.Lstat(dst); err == nil {
		return fmt.Errorf("destination %q already exists", to)
	}
	if err := root.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("creating parent directories for %q: %w", to, err)
	}
	if err := root.Rename(src, dst); err != nil {
		return fmt.Errorf("moving %q to %q: %w", from, to, err)
	}
	return nil
}

// Delete removes the note at path. By default it is moved into the vault's
// .trash directory (recoverable, and the move syncs); when permanent is
// true the note is removed outright.
func (v *Vault) Delete(rel string, permanent bool) (trashedTo string, err error) {
	root, clean, err := v.open(rel)
	if err != nil {
		return "", err
	}
	defer root.Close()
	if _, err := root.Stat(clean); err != nil {
		return "", fmt.Errorf("deleting %q: %w", rel, err)
	}
	if strings.HasPrefix(filepath.ToSlash(rel), TrashDir+"/") || rel == TrashDir {
		permanent = true
	}
	if permanent {
		if err := root.RemoveAll(clean); err != nil {
			return "", fmt.Errorf("deleting %q: %w", rel, err)
		}
		return "", nil
	}
	if err := root.MkdirAll(TrashDir, 0o755); err != nil {
		return "", fmt.Errorf("creating trash directory: %w", err)
	}
	dst := uniquePath(root, filepath.Join(TrashDir, filepath.Base(clean)))
	if err := root.Rename(clean, dst); err != nil {
		return "", fmt.Errorf("moving %q to trash: %w", rel, err)
	}
	return TrashDir + "/" + filepath.Base(dst), nil
}

// uniquePath returns p, or p with " (n)" inserted before the extension when
// p already exists, matching how Obsidian resolves trash collisions.
func uniquePath(root *os.Root, p string) string {
	if _, err := root.Lstat(p); errors.Is(err, fs.ErrNotExist) {
		return p
	}
	ext := filepath.Ext(p)
	stem := strings.TrimSuffix(p, ext)
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s (%d)%s", stem, i, ext)
		if _, err := root.Lstat(candidate); errors.Is(err, fs.ErrNotExist) {
			return candidate
		}
	}
}
