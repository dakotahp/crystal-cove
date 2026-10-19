package notes

import "testing"

func resolve(files []string, target, source string) (string, bool) {
	return NewResolver(files).Resolve(target, source)
}

func TestResolveMatchesByName(t *testing.T) {
	files := []string{"Areas/Bike Maintenance.md", "Inbox/today.md"}

	got, ok := resolve(files, "Bike Maintenance", "Inbox/today.md")
	if !ok || got != "Areas/Bike Maintenance.md" {
		t.Errorf("Resolve = %q, %v", got, ok)
	}
}

func TestResolveIgnoresCaseAndExtension(t *testing.T) {
	files := []string{"Areas/Bike Maintenance.md"}

	for _, target := range []string{"bike maintenance", "Bike Maintenance.md", "BIKE MAINTENANCE"} {
		if got, ok := resolve(files, target, ""); !ok || got != files[0] {
			t.Errorf("Resolve(%q) = %q, %v", target, got, ok)
		}
	}
}

func TestResolveAFullPath(t *testing.T) {
	files := []string{"Areas/Notes.md", "Archive/Notes.md"}

	if got, ok := resolve(files, "Archive/Notes", ""); !ok || got != "Archive/Notes.md" {
		t.Errorf("Resolve = %q, %v", got, ok)
	}
	if _, ok := resolve(files, "Missing/Notes", ""); ok {
		t.Error("a path link matched a note in another folder")
	}
}

func TestResolveAPartialPath(t *testing.T) {
	files := []string{"Areas/Real Estate/Comp Analysis/Method.md", "Areas/Method.md"}

	if got, ok := resolve(files, "Comp Analysis/Method", ""); !ok || got != files[0] {
		t.Errorf("Resolve = %q, %v, want the note whose path ends that way", got, ok)
	}
	if _, ok := resolve(files, "Analysis/Method", "x.md"); ok {
		t.Error("a partial path matched part of a folder name")
	}
}

func TestResolveAttachmentsAndOtherFileTypes(t *testing.T) {
	files := []string{"Assets/chart 1.png", "Jobs/Applications/Applications.base", "Board.canvas"}

	for _, target := range []string{"chart 1.png", "Applications.base", "Applications/Applications.base", "board.canvas"} {
		if _, ok := resolve(files, target, ""); !ok {
			t.Errorf("Resolve(%q) found nothing", target)
		}
	}
}

func TestResolveNeedsTheExtensionForAFileThatIsNotANote(t *testing.T) {
	files := []string{"Jobs/Applications/Applications.base", "Board.canvas"}

	for _, target := range []string{"Applications/Applications", "Board"} {
		if got, ok := resolve(files, target, ""); ok {
			t.Errorf("Resolve(%q) = %q, but Obsidian leaves a link without the extension unresolved", target, got)
		}
	}
}

func TestResolveRelativePaths(t *testing.T) {
	files := []string{"Projects/Plan.md", "Projects/Specs/Data.md", "Data.md"}

	if got, ok := resolve(files, "./Specs/Data.md", "Projects/Plan.md"); !ok || got != "Projects/Specs/Data.md" {
		t.Errorf("Resolve ./ = %q, %v", got, ok)
	}
	if got, ok := resolve(files, "../Plan", "Projects/Specs/Data.md"); !ok || got != "Projects/Plan.md" {
		t.Errorf("Resolve ../ = %q, %v", got, ok)
	}
	if _, ok := resolve(files, "../../Plan", "Projects/Specs/Data.md"); ok {
		t.Error("a relative path above the vault resolved")
	}
}

func TestResolveAVaultRootPath(t *testing.T) {
	if got, ok := resolve([]string{"Projects/Plan.md"}, "/Projects/Plan.md", "x.md"); !ok || got != "Projects/Plan.md" {
		t.Errorf("Resolve = %q, %v", got, ok)
	}
}

func TestResolvePrefersTheFirstPathWhenNamesRepeat(t *testing.T) {
	if got, _ := resolve([]string{"B/Note.md", "A/Note.md"}, "Note", ""); got != "A/Note.md" {
		t.Errorf("Resolve = %q, want the first in path order", got)
	}
}

func TestResolveReportsAMissingNote(t *testing.T) {
	if _, ok := resolve([]string{"Inbox/today.md"}, "Nothing Yet", ""); ok {
		t.Error("Resolve matched a note that does not exist")
	}
	if _, ok := resolve([]string{"Inbox/today.md"}, "", ""); ok {
		t.Error("Resolve matched an empty target")
	}
}

func TestResolveDoesNotMatchAFolder(t *testing.T) {
	if _, ok := resolve([]string{"Table Schemas/Drugs.md"}, "Table Schemas", ""); ok {
		t.Error("a link to a folder resolved")
	}
}
