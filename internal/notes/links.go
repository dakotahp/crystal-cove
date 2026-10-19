package notes

import (
	"net/url"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// Link is one link found in a note.
type Link struct {
	// Target is the linked file, as written: a name or a path, without the
	// heading or block reference.
	Target string `json:"target"`
	// Heading is the part after "#", empty when the link has none.
	Heading string `json:"heading,omitempty"`
	// Alias is the display text, empty when the link has none.
	Alias string `json:"alias,omitempty"`
	// Embed reports an embedded link, written "![[...]]" or "![...](...)".
	Embed bool `json:"embed,omitempty"`
	// Markdown reports a link written "[text](target)" rather than as a
	// wikilink.
	Markdown bool `json:"markdown,omitempty"`
}

// wikilink matches [[target]] and ![[target]] with optional #heading,
// ^block and |alias parts.
var wikilink = regexp.MustCompile(`(!?)\[\[([^\]\n]+)\]\]`)

// markdownLink matches [text](target) and ![text](target). The target may
// hold one level of balanced parentheses, as in "Plan (Done).md", but no
// brackets, so a link wrapped around another link is read as the inner one.
var markdownLink = regexp.MustCompile(`(!?)\[([^\]\n]*)\]\(((?:[^()\[\]\n]|\([^()\[\]\n]*\))+)\)`)

// urlScheme matches a target that is a web address or another URI rather
// than a file in the vault.
var urlScheme = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)

// inlineCode matches a span of code, whose contents are examples rather
// than links.
var inlineCode = regexp.MustCompile("`[^`\n]*`")

// Links returns the wikilinks and Markdown links in a note's body, in the
// order they appear, with repeats of the same target and heading collapsed.
// Links inside code fences and inline code spans are skipped, because those
// are examples, and so are Markdown links to web addresses.
func Links(body string) []Link {
	masked := maskCode(body)
	type found struct {
		at   int
		link Link
	}
	var all []found
	for _, m := range wikilink.FindAllStringSubmatchIndex(masked, -1) {
		all = append(all, found{m[0], parseLink(m[3] > m[2], masked[m[4]:m[5]])})
	}
	for _, m := range markdownLink.FindAllStringSubmatchIndex(masked, -1) {
		if link, ok := parseMarkdownLink(m[3] > m[2], masked[m[4]:m[5]], masked[m[6]:m[7]]); ok {
			all = append(all, found{m[0], link})
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].at < all[j].at })

	links := make([]Link, 0, len(all))
	for _, f := range all {
		links = append(links, f.link)
	}
	return dedupe(links)
}

// Links returns the note's links: wikilinks held in frontmatter values,
// which Obsidian counts as links too, followed by the body's links.
func (n *Note) Links() []Link {
	var links []Link
	keys := make([]string, 0, len(n.Frontmatter))
	for k := range n.Frontmatter {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		links = append(links, valueLinks(n.Frontmatter[k])...)
	}
	return dedupe(append(links, Links(n.Body)...))
}

// valueLinks returns the wikilinks in a frontmatter value and the values
// nested inside it.
func valueLinks(value any) []Link {
	var links []Link
	switch v := value.(type) {
	case string:
		for _, m := range wikilink.FindAllStringSubmatch(v, -1) {
			links = append(links, parseLink(m[1] == "!", m[2]))
		}
	case []any:
		for _, item := range v {
			links = append(links, valueLinks(item)...)
		}
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			links = append(links, valueLinks(v[k])...)
		}
	}
	return links
}

