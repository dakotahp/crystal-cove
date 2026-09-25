package vault

import (
	"strings"
	"testing"
)

// replaceSection replaces a section's body the way an agent must: with the
// version it read first.
func replaceSection(t *testing.T, v *Vault, rel string, headingPath []string, content string) error {
	t.Helper()
	read, err := v.GetSection(rel, headingPath, 0)
	if err != nil {
		return err
	}
	_, err = v.EditSection(rel, headingPath, SectionReplace, content, read.Version)
	return err
}

func TestEditSectionAppendsAfterTheSectionsOwnText(t *testing.T) {
	for _, tc := range []struct{ name, note, content, want string }{
		{"list", "## Log\n- a\n## Next\n", "- b", "## Log\n- a\n- b\n## Next\n"},
		{"blank line kept", "## Log\n- a\n\n## Next\n", "- b\n", "## Log\n- a\n- b\n\n## Next\n"},
		{"before a subheading", "## Tasks\n- a\n### Done\n- x\n", "- b", "## Tasks\n- a\n- b\n### Done\n- x\n"},
		{"heading at the end", "## Log", "- a", "## Log\n- a\n"},
		{"empty body", "## Log\n## Next\n", "- a", "## Log\n- a\n## Next\n"},
		{"no final newline", "## Log\n- a", "- b", "## Log\n- a\n- b\n"},
		{"crlf", "## Log\r\n- a\r\n## Next\r\n", "- b", "## Log\r\n- a\r\n- b\r\n## Next\r\n"},
		{"before a setext sibling", "Log\n---\n- a\n\nNext\n----\n", "- b", "Log\n---\n- a\n- b\n\nNext\n----\n"},
		{"before a setext child", "# Log\nSub\n---\nbody\n", "- a", "# Log\n- a\n\nSub\n---\nbody\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := newTestVault(t)
			mustWrite(t, v, "note.md", tc.note)
			heading := "Log"
			if strings.Contains(tc.note, "Tasks") {
				heading = "Tasks"
			}
			if _, err := v.EditSection("note.md", []string{heading}, SectionAppend, tc.content, ""); err != nil {
				t.Fatal(err)
			}
			if got := mustReadFile(t, v, "note.md"); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEditSectionPrependsBelowTheHeading(t *testing.T) {
	for _, tc := range []struct{ name, note, want string }{
		{"list", "## Log\n- a\n", "## Log\n- b\n- a\n"},
		{"after blank lines", "## Log\n\n- a\n", "## Log\n\n- b\n- a\n"},
		{"heading at the end", "## Log", "## Log\n- b\n"},
		{"before a subheading", "## Log\n### Old\nx\n", "## Log\n- b\n### Old\nx\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := newTestVault(t)
			mustWrite(t, v, "note.md", tc.note)
			if _, err := v.EditSection("note.md", []string{"Log"}, SectionPrepend, "- b", ""); err != nil {
				t.Fatal(err)
			}
			if got := mustReadFile(t, v, "note.md"); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEditSectionReplaceNeedsTheCurrentVersion(t *testing.T) {
	v := newTestVault(t)
	mustWrite(t, v, "note.md", "## Log\n- a\n### Sub\nkeep?\n## Next\n")

	if _, err := v.EditSection("note.md", []string{"Log"}, SectionReplace, "new\n", ""); err == nil || !strings.Contains(err.Error(), "version") {
		t.Errorf("replace without a version: err = %v, want one asking for the version", err)
	}
	read, err := v.GetSection("note.md", []string{"Log"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, v, "note.md", "## Log\n- a\n- added elsewhere\n### Sub\nkeep?\n## Next\n")
	if _, err := v.EditSection("note.md", []string{"Log"}, SectionReplace, "new\n", read.Version); err == nil || !strings.Contains(err.Error(), "changed since") {
		t.Errorf("replace with a stale version: err = %v, want it refused", err)
	}
	if got := mustReadFile(t, v, "note.md"); !strings.Contains(got, "added elsewhere") {
		t.Errorf("note = %q, want the newer text kept", got)
	}

	read, err = v.GetSection("note.md", []string{"Log"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	next, err := v.EditSection("note.md", []string{"Log"}, SectionReplace, "new\n", read.Version)
	if err != nil {
		t.Fatal(err)
	}
	if got := mustReadFile(t, v, "note.md"); got != "## Log\nnew\n## Next\n" {
		t.Errorf("note = %q", got)
	}
	reread, err := v.GetSection("note.md", []string{"Log"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if next != reread.Version {
		t.Errorf("EditSection returned version %q, want %q", next, reread.Version)
	}
}

func TestEditSectionAcceptsAVersionFromBeforeAnEditElsewhere(t *testing.T) {
	v := newTestVault(t)
	mustWrite(t, v, "note.md", "## Log\n- a\n## Notes\nx\n")
	read, err := v.GetSection("note.md", []string{"Log"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, v, "note.md", "## Log\n- a\n## Notes\nx\nsynced\n")

	if _, err := v.EditSection("note.md", []string{"Log"}, SectionAppend, "- b", read.Version); err != nil {
		t.Fatal(err)
	}
	if got := mustReadFile(t, v, "note.md"); got != "## Log\n- a\n- b\n## Notes\nx\nsynced\n" {
		t.Errorf("note = %q, want both changes", got)
	}
}

func TestEditSectionThatDuplicatesItsHeadingReturnsNoVersion(t *testing.T) {
	v := newTestVault(t)
	mustWrite(t, v, "note.md", "# A\n## Log\n- a\n")

	version, err := v.EditSection("note.md", []string{"Log"}, SectionAppend, "## Log", "")
	if err != nil || version != "" {
		t.Errorf("EditSection = %q, %v; want the edit made and no version, as the heading is no longer unique", version, err)
	}
}

func TestEditSectionRejectsBadRequests(t *testing.T) {
	v := newTestVault(t)
	mustWrite(t, v, "note.md", "## Log\n- a\n")
	for _, tc := range []struct {
		mode    SectionMode
		content string
		want    string
	}{
		{SectionAppend, "", "content"},
		{SectionPrepend, "", "content"},
		{"rewrite", "x", "not one of"},
	} {
		if _, err := v.EditSection("note.md", []string{"Log"}, tc.mode, tc.content, ""); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%+v: err = %v, want it to mention %q", tc, err, tc.want)
		}
	}
	if got := mustReadFile(t, v, "note.md"); got != "## Log\n- a\n" {
		t.Errorf("note = %q, want it untouched", got)
	}
}
