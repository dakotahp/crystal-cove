package notes

import "testing"

func TestResolveLinkMatchesByName(t *testing.T) {
	paths := []string{"Areas/Bike Maintenance.md", "Inbox/today.md"}

	got, ok := ResolveLink("Bike Maintenance", paths)
	if !ok || got != "Areas/Bike Maintenance.md" {
		t.Errorf("ResolveLink = %q, %v", got, ok)
	}
}

func TestResolveLinkIgnoresCaseAndExtension(t *testing.T) {
	paths := []string{"Areas/Bike Maintenance.md"}

	for _, target := range []string{"bike maintenance", "Bike Maintenance.md", "BIKE MAINTENANCE"} {
		if got, ok := ResolveLink(target, paths); !ok || got != paths[0] {
			t.Errorf("ResolveLink(%q) = %q, %v", target, got, ok)
		}
	}
}

func TestResolveLinkWithASlashNeedsTheWholePath(t *testing.T) {
	paths := []string{"Areas/Notes.md", "Archive/Notes.md"}

	if got, ok := ResolveLink("Archive/Notes", paths); !ok || got != "Archive/Notes.md" {
		t.Errorf("ResolveLink = %q, %v", got, ok)
	}
	if _, ok := ResolveLink("Missing/Notes", paths); ok {
		t.Error("a path link matched a note in another folder")
	}
}

func TestResolveLinkReportsAMissingNote(t *testing.T) {
	if _, ok := ResolveLink("Nothing Yet", []string{"Inbox/today.md"}); ok {
		t.Error("ResolveLink matched a note that does not exist")
	}
	if _, ok := ResolveLink("", []string{"Inbox/today.md"}); ok {
		t.Error("ResolveLink matched an empty target")
	}
}
