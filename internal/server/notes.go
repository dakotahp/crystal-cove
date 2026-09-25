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
}

type listNotesOutput struct {
	Entries []vault.Entry `json:"entries" jsonschema:"files and directories found"`
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
	return nil, listNotesOutput{Entries: entries}, nil
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
	Vault   string `json:"vault,omitempty" jsonschema:"name of the vault; optional when the server holds one vault"`
	Path    string `json:"path" jsonschema:"current vault-relative path of the note"`
	NewPath string `json:"new_path" jsonschema:"destination vault-relative path"`
}

func (s *Server) moveNote(_ context.Context, _ *mcp.CallToolRequest, in moveNoteInput) (*mcp.CallToolResult, okOutput, error) {
	v, err := s.writableNote(in.Vault, in.Path)
	if err != nil {
		return nil, okOutput{}, err
	}
	if err := requireWritableNote(in.NewPath); err != nil {
		return nil, okOutput{}, err
	}
	if err := v.Move(in.Path, in.NewPath); err != nil {
		return nil, okOutput{}, err
	}
	return nil, okOutput{OK: true}, nil
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

type restoreNoteInput struct {
	Vault string `json:"vault,omitempty" jsonschema:"name of the vault; optional when the server holds one vault"`
	Path  string `json:"path" jsonschema:"vault-relative path of the note inside .trash"`
	To    string `json:"to,omitempty" jsonschema:"destination vault-relative path; defaults to the note's path inside .trash"`
}

type restoreNoteOutput struct {
	OK bool `json:"ok"`
	// RestoredTo is the vault-relative path the note was restored to.
	RestoredTo string `json:"restored_to" jsonschema:"vault-relative path the note was restored to"`
}

func (s *Server) restoreNote(_ context.Context, _ *mcp.CallToolRequest, in restoreNoteInput) (*mcp.CallToolResult, restoreNoteOutput, error) {
	v, err := s.note(in.Vault, in.Path)
	if err != nil {
		return nil, restoreNoteOutput{}, err
	}
	to, err := vault.RestoreDestination(in.Path, in.To)
	if err != nil {
		return nil, restoreNoteOutput{}, err
	}
	if err := requireWritableNote(to); err != nil {
		return nil, restoreNoteOutput{}, err
	}
	restoredTo, err := v.Restore(in.Path, in.To)
	if err != nil {
		return nil, restoreNoteOutput{}, err
	}
	return nil, restoreNoteOutput{OK: true, RestoredTo: restoredTo}, nil
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
