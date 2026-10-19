package server

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dakotahp/crystal-cove/internal/vault"
)

// DefaultRecentNotes caps recent_notes when the caller gives no limit.
const DefaultRecentNotes = 20

type recentNotesInput struct {
	Vault  string `json:"vault,omitempty" jsonschema:"the vault to list; optional when the server holds one vault"`
	Since  string `json:"since,omitempty" jsonschema:"only notes changed at or after this RFC 3339 timestamp"`
	Offset int    `json:"offset,omitempty" jsonschema:"note to start from; use next_offset from the previous page"`
	Limit  int    `json:"limit,omitempty" jsonschema:"maximum notes to return (default 20, max 1000)"`
}

// RecentNotes is the outcome of recent_notes.
type RecentNotes struct {
	Notes      []vault.Entry `json:"notes"`
	Total      int           `json:"total" jsonschema:"notes changed since the given time, or in the whole vault, across every page"`
	NextOffset int           `json:"next_offset" jsonschema:"offset of the next page; -1 when this is the last page"`
}

func (s *Server) recentNotes(_ context.Context, _ *mcp.CallToolRequest, in recentNotesInput) (*mcp.CallToolResult, *RecentNotes, error) {
	v, err := s.vault(in.Vault)
	if err != nil {
		return nil, nil, err
	}
	var since time.Time
	if in.Since != "" {
		since, err = time.Parse(time.RFC3339, in.Since)
		if err != nil {
			return nil, nil, fmt.Errorf("since %q must be an RFC 3339 timestamp such as 2026-09-23T14:00:00Z: %w", in.Since, err)
		}
	}
	entries, err := v.RecentNotes(0, since)
	if err != nil {
		return nil, nil, err
	}
	page, next, err := pageOf(entries, in.Offset, in.Limit, DefaultRecentNotes)
	if err != nil {
		return nil, nil, err
	}
	return nil, &RecentNotes{Notes: page, Total: len(entries), NextOffset: next}, nil
}
