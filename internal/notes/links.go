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
	text := inlineCode.ReplaceAllString(stripCodeFences(body), "")

	var links []Link
	seen := map[string]bool{}
	for _, m := range wikilink.FindAllStringSubmatch(text, -1) {
		link := Link{Embed: m[1] == "!"}
		rest := m[2]

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
