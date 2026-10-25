package server

import (
	"context"
	"fmt"
	"regexp"
	"slices"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dakotahp/crystal-cove/internal/search"
	"github.com/dakotahp/crystal-cove/internal/vault"
)

type grepNotesInput struct {
	Vault           string `json:"vault,omitempty" jsonschema:"name of the vault to search; optional when the server holds one vault"`
	Query           string `json:"query" jsonschema:"words to look for, an exact phrase, or a regular expression in ripgrep syntax"`
	Mode            string `json:"mode,omitempty" jsonschema:"how to read the query: words (every word, any order); phrase (the exact text, case-insensitive, with no escaping: use it to find a line before editing it, or for text with punctuation, quotes, dashes or emoji); regex (ripgrep syntax); or auto (the default: phrase when the query is wrapped in double quotes, regex when it holds regex characters, words otherwise)"`
	MaxLinesPerNote int    `json:"max_lines_per_note,omitempty" jsonschema:"lines to return per note; 0 or leaving it out means 5, and -1 means every matching line. For no lines at all, use output files"`
	Glob            string `json:"glob,omitempty" jsonschema:"restrict the search, name matches included, to note paths matching this glob, e.g. daily/**"`
	CaseSensitive   bool   `json:"case_sensitive,omitempty" jsonschema:"match case exactly instead of the default case-insensitive search"`
	ContextLines    int    `json:"context_lines,omitempty" jsonschema:"lines of context to include around each match"`
	Offset          int    `json:"offset,omitempty" jsonschema:"note to start from; use next_offset from the previous page"`
	Limit           int    `json:"limit,omitempty" jsonschema:"maximum notes to return (default 50, max 500)"`
	Output          string `json:"output,omitempty" jsonschema:"lines (the default) returns matching lines per note; files returns only each note's path, total_matches and title_match, to choose which note to read"`
}

const (
	outputLines = "lines"
	outputFiles = "files"
)

func (s *Server) grepNotes(ctx context.Context, _ *mcp.CallToolRequest, in grepNotesInput) (*mcp.CallToolResult, *search.Result, error) {
	v, err := s.vault(in.Vault)
	if err != nil {
		return nil, nil, err
	}
	mode, err := searchMode(in.Mode)
	if err != nil {
		return nil, nil, err
	}
	if in.Output != "" && in.Output != outputLines && in.Output != outputFiles {
		return nil, nil, fmt.Errorf("output %q is not one of lines or files", in.Output)
	}
	opts := search.Options{
		Query:           in.Query,
		Mode:            mode,
		MaxLinesPerFile: in.MaxLinesPerNote,
		Glob:            in.Glob,
		CaseSensitive:   in.CaseSensitive,
		ContextLines:    in.ContextLines,
	}
	res, err := s.searcher.Search(ctx, v.Root(), opts)
	if err != nil {
		return nil, nil, err
	}
	titles, err := s.titlesInGlob(ctx, v, in.Glob, titleMatcher(opts))
	if err != nil {
		return nil, nil, err
	}
	res = withTitleMatches(res, titles)
	rankFiles(res)
	if err := res.Page(in.Offset, in.Limit); err != nil {
		return nil, nil, err
	}
	if in.Output == outputFiles {
		for i := range res.Files {
			res.Files[i].Lines = []search.Line{}
		}
	}
	return nil, res, nil
}

// searchMode converts the tool's mode argument, naming the choices when it
// is not one of them.
func searchMode(mode string) (search.Mode, error) {
	if mode == "auto" {
		return search.ModeAuto, nil
	}
	switch search.Mode(mode) {
	case search.ModeAuto, search.ModeWords, search.ModePhrase, search.ModeRegex:
		return search.Mode(mode), nil
	default:
		return "", fmt.Errorf("mode %q is not one of auto, words, phrase or regex", mode)
	}
}

// titleMatcher finds notes by name the same way the content search read the
// query: every word in any order, the literal phrase, or the raw pattern for
// a regex query.
func titleMatcher(opts search.Options) func(v *vault.Vault) ([]string, error) {
	if words := search.QueryWords(opts); len(words) > 0 {
		return func(v *vault.Vault) ([]string, error) {
			return v.MatchTitleWords(words, opts.CaseSensitive)
		}
	}
	pattern := opts.Query
	if phrase, ok := search.QueryPhrase(opts); ok {
		pattern = regexp.QuoteMeta(phrase)
	}
	return func(v *vault.Vault) ([]string, error) {
		return v.MatchTitles(pattern, opts.CaseSensitive)
	}
}

// titlesInGlob finds every note named after the query that the glob, when
// given, also selects. It cuts nothing, so each page of a search ranks the
// same full list.
func (s *Server) titlesInGlob(ctx context.Context, v *vault.Vault, glob string, match func(*vault.Vault) ([]string, error)) ([]string, error) {
	titles, err := match(v)
	if err != nil || glob == "" {
		return titles, err
	}
	inGlob, err := s.searcher.Files(ctx, v.Root(), glob)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(titles, func(p string) bool { return !inGlob[p] }), nil
}

// withTitleMatches moves the notes whose name matched to the front of the
// result, keeping any body matches they also have. A note matched only by
// name is added with no lines.
func withTitleMatches(res *search.Result, titles []string) *search.Result {
	if len(titles) == 0 {
		return res
	}
	byPath := make(map[string]search.FileMatches, len(res.Files))
	for _, f := range res.Files {
		byPath[f.Path] = f
	}
	files := make([]search.FileMatches, 0, len(res.Files)+len(titles))
	for _, path := range titles {
		f, ok := byPath[path]
		if !ok {
			f = search.FileMatches{Path: path, Lines: []search.Line{}}
		}
		f.TitleMatch = true
		files = append(files, f)
		delete(byPath, path)
	}
	for _, f := range res.Files {
		if _, ok := byPath[f.Path]; ok {
			files = append(files, f)
		}
	}
	res.Files = files
	return res
}
