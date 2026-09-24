package vault

import (
	"os"
	"path/filepath"
	"testing"
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
	if err := v.WriteAll("Out/secret.md", []byte("x")); err == nil {
		t.Error("WriteAll wrote through a symlinked folder")
	}
	if _, err := v.Edit("leak.md", "key", "gone", false); err == nil {
		t.Error("Edit followed a symlink out of the vault")
	}
	if err := v.ReplaceSection("Out/secret.md", []string{"Secret"}, "gone\n"); err == nil {
		t.Error("ReplaceSection wrote through a symlinked folder")
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
