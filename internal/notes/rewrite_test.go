package notes

import "testing"

func renameOld(l Link) (string, bool) {
	if l.Target != "Old" {
		return "", false
	}
	return "New", true
}

func TestRewriteLinksKeepsEverythingButTheTarget(t *testing.T) {
	doc := "[[Old]] [[Old#Tasks|the tasks]] ![[Old]] [[Old^block]] [[Old|alias]] [[Other]]\n"

	got, n := RewriteLinks(doc, renameOld)
	want := "[[New]] [[New#Tasks|the tasks]] ![[New]] [[New^block]] [[New|alias]] [[Other]]\n"
	if got != want || n != 5 {
		t.Errorf("RewriteLinks = %q, %d; want %q, 5", got, n, want)
	}
}

func TestRewriteLinksLeavesCodeAlone(t *testing.T) {
	doc := "```\n[[Old]]\n```\n`[[Old]]` [[Old]]\n"

	got, n := RewriteLinks(doc, renameOld)
	want := "```\n[[Old]]\n```\n`[[Old]]` [[New]]\n"
	if got != want || n != 1 {
		t.Errorf("RewriteLinks = %q, %d; want %q, 1", got, n, want)
	}
}

func TestRewriteLinksCoversFrontmatter(t *testing.T) {
	doc := "---\nup: \"[[Old]]\"\n---\nbody\n"

	got, n := RewriteLinks(doc, renameOld)
	if got != "---\nup: \"[[New]]\"\n---\nbody\n" || n != 1 {
		t.Errorf("RewriteLinks = %q, %d", got, n)
	}
}

func TestRewriteLinksWithNothingToChange(t *testing.T) {
	doc := "[[Other]] and text\n"

	got, n := RewriteLinks(doc, renameOld)
	if got != doc || n != 0 {
		t.Errorf("RewriteLinks = %q, %d; want the document unchanged", got, n)
	}
}

func TestLinksIgnoresCodeThatSplitsBrackets(t *testing.T) {
	if got := Links("[`x`[Old]]\n"); len(got) != 0 {
		t.Errorf("Links = %+v, want no link built from text around inline code", got)
	}
}
