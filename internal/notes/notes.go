// Package notes parses a Markdown note's YAML frontmatter and the tags it
// carries, and rewrites frontmatter fields without disturbing the body.
package notes

import (
	"bytes"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// Note is a parsed Markdown note.
type Note struct {
	// Frontmatter holds the YAML fields, empty when the note has none.
	Frontmatter map[string]any
	// HasFrontmatter distinguishes a note with an empty block from one
	// with no block at all.
	HasFrontmatter bool
	// Body is everything after the frontmatter block, unchanged.
	Body string
	// Tags are the note's tags, from the frontmatter "tags" field and from
	// inline hashtags, deduplicated and in first-seen order.
	Tags []string
}

const fence = "---"

// inlineTag matches a hashtag that starts a word and holds at least one
// letter, which keeps "#42" and a URL fragment from counting as tags.
var inlineTag = regexp.MustCompile(`(^|[\s(\[])#([A-Za-z0-9_/-]*[A-Za-z][A-Za-z0-9_/-]*)`)

// Parse reads a note's frontmatter, body and tags. A note without a
// frontmatter block is not an error; malformed YAML inside one is.
func Parse(data []byte) (*Note, error) {
	n := &Note{Frontmatter: map[string]any{}, Body: string(data)}

	if block, body, ok := splitFrontmatter(string(data)); ok {
		n.HasFrontmatter = true
		n.Body = body
		if strings.TrimSpace(block) != "" {
			var doc yaml.Node
			if err := yaml.Unmarshal([]byte(block), &doc); err != nil {
				return nil, fmt.Errorf("parsing frontmatter: %w", err)
			}
			datesAsText(&doc)
			if err := doc.Decode(&n.Frontmatter); err != nil {
				return nil, fmt.Errorf("parsing frontmatter: %w", err)
			}
			if n.Frontmatter == nil {
				n.Frontmatter = map[string]any{}
			}
		}
	}

	n.Tags = collectTags(n.Frontmatter, n.Body)
	return n, nil
}

// datesAsText keeps dates as the text the note holds. Decoded to time.Time
// they would come back reformatted, and a client writing one back would
// change the file.
func datesAsText(node *yaml.Node) {
	if isDate(node) {
		node.Tag = "!!str"
	}
	for _, child := range node.Content {
		datesAsText(child)
	}
}

func isDate(node *yaml.Node) bool {
	return node.Kind == yaml.ScalarNode && node.ShortTag() == "!!timestamp"
}

// splitFrontmatter returns the YAML block and the body that follows it. The
// block counts only when the note opens with a fence line and a closing
// fence follows, which is Obsidian's own rule.
func splitFrontmatter(doc string) (block, body string, ok bool) {
	rest, found := strings.CutPrefix(doc, fence+"\n")
	if !found {
		return "", doc, false
	}
	// Walk line starts until one is a closing fence of its own. Scanning by
	// line also accepts an empty block, whose closing fence is the very
	// first line and so has no newline in front of it.
	for at := 0; at <= len(rest); {
		line := rest[at:]
		if end := strings.IndexByte(line, '\n'); end >= 0 {
			line = line[:end]
		}
		if line == fence {
			return rest[:at], strings.TrimPrefix(rest[at+len(fence):], "\n"), true
		}
		next := strings.IndexByte(rest[at:], '\n')
		if next < 0 {
			break
		}
		at += next + 1
	}
	return "", doc, false
}

func collectTags(fm map[string]any, body string) []string {
	var tags []string
	seen := map[string]bool{}
	add := func(tag string) {
		tag = strings.TrimPrefix(strings.TrimSpace(tag), "#")
		if tag == "" || seen[tag] {
			return
		}
		seen[tag] = true
		tags = append(tags, tag)
	}

	switch v := fm["tags"].(type) {
	case string:
		for _, tag := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ' ' }) {
			add(tag)
		}
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok {
				add(s)
			}
		}
	}

	for _, m := range inlineTag.FindAllStringSubmatch(stripCodeFences(body), -1) {
		add(m[2])
	}
	return tags
}

// stripCodeFences blanks out fenced code blocks so that a shell comment or
// a CSS colour inside one is not read as a tag.
func stripCodeFences(body string) string {
	var out strings.Builder
	inFence := false
	for line := range strings.SplitSeq(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		if !inFence {
			out.WriteString(line)
			out.WriteByte('\n')
		}
	}
	return out.String()
}

// UpdateFrontmatter applies set and remove to a note's frontmatter and
// returns the whole note. Fields that are not mentioned keep their value,
// their order and their formatting, and the body is untouched. A note with
// no block gets one when set is non-empty; removing every field drops the
// block again.
func UpdateFrontmatter(data []byte, set map[string]any, remove []string) ([]byte, error) {
	block, body, hadBlock := splitFrontmatter(string(data))

	var doc yaml.Node
	if strings.TrimSpace(block) != "" {
		if err := yaml.Unmarshal([]byte(block), &doc); err != nil {
			return nil, fmt.Errorf("parsing frontmatter: %w", err)
		}
	}
	mapping := mappingNode(&doc)

	for _, key := range remove {
		deleteKey(mapping, key)
	}
	for key, value := range sortedKeys(set) {
		var valueNode yaml.Node
		if err := valueNode.Encode(value); err != nil {
			return nil, fmt.Errorf("encoding %q: %w", key, err)
		}
		if s, ok := value.(string); ok && isDate(&yaml.Node{Kind: yaml.ScalarNode, Value: s}) {
			valueNode = yaml.Node{Kind: yaml.ScalarNode, Tag: "!!timestamp", Value: s}
		}
		setKey(mapping, key, &valueNode)
	}

	if len(mapping.Content) == 0 {
		if !hadBlock {
			return data, nil
		}
		return []byte(body), nil
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(mapping); err != nil {
		return nil, fmt.Errorf("writing frontmatter: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("writing frontmatter: %w", err)
	}
	return []byte(fence + "\n" + buf.String() + fence + "\n" + body), nil
}

func mappingNode(doc *yaml.Node) *yaml.Node {
	if len(doc.Content) == 1 && doc.Content[0].Kind == yaml.MappingNode {
		return doc.Content[0]
	}
	return &yaml.Node{Kind: yaml.MappingNode}
}

func deleteKey(mapping *yaml.Node, key string) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
			return
		}
	}
}

func setKey(mapping *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content[i+1] = value
			return
		}
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: key},
		value)
}

// sortedKeys yields set's entries in a stable order, so that adding several
// new fields at once produces the same file every time.
func sortedKeys(set map[string]any) func(func(string, any) bool) {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return func(yield func(string, any) bool) {
		for _, k := range keys {
			if !yield(k, set[k]) {
				return
			}
		}
	}
}