// dedupe drops links with no target and repeats of a target and heading
// already seen.
func dedupe(links []Link) []Link {
	out := []Link{}
	seen := map[string]bool{}
	for _, link := range links {
		key := link.Target + "#" + link.Heading
		if link.Target == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, link)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// parseLink reads the text between a wikilink's brackets.
func parseLink(embed bool, inner string) Link {
	link := Link{Embed: embed}
	rest := inner
	if i := strings.Index(rest, "|"); i >= 0 {
		link.Alias = strings.TrimSpace(rest[i+1:])
		rest = rest[:i]
	}
	if i := strings.Index(rest, "^"); i >= 0 {
		rest = rest[:i]
	}
	if i := strings.Index(rest, "#"); i >= 0 {
		link.Heading = strings.TrimSpace(rest[i+1:])
		rest = rest[:i]
	}
	link.Target = strings.TrimSpace(rest)
	return link
}

// parseMarkdownLink reads a Markdown link's text and destination. It
// reports false for a destination outside the vault, such as a web address,
// and for a link to a heading in the same note.
func parseMarkdownLink(embed bool, text, dest string) (Link, bool) {
	start, end, _ := markdownDest(dest)
	raw := dest[start:end]
	if urlScheme.MatchString(raw) {
		return Link{}, false
	}
	target, heading, _ := strings.Cut(raw, "#")
	heading, _, _ = strings.Cut(heading, "^")
	link := Link{
		Target:   strings.TrimSpace(unescape(target)),
		Heading:  unescape(heading),
		Alias:    strings.TrimSpace(text),
		Embed:    embed,
		Markdown: true,
	}
	return link, link.Target != ""
}

// markdownDest returns where the destination of a Markdown link sits in
// dest, the text between its parentheses: without the angle brackets that
// may wrap it and without a quoted title after it.
func markdownDest(dest string) (start, end int, angle bool) {
	start = len(dest) - len(strings.TrimLeft(dest, " \t"))
	end = len(strings.TrimRight(dest, " \t"))
	if strings.HasPrefix(dest[start:], "<") {
		start++
		if i := strings.IndexByte(dest[start:end], '>'); i >= 0 {
			end = start + i
		}
		return start, end, true
	}
	rest := dest[start:end]
	if i := strings.IndexAny(rest, "\"'"); i > 0 && (rest[i-1] == ' ' || rest[i-1] == '\t') {
		end = start + len(strings.TrimRight(rest[:i], " \t"))
	}
	return start, end, false
}

func unescape(s string) string {
	if unescaped, err := url.PathUnescape(s); err == nil {
		return unescaped
	}
	return s
}

// RewriteLinks returns doc with the target of every link replaced by what
// rewrite returns for it, and how many links it changed. Headings, block
// references, aliases, titles and the embed marker stay as written. A
// Markdown link keeps its form: inside angle brackets the new target is
// written as is, and otherwise its spaces become %20. rewrite reports false
// to leave a link alone. Links inside code are never passed to it, because
// those are examples.
func RewriteLinks(doc string, rewrite func(Link) (string, bool)) (string, int) {
	type edit struct {
		start, end int
		text       string
	}
	var edits []edit
	masked := maskCode(doc)
	for _, m := range wikilink.FindAllStringSubmatchIndex(masked, -1) {
		inner := doc[m[4]:m[5]]
		link := parseLink(m[3] > m[2], inner)
		if link.Target == "" {
			continue
		}
		target, ok := rewrite(link)
		if !ok {
			continue
		}
		targetEnd := strings.IndexAny(inner, "|#^")
		if targetEnd < 0 {
			targetEnd = len(inner)
		}
		edits = append(edits, edit{m[4], m[4] + targetEnd, target})
	}
	for _, m := range markdownLink.FindAllStringSubmatchIndex(masked, -1) {
		dest := doc[m[6]:m[7]]
		link, ok := parseMarkdownLink(m[3] > m[2], doc[m[4]:m[5]], dest)
		if !ok {
			continue
		}
		target, ok := rewrite(link)
		if !ok {
			continue
		}
		start, end, angle := markdownDest(dest)
		file, _, _ := strings.Cut(dest[start:end], "#")
		if !angle {
			target = strings.ReplaceAll(target, " ", "%20")
		}
		edits = append(edits, edit{m[6] + start, m[6] + start + len(strings.TrimRight(file, " \t")), target})
	}
	if len(edits) == 0 {
		return doc, 0
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].start < edits[j].start })

	var out strings.Builder
	last := 0
	for _, e := range edits {
		out.WriteString(doc[last:e.start])
		out.WriteString(e.text)
		last = e.end
	}
	out.WriteString(doc[last:])
	return out.String(), len(edits)
}

