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

func TestLinksSkipALongerFenceHoldingShorterFences(t *testing.T) {
	body := "````markdown\n```bash\necho\n```\n- [[Template Placeholder]]\n````\n[[Real Link]]\n"

	if got := targets(Links(body)); !slices.Equal(got, []string{"Real Link"}) {
		t.Errorf("targets = %q, want only the link after the outer fence", got)
	}
}

func TestLinksSkipTildeFences(t *testing.T) {
	body := "~~~\n[[Not A Link]]\n~~~\n[[Real Link]]\n"

	if got := targets(Links(body)); !slices.Equal(got, []string{"Real Link"}) {
		t.Errorf("targets = %q, want only the real link", got)
	}
}

func TestLinksReadMarkdownLinks(t *testing.T) {
	body := "See [the plan](Projects/Plan.md#Goals), ![chart](chart%201.png) and [spaced](<My Note.md>).\n" +
		"Not [a site](https://example.com), [mail](mailto:a@b.c) or [here](#Heading).\n" +
		"[titled](Other.md \"A title\")\n"

	got := Links(body)
	if want := []string{"Projects/Plan.md", "chart 1.png", "My Note.md", "Other.md"}; !slices.Equal(targets(got), want) {
		t.Fatalf("targets = %q, want %q", targets(got), want)
	}
	if !got[0].Markdown || got[0].Heading != "Goals" || got[0].Alias != "the plan" {
		t.Errorf("first link = %+v", got[0])
	}
	if !got[1].Embed {
		t.Errorf("image link = %+v, want an embed", got[1])
	}
}

func TestLinksReadMarkdownTargetsHoldingParentheses(t *testing.T) {
	body := "[TRD](TRD-002%20Migrations%20(Done).md) and [source]([paper](https://example.com/paper.pdf))\n"

	if got := targets(Links(body)); !slices.Equal(got, []string{"TRD-002 Migrations (Done).md"}) {
		t.Errorf("targets = %q, want the whole file name and no link around a web address", got)
	}
}

func TestLinksKeepAMarkdownTargetThatIsNotEscaped(t *testing.T) {
	if got := targets(Links("[odd](100%.md)\n")); !slices.Equal(got, []string{"100%.md"}) {
		t.Errorf("targets = %q, want the target as written", got)
	}
}

func TestLinksSkipMarkdownLinksInCode(t *testing.T) {
	if got := Links("`[x](Note.md)`\n```\n[y](Other.md)\n```\n"); len(got) != 0 {
		t.Errorf("links = %+v, want none", got)
	}
}

func TestNoteLinksIncludeFrontmatterWikilinks(t *testing.T) {
	n, err := Parse([]byte("---\nauthor: \"[[Ed Example]]\"\nrelated:\n  - \"[[One]]\"\n  - plain text\nnested:\n  key: \"[[Three]]\"\nurl: https://example.com\n---\nBody [[Two]] and [[One]].\n"))
	if err != nil {
		t.Fatal(err)
	}
	got := targets(n.Links())
	if want := []string{"Ed Example", "Three", "One", "Two"}; !slices.Equal(got, want) {
		t.Errorf("links = %q, want %q", got, want)
	}
}
