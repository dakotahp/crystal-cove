package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// callsRejecting runs every note tool against path and reports the tools
// that accepted it.
func callsRejecting(t *testing.T, s *Server, path string) []string {
	t.Helper()
	ctx := context.Background()
	var accepted []string
	check := func(name string, err error) {
		if err == nil {
			accepted = append(accepted, name)
		}
	}
	_, _, err := s.readNote(ctx, &mcp.CallToolRequest{}, readNoteInput{Path: path})
	check("read_note", err)
	_, _, err = s.createNote(ctx, &mcp.CallToolRequest{}, writeNoteInput{Path: path, Content: "x"})
	check("create_note", err)
	_, _, err = s.appendNote(ctx, &mcp.CallToolRequest{}, writeNoteInput{Path: path, Content: "x"})
	check("append_note", err)
	_, _, err = s.editNote(ctx, &mcp.CallToolRequest{}, editNoteInput{Path: path, Find: "a", Replace: "b"})
	check("edit_note", err)
	_, _, err = s.moveNote(ctx, &mcp.CallToolRequest{}, moveNoteInput{Path: path, NewPath: "Inbox/moved.md"})
	check("move_note", err)
	_, _, err = s.moveNote(ctx, &mcp.CallToolRequest{}, moveNoteInput{Path: "Inbox/a.md", NewPath: path})
	check("move_note destination", err)
	_, _, err = s.deleteNote(ctx, &mcp.CallToolRequest{}, deleteNoteInput{Path: path})
	check("delete_note", err)
	_, _, err = s.getSection(ctx, &mcp.CallToolRequest{}, getSectionInput{Path: path, HeadingPath: []string{"One"}})
	check("get_section", err)
	_, _, err = s.replaceSection(ctx, &mcp.CallToolRequest{}, replaceSectionInput{Path: path, HeadingPath: []string{"One"}, Content: "x"})
	check("replace_section", err)
	return accepted
}

func TestNoteToolsRejectFilesThatAreNotNotes(t *testing.T) {
	s, _ := metaServer(t, map[string]string{
		"Inbox/a.md":      "## One\n\nbody\n",
		"Areas/theme.css": "body { color: red }\n",
	})

	if accepted := callsRejecting(t, s, "Areas/theme.css"); len(accepted) != 0 {
		t.Errorf("these tools accepted a non-note file: %v", accepted)
	}
}

func TestNoteToolsRejectObsidianConfig(t *testing.T) {
	s, _ := metaServer(t, map[string]string{"Inbox/a.md": "## One\n\nbody\n"})

	if accepted := callsRejecting(t, s, ".obsidian/notes.md"); len(accepted) != 0 {
		t.Errorf("these tools reached into .obsidian: %v", accepted)
	}
}

func TestNonNoteFilesSurviveRejectedWrites(t *testing.T) {
	s, v := metaServer(t, map[string]string{
		"Inbox/a.md":      "## One\n\nbody\n",
		"Areas/theme.css": "body { color: red }\n",
	})

	callsRejecting(t, s, "Areas/theme.css")

	data, err := v.ReadAll("Areas/theme.css")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "body { color: red }\n" {
		t.Errorf("file = %q, want it untouched", data)
	}
}

func TestListNotesRejectsHiddenFolders(t *testing.T) {
	s, _ := metaServer(t, map[string]string{
		".obsidian/workspace.json": "{}",
		"Inbox/.drafts/a.md":       "draft\n",
	})
	ctx := context.Background()

	for _, dir := range []string{".obsidian", "Inbox/.drafts", "./.obsidian"} {
		if _, out, err := s.listNotes(ctx, &mcp.CallToolRequest{}, listNotesInput{Dir: dir}); err == nil {
			t.Errorf("list_notes listed %s: %+v", dir, out.Entries)
		}
	}
}

func TestTrashToolsStillWork(t *testing.T) {
	s, v := metaServer(t, map[string]string{"Inbox/a.md": "## One\n\nbody\n"})
	ctx := context.Background()

	if _, _, err := s.deleteNote(ctx, &mcp.CallToolRequest{}, deleteNoteInput{Path: "Inbox/a.md"}); err != nil {
		t.Fatalf("delete_note: %v", err)
	}
	if _, err := os.Stat(filepath.Join(v.Root(), ".trash", "a.md")); err != nil {
		t.Fatalf("note did not reach the trash: %v", err)
	}
	if _, _, err := s.restoreNote(ctx, &mcp.CallToolRequest{}, restoreNoteInput{Path: ".trash/a.md", To: "Inbox/a.md"}); err != nil {
		t.Fatalf("restore_note: %v", err)
	}
	if _, _, err := s.listNotes(ctx, &mcp.CallToolRequest{}, listNotesInput{Dir: ".trash"}); err != nil {
		t.Errorf("list_notes on the trash: %v", err)
	}
}

func TestNoteToolsAcceptOrdinaryNotes(t *testing.T) {
	s, _ := metaServer(t, map[string]string{"Inbox/a.md": "## One\n\nbody\n"})
	ctx := context.Background()

	if _, _, err := s.readNote(ctx, &mcp.CallToolRequest{}, readNoteInput{Path: "Inbox/a.md"}); err != nil {
		t.Errorf("read_note: %v", err)
	}
	if _, _, err := s.createNote(ctx, &mcp.CallToolRequest{}, writeNoteInput{Path: "Inbox/new.md", Content: "x"}); err != nil {
		t.Errorf("create_note: %v", err)
	}
}
