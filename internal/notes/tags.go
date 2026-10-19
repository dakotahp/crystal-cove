package notes

import (
	"errors"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// ChangeTags adds tags to the note's frontmatter tags field and removes
// others, and returns the whole note. Tags compare without case and
// without a leading "#", as Obsidian compares them; an added tag the note
// already has is skipped, and new tags go at the end. A list keeps its
// style, flow or block. A tags field written as a string, such as
// "a, b", becomes a block list, which is how Obsidian rewrites it, and a
// missing field becomes a block list. Removing the last tag drops the
// field. Inline hashtags in the body are left alone. A note that ends up
// unchanged comes back byte for byte.
func ChangeTags(data []byte, add, remove []string) ([]byte, error) {
	fm, err := readFrontmatter(data)
	if err != nil {
		return nil, err
	}
	node := valueOf(fm.mapping, "tags")
	tags, err := tagItems(node)
	if err != nil {
		return nil, err
	}

	changed := false
	kept := tags[:0:0]
	for _, item := range tags {
		if containsTag(remove, item.Value) {
			changed = true
			continue
		}
		kept = append(kept, item)
	}
	for _, tag := range add {
		tag = cleanTag(tag)
		if tag == "" || slices.ContainsFunc(kept, func(n *yaml.Node) bool { return sameTag(n.Value, tag) }) {
			continue
		}
		kept = append(kept, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: tag})
		changed = true
	}
	if !changed {
		return data, nil
	}

	if len(kept) == 0 {
		deleteKey(fm.mapping, "tags")
		return fm.write()
	}
	list := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	if node != nil && node.Kind == yaml.SequenceNode {
		list.Style = node.Style
	}
	list.Content = kept
	setKey(fm.mapping, "tags", list)
	return fm.write()
}

func valueOf(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// tagItems returns the tags field's entries as scalar nodes, splitting a
// string the way Parse reads one.
func tagItems(node *yaml.Node) ([]*yaml.Node, error) {
	switch {
	case node == nil || node.ShortTag() == "!!null":
		return nil, nil
	case node.Kind == yaml.SequenceNode:
		return node.Content, nil
	case node.Kind == yaml.ScalarNode:
		var items []*yaml.Node
		for _, tag := range strings.FieldsFunc(node.Value, func(r rune) bool { return r == ',' || r == ' ' }) {
			if tag = cleanTag(tag); tag != "" {
				items = append(items, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: tag})
			}
		}
		return items, nil
	}
	return nil, errors.New("the tags field is neither a list nor text, so it cannot take tags")
}

func cleanTag(tag string) string {
	return strings.TrimPrefix(strings.TrimSpace(tag), "#")
}

func sameTag(a, b string) bool {
	return strings.EqualFold(cleanTag(a), cleanTag(b))
}

func containsTag(tags []string, tag string) bool {
	return slices.ContainsFunc(tags, func(t string) bool { return cleanTag(t) != "" && sameTag(t, tag) })
}
