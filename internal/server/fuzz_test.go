package server

import (
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andyjmorgan/obsidian-hosted-mcp/internal/vault"
)

func FuzzWritableNotesAreVisibleNotes(f *testing.F) {
	for _, seed := range []string{
		"Inbox/a.md", ".obsidian/a.md", "a/.git/b.md", ".trash/a.md", "mcp-instructions.md",
		"MCP-INSTRUCTIONS.MD", "a/../mcp-instructions.md", "theme.css", "a.MD", ".mcp-instructions.md",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, p string) {
		if requireWritableNote(p) != nil {
			return
		}
		if !strings.EqualFold(filepath.Ext(p), vault.NoteExtension) {
			t.Fatalf("accepted %q, which is not a note", p)
		}
		for _, part := range strings.Split(filepath.ToSlash(p), "/") {
			if strings.HasPrefix(part, ".") && part != vault.TrashDir {
				t.Fatalf("accepted %q, which is inside a hidden folder", p)
			}
		}
		if strings.EqualFold(path.Clean(filepath.ToSlash(p)), SyncedInstructionsFile) {
			t.Fatalf("accepted %q, which is the instructions note", p)
		}
	})
}