// maskCode returns text with fenced code blocks and inline code spans
// replaced by spaces, so matches found in the result can be applied to text
// at the same byte offsets.
func maskCode(text string) string {
	lines := strings.SplitAfter(text, "\n")
	for i, fenced := range fencedLines(lines) {
		if fenced {
			lines[i] = blank(lines[i])
		}
	}
	return inlineCode.ReplaceAllStringFunc(strings.Join(lines, ""), blank)
}

// fencedLines reports which lines belong to a fenced code block, the fence
// lines included. As in CommonMark, a block opened with backticks or tildes
// closes only on a run of the same character at least as long, so a block
// opened with four backticks can hold examples fenced with three.
func fencedLines(lines []string) []bool {
	fenced := make([]bool, len(lines))
	open := ""
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		run := fenceRun(trimmed)
		switch {
		case open == "":
			if run != "" {
				open = run
				fenced[i] = true
			}
		default:
			fenced[i] = true
			if run != "" && run[0] == open[0] && len(run) >= len(open) && trimmed == run {
				open = ""
			}
		}
	}
	return fenced
}

// fenceRun returns the run of backticks or tildes a fence line starts with,
// or "" when the line is not a fence.
func fenceRun(line string) string {
	if line == "" || (line[0] != '`' && line[0] != '~') {
		return ""
	}
	n := 1
	for n < len(line) && line[n] == line[0] {
		n++
	}
	if n < 3 {
		return ""
	}
	return line[:n]
}

// blank replaces every byte of s except line breaks with a space.
func blank(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c != '\n' {
			b[i] = ' '
		}
	}
	return string(b)
}

// Resolver finds the file a link points at, the way Obsidian does, given
// every file in the vault: notes, attachments and any other file type.
type Resolver struct {
	// paths maps each lower-cased path to the path as stored.
	paths map[string]string
	// byName maps each lower-cased file name to the paths carrying it, in
	// path order.
	byName map[string][]string
}

// NewResolver returns a Resolver over files, which are vault-relative paths.
func NewResolver(files []string) *Resolver {
	sorted := slices.Clone(files)
	slices.Sort(sorted)
	r := &Resolver{paths: map[string]string{}, byName: map[string][]string{}}
	for _, f := range sorted {
		lower := strings.ToLower(f)
		if _, ok := r.paths[lower]; !ok {
			r.paths[lower] = f
		}
		name := path.Base(lower)
		r.byName[name] = append(r.byName[name], f)
	}
	return r
}

// Resolve returns the file target points at, linked from the note at
// source. Matching ignores case. A target starting with ./ or ../ is a path
// relative to source's folder. Any other target is a vault-relative path,
// or the end of one: a bare name matches a file with that name in any
// folder, and Folder/Name matches a file whose path ends that way, the
// first in path order when several do. A target without an extension
// matches only a note, so a link to Board does not reach Board.canvas. The
// second return value is false when nothing matches, which is a link to a
// note that does not exist yet.
func (r *Resolver) Resolve(target, source string) (string, bool) {
	want := strings.ToLower(strings.TrimSpace(target))
	if strings.HasPrefix(want, "./") || strings.HasPrefix(want, "../") {
		want = path.Join(path.Dir(strings.ToLower(source)), want)
		if want == ".." || strings.HasPrefix(want, "../") {
			return "", false
		}
		return r.exact(want)
	}
	want = strings.TrimPrefix(want, "/")
	if want == "" {
		return "", false
	}
	if p, ok := r.exact(want); ok {
		return p, true
	}
	for _, candidate := range []string{want, want + ".md"} {
		for _, p := range r.byName[path.Base(candidate)] {
			if strings.HasSuffix(strings.ToLower(p), "/"+candidate) {
				return p, true
			}
		}
	}
	return "", false
}

func (r *Resolver) exact(want string) (string, bool) {
	if p, ok := r.paths[want]; ok {
		return p, true
	}
	p, ok := r.paths[want+".md"]
	return p, ok
}
