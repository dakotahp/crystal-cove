package vault

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// symlinkedVault returns a vault holding Inbox/a.md, a note symlinked to a
// file outside the vault (leak.md), a folder symlinked outside (Out), and a
// symlink that stays inside the vault (alias.md). It also returns the
// outside folder, which holds secret.md.
func symlinkedVault(t *testing.T) (*Vault, string) {
	t.Helper()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.md"), []byte("## Secret\n\nkey\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	v := newNotesVault(t, "Inbox/a.md")
	links := map[string]string{
		"leak.md":  filepath.Join(outside, "secret.md"),
		"Out":      outside,
		"alias.md": filepath.Join("Inbox", "a.md"),
	}
	for name, target := range links {
		if err := os.Symlink(target, filepath.Join(v.Root(), name)); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
	}
	return v, outside
}

func TestSymlinksCannotReadOutsideTheVault(t *testing.T) {
	v, _ := symlinkedVault(t)

	if _, err := v.Read("leak.md", 0); err == nil {
		t.Error("Read followed a symlink out of the vault")
	}
	if _, err := v.ReadAll("Out/secret.md"); err == nil {
		t.Error("ReadAll followed a symlinked folder out of the vault")
	}
	if _, err := v.GetSection("leak.md", []string{"Secret"}, 0); err == nil {
		t.Error("GetSection followed a symlink out of the vault")
	}
	if _, err := v.List("Out", false); err == nil {
		t.Error("List followed a symlinked folder out of the vault")
	}
	if _, err := v.List("Out", true); err == nil {
		t.Error("recursive List followed a symlinked folder out of the vault")
	}
}

func TestSymlinksCannotWriteOutsideTheVault(t *testing.T) {
	v, outside := symlinkedVault(t)
	secret := filepath.Join(outside, "secret.md")

	if err := v.Create("Out/new.md", "x"); err == nil {
		t.Error("Create wrote through a symlinked folder")
	}
	if err := v.Append("Out/secret.md", "x"); err == nil {
		t.Error("Append wrote through a symlinked folder")
	}
	if _, err := v.Update("Out/secret.md", setTo("x")); err == nil {
		t.Error("Update wrote through a symlinked folder")
	}
	if _, _, err := v.Edit("leak.md", "key", "gone", false, ""); err == nil {
		t.Error("Edit followed a symlink out of the vault")
	}
	if _, err := v.EditSection("Out/secret.md", []string{"Secret"}, SectionReplace, "gone\n", "any"); err == nil {
		t.Error("EditSection wrote through a symlinked folder")
	}
	if err := v.Move("Inbox/a.md", "Out/a.md"); err == nil {
		t.Error("Move put a note outside the vault")
	}
	if err := v.Move("Out/secret.md", "stolen.md"); err == nil {
		t.Error("Move pulled a file in from outside the vault")
	}
	if _, err := v.Delete("Out/secret.md", false); err == nil {
		t.Error("Delete trashed a file outside the vault")
	}
	if _, err := v.Delete("Out/secret.md", true); err == nil {
		t.Error("Delete removed a file outside the vault")
	}

	data, err := os.ReadFile(secret)
	if err != nil || string(data) != "## Secret\n\nkey\n" {
		t.Errorf("outside file = %q, %v; want it untouched", data, err)
	}
	if _, err := os.Stat(filepath.Join(outside, "new.md")); !os.IsNotExist(err) {
		t.Errorf("a note was created outside the vault: %v", err)
	}
}

func TestWalksSkipSymlinksLeadingOutOfTheVault(t *testing.T) {
	v, _ := symlinkedVault(t)

	notes, err := v.Notes()
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"Inbox/a.md", "alias.md"}; !slices.Equal(notes, want) {
		t.Errorf("Notes = %q, want %q", notes, want)
	}

	recent, err := v.RecentNotes(0, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	var recentPaths []string
	for _, e := range recent {
		recentPaths = append(recentPaths, e.Path)
	}
	slices.Sort(recentPaths)
	if want := []string{"Inbox/a.md", "alias.md"}; !slices.Equal(recentPaths, want) {
		t.Errorf("RecentNotes = %q, want %q", recentPaths, want)
	}

	for _, recursive := range []bool{false, true} {
		entries, err := v.List("", recursive)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if e.Path == "leak.md" || strings.HasPrefix(e.Path, "Out") {
				t.Errorf("List(recursive=%v) showed %q, which leads outside the vault", recursive, e.Path)
			}
		}
	}

	titles, _, err := v.MatchTitles("leak|secret", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(titles) != 0 {
		t.Errorf("MatchTitles = %q, want no note outside the vault", titles)
	}
}

func TestSymlinksInsideTheVaultStillWork(t *testing.T) {
	v, _ := symlinkedVault(t)

	res, err := v.Read("alias.md", 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Content != "body\n" {
		t.Errorf("alias content = %q", res.Content)
	}
}
