package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadReturnsAVersionThatFollowsTheText(t *testing.T) {
	v := New("Personal", t.TempDir())
	mustWrite(t, v, "a.md", "one\n")

	first, err := v.Read("a.md", 0)
	if err != nil {
		t.Fatal(err)
	}
	again, err := v.Read("a.md", 0)
	if err != nil {
		t.Fatal(err)
	}
	if first.Version == "" || first.Version != again.Version {
		t.Errorf("versions %q and %q, want one stable version for unchanged text", first.Version, again.Version)
	}
	mustWrite(t, v, "a.md", "two\n")
	changed, err := v.Read("a.md", 0)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Version == first.Version {
		t.Error("the version did not change with the text")
	}
}

func TestEditRefusesAStaleVersion(t *testing.T) {
	v := New("Personal", t.TempDir())
	mustWrite(t, v, "a.md", "alpha\nbeta\n")
	read, err := v.Read("a.md", 0)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, v, "a.md", "alpha\nbeta\nsynced\n")

	_, _, err = v.Edit("a.md", "alpha", "ALPHA", false, read.Version)
	if err == nil || !strings.Contains(err.Error(), "changed since") {
		t.Fatalf("err = %v, want a changed-since error", err)
	}
	if got := mustReadFile(t, v, "a.md"); got != "alpha\nbeta\nsynced\n" {
		t.Errorf("note = %q, want it untouched", got)
	}
}

func TestEditWithTheCurrentVersionReturnsTheNextOne(t *testing.T) {
	v := New("Personal", t.TempDir())
	mustWrite(t, v, "a.md", "alpha\n")
	read, err := v.Read("a.md", 0)
	if err != nil {
		t.Fatal(err)
	}

	_, next, err := v.Edit("a.md", "alpha", "beta", false, read.Version)
	if err != nil {
		t.Fatal(err)
	}
	reread, err := v.Read("a.md", 0)
	if err != nil {
		t.Fatal(err)
	}
	if next != reread.Version {
		t.Errorf("Edit returned version %q, want %q, the version a new read reports", next, reread.Version)
	}
	if _, _, err := v.Edit("a.md", "beta", "gamma", false, next); err != nil {
		t.Errorf("a second edit with the returned version failed: %v", err)
	}
}

func TestSectionVersionIgnoresOtherSections(t *testing.T) {
	v := New("Personal", t.TempDir())
	target := filepath.Join(v.Root(), "a.md")
	mustWrite(t, v, "a.md", "## Log\n- one\n## Notes\nx\n")

	first, err := v.GetSection("a.md", []string{"Log"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("## Log\n- one\n## Notes\nx\ny\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := v.GetSection("a.md", []string{"Log"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if first.Version == "" || first.Version != second.Version {
		t.Errorf("section versions %q and %q, want an edit elsewhere to leave it alone", first.Version, second.Version)
	}
}
