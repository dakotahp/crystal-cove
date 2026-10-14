package server

import (
	"context"
	"errors"
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

// MaxReadNotes caps the paths one read_notes call takes, and
// MaxReadNotesChars the characters it returns across all of them, so loading
// context in one call cannot fill a model's context.
const (
	MaxReadNotes      = 20
	MaxReadNotesChars = 5 * vault.ReadPageSize
)

type readNotesInput struct {
	Vault string   `json:"vault,omitempty" jsonschema:"name of the vault; optional when the server holds one vault"`
	Paths []string `json:"paths" jsonschema:"vault-relative paths of the notes to read, at most 20"`
}

// NotePage is the first page of one note read by read_notes, or why it
// could not be read.
type NotePage struct {
	Path  string `json:"path"`
	Error string `json:"error,omitempty" jsonschema:"why this note could not be read; the other notes are still returned"`
	*vault.ReadResult
}

// ReadNotes is the outcome of read_notes.
type ReadNotes struct {
	Notes []NotePage `json:"notes"`
	// BudgetExhausted reports that the character budget ran out, so some
	// notes came back short or empty and need read_note to continue.
	BudgetExhausted bool `json:"budget_exhausted" jsonschema:"true when the shared character budget ran out; continue any truncated note with read_note from its next_offset"`
}

func (s *Server) readNotes(_ context.Context, _ *mcp.CallToolRequest, in readNotesInput) (*mcp.CallToolResult, *ReadNotes, error) {
	if len(in.Paths) == 0 {
		return nil, nil, errors.New("give paths: the notes to read")
	}
	if len(in.Paths) > MaxReadNotes {
		return nil, nil, fmt.Errorf("read_notes takes at most %d paths, got %d: split the call", MaxReadNotes, len(in.Paths))
	}
	if _, err := s.vault(in.Vault); err != nil {
		return nil, nil, err
	}
	out := &ReadNotes{Notes: make([]NotePage, 0, len(in.Paths))}
	budget := MaxReadNotesChars
	for _, path := range in.Paths {
		page, err := s.readFirst(in.Vault, path, budget)
		if err != nil {
			out.Notes = append(out.Notes, NotePage{Path: path, Error: err.Error(), ReadResult: &vault.ReadResult{NextOffset: -1}})
			continue
		}
		budget -= len([]rune(page.Content))
		if budget == 0 && page.Truncated {
			out.BudgetExhausted = true
		}
		out.Notes = append(out.Notes, NotePage{Path: path, ReadResult: page})
	}
	return nil, out, nil
}

func (s *Server) readFirst(vaultName, path string, limit int) (*vault.ReadResult, error) {
	v, err := s.note(vaultName, path)
	if err != nil {
		return nil, err
	}
	return v.ReadFirst(path, limit)
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
	Vault   string              `json:"vault,omitempty" jsonschema:"name of the vault; optional when the server holds one vault"`
	Path    string              `json:"path" jsonschema:"vault-relative path of the note"`
	Edits   []vault.Replacement `json:"edits" jsonschema:"one or more replacements, applied in order: each finds its text in the note as the edits before it left it. If any fails, nothing is written"`
	Version string              `json:"version,omitempty" jsonschema:"the version read_note returned; the edits are refused if the note changed since"`
}

type editNoteOutput struct {
	Replacements int    `json:"replacements" jsonschema:"number of replacements made"`
	Version      string `json:"version" jsonschema:"the note's version after the edit, for a following edit"`
}

func (s *Server) editNote(_ context.Context, _ *mcp.CallToolRequest, in editNoteInput) (*mcp.CallToolResult, editNoteOutput, error) {
	v, err := s.writableNote(in.Vault, in.Path)
	if err != nil {
		return nil, editNoteOutput{}, err
	}
	n, version, err := v.Edit(in.Path, in.Version, in.Edits...)
	if err != nil {
		return nil, editNoteOutput{}, err
	}
	return nil, editNoteOutput{Replacements: n, Version: version}, nil
}

type moveNoteInput struct {
	Vault       string `json:"vault,omitempty" jsonschema:"name of the vault; optional when the server holds one vault"`
	Path        string `json:"path" jsonschema:"current vault-relative path of the note, which may be inside .trash"`
	NewPath     string `json:"new_path,omitempty" jsonschema:"destination vault-relative path; for a note inside .trash it defaults to where the note was deleted from"`
	UpdateLinks bool   `json:"update_links,omitempty" jsonschema:"also rewrite the [[links]] to this note in every note, so they keep pointing at it after a rename"`
}

type moveNoteOutput struct {
	OK             bool     `json:"ok"`
	MovedTo        string   `json:"moved_to" jsonschema:"vault-relative path the note now has"`
	LinksUpdated   bool     `json:"links_updated" jsonschema:"whether links to the note were rewritten; false unless update_links was set, and false for a move into or out of .trash"`
	LinksUpdatedIn []string `json:"links_updated_in" jsonschema:"notes whose links were rewritten to the new path"`
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
	changed, updated, err := moveWithLinks(v, in.Path, to, in.UpdateLinks)
	if err != nil {
		return nil, moveNoteOutput{}, err
	}
	return nil, moveNoteOutput{OK: true, MovedTo: to, LinksUpdated: updated, LinksUpdatedIn: changed}, nil
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
