// Package search runs full-text searches over a vault directory using
// ripgrep, parsing its JSON event stream into structured matches.
package search

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// DefaultMaxResults caps the notes returned when the caller does not
// specify a limit.
const DefaultMaxResults = 50

// MaxResultsCeiling is the hard upper bound on notes per search.
const MaxResultsCeiling = 500

// DefaultLinesPerNote caps how many lines one note contributes, so a long
// note cannot fill a whole result. A negative MaxLinesPerFile lifts it.
const DefaultLinesPerNote = 5

// Mode selects how a query's text is read.
type Mode string

const (
	// ModeAuto reads a query holding regular-expression characters as a
	// regex, and anything else as a set of words.
	ModeAuto Mode = ""
	// ModeWords requires every word of the query, in any order, ignoring
	// regular-expression characters.
	ModeWords Mode = "words"
	// ModeRegex reads the whole query as one regular expression.
	ModeRegex Mode = "regex"
)

// regexChars are the characters that make a query a regular expression
// under ModeAuto. A person typing two plain words means both words; a
// person typing "^tags:" means an anchored pattern.
const regexChars = `^$\.[]()|*+?{}`

// Options configures a single search.
type Options struct {
	// Query is the text to search for: words, or a regular expression in
	// ripgrep syntax, as Mode decides.
	Query string
	// Mode selects how Query is read. The zero value is ModeAuto.
	Mode Mode
	// MaxLinesPerFile caps the lines returned for any one note. Zero means
	// DefaultLinesPerNote, and a negative value returns every line.
	MaxLinesPerFile int
	// Glob optionally restricts the search to matching paths,
	// e.g. "*.md" or "daily/**".
	Glob string
	// CaseSensitive disables the default case-insensitive matching.
	CaseSensitive bool
	// ContextLines is the number of lines of context to include around
	// each match.
	ContextLines int
	// MaxResults caps the number of notes returned; zero means
	// DefaultMaxResults.
	MaxResults int
}

// Line is a single line of search output.
type Line struct {
	// Number is the 1-based line number within the file.
	Number int `json:"line"`
	// Text is the line's content, without a trailing newline.
	Text string `json:"text"`
	// Match reports whether the line matched the query (false for
	// context lines).
	Match bool `json:"match"`
}

// FileMatches groups the matching lines of one file.
type FileMatches struct {
	// Path is the vault-relative path of the file.
	Path string `json:"path"`
	// Lines are the matching and context lines, in file order, capped by
	// MaxLinesPerFile. It is empty when only the note's name matched.
	Lines []Line `json:"lines"`
	// TotalMatches counts this note's matching lines, including any the
	// line cap left out.
	TotalMatches int `json:"total_matches"`
	// TitleMatch reports that the note's name matched the query.
	TitleMatch bool `json:"title_match,omitempty"`
}

// Result is the outcome of a search.
type Result struct {
	// Files lists each note with matches.
	Files []FileMatches `json:"files"`
	// TotalMatches counts matching lines across every note returned.
	TotalMatches int `json:"total_matches"`
	// Truncated reports whether notes were left out by MaxResults.
	Truncated bool `json:"truncated"`
}

// RunFunc executes a command in dir and returns its stdout, stderr, and
// exit code. It only returns an error when the command could not be run at
// all.
type RunFunc func(ctx context.Context, dir, name string, args ...string) (stdout, stderr []byte, exitCode int, err error)

// Searcher runs ripgrep searches rooted at vault directories.
type Searcher struct {
	binary string
	run    RunFunc
}

// New returns a Searcher that executes ripgrep as binary (typically "rg")
// via run. Passing nil for run uses os/exec.
func New(binary string, run RunFunc) *Searcher {
	if run == nil {
		run = execRun
	}
	return &Searcher{binary: binary, run: run}
}

func execRun(ctx context.Context, dir, name string, args ...string) ([]byte, []byte, int, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return stdout.Bytes(), stderr.Bytes(), exitErr.ExitCode(), nil
	}
	if err != nil {
		return nil, nil, 0, err
	}
	return stdout.Bytes(), stderr.Bytes(), 0, nil
}

