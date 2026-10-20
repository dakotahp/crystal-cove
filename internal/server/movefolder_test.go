package server

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func moveFolder(t *testing.T, s *Server, in moveFolderInput) moveFolderOutput {
	t.Helper()
	_, out, err := s.moveFolder(context.Background(), &mcp.CallToolRequest{}, in)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestMoveFolderUpdatesLinksIntoOutOfAndWithinTheFolder(t *testing.T) {
	s, v := metaServer(t, map[string]string{
		"Projects/Survey/Survey.md": "[[Plan]] [p](./Plans/Plan.md) [[Projects/Survey/Plans/Plan]] " +
			"[o](../../Areas/Other.md) [[Other]] ![[report.html]] [r](Artifacts/report.html)\n",
		"Projects/Survey/Plans/Plan.md":         "[s](../Survey.md) [[Survey]]\n",
		"Projects/Survey/Artifacts/report.html": "<p>report</p>\n",
		"Areas/Other.md":                        "[[Survey]] [[Projects/Survey/Plans/Plan|plan]] [r](../Projects/Survey/Artifacts/report.html) [[Unrelated]]\n",
		"Areas/Unrelated.md":                    "[[Other]]\n",
	})

	out := moveFolder(t, s, moveFolderInput{Path: "Projects/Survey", NewPath: "Archive/Projects/Survey", UpdateLinks: true})

	if !out.OK || out.MovedTo != "Archive/Projects/Survey" || out.FilesMoved != 3 || !out.LinksUpdated {
		t.Errorf("result = %+v", out)
	}
	want := []string{"Archive/Projects/Survey/Survey.md", "Areas/Other.md"}
	if !slices.Equal(out.LinksUpdatedIn, want) {
		t.Errorf("LinksUpdatedIn = %q, want %q", out.LinksUpdatedIn, want)
	}
	for p, want := range map[string]string{
		"Archive/Projects/Survey/Survey.md": "[[Plan]] [p](./Plans/Plan.md) [[Archive/Projects/Survey/Plans/Plan]] " +
			"[o](../../../Areas/Other.md) [[Other]] ![[report.html]] [r](Archive/Projects/Survey/Artifacts/report.html)\n",
		"Archive/Projects/Survey/Plans/Plan.md": "[s](../Survey.md) [[Survey]]\n",
		"Areas/Other.md": "[[Survey]] [[Archive/Projects/Survey/Plans/Plan|plan]] " +
			"[r](../Archive/Projects/Survey/Artifacts/report.html) [[Unrelated]]\n",
		"Areas/Unrelated.md": "[[Other]]\n",
	} {
		if got := readFile(t, v, p); got != want {
			t.Errorf("%s =\n%q, want\n%q", p, got, want)
		}
	}
}

func TestMoveFolderLeavesLinksAloneByDefault(t *testing.T) {
	s, v := metaServer(t, map[string]string{
		"Projects/Survey/Plan.md": "[o](../../Other.md)\n",
		"Other.md":                "[[Projects/Survey/Plan]]\n",
	})

	out := moveFolder(t, s, moveFolderInput{Path: "Projects/Survey", NewPath: "Archive/Survey"})
	if out.LinksUpdated || len(out.LinksUpdatedIn) != 0 || out.FilesMoved != 1 {
		t.Errorf("result = %+v", out)
	}
	if got := readFile(t, v, "Other.md"); got != "[[Projects/Survey/Plan]]\n" {
		t.Errorf("Other.md = %q, want it untouched", got)
	}
	if got := readFile(t, v, "Archive/Survey/Plan.md"); got != "[o](../../Other.md)\n" {
		t.Errorf("Plan.md = %q, want it untouched", got)
	}
}

func TestMoveFolderRefusesHiddenFoldersAndTheRoot(t *testing.T) {
	s, v := metaServer(t, map[string]string{
		"Projects/Survey/Plan.md": "body\n",
		".trash/Old/a.md":         "trashed\n",
		".obsidian/app.json":      "{}\n",
	})

	for _, in := range []moveFolderInput{
		{Path: ".trash/Old", NewPath: "Restored"},
		{Path: ".obsidian", NewPath: "Config"},
		{Path: "Projects/Survey", NewPath: ".trash/Survey"},
		{Path: "Projects/Survey", NewPath: ".obsidian/Survey"},
		{Path: "Projects/.hidden", NewPath: "Archive/x"},
		{Path: "", NewPath: "Archive"},
		{Path: "/", NewPath: "Archive"},
		{Path: "Projects/Survey", NewPath: ""},
	} {
		if _, _, err := s.moveFolder(context.Background(), &mcp.CallToolRequest{}, in); err == nil {
			t.Errorf("move_folder %q to %q succeeded", in.Path, in.NewPath)
		}
	}
	if _, err := os.Stat(filepath.Join(v.Root(), "Projects/Survey/Plan.md")); err != nil {
		t.Errorf("the folder moved after a refusal: %v", err)
	}
}

func TestMoveFolderRefusesAFolderInsideItself(t *testing.T) {
	s, _ := metaServer(t, map[string]string{"Projects/Survey/Plan.md": "body\n"})

	_, _, err := s.moveFolder(context.Background(), &mcp.CallToolRequest{}, moveFolderInput{Path: "Projects/Survey", NewPath: "Projects/Survey/Old"})
	if err == nil || !strings.Contains(err.Error(), "into itself") {
		t.Errorf("err = %v", err)
	}
}

func TestMoveFolderReportsALinkUpdateItCouldNotFinish(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permissions do not restrict root")
	}
	s, v := metaServer(t, map[string]string{
		"Projects/Survey/Plan.md": "body\n",
		"Locked/a.md":             "[[Projects/Survey/Plan]]\n",
	})
	locked := filepath.Join(v.Root(), "Locked")
	if err := os.Chmod(locked, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	_, _, err := s.moveFolder(context.Background(), &mcp.CallToolRequest{}, moveFolderInput{Path: "Projects/Survey", NewPath: "Archive/Survey", UpdateLinks: true})
	if err == nil || !strings.Contains(err.Error(), "updating links stopped after 0 notes") {
		t.Errorf("err = %v, want it to say the move happened and the link update stopped", err)
	}
}
