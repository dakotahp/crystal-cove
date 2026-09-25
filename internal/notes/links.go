package notes

import (
	"regexp"
	"strings"
)

// Link is one wikilink found in a note's body.
type Link struct {
	// Target is the linked note, as written: a name or a vault-relative
	// path, without the extension, heading or block reference.
	Target string `json:"target"`
	// Heading is the part after "#", empty when the link has none.
	Heading string `json:"heading,omitempty"`
	// Alias is the display text after "|", empty when the link has none.
	Alias string `json:"alias,omitempty"`
	// Embed reports an embedded link, written "![[...]]".
	Embed bool `json:"embed,omitempty"`
}

// wikilink matches [[target]] and ![[target]] with optional #heading,
// ^block and |alias parts.
var wikilink = regexp.MustCompile(`(!?)\[\[([^\]\n]+)\]\]`)

// inlineCode matches a span of code, whose contents are examples rather
// than links.
var inlineCode = regexp.MustCompile("`[^`\n]*`")

// Links returns the wikilinks in a note's body, in the order they appear,
// with repeats of the same target and heading collapsed. Links inside code
// fences and inline code spans are skipped, because those are examples.
func Links(body string) []Link {
	var links []Link
	seen := map[string]bool{}
	for _, m := range wikilink.FindAllStringSubmatch(maskCode(body), -1) {
		link := parseLink(m[1] == "!", m[2])
		if link.Target == "" {
			continue
		}
		key := link.Target + "#" + link.Heading
		if seen[key] {
			continue
		}
		seen[key] = true
		links = append(links, link)
	}
	return links
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

// RewriteLinks returns doc with the target of every wikilink replaced by what
// rewrite returns for it, and how many links it changed. Headings, block
// references, aliases and the embed marker stay as written. rewrite reports
// false to leave a link alone. Links inside code are never passed to it,
// because those are examples.
func RewriteLinks(doc string, rewrite func(Link) (string, bool)) (string, int) {
	var out strings.Builder
	changed, last := 0, 0
	for _, m := range wikilink.FindAllStringSubmatchIndex(maskCode(doc), -1) {
		start, end := m[4], m[5]
		inner := doc[start:end]
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
		out.WriteString(doc[last:start])
		out.WriteString(target)
		last = start + targetEnd
		changed++
	}
	if changed == 0 {
		return doc, 0
	}
	out.WriteString(doc[last:])
	return out.String(), changed
}

// maskCode returns text with fenced code blocks and inline code spans
// replaced by spaces, so matches found in the result can be applied to text
// at the same byte offsets.
func maskCode(text string) string {
	lines := strings.SplitAfter(text, "\n")
	inFence := false
	for i, line := range lines {
		fence := strings.HasPrefix(strings.TrimSpace(line), "```")
		if fence || inFence {
			lines[i] = blank(line)
		}
		if fence {
			inFence = !inFence
		}
	}
	return inlineCode.ReplaceAllStringFunc(strings.Join(lines, ""), blank)
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

// ResolveLink finds the note a link points at, given every note path in the
// vault. Obsidian links usually carry a name rather than a path, so a bare
// name matches a note with that name anywhere in the vault; a link holding
// a slash is treated as a vault-relative path. Matching ignores case, as
// Obsidian's own resolution does. The second return value is false when
// nothing matches, which is a link to a note that does not exist yet.
func ResolveLink(target string, notePaths []string) (string, bool) {
	want := strings.ToLower(strings.TrimSuffix(target, ".md"))
	if want == "" {
		return "", false
	}
	wantsPath := strings.Contains(want, "/")

	for _, path := range notePaths {
		candidate := strings.ToLower(strings.TrimSuffix(path, ".md"))
		if wantsPath {
			if candidate == want {
				return path, true
			}
			continue
		}
		if name := candidate[strings.LastIndex(candidate, "/")+1:]; name == want {
			return path, true
		}
	}
	return "", false
}
