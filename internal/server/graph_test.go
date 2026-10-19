package server

import (
	"context"
	"slices"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func unresolvedTargets(list *UnresolvedLinks) []string {
	out := []string{}
	for _, l := range list.Links {
		out = append(out, l.Source+" -> "+l.Target)
	}
	return out
}

func TestUnresolvedLinksFollowsObsidianRules(t *testing.T) {
	s, _ := metaServer(t, map[string]string{
		"Jobs/Jobs.md": "[[Applications/Applications]] [[Applications/Applications.base]] [[Note#Heading]]\n" +
			"![[chart.png]] [[Missing]] [[Jobs]]\n```\n[[In Code]]\n```\n`[[Inline]]` [md](Gone.md)\n",
		"Jobs/Applications/Applications.base": "views: []\n",
		"Areas/Note.md":                       "---\nauthor: \"[[Nobody]]\"\n---\nbody\n",
		"Assets/chart.png":                    "png",
	})

	_, res, err := s.unresolvedLinks(context.Background(), &mcp.CallToolRequest{}, unresolvedLinksInput{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"Areas/Note.md -> Nobody",
		"Jobs/Jobs.md -> Applications/Applications",
		"Jobs/Jobs.md -> Missing",
		"Jobs/Jobs.md -> Gone.md",
	}
	if got := unresolvedTargets(res); !slices.Equal(got, want) {
		t.Errorf("links = %q, want %q", got, want)
	}
	if res.Total != 4 || res.NextOffset != -1 || res.NotesScanned != 2 {
		t.Errorf("total = %d, next = %d, scanned = %d", res.Total, res.NextOffset, res.NotesScanned)
	}
}

func TestUnresolvedLinksFiltersByFolderAndPages(t *testing.T) {
	s, _ := metaServer(t, map[string]string{
		"A/one.md":       "[[x]] [[y]]\n",
		"A/Deep/two.md":  "[[z]]\n",
		"Archive/old.md": "[[w]]\n",
		"Bible/gen.md":   "[[v]]\n",
	})
	ctx := context.Background()

	_, res, err := s.unresolvedLinks(ctx, &mcp.CallToolRequest{}, unresolvedLinksInput{
		folderFilter: folderFilter{Folders: []string{"A/", "Archive"}, ExcludeFolders: []string{"A/Deep"}},
		Limit:        2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := unresolvedTargets(res); !slices.Equal(got, []string{"A/one.md -> x", "A/one.md -> y"}) {
		t.Errorf("first page = %q", got)
	}
	if res.Total != 3 || res.NextOffset != 2 {
		t.Errorf("total = %d, next = %d", res.Total, res.NextOffset)
	}

	_, res, err = s.unresolvedLinks(ctx, &mcp.CallToolRequest{}, unresolvedLinksInput{
		folderFilter: folderFilter{Folders: []string{"A/", "Archive"}, ExcludeFolders: []string{"A/Deep"}},
		Offset:       2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := unresolvedTargets(res); !slices.Equal(got, []string{"Archive/old.md -> w"}) {
		t.Errorf("second page = %q", got)
	}
}

func TestUnresolvedLinksRejectsABadOffset(t *testing.T) {
	s, _ := metaServer(t, map[string]string{"a.md": "[[x]]\n"})
	if _, _, err := s.unresolvedLinks(context.Background(), &mcp.CallToolRequest{}, unresolvedLinksInput{Offset: 5}); err == nil {
		t.Error("an offset past the end was accepted")
	}
}

func orphanPaths(list *OrphanList) []string {
	out := []string{}
	for _, n := range list.Notes {
		out = append(out, n.Path)
	}
	return out
}

func TestOrphansAreNotesNothingLinksTo(t *testing.T) {
	s, _ := metaServer(t, map[string]string{
		"hub.md":              "[[linked]] and [[Sub/partial]] and ![[pic.png]]\n",
		"linked.md":           "back to [[hub]]\n",
		"Deep/Sub/partial.md": "body\n",
		"self.md":             "only [[self]]\n",
		"alone.md":            "nothing\n",
		"pointer.md":          "---\nup: \"[[fm target]]\"\n---\n[[Nowhere]]\n",
		"fm target.md":        "body\n",
		"pic.png":             "png",
	})
	ctx := context.Background()

	_, res, err := s.orphans(ctx, &mcp.CallToolRequest{}, orphansInput{})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"alone.md", "pointer.md", "self.md"}; !slices.Equal(orphanPaths(res), want) {
		t.Errorf("orphans = %q, want %q", orphanPaths(res), want)
	}
	if res.Notes[1].OutgoingLinks != 1 || res.Notes[2].OutgoingLinks != 0 {
		t.Errorf("outgoing = %+v, want the frontmatter link counted and the self-link not", res.Notes)
	}

	_, res, err = s.orphans(ctx, &mcp.CallToolRequest{}, orphansInput{Unlinked: true})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"alone.md", "self.md"}; !slices.Equal(orphanPaths(res), want) {
		t.Errorf("unlinked = %q, want %q", orphanPaths(res), want)
	}
}

func TestOrphansCountLinksFromExcludedFolders(t *testing.T) {
	s, _ := metaServer(t, map[string]string{
		"Notes/kept.md":  "body\n",
		"Archive/old.md": "[[kept]]\n",
	})

	_, res, err := s.orphans(context.Background(), &mcp.CallToolRequest{}, orphansInput{
		folderFilter: folderFilter{ExcludeFolders: []string{"Archive"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Notes) != 0 || res.Total != 0 {
		t.Errorf("orphans = %q, want none: a link from an excluded folder still counts", orphanPaths(res))
	}
}

func TestGraphToolsReportAMissingVault(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()
	if _, _, err := s.orphans(ctx, &mcp.CallToolRequest{}, orphansInput{Vault: "Nope"}); err == nil {
		t.Error("orphans accepted an unknown vault")
	}
	if _, _, err := s.unresolvedLinks(ctx, &mcp.CallToolRequest{}, unresolvedLinksInput{Vault: "Nope"}); err == nil {
		t.Error("unresolved_links accepted an unknown vault")
	}
}

func TestFolderFilter(t *testing.T) {
	f := folderFilter{Folders: []string{"Projects", "/Areas/"}, ExcludeFolders: []string{"Projects/Old"}}
	for p, want := range map[string]bool{
		"Projects/a.md":       true,
		"Areas/b/c.md":        true,
		"Projects/Old/d.md":   false,
		"ProjectsX/e.md":      false,
		"root.md":             false,
		"Projects/Older/f.md": true,
	} {
		if got := f.includes(p); got != want {
			t.Errorf("includes(%q) = %v, want %v", p, got, want)
		}
	}
	if !(folderFilter{}).includes("anything.md") {
		t.Error("an empty filter left a note out")
	}
}
