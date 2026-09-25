package vault

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// SectionResult describes a heading and a page of its body, including subsections.
// Offsets are Unicode character offsets relative to the body, not the file.
type SectionResult struct {
	HeadingPath []string `json:"heading_path" jsonschema:"full heading hierarchy of the selected section"`
	Level       int      `json:"level" jsonschema:"heading level from 1 to 6"`
	*ReadResult
}

type section struct {
	path                    []string
	level, start, body, end int
	nextSetext              bool
	// ownEnd is where the section's own text ends: at its first subheading,
	// or at end when it has none. ownEndSetext reports that the heading
	// found there is a setext one.
	ownEnd       int
	ownEndSetext bool
	hasChild     bool
}

// positionedHeading preserves source boundaries even for empty ATX headings.
// Setext heading Open runs on the underline; Close supplies its title lines.
type positionedHeading struct{ parser.BlockParser }

func (p positionedHeading) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	_, pos := reader.PeekLine()
	n, state := p.BlockParser.Open(parent, reader, pc)
	if n != nil {
		n.SetAttributeString("section_start", pos.Start)
		n.SetAttributeString("section_body", pos.Stop)
		n.SetAttributeString("section_setext", slices.Equal(p.Trigger(), []byte{'-', '='}))
	}
	return n, state
}
func (p positionedHeading) Close(n ast.Node, reader text.Reader, pc parser.Context) {
	p.BlockParser.Close(n, reader, pc)
	if n.Lines().Len() > 0 {
		start, _ := n.AttributeString("section_start")
		if first := n.Lines().At(0).Start; first < start.(int) {
			n.SetAttributeString("section_start", bytes.LastIndexByte(reader.Source()[:first], '\n')+1)
		}
	}
}

func markdownSections(source []byte) []section {
	// Parse after YAML frontmatter without changing the original file's offsets.
	base := 0
	lines := bytes.SplitAfter(source, []byte("\n"))
	if len(lines) > 0 && strings.TrimSpace(string(lines[0])) == "---" {
		base = len(lines[0])
		for _, line := range lines[1:] {
			base += len(line)
			marker := strings.TrimSpace(string(line))
			if marker == "---" || marker == "..." {
				break
			}
		}
	}
	blocks := parser.DefaultBlockParsers()
	for i, b := range blocks {
		// The standard heading parsers have distinctive trigger sets.
		bp := b.Value.(parser.BlockParser)
		triggers := bp.Trigger()
		if slices.Equal(triggers, []byte{'#'}) || slices.Equal(triggers, []byte{'-', '='}) {
			blocks[i] = util.Prioritized(positionedHeading{bp}, b.Priority)
		}
	}
	doc := parser.NewParser(parser.WithBlockParsers(blocks...), parser.WithInlineParsers(parser.DefaultInlineParsers()...)).Parse(text.NewReader(source[base:]))
	result := []section{}
	stack := []int{}
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		h, ok := n.(*ast.Heading)
		if !ok {
			continue
		}
		start, _ := h.AttributeString("section_start")
		body, _ := h.AttributeString("section_body")
		setext, _ := h.AttributeString("section_setext")
		title := strings.TrimSpace(string(h.Lines().Value(source[base:])))
		for len(stack) > 0 && result[stack[len(stack)-1]].level >= h.Level {
			result[stack[len(stack)-1]].end = base + start.(int)
			result[stack[len(stack)-1]].nextSetext = setext.(bool)
			stack = stack[:len(stack)-1]
		}
		path := []string{}
		if len(stack) > 0 {
			parent := &result[stack[len(stack)-1]]
			path = slices.Clone(parent.path)
			if !parent.hasChild {
				parent.hasChild = true
				parent.ownEnd = base + start.(int)
				parent.ownEndSetext = setext.(bool)
			}
		}
		path = append(path, title)
		result = append(result, section{path: path, level: h.Level, start: base + start.(int), body: base + body.(int), end: len(source)})
		stack = append(stack, len(result)-1)
	}
	for i := range result {
		if !result[i].hasChild {
			result[i].ownEnd = result[i].end
			result[i].ownEndSetext = result[i].nextSetext
		}
	}
	return result
}

func selectSection(source []byte, headingPath []string) (section, error) {
	if len(headingPath) == 0 {
		return section{}, errors.New("heading_path must contain at least one heading title")
	}
	matches := []section{}
	for _, s := range markdownSections(source) {
		if len(s.path) >= len(headingPath) && slices.Equal(s.path[len(s.path)-len(headingPath):], headingPath) {
			matches = append(matches, s)
		}
	}
	if len(matches) == 0 {
		return section{}, fmt.Errorf("section %q not found", headingPath)
	}
	if len(matches) > 1 {
		return section{}, fmt.Errorf("section %q is ambiguous (%d matches): provide more ancestor headings in heading_path", headingPath, len(matches))
	}
	return matches[0], nil
}

func (v *Vault) loadSection(rel string, headingPath []string) ([]byte, section, error) {
	data, err := v.ReadAll(rel)
	if err != nil {
		return nil, section{}, err
	}
	s, err := selectSection(data, headingPath)
	return data, s, err
}

