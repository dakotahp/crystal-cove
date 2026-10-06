package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dakotahp/crystal-cove/internal/vault"
)

func TestMoveNoteWithLinksStillRefusesATakenDestination(t *testing.T) {
	s, v := metaServer(t, map[string]string{
		"Old.md":     "body\n",
		"New.md":     "taken\n",
		"Inbox/a.md": "[[Old]]\n",
	})

	_, _, err := s.moveNote(context.Background(), &mcp.CallToolRequest{}, moveNoteInput{Path: "Old.md", NewPath: "New.md", UpdateLinks: true})
	if err == nil {
		t.Fatal("move_note replaced an existing note")
	}
	if got := readFile(t, v, "Inbox/a.md"); got != "[[Old]]\n" {
		t.Errorf("a.md = %q, want links untouched after a refused move", got)
	}
}

func TestMoveNoteReportsALinkUpdateItCouldNotFinish(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permissions do not restrict root")
	}
	s, v := metaServer(t, map[string]string{
		"Old.md":      "body\n",
		"Locked/a.md": "[[Old]]\n",
	})
	locked := filepath.Join(v.Root(), "Locked")
	if err := os.Chmod(locked, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	_, _, err := s.moveNote(context.Background(), &mcp.CallToolRequest{}, moveNoteInput{Path: "Old.md", NewPath: "New.md", UpdateLinks: true})
	if err == nil || !strings.Contains(err.Error(), "updating links stopped after 0 notes") {
		t.Errorf("err = %v, want it to say the move happened and the link update stopped", err)
	}
}

func readFile(t *testing.T, v *vault.Vault, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(v.Root(), filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func move(t *testing.T, s *Server, in moveNoteInput) moveNoteOutput {
	t.Helper()
	_, out, err := s.moveNote(context.Background(), &mcp.CallToolRequest{}, in)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestMoveNoteUpdatesLinksWhenAsked(t *testing.T) {
	s, v := metaServer(t, map[string]string{
		"Areas/Old Name.md": "see [[Old Name#Top]]\n",
		"Inbox/a.md":        "[[Old Name]], [[old name#Tasks|tasks]], ![[Old Name]] and [[Areas/Old Name]]\n```\n[[Old Name]]\n```\n",
		"Inbox/b.md":        "no links here\n",
		"Inbox/c.md":        "---\nup: \"[[Old Name]]\"\n---\nbody\n",
		"Inbox/d.md":        "[[Other]]\n",
	})

	out := move(t, s, moveNoteInput{Path: "Areas/Old Name.md", NewPath: "Areas/New Name.md", UpdateLinks: true})

	want := []string{"Areas/New Name.md", "Inbox/a.md", "Inbox/c.md"}
	if !out.LinksUpdated {
		t.Error("LinksUpdated = false")
	}
	if !slices.Equal(out.LinksUpdatedIn, want) {
		t.Errorf("LinksUpdatedIn = %q, want %q", out.LinksUpdatedIn, want)
	}
	if got := readFile(t, v, "Inbox/a.md"); got != "[[New Name]], [[New Name#Tasks|tasks]], ![[New Name]] and [[Areas/New Name]]\n```\n[[Old Name]]\n```\n" {
		t.Errorf("a.md = %q", got)
	}
	if got := readFile(t, v, "Inbox/c.md"); got != "---\nup: \"[[New Name]]\"\n---\nbody\n" {
		t.Errorf("c.md = %q", got)
	}
	if got := readFile(t, v, "Areas/New Name.md"); got != "see [[New Name#Top]]\n" {
		t.Errorf("the moved note's own link = %q", got)
	}
	if got := readFile(t, v, "Inbox/d.md"); got != "[[Other]]\n" {
		t.Errorf("d.md = %q, want it untouched", got)
	}
}

func TestMoveNoteLeavesLinksAloneByDefault(t *testing.T) {
	s, v := metaServer(t, map[string]string{
		"Old.md":     "body\n",
		"Inbox/a.md": "[[Old]]\n",
	})

	out := move(t, s, moveNoteInput{Path: "Old.md", NewPath: "New.md"})
	data, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"links_updated":false,"links_updated_in":[]`) {
		t.Errorf("result = %s, want it to say that no links were rewritten", data)
	}
	if got := readFile(t, v, "Inbox/a.md"); got != "[[Old]]\n" {
		t.Errorf("a.md = %q, want it untouched", got)
	}
}

func TestMoveNoteUsesAPathWhenTheNewNameIsTaken(t *testing.T) {
	s, v := metaServer(t, map[string]string{
		"Areas/Old.md": "body\n",
		"Other/New.md": "another note\n",
		"Inbox/a.md":   "[[Old]] and [[Old.md]]\n",
	})

	move(t, s, moveNoteInput{Path: "Areas/Old.md", NewPath: "Areas/New.md", UpdateLinks: true})
	if got := readFile(t, v, "Inbox/a.md"); got != "[[Areas/New]] and [[Areas/New.md]]\n" {
		t.Errorf("a.md = %q, want paths so the links cannot reach Other/New.md", got)
	}
}

func TestMoveNoteToAnotherFolderKeepsNameLinks(t *testing.T) {
	s, v := metaServer(t, map[string]string{
		"Areas/Plan.md": "body\n",
		"Inbox/a.md":    "[[Plan]] and [[Areas/Plan]]\n",
	})

	out := move(t, s, moveNoteInput{Path: "Areas/Plan.md", NewPath: "Archive/Plan.md", UpdateLinks: true})
	if got := readFile(t, v, "Inbox/a.md"); got != "[[Plan]] and [[Archive/Plan]]\n" {
		t.Errorf("a.md = %q", got)
	}
	if !slices.Equal(out.LinksUpdatedIn, []string{"Inbox/a.md"}) {
		t.Errorf("LinksUpdatedIn = %q", out.LinksUpdatedIn)
	}
}

func TestMoveNoteIntoOrOutOfTheTrashUpdatesNoLinks(t *testing.T) {
	s, v := metaServer(t, map[string]string{
		"Plan.md":    "body\n",
		"Inbox/a.md": "[[Plan]]\n",
	})

	out := move(t, s, moveNoteInput{Path: "Plan.md", NewPath: ".trash/Plan.md", UpdateLinks: true})
	if out.LinksUpdated || len(out.LinksUpdatedIn) != 0 || readFile(t, v, "Inbox/a.md") != "[[Plan]]\n" {
		t.Errorf("moving into the trash rewrote links: %q", out.LinksUpdatedIn)
	}
	out = move(t, s, moveNoteInput{Path: ".trash/Plan.md", NewPath: "Archive/Plan.md", UpdateLinks: true})
	if out.LinksUpdated || len(out.LinksUpdatedIn) != 0 || readFile(t, v, "Inbox/a.md") != "[[Plan]]\n" {
		t.Errorf("moving out of the trash rewrote links: %q", out.LinksUpdatedIn)
	}
}
