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
			path = slices.Clone(result[stack[len(stack)-1]].path)
		}
		path = append(path, title)
		result = append(result, section{path: path, level: h.Level, start: base + start.(int), body: base + body.(int), end: len(source)})
		stack = append(stack, len(result)-1)
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
	body, err := page(string(data[s.body:s.end]), offset, "section")
	if err != nil {
		return nil, err
	}
	return &SectionResult{HeadingPath: s.path, Level: s.level, ReadResult: body}, nil
}

// ReplaceSection replaces a body and its subsections, preserving the selected
// heading and all bytes outside the body. Content excludes the selected heading.
func (v *Vault) ReplaceSection(rel string, headingPath []string, content string) error {
	_, err := v.Update(rel, func(data []byte) ([]byte, error) {
		s, err := selectSection(data, headingPath)
		if err != nil {
			return nil, err
		}
		body := content
		// Keep subsequent headings on their own line. Preserve the note's newline style.
		newline := "\n"
		if bytes.Contains(data[s.start:s.body], []byte("\r\n")) {
			newline = "\r\n"
		}
		prefix := string(data[:s.body])
		if body != "" && !strings.HasSuffix(prefix, "\n") {
			prefix += newline
		}
		if s.end < len(data) && body != "" {
			if !strings.HasSuffix(body, "\n") {
				body += newline
			}
			// A setext title must not merge into the replacement's last paragraph.
			if s.nextSetext && !strings.HasSuffix(body, newline+newline) {
				body += newline
			}
		}
		return []byte(prefix + body + string(data[s.end:])), nil
	})
	return err
}