// GetSection reads the body of a uniquely selected heading. A heading path is
// an exact, case-sensitive suffix of the full hierarchy, using Markdown titles.
func (v *Vault) GetSection(rel string, headingPath []string, offset int) (*SectionResult, error) {
	data, s, err := v.loadSection(rel, headingPath)
	if err != nil {
		return nil, err
	}
	body, err := page(data[s.body:s.end], offset, "section")
	if err != nil {
		return nil, err
	}
	return &SectionResult{HeadingPath: s.path, Level: s.level, ReadResult: body}, nil
}

// SectionMode says how EditSection changes a section.
type SectionMode string

const (
	// SectionAppend adds content after the section's own text, before its
	// first subheading.
	SectionAppend SectionMode = "append"
	// SectionPrepend adds content right below the heading, after any blank
	// lines there.
	SectionPrepend SectionMode = "prepend"
	// SectionReplace replaces the whole body, subsections included.
	SectionReplace SectionMode = "replace"
)

// EditSection changes the body of a uniquely selected heading and returns
// the section's new version, which is empty when the edit leaves the heading
// path ambiguous. The heading and every byte outside the section are kept.
//
// version is the one GetSection returned for this section; the edit is
// refused when the section has changed since. It is required for
// SectionReplace, which would otherwise remove text its caller never saw,
// and optional for the modes that only add text. A change elsewhere in the
// note does not count.
func (v *Vault) EditSection(rel string, headingPath []string, mode SectionMode, content, version string) (string, error) {
	switch mode {
	case SectionReplace:
		if version == "" {
			return "", errors.New("replace needs the version get_section returned for this section, so it cannot remove text it has not seen")
		}
	case SectionAppend, SectionPrepend:
		if content == "" {
			return "", fmt.Errorf("%s needs content to add", mode)
		}
	default:
		return "", fmt.Errorf("mode %q is not one of append, prepend or replace", mode)
	}
	updated, err := v.Update(rel, func(data []byte) ([]byte, error) {
		s, err := selectSection(data, headingPath)
		if err != nil {
			return nil, err
		}
		if err := checkVersion(fmt.Sprintf("section %q", headingPath), data[s.body:s.end], version); err != nil {
			return nil, err
		}
		if mode == SectionReplace {
			return replaceBody(data, s, content), nil
		}
		return insertIntoSection(data, s, mode, content), nil
	})
	if err != nil {
		return "", err
	}
	s, err := selectSection(updated, headingPath)
	if err != nil {
		return "", nil
	}
	return Version(updated[s.body:s.end]), nil
}

// lineEnding returns the line ending a heading line uses.
func lineEnding(heading []byte) string {
	if bytes.Contains(heading, []byte("\r\n")) {
		return "\r\n"
	}
	return "\n"
}

// replaceBody puts content in place of the section's body, keeping any
// following heading on its own line.
func replaceBody(data []byte, s section, content string) []byte {
	newline := lineEnding(data[s.start:s.body])
	prefix := string(data[:s.body])
	if content != "" && !strings.HasSuffix(prefix, "\n") {
		prefix += newline
	}
	if s.end < len(data) && content != "" {
		if !strings.HasSuffix(content, "\n") {
			content += newline
		}
		// A setext title must not merge into the replacement's last paragraph.
		if s.nextSetext && !strings.HasSuffix(content, newline+newline) {
			content += newline
		}
	}
	return []byte(prefix + content + string(data[s.end:]))
}

// insertIntoSection adds content as whole lines inside the section's own
// text: after its last line of text for SectionAppend, before its first for
// SectionPrepend.
func insertIntoSection(data []byte, s section, mode SectionMode, content string) []byte {
	newline := lineEnding(data[s.start:s.body])
	own := data[s.body:s.ownEnd]
	at := s.body + firstTextLine(own)
	if mode == SectionAppend {
		at = s.body + afterLastTextLine(own)
	}
	insert := content
	if !strings.HasSuffix(insert, "\n") {
		insert += newline
	}
	if at > 0 && data[at-1] != '\n' {
		insert = newline + insert
	}
	// A setext title must not merge into the inserted paragraph.
	if at == s.ownEnd && s.ownEndSetext && !strings.HasSuffix(insert, newline+newline) {
		insert += newline
	}
	return slices.Concat(data[:at], []byte(insert), data[at:])
}

func isSpace(r rune) bool { return r == ' ' || r == '\t' || r == '\r' || r == '\n' }

// firstTextLine returns the offset of the first line in text that is not
// blank, or the end of text when every line is.
func firstTextLine(text []byte) int {
	i := bytes.IndexFunc(text, func(r rune) bool { return !isSpace(r) })
	if i < 0 {
		return len(text)
	}
	return bytes.LastIndexByte(text[:i], '\n') + 1
}

// afterLastTextLine returns the offset just past the last line in text that
// is not blank, or zero when every line is.
func afterLastTextLine(text []byte) int {
	i := bytes.LastIndexFunc(text, func(r rune) bool { return !isSpace(r) })
	if i < 0 {
		return 0
	}
	if j := bytes.IndexByte(text[i:], '\n'); j >= 0 {
		return i + j + 1
	}
	return len(text)
}
