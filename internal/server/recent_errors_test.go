package server

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestRecentNotesRejectsAnUnknownVault(t *testing.T) {
	s, _ := metaServer(t, map[string]string{"Inbox/a.md": tagged("idea")})

	if _, _, err := s.recentNotes(context.Background(), &mcp.CallToolRequest{}, recentNotesInput{Vault: "Missing"}); err == nil {
		t.Error("recentNotes accepted an unknown vault")
	}
}

func TestRecentNotesDefaultsToTheOnlyVault(t *testing.T) {
	s, _ := metaServer(t, map[string]string{"Inbox/a.md": tagged("idea")})

	_, res, err := s.recentNotes(context.Background(), &mcp.CallToolRequest{}, recentNotesInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Notes) != 1 {
		t.Errorf("Notes = %+v, want the one note", res.Notes)
	}
}
