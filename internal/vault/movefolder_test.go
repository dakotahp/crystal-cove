package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMoveFolderMovesEveryFileInIt(t *testing.T) {
	v := newTestVault(t)
	mustWrite(t, v, "Projects/Survey/Survey.md", "plan")
	mustWrite(t, v, "Projects/Survey/Plans/a.md", "a")
	mustWrite(t, v, "Projects/Survey/Artifacts/report.html", "<p>")
	mustWrite(t, v, "Projects/Survey/.hidden", "h")
	mustWrite(t, v, "Projects/Other.md", "stays")

	if err := v.MoveFolder("Projects/Survey", "Archive/Projects/Survey"); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"Survey.md", "Plans/a.md", "Artifacts/report.html", ".hidden"} {
		if _, err := os.Stat(filepath.Join(v.Root(), "Archive/Projects/Survey", p)); err != nil {
			t.Errorf("%s did not move: %v", p, err)
		}
	}
	if _, err := os.Stat(filepath.Join(v.Root(), "Projects/Survey")); !os.IsNotExist(err) {
		t.Errorf("the source folder is still there: %v", err)
	}
	if got := mustReadFile(t, v, "Projects/Other.md"); got != "stays" {
		t.Errorf("Other.md = %q", got)
	}
}

func TestMoveFolderRefuses(t *testing.T) {
	v := newTestVault(t)
	mustWrite(t, v, "Projects/Survey/a.md", "a")
	mustWrite(t, v, "Archive/Survey/b.md", "b")
	mustWrite(t, v, "Archive/survey2/c.md", "c")

	for _, tc := range []struct{ from, to, want string }{
		{"Projects/Survey/a.md", "Archive/a", "not a folder"},
		{"Projects/Missing", "Archive/Missing", "no folder"},
		{"Projects/Survey", "Archive/Survey", "already exists"},
		{"Projects/Survey", "Projects/Survey/Inner", "into itself"},
		{"Projects/Survey", "Projects/Survey", "into itself"},
		{"Projects/Survey", "Archive/Survey2", "differs only in letter case"},
		{".", "Archive/Root", "vault root"},
		{"Projects/Survey", ".", "vault root"},
		{"Projects/Survey", "../Outside", "escapes"},
	} {
		err := v.MoveFolder(tc.from, tc.to)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("MoveFolder(%q, %q) = %v, want an error containing %q", tc.from, tc.to, err, tc.want)
		}
	}
	if got := mustReadFile(t, v, "Projects/Survey/a.md"); got != "a" {
		t.Errorf("a.md = %q after refused moves", got)
	}
}
