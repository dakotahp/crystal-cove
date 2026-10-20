package server

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dakotahp/crystal-cove/internal/notes"
	"github.com/dakotahp/crystal-cove/internal/vault"
)

// updateLinks rewrites every link that pointed at a file moved from the
// path from, a note or a folder, so it points at the file's new path under
// to, and returns the notes it changed. A relative link inside a moved note
// is rewritten too when the move changes its folder, so it still reaches the
// same file. before lists every file as it was before the move, which is
// what the old links resolved against.
func updateLinks(v *vault.Vault, from, to string, before []string) ([]string, error) {
	from, to = path.Clean(filepath.ToSlash(from)), path.Clean(filepath.ToSlash(to))
	resolver := notes.NewResolver(before)
	newPath := map[string]string{}
	for _, p := range before {
		if moved, ok := movedPath(p, from, to); ok {
			newPath[p] = moved
		}
	}
	after, err := v.Files()
	if err != nil {
		return nil, err
	}
	names := map[string]int{}
	for _, p := range after {
		names[strings.ToLower(path.Base(p))]++
	}

	changed := []string{}
	for _, p := range after {
		if !strings.EqualFold(path.Ext(p), vault.NoteExtension) {
			continue
		}
		oldSource, sourceMoved := movedPath(p, to, from)
		if !sourceMoved {
			oldSource = p
		}
		rewrite := func(l notes.Link) (string, bool) {
			dest, ok := resolver.Resolve(l.Target, oldSource)
			if !ok {
				return "", false
			}
			if moved, ok := newPath[dest]; ok {
				dest = moved
			} else if !sourceMoved || !isRelative(l.Target) {
				return "", false
			}
			t := linkText(l.Target, p, dest, names[strings.ToLower(path.Base(dest))] <= 1)
			return t, t != l.Target
		}
		var n int
		_, err := v.Update(p, func(old []byte) ([]byte, error) {
			updated, count := notes.RewriteLinks(string(old), rewrite)
			n = count
			return []byte(updated), nil
		})
		if err != nil {
			return changed, err
		}
		if n > 0 {
			changed = append(changed, p)
		}
	}
	return changed, nil
}

// movedPath returns where the file at p went when the note or folder at from
// moved to to, and false when the move did not include p.
func movedPath(p, from, to string) (string, bool) {
	if p == from {
		return to, true
	}
	if rest, ok := strings.CutPrefix(p, from+"/"); ok {
		return to + "/" + rest, true
	}
	return "", false
}

func isRelative(target string) bool {
	return strings.HasPrefix(target, "./") || strings.HasPrefix(target, "../")
}

// linkText writes a link from the note at source to the file at dest in the
// form the old link had: a ./ or ../ path stays relative to source, a path
// from the vault root keeps its leading slash, any other path becomes the
// full path, and a bare name stays a bare name while no other note shares
// it. A note's extension is written only when the old link had one.
func linkText(old, source, dest string, nameIsUnique bool) string {
	if strings.EqualFold(path.Ext(dest), vault.NoteExtension) && !strings.EqualFold(path.Ext(old), vault.NoteExtension) {
		dest = strings.TrimSuffix(dest, path.Ext(dest))
	}
	switch {
	case isRelative(old):
		return relativePath(path.Dir(source), dest)
	case strings.HasPrefix(old, "/"):
		return "/" + dest
	case strings.Contains(old, "/") || !nameIsUnique:
		return dest
	}
	return path.Base(dest)
}

// relativePath returns the path from the folder dir to the file at dest,
// both vault-relative, starting with ./ or ../ so it reads as relative.
func relativePath(dir, dest string) string {
	var from []string
	if dir != "." {
		from = strings.Split(dir, "/")
	}
	to := strings.Split(dest, "/")
	shared := 0
	for shared < len(from) && shared < len(to)-1 && from[shared] == to[shared] {
		shared++
	}
	up := strings.Repeat("../", len(from)-shared)
	if up == "" {
		up = "./"
	}
	return up + strings.Join(to[shared:], "/")
}

