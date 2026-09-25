package server

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dakotahp/crystal-cove/internal/vault"
)

type listVaultsOutput struct {
	Vaults []string `json:"vaults" jsonschema:"names of the available vaults"`
}

func (s *Server) listVaults(context.Context, *mcp.CallToolRequest, any) (*mcp.CallToolResult, listVaultsOutput, error) {
	return nil, listVaultsOutput{Vaults: s.vaultNames()}, nil
}

type listNotesInput struct {
	Vault     string `json:"vault,omitempty" jsonschema:"name of the vault to list; optional when the server holds one vault"`
	Dir       string `json:"dir,omitempty" jsonschema:"vault-relative directory to list; defaults to the vault root"`
	Recursive bool   `json:"recursive,omitempty" jsonschema:"list subdirectories recursively"`
	Offset    int    `json:"offset,omitempty" jsonschema:"entry to start from; use next_offset from the previous page"`
	Limit     int    `json:"limit,omitempty" jsonschema:"maximum entries to return (default 200, max 1000)"`
}

// DefaultListLimit and MaxListLimit bound one page of list_notes, so a
// recursive listing of a large vault cannot fill a model's context.
const (
	DefaultListLimit = 200
	MaxListLimit     = 1000
)

type listNotesOutput struct {
	Entries []vault.Entry `json:"entries" jsonschema:"files and directories found, in path order"`
	// Total counts every entry in the listing, not only this page.
	Total int `json:"total" jsonschema:"entries in the whole listing"`
	// NextOffset is -1 on the last page.
	NextOffset int `json:"next_offset" jsonschema:"offset of the next page; -1 when this is the last page"`
}

func (s *Server) listNotes(_ context.Context, _ *mcp.CallToolRequest, in listNotesInput) (*mcp.CallToolResult, listNotesOutput, error) {
	v, err := s.vault(in.Vault)
	if err != nil {
		return nil, listNotesOutput{}, err
	}
	if err := requireVisibleDir(in.Dir); err != nil {
		return nil, listNotesOutput{}, err
	}
	entries, err := v.List(in.Dir, in.Recursive)
	if err != nil {
		return nil, listNotesOutput{}, err
	}
	total := len(entries)
	if in.Offset < 0 || in.Offset > total {
		return nil, listNotesOutput{}, fmt.Errorf("offset %d is out of range: the listing has %d entries", in.Offset, total)
	}
	limit := in.Limit
	if limit <= 0 {
		limit = DefaultListLimit
	}
	end := min(in.Offset+min(limit, MaxListLimit), total)
	out := listNotesOutput{Entries: entries[in.Offset:end], Total: total, NextOffset: -1}
	if end < total {
		out.NextOffset = end
	}
	return nil, out, nil
}

type readNoteInput struct {
	Vault  string `json:"vault,omitempty" jsonschema:"name of the vault; optional when the server holds one vault"`
	Path   string `json:"path" jsonschema:"vault-relative path of the note"`
	Offset int    `json:"offset,omitempty" jsonschema:"character offset to start reading from; use next_offset from a previous truncated read"`
}

func (s *Server) readNote(_ context.Context, _ *mcp.CallToolRequest, in readNoteInput) (*mcp.CallToolResult, *vault.ReadResult, error) {
	v, err := s.note(in.Vault, in.Path)
	if err != nil {
		return nil, nil, err
	}
	res, err := v.Read(in.Path, in.Offset)
	if err != nil {
		return nil, nil, err
	}
	return nil, res, nil
}

type writeNoteInput struct {
	Vault   string `json:"vault,omitempty" jsonschema:"name of the vault; optional when the server holds one vault"`
	Path    string `json:"path" jsonschema:"vault-relative path of the note"`
	Content string `json:"content" jsonschema:"markdown content"`
}

type okOutput struct {
	OK bool `json:"ok"`
}

func (s *Server) createNote(_ context.Context, _ *mcp.CallToolRequest, in writeNoteInput) (*mcp.CallToolResult, okOutput, error) {
	v, err := s.writableNote(in.Vault, in.Path)
	if err != nil {
		return nil, okOutput{}, err
	}
	if err := v.Create(in.Path, in.Content); err != nil {
		return nil, okOutput{}, err
	}
	return nil, okOutput{OK: true}, nil
}

func (s *Server) appendNote(_ context.Context, _ *mcp.CallToolRequest, in writeNoteInput) (*mcp.CallToolResult, okOutput, error) {
	v, err := s.writableNote(in.Vault, in.Path)
	if err != nil {
		return nil, okOutput{}, err
	}
	if err := v.Append(in.Path, in.Content); err != nil {
		return nil, okOutput{}, err
	}
	return nil, okOutput{OK: true}, nil
}

