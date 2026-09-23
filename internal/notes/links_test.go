package notes

import (
	"slices"
	"testing"
)

func targets(links []Link) []string {
	out := make([]string, 0, len(links))
	for _, l := range links {
		out = append(out, l.Target)
	}
	return out
}

func TestLinksReadsPlainAliasHeadingAndBlockForms(t *testing.T) {
	body := "See [[Bike Maintenance]] and [[Tyre Pressure|the numbers]].\n" +
		"Also [[Bike Maintenance#Chain]] and [[Notes^abc123]].\n"

	got := Links(body)
	want := []string{"Bike Maintenance", "Tyre Pressure", "Bike Maintenance", "Notes"}
	if !slices.Equal(targets(got), want) {
		t.Errorf("targets = %q, want %q", targets(got), want)
	}
	if got[1].Alias != "the numbers" {
		t.Errorf("alias = %q", got[1].Alias)
	}
	if got[2].Heading != "Chain" {
		t.Errorf("heading = %q", got[2].Heading)
	}
}

func TestLinksMarksEmbeds(t *testing.T) {
	got := Links("![[Diagram.png]] and [[Plain Note]]\n")
	if len(got) != 2 {
		t.Fatalf("links = %+v", got)
	}
	if !got[0].Embed || got[1].Embed {
		t.Errorf("embed flags = %v, %v", got[0].Embed, got[1].Embed)
	}
}

func TestLinksIgnoreCodeAndEmptyTargets(t *testing.T) {
	body := "```\n[[Not A Link]]\n```\n\n`[[Also Not]]` but [[Real Link]] counts.\n[[]] is nothing.\n"

	if got := targets(Links(body)); !slices.Equal(got, []string{"Real Link"}) {
		t.Errorf("targets = %q, want only the real link", got)
	}
}

func TestLinksKeepFolderPaths(t *testing.T) {
	got := Links("[[Areas/Bike Maintenance]]\n")
	if len(got) != 1 || got[0].Target != "Areas/Bike Maintenance" {
		t.Errorf("links = %+v, want the path kept", got)
	}
}

func TestLinksDeduplicateRepeats(t *testing.T) {
	got := Links("[[One]] and [[One]] again, plus [[Two]]\n")
	if want := []string{"One", "Two"}; !slices.Equal(targets(got), want) {
		t.Errorf("targets = %q, want %q", targets(got), want)
	}
}
