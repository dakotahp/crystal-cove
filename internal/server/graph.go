package server

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dakotahp/crystal-cove/internal/notes"
	"github.com/dakotahp/crystal-cove/internal/vault"
)

// folderFilter limits which notes a vault-wide tool reports.
type folderFilter struct {
	Folders        []string `json:"folders,omitempty" jsonschema:"only notes inside these folders, subfolders included; a note in any of them counts"`
	ExcludeFolders []string `json:"exclude_folders,omitempty" jsonschema:"leave out notes inside these folders, subfolders included, such as an imported reference set"`
}

func (f folderFilter) includes(path string) bool {
	if len(f.Folders) > 0 && !inAnyFolder(path, f.Folders) {
		return false
	}
	return !inAnyFolder(path, f.ExcludeFolders)
}

func inAnyFolder(path string, folders []string) bool {
	for _, folder := range folders {
		folder = strings.Trim(folder, "/")
		if folder == "" || strings.HasPrefix(path, folder+"/") {
			return true
		}
	}
	return false
}

// resolverFor returns a link resolver over every file in the vault.
func resolverFor(v *vault.Vault) (*notes.Resolver, error) {
	files, err := v.Files()
	if err != nil {
		return nil, err
	}
	return notes.NewResolver(files), nil
}

type unresolvedLinksInput struct {
	Vault string `json:"vault,omitempty" jsonschema:"the vault to scan; optional when the server holds one vault"`
	folderFilter
	Offset int `json:"offset,omitempty" jsonschema:"link to start from; use next_offset from the previous page"`
	Limit  int `json:"limit,omitempty" jsonschema:"maximum links to return (default 200, max 1000)"`
}

// UnresolvedLink is one link that points at no file in the vault.
type UnresolvedLink struct {
	// Source is the note holding the link.
	Source string `json:"source"`
	notes.Link
}

// UnresolvedLinks is the outcome of unresolved_links.
type UnresolvedLinks struct {
	Links        []UnresolvedLink `json:"links"`
	NotesScanned int              `json:"notes_scanned"`
	Total        int              `json:"total" jsonschema:"unresolved links across every page"`
	NextOffset   int              `json:"next_offset" jsonschema:"offset of the next page; -1 when this is the last page"`
}

func (s *Server) unresolvedLinks(_ context.Context, _ *mcp.CallToolRequest, in unresolvedLinksInput) (*mcp.CallToolResult, *UnresolvedLinks, error) {
	v, err := s.vault(in.Vault)
	if err != nil {
		return nil, nil, err
	}
	resolver, err := resolverFor(v)
	if err != nil {
		return nil, nil, err
	}
	found := []UnresolvedLink{}
	scanned, err := eachNote(v, func(path string, n *notes.Note) error {
		if !in.includes(path) {
			return nil
		}
		for _, link := range n.Links() {
			if _, ok := resolver.Resolve(link.Target, path); !ok {
				found = append(found, UnresolvedLink{Source: path, Link: link})
			}
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	page, next, err := pageOf(found, in.Offset, in.Limit, DefaultListLimit)
	if err != nil {
		return nil, nil, err
	}
	return nil, &UnresolvedLinks{Links: page, NotesScanned: scanned, Total: len(found), NextOffset: next}, nil
}

type orphansInput struct {
	Vault string `json:"vault,omitempty" jsonschema:"the vault to scan; optional when the server holds one vault"`
	folderFilter
	Unlinked bool `json:"unlinked,omitempty" jsonschema:"only notes that also link to nothing, which Obsidian's graph shows unconnected"`
	Offset   int  `json:"offset,omitempty" jsonschema:"note to start from; use next_offset from the previous page"`
	Limit    int  `json:"limit,omitempty" jsonschema:"maximum notes to return (default 200, max 1000)"`
}

// Orphan is a note no other note links to.
type Orphan struct {
	Path string `json:"path"`
	// OutgoingLinks counts the files this note links to, itself excluded.
	OutgoingLinks int `json:"outgoing_links"`
}

// OrphanList is the outcome of orphans.
type OrphanList struct {
	Notes        []Orphan `json:"notes"`
	NotesScanned int      `json:"notes_scanned"`
	Total        int      `json:"total" jsonschema:"orphan notes across every page"`
	NextOffset   int      `json:"next_offset" jsonschema:"offset of the next page; -1 when this is the last page"`
}

func (s *Server) orphans(_ context.Context, _ *mcp.CallToolRequest, in orphansInput) (*mcp.CallToolResult, *OrphanList, error) {
	v, err := s.vault(in.Vault)
	if err != nil {
		return nil, nil, err
	}
	resolver, err := resolverFor(v)
	if err != nil {
		return nil, nil, err
	}
	linkedTo := map[string]bool{}
	outgoing := map[string]int{}
	var paths []string
	scanned, err := eachNote(v, func(path string, n *notes.Note) error {
		paths = append(paths, path)
		targets := map[string]bool{}
		for _, link := range n.Links() {
			if target, ok := resolver.Resolve(link.Target, path); ok && target != path {
				targets[target] = true
				linkedTo[target] = true
			}
		}
		outgoing[path] = len(targets)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	found := []Orphan{}
	for _, path := range paths {
		if linkedTo[path] || !in.includes(path) || (in.Unlinked && outgoing[path] > 0) {
			continue
		}
		found = append(found, Orphan{Path: path, OutgoingLinks: outgoing[path]})
	}
	page, next, err := pageOf(found, in.Offset, in.Limit, DefaultListLimit)
	if err != nil {
		return nil, nil, err
	}
	return nil, &OrphanList{Notes: page, NotesScanned: scanned, Total: len(found), NextOffset: next}, nil
}
