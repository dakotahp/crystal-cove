package server

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dakotahp/crystal-cove/internal/daily"
	"github.com/dakotahp/crystal-cove/internal/vault"
)

type dailyNoteInput struct {
	Vault string `json:"vault,omitempty" jsonschema:"name of the vault; optional when the server holds one vault"`
	Date  string `json:"date,omitempty" jsonschema:"which day's note: YYYY-MM-DD, today, yesterday or tomorrow; defaults to today in the server's time zone"`
}

type dailyNotePathOutput struct {
	Path   string `json:"path" jsonschema:"vault-relative path of the daily note"`
	Exists bool   `json:"exists" jsonschema:"whether the note has been created yet"`
}

type readDailyNoteInput struct {
	Vault  string `json:"vault,omitempty" jsonschema:"name of the vault; optional when the server holds one vault"`
	Date   string `json:"date,omitempty" jsonschema:"which day's note: YYYY-MM-DD, today, yesterday or tomorrow; defaults to today in the server's time zone"`
	Offset int    `json:"offset,omitempty" jsonschema:"character offset to start reading from; use next_offset from a previous truncated read"`
}

type readDailyNoteOutput struct {
	Path   string `json:"path" jsonschema:"vault-relative path of the daily note"`
	Exists bool   `json:"exists" jsonschema:"false when the note is not created yet; append_daily_note or prepend_daily_note creates it"`
	*vault.ReadResult
}

type writeDailyNoteInput struct {
	Vault   string `json:"vault,omitempty" jsonschema:"name of the vault; optional when the server holds one vault"`
	Date    string `json:"date,omitempty" jsonschema:"which day's note: YYYY-MM-DD, today, yesterday or tomorrow; defaults to today in the server's time zone"`
	Content string `json:"content" jsonschema:"markdown content"`
	Inline  bool   `json:"inline,omitempty" jsonschema:"join content to the line it lands next to instead of giving it its own line"`
}

type writeDailyNoteOutput struct {
	OK      bool   `json:"ok"`
	Path    string `json:"path" jsonschema:"vault-relative path of the daily note"`
	Created bool   `json:"created" jsonschema:"whether this call created the note, from the daily note template when one is set"`
}

// dailyNote is one day's note as Obsidian's Daily notes plugin names it.
type dailyNote struct {
	v        *vault.Vault
	settings daily.Settings
	day      time.Time
	path     string
}

func (s *Server) dailyNote(vaultName, date string, check func(string) error) (*dailyNote, error) {
	v, err := s.vault(vaultName)
	if err != nil {
		return nil, err
	}
	data, err := v.ReadAll(daily.SettingsFile)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	settings, err := daily.ParseSettings(data)
	if err != nil {
		return nil, err
	}
	day, err := daily.Day(date, s.now(), s.timezone)
	if err != nil {
		return nil, err
	}
	p, err := settings.Path(day)
	if err != nil {
		return nil, err
	}
	if err := check(p); err != nil {
		return nil, err
	}
	return &dailyNote{v: v, settings: settings, day: day, path: p}, nil
}

// initial is a new note's text: the template with its variables filled in,
// or nothing when no template is set.
func (n *dailyNote) initial() ([]byte, error) {
	tp := n.settings.TemplatePath()
	if tp == "" {
		return nil, nil
	}
	tmpl, err := n.v.ReadAll(tp)
	if err != nil {
		return nil, fmt.Errorf("reading the daily note template %q set in Obsidian's Daily notes settings: %w", tp, err)
	}
	text, err := daily.RenderTemplate(string(tmpl), strings.TrimSuffix(path.Base(n.path), path.Ext(n.path)), n.day)
	if err != nil {
		return nil, fmt.Errorf("filling in the daily note template %q: %w", tp, err)
	}
	return []byte(text), nil
}

func (s *Server) dailyNotePath(_ context.Context, _ *mcp.CallToolRequest, in dailyNoteInput) (*mcp.CallToolResult, dailyNotePathOutput, error) {
	n, err := s.dailyNote(in.Vault, in.Date, requireNote)
	if err != nil {
		return nil, dailyNotePathOutput{}, err
	}
	_, err = n.v.ReadAll(n.path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, dailyNotePathOutput{}, err
	}
	return nil, dailyNotePathOutput{Path: n.path, Exists: err == nil}, nil
}

func (s *Server) readDailyNote(_ context.Context, _ *mcp.CallToolRequest, in readDailyNoteInput) (*mcp.CallToolResult, *readDailyNoteOutput, error) {
	n, err := s.dailyNote(in.Vault, in.Date, requireNote)
	if err != nil {
		return nil, nil, err
	}
	res, err := n.v.Read(n.path, in.Offset)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, &readDailyNoteOutput{Path: n.path}, nil
	}
	if err != nil {
		return nil, nil, err
	}
	return nil, &readDailyNoteOutput{Path: n.path, Exists: true, ReadResult: res}, nil
}

// readDailyNoteOutputSchema is the inferred schema with only path and exists
// required, since a note not created yet has no text to return.
func readDailyNoteOutputSchema() *jsonschema.Schema {
	schema, err := jsonschema.For[readDailyNoteOutput](nil)
	if err != nil {
		return nil
	}
	schema.Required = []string{"path", "exists"}
	return schema
}

func (s *Server) appendDailyNote(_ context.Context, _ *mcp.CallToolRequest, in writeDailyNoteInput) (*mcp.CallToolResult, writeDailyNoteOutput, error) {
	return s.writeDailyNote(in, vault.Appending(in.Content, in.Inline))
}

func (s *Server) prependDailyNote(_ context.Context, _ *mcp.CallToolRequest, in writeDailyNoteInput) (*mcp.CallToolResult, writeDailyNoteOutput, error) {
	return s.writeDailyNote(in, vault.Prepending(in.Content, in.Inline))
}

func (s *Server) writeDailyNote(in writeDailyNoteInput, change func([]byte) ([]byte, error)) (*mcp.CallToolResult, writeDailyNoteOutput, error) {
	n, err := s.dailyNote(in.Vault, in.Date, requireWritableNote)
	if err != nil {
		return nil, writeDailyNoteOutput{}, err
	}
	created, err := n.v.Upsert(n.path, n.initial, change)
	if err != nil {
		return nil, writeDailyNoteOutput{}, err
	}
	return nil, writeDailyNoteOutput{OK: true, Path: n.path, Created: created}, nil
}
