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
	Vault string `json:"vault,omitempty" jsonschema:"the vault to list; optional when the server holds one vault"`
	Since string `json:"since,omitempty" jsonschema:"only notes changed at or after this RFC 3339 timestamp"`
	Limit int    `json:"limit,omitempty" jsonschema:"maximum notes to return"`
}

// RecentNotes is the outcome of recent_notes.
type RecentNotes struct {
	Notes []vault.Entry `json:"notes"`
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
	limit := in.Limit
	if limit <= 0 {
		limit = DefaultRecentNotes
	}
	entries, err := v.RecentNotes(limit, since)
	if err != nil {
		return nil, nil, err
	}
	return nil, &RecentNotes{Notes: entries}, nil
}