// Search runs the query over the vault rooted at root. Hidden directories
// (.obsidian, .trash, ...) are excluded because ripgrep skips hidden files
// by default; --no-ignore prevents any stray ignore files in the vault from
// silently hiding notes.
func (s *Searcher) Search(ctx context.Context, root string, opts Options) (*Result, error) {
	if strings.TrimSpace(opts.Query) == "" {
		return nil, errors.New("query must not be empty")
	}
	maxResults := opts.MaxResults
	if maxResults <= 0 {
		maxResults = DefaultMaxResults
	}
	maxResults = min(maxResults, MaxResultsCeiling)
	maxLines := opts.MaxLinesPerFile
	if maxLines == 0 {
		maxLines = DefaultLinesPerNote
	}

	args := []string{"--json", "--no-ignore", "--sort", "path"}
	if !opts.CaseSensitive {
		args = append(args, "--ignore-case")
	}
	if opts.ContextLines > 0 {
		args = append(args, "--context", fmt.Sprint(opts.ContextLines))
	}
	if opts.Glob != "" {
		args = append(args, "--glob", opts.Glob)
	}
	words := QueryWords(opts)
	if len(words) > 0 {
		// One pass finds any word; notes missing a word are dropped below.
		quoted := make([]string, len(words))
		for i, w := range words {
			quoted[i] = regexp.QuoteMeta(w)
		}
		args = append(args, "--regexp", strings.Join(quoted, "|"))
	} else {
		args = append(args, "--regexp", opts.Query)
	}
	args = append(args, "--", ".")

	stdout, stderr, exitCode, err := s.run(ctx, root, s.binary, args...)
	if err != nil {
		return nil, fmt.Errorf("running %s: %w", s.binary, err)
	}
	// ripgrep exits 0 on matches, 1 on no matches, 2 on error.
	if exitCode > 1 {
		return nil, fmt.Errorf("search failed: %s", strings.TrimSpace(string(stderr)))
	}
	res, err := parseJSONEvents(stdout, maxResults, maxLines)
	if err != nil {
		return nil, err
	}
	if len(words) > 0 {
		keepNotesHoldingEveryWord(res, words)
	}
	return res, nil
}

// QueryWords returns the words a query asks for, or nothing when the query
// is a regular expression. Callers use it to rank results and to match
// note names the same way the content search did.
func QueryWords(opts Options) []string {
	switch opts.Mode {
	case ModeRegex:
		return nil
	case ModeWords:
		return strings.Fields(opts.Query)
	}
	words := strings.Fields(opts.Query)
	if len(words) < 2 || strings.ContainsAny(opts.Query, regexChars) {
		return nil
	}
	return words
}

// keepNotesHoldingEveryWord drops notes whose matched lines do not cover
// every word, which is what makes a multi-word query mean "all of these".
func keepNotesHoldingEveryWord(res *Result, words []string) {
	kept := make([]FileMatches, 0, len(res.Files))
	total := 0
	for _, f := range res.Files {
		var text strings.Builder
		for _, l := range f.Lines {
			if l.Match {
				text.WriteString(strings.ToLower(l.Text))
				text.WriteByte('\n')
			}
		}
		body := text.String()
		holdsAll := true
		for _, w := range words {
			if !strings.Contains(body, strings.ToLower(w)) {
				holdsAll = false
				break
			}
		}
		if !holdsAll {
			continue
		}
		kept = append(kept, f)
		for _, l := range f.Lines {
			if l.Match {
				total++
			}
		}
	}
	res.Files = kept
	res.TotalMatches = total
}

// event is the subset of ripgrep's --json output the parser consumes.
type event struct {
	Type string `json:"type"`
	Data struct {
		Path struct {
			Text string `json:"text"`
		} `json:"path"`
		Lines struct {
			Text string `json:"text"`
		} `json:"lines"`
		LineNumber int `json:"line_number"`
	} `json:"data"`
}

func parseJSONEvents(out []byte, maxResults, maxLinesPerFile int) (*Result, error) {
	// Files starts non-nil so a zero-match result marshals as "files": []
	// rather than "files": null.
	res := &Result{Files: []FileMatches{}}
	var current *FileMatches
	scanner := bufio.NewScanner(bytes.NewReader(out))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var ev event
		if err := json.Unmarshal(line, &ev); err != nil {
			return nil, fmt.Errorf("parsing search output: %w", err)
		}
		if ev.Type != "match" && ev.Type != "context" {
			continue
		}
		path := strings.TrimPrefix(ev.Data.Path.Text, "./")
		if current == nil || current.Path != path {
			// The budget counts notes, so a long note cannot crowd the
			// others out of the result.
			if len(res.Files) == maxResults {
				res.Truncated = true
				break
			}
			res.Files = append(res.Files, FileMatches{Path: path})
			current = &res.Files[len(res.Files)-1]
		}
		if ev.Type == "match" {
			current.TotalMatches++
			res.TotalMatches++
		}
		if maxLinesPerFile > 0 && len(current.Lines) >= maxLinesPerFile {
			continue
		}
		current.Lines = append(current.Lines, Line{
			Number: ev.Data.LineNumber,
			Text:   strings.TrimRight(ev.Data.Lines.Text, "\n"),
			Match:  ev.Type == "match",
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning search output: %w", err)
	}
	return res, nil
}
