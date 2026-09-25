package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dakotahp/crystal-cove/internal/vault"
)

type getSectionInput struct {
	Vault       string   `json:"vault,omitempty" jsonschema:"name of the vault; optional when the server holds one vault"`
	Path        string   `json:"path" jsonschema:"vault-relative path of the note"`
	HeadingPath []string `json:"heading_path" jsonschema:"one or more exact heading titles from ancestor to target; a unique suffix of the full hierarchy"`
	Offset      int      `json:"offset,omitempty" jsonschema:"character offset within the section body; defaults to zero"`
}

func (s *Server) getSection(_ context.Context, _ *mcp.CallToolRequest, in getSectionInput) (*mcp.CallToolResult, *vault.SectionResult, error) {
	v, err := s.note(in.Vault, in.Path)
	if err != nil {
		return nil, nil, err
	}
	out, err := v.GetSection(in.Path, in.HeadingPath, in.Offset)
	return nil, out, err
}

type editSectionInput struct {
	Vault       string   `json:"vault,omitempty" jsonschema:"name of the vault; optional when the server holds one vault"`
	Path        string   `json:"path" jsonschema:"vault-relative path of the note"`
	HeadingPath []string `json:"heading_path" jsonschema:"one or more exact heading titles from ancestor to target; a unique suffix of the full hierarchy"`
	Mode        string   `json:"mode" jsonschema:"append (after the section's own text, before its first subheading), prepend (right below the heading) or replace (the whole body, subsections included)"`
	Content     string   `json:"content" jsonschema:"lines to add, or for replace the new body without the heading; empty with replace clears the body"`
	Version     string   `json:"version,omitempty" jsonschema:"the version get_section returned for this section; required for replace, and the edit is refused if the section changed since"`
}

type editSectionOutput struct {
	OK      bool   `json:"ok"`
	Version string `json:"version" jsonschema:"the section's version after the edit, for a following edit"`
}

func (s *Server) editSection(_ context.Context, _ *mcp.CallToolRequest, in editSectionInput) (*mcp.CallToolResult, editSectionOutput, error) {
	v, err := s.writableNote(in.Vault, in.Path)
	if err != nil {
		return nil, editSectionOutput{}, err
	}
	version, err := v.EditSection(in.Path, in.HeadingPath, vault.SectionMode(in.Mode), in.Content, in.Version)
	if err != nil {
		return nil, editSectionOutput{}, err
	}
	return nil, editSectionOutput{OK: true, Version: version}, nil
}