type editNoteInput struct {
	Vault      string `json:"vault,omitempty" jsonschema:"name of the vault; optional when the server holds one vault"`
	Path       string `json:"path" jsonschema:"vault-relative path of the note"`
	Find       string `json:"find" jsonschema:"exact text to replace; must occur exactly once unless replace_all is set"`
	Replace    string `json:"replace" jsonschema:"replacement text"`
	ReplaceAll bool   `json:"replace_all,omitempty" jsonschema:"replace every occurrence instead of requiring a unique match"`
}

type editNoteOutput struct {
	Replacements int `json:"replacements" jsonschema:"number of replacements made"`
}

func (s *Server) editNote(_ context.Context, _ *mcp.CallToolRequest, in editNoteInput) (*mcp.CallToolResult, editNoteOutput, error) {
	v, err := s.writableNote(in.Vault, in.Path)
	if err != nil {
		return nil, editNoteOutput{}, err
	}
	n, err := v.Edit(in.Path, in.Find, in.Replace, in.ReplaceAll)
	if err != nil {
		return nil, editNoteOutput{}, err
	}
	return nil, editNoteOutput{Replacements: n}, nil
}

type moveNoteInput struct {
	Vault       string `json:"vault,omitempty" jsonschema:"name of the vault; optional when the server holds one vault"`
	Path        string `json:"path" jsonschema:"current vault-relative path of the note, which may be inside .trash"`
	NewPath     string `json:"new_path,omitempty" jsonschema:"destination vault-relative path; for a note inside .trash it defaults to the note's path inside .trash, taken from the vault root"`
	UpdateLinks bool   `json:"update_links,omitempty" jsonschema:"also rewrite the [[links]] to this note in every note, so they keep pointing at it after a rename"`
}

type moveNoteOutput struct {
	OK             bool     `json:"ok"`
	MovedTo        string   `json:"moved_to" jsonschema:"vault-relative path the note now has"`
	LinksUpdatedIn []string `json:"links_updated_in,omitempty" jsonschema:"notes whose links were rewritten to the new path"`
}

func (s *Server) moveNote(_ context.Context, _ *mcp.CallToolRequest, in moveNoteInput) (*mcp.CallToolResult, moveNoteOutput, error) {
	v, err := s.writableNote(in.Vault, in.Path)
	if err != nil {
		return nil, moveNoteOutput{}, err
	}
	to := in.NewPath
	if to == "" {
		var ok bool
		if to, ok = outOfTrash(in.Path); !ok {
			return nil, moveNoteOutput{}, fmt.Errorf("give new_path: only a note inside %s can be moved without one", vault.TrashDir)
		}
	}
	if err := requireWritableNote(to); err != nil {
		return nil, moveNoteOutput{}, err
	}
	changed, err := moveWithLinks(v, in.Path, to, in.UpdateLinks)
	if err != nil {
		return nil, moveNoteOutput{}, err
	}
	return nil, moveNoteOutput{OK: true, MovedTo: to, LinksUpdatedIn: changed}, nil
}

type deleteNoteInput struct {
	Vault     string `json:"vault,omitempty" jsonschema:"name of the vault; optional when the server holds one vault"`
	Path      string `json:"path" jsonschema:"vault-relative path of the note"`
	Permanent bool   `json:"permanent,omitempty" jsonschema:"remove the note outright instead of moving it to .trash"`
}

type deleteNoteOutput struct {
	OK bool `json:"ok"`
	// TrashedTo is the vault-relative path the note was moved to inside
	// .trash; empty for permanent deletions.
	TrashedTo string `json:"trashed_to,omitempty" jsonschema:"where the note was moved inside .trash; empty when deleted permanently"`
}

func (s *Server) deleteNote(_ context.Context, _ *mcp.CallToolRequest, in deleteNoteInput) (*mcp.CallToolResult, deleteNoteOutput, error) {
	v, err := s.writableNote(in.Vault, in.Path)
	if err != nil {
		return nil, deleteNoteOutput{}, err
	}
	if !s.policy.AllowPermanentDelete && (in.Permanent || inTrash(in.Path)) {
		return nil, deleteNoteOutput{}, fmt.Errorf("permanent deletion is turned off on this server: delete %q without "+
			"permanent to move it to the trash, or set MCP_ALLOW_PERMANENT_DELETE=true to allow it", in.Path)
	}
	trashedTo, err := v.Delete(in.Path, in.Permanent)
	if err != nil {
		return nil, deleteNoteOutput{}, err
	}
	return nil, deleteNoteOutput{OK: true, TrashedTo: trashedTo}, nil
}
