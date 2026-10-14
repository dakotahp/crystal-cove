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

type searchNotesInput struct {
	Vault           string `json:"vault,omitempty" jsonschema:"name of the vault to search; optional when the server holds one vault"`
	Query           string `json:"query" jsonschema:"words to look for, an exact phrase, or a regular expression in ripgrep syntax"`
	Mode            string `json:"mode,omitempty" jsonschema:"how to read the query: words (every word, any order), phrase (the exact text, punctuation included, no escaping), regex, or auto (the default: phrase when the query is wrapped in double quotes, regex when it holds regex characters, words otherwise)"`
	MaxLinesPerNote int    `json:"max_lines_per_note,omitempty" jsonschema:"lines to return per note (default 5); use -1 for every matching line"`
	Glob            string `json:"glob,omitempty" jsonschema:"restrict the search, name matches included, to note paths matching this glob, e.g. daily/**"`
	CaseSensitive   bool   `json:"case_sensitive,omitempty" jsonschema:"match case exactly instead of the default case-insensitive search"`
	ContextLines    int    `json:"context_lines,omitempty" jsonschema:"lines of context to include around each match"`
	MaxResults      int    `json:"max_results,omitempty" jsonschema:"maximum notes to return (default 50, max 500)"`
	Output          string `json:"output,omitempty" jsonschema:"lines (the default) returns matching lines per note; files returns only each note's path, total_matches and title_match, to choose which note to read"`
}

const (
	outputLines = "lines"
	outputFiles = "files"
)

func (s *Server) searchNotes(ctx context.Context, _ *mcp.CallToolRequest, in searchNotesInput) (*mcp.CallToolResult, *search.Result, error) {
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
	limit := in.MaxResults
	if limit <= 0 {
		limit = search.DefaultMaxResults
	}
	titles, truncated, err := s.titlesInGlob(ctx, v, in.Glob, titleMatcher(opts), limit)
	if err != nil {
		return nil, nil, err
	}
	res = withTitleMatches(res, titles, truncated)
	rankFiles(res)
	res.Limit(limit)
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
func titleMatcher(opts search.Options) func(v *vault.Vault, limit int) ([]string, bool, error) {
	if words := search.QueryWords(opts); len(words) > 0 {
		return func(v *vault.Vault, limit int) ([]string, bool, error) {
			return v.MatchTitleWords(words, opts.CaseSensitive, limit)
		}
	}
	pattern := opts.Query
	if phrase, ok := search.QueryPhrase(opts); ok {
		pattern = regexp.QuoteMeta(phrase)
	}
	return func(v *vault.Vault, limit int) ([]string, bool, error) {
		return v.MatchTitles(pattern, opts.CaseSensitive, limit)
	}
}

// titlesInGlob finds the notes named after the query that the glob, when
// given, also selects, cutting them to limit only after the glob filter.
func (s *Server) titlesInGlob(ctx context.Context, v *vault.Vault, glob string, match func(*vault.Vault, int) ([]string, bool, error), limit int) ([]string, bool, error) {
	if glob == "" {
		return match(v, limit)
	}
	titles, _, err := match(v, 0)
	if err != nil {
		return nil, false, err
	}
	inGlob, err := s.searcher.Files(ctx, v.Root(), glob)
	if err != nil {
		return nil, false, err
	}
	kept := slices.DeleteFunc(titles, func(p string) bool { return !inGlob[p] })
	if len(kept) > limit {
		return kept[:limit], true, nil
	}
	return kept, false, nil
}

// withTitleMatches moves the notes whose name matched to the front of the
// result, keeping any body matches they also have. A note matched only by
// name is added with no lines.
func withTitleMatches(res *search.Result, titles []string, truncated bool) *search.Result {
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
	res.Truncated = res.Truncated || truncated
	return res
}