// moveWithLinks moves a note or folder with move and, when asked, points the
// links to what moved at its new path, reporting whether it did. A note
// moved into or out of the trash keeps its links as they are: a deleted note
// is not a link target, and links to it resolve again by name once it is
// back.
func moveWithLinks(v *vault.Vault, move func(from, to string) error, from, to string, update bool) (changed []string, updated bool, err error) {
	update = update && !inTrash(from) && !inTrash(to)
	var before []string
	if update {
		if before, err = v.Files(); err != nil {
			return nil, false, err
		}
	}
	if err := move(from, to); err != nil {
		return nil, false, err
	}
	if !update {
		return []string{}, false, nil
	}
	changed, err = updateLinks(v, from, to, before)
	if err != nil {
		return changed, true, fmt.Errorf("moved %q to %q, but updating links stopped after %d notes: %w", from, to, len(changed), err)
	}
	return changed, true, nil
}

// ResolvedLink is one wikilink and the note it points at.
type ResolvedLink struct {
	notes.Link
	// Path is the linked file's vault-relative path, empty when the link
	// points at a file that does not exist.
	Path string `json:"path,omitempty"`
	// Resolved reports whether the linked file exists.
	Resolved bool `json:"resolved"`
}

// LinkList is the outcome of get_links.
type LinkList struct {
	Path  string         `json:"path"`
	Links []ResolvedLink `json:"links"`
}

func (s *Server) getLinks(_ context.Context, _ *mcp.CallToolRequest, in noteRef) (*mcp.CallToolResult, *LinkList, error) {
	v, err := s.note(in.Vault, in.Path)
	if err != nil {
		return nil, nil, err
	}
	n, err := parseNote(v, in.Path)
	if err != nil {
		return nil, nil, err
	}
	resolver, err := resolverFor(v)
	if err != nil {
		return nil, nil, err
	}

	out := &LinkList{Path: in.Path, Links: []ResolvedLink{}}
	for _, link := range n.Links() {
		resolved := ResolvedLink{Link: link}
		if path, ok := resolver.Resolve(link.Target, in.Path); ok {
			resolved.Path = path
			resolved.Resolved = true
		}
		out.Links = append(out.Links, resolved)
	}
	return nil, out, nil
}

// Backlink is one note that links to the note asked about.
type Backlink struct {
	// Path is the linking note.
	Path string `json:"path"`
	// Links are that note's links pointing here, keeping any heading or
	// alias so the caller can see how it refers to the note.
	Links []notes.Link `json:"links"`
}

// BacklinkList is the outcome of get_backlinks.
type BacklinkList struct {
	Path      string     `json:"path"`
	Backlinks []Backlink `json:"backlinks"`
	// NotesScanned is how many notes were read to find them.
	NotesScanned int `json:"notes_scanned"`
}

func (s *Server) getBacklinks(_ context.Context, _ *mcp.CallToolRequest, in noteRef) (*mcp.CallToolResult, *BacklinkList, error) {
	v, err := s.note(in.Vault, in.Path)
	if err != nil {
		return nil, nil, err
	}
	if _, err := v.ReadAll(in.Path); err != nil {
		return nil, nil, err
	}
	resolver, err := resolverFor(v)
	if err != nil {
		return nil, nil, err
	}

	out := &BacklinkList{Path: in.Path, Backlinks: []Backlink{}}
	scanned, err := eachNote(v, func(path string, n *notes.Note) error {
		if path == in.Path {
			return nil
		}
		var hits []notes.Link
		for _, link := range n.Links() {
			if target, ok := resolver.Resolve(link.Target, path); ok && target == in.Path {
				hits = append(hits, link)
			}
		}
		if len(hits) > 0 {
			out.Backlinks = append(out.Backlinks, Backlink{Path: path, Links: hits})
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	out.NotesScanned = scanned
	return nil, out, nil
}
