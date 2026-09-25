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
	v, err := s.vault(in.Vault)
	if err != nil {
		return nil, nil, err
	}
	if err := requireNote(in.Path); err != nil {
		return nil, nil, err
	}
	out, err := v.GetSection(in.Path, in.HeadingPath, in.Offset)
	return nil, out, err
}

type replaceSectionInput struct {
	Vault       string   `json:"vault,omitempty" jsonschema:"name of the vault; optional when the server holds one vault"`
	Path        string   `json:"path" jsonschema:"vault-relative path of the note"`
	HeadingPath []string `json:"heading_path" jsonschema:"one or more exact heading titles from ancestor to target; a unique suffix of the full hierarchy"`
	Content     string   `json:"content" jsonschema:"replacement body including any desired subsections, without the selected heading; empty clears the body"`
}

func (s *Server) replaceSection(_ context.Context, _ *mcp.CallToolRequest, in replaceSectionInput) (*mcp.CallToolResult, okOutput, error) {
	v, err := s.vault(in.Vault)
	if err != nil {
		return nil, okOutput{}, err
	}
	if err := requireWritableNote(in.Path); err != nil {
		return nil, okOutput{}, err
	}
	if err := v.ReplaceSection(in.Path, in.HeadingPath, in.Content); err != nil {
		return nil, okOutput{}, err
	}
	return nil, okOutput{OK: true}, nil
}
