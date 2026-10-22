package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dakotahp/crystal-cove/internal/vault"
)

const dailySettings = `{"folder": "Daily", "format": "YYYY/MM/YYYY-MM-DD", "template": "Templates/Daily"}`

// dailyServer is a one-vault server whose clock reads 2026-03-01 03:00 UTC.
func dailyServer(t *testing.T, files map[string]string) (*Server, *vault.Vault) {
	t.Helper()
	s, v := metaServer(t, files)
	s.now = func() time.Time { return time.Date(2026, time.March, 1, 3, 0, 0, 0, time.UTC) }
	return s, v
}

func TestDailyNotePathUsesObsidianDefaultsWithoutSettings(t *testing.T) {
	s, _ := dailyServer(t, nil)
	_, out, err := s.dailyNotePath(context.Background(), nil, dailyNoteInput{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Path != "2026-03-01.md" || out.Exists {
		t.Errorf("out = %+v, want 2026-03-01.md, not existing", out)
	}
}

func TestDailyNotePathFollowsTheVaultsSettings(t *testing.T) {
	s, _ := dailyServer(t, map[string]string{
		".obsidian/daily-notes.json":  dailySettings,
		"Daily/2026/02/2026-02-28.md": "yesterday\n",
	})
	cases := []struct {
		date, want string
		exists     bool
	}{
		{"", "Daily/2026/03/2026-03-01.md", false},
		{"yesterday", "Daily/2026/02/2026-02-28.md", true},
		{"2025-12-31", "Daily/2025/12/2025-12-31.md", false},
	}
	for _, c := range cases {
		_, out, err := s.dailyNotePath(context.Background(), nil, dailyNoteInput{Date: c.date})
		if err != nil {
			t.Fatal(err)
		}
		if out.Path != c.want || out.Exists != c.exists {
			t.Errorf("date %q: out = %+v, want %s exists=%v", c.date, out, c.want, c.exists)
		}
	}
}

func TestDailyNoteTodayFollowsTheServerTimezone(t *testing.T) {
	s, _ := dailyServer(t, nil)
	denver, err := time.LoadLocation("America/Denver")
	if err != nil {
		t.Fatal(err)
	}
	s.SetTimezone(denver)
	_, out, err := s.dailyNotePath(context.Background(), nil, dailyNoteInput{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Path != "2026-02-28.md" {
		t.Errorf("Path = %q, want the Denver day 2026-02-28.md", out.Path)
	}
}

func TestReadDailyNote(t *testing.T) {
	s, _ := dailyServer(t, map[string]string{"2026-03-01.md": "today's note\n"})
	_, out, err := s.readDailyNote(context.Background(), nil, readDailyNoteInput{})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Exists || out.Path != "2026-03-01.md" || out.ReadResult == nil || out.Content != "today's note\n" {
		t.Errorf("out = %+v", out)
	}
}

func TestReadDailyNoteDoesNotCreateAMissingNote(t *testing.T) {
	s, v := dailyServer(t, map[string]string{".obsidian/daily-notes.json": dailySettings, "Templates/Daily.md": "# {{title}}\n"})
	_, out, err := s.readDailyNote(context.Background(), nil, readDailyNoteInput{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Exists || out.ReadResult != nil || out.Path != "Daily/2026/03/2026-03-01.md" {
		t.Errorf("out = %+v, want a missing note with its path", out)
	}
	if _, err := os.Stat(filepath.Join(v.Root(), "Daily")); !os.IsNotExist(err) {
		t.Errorf("reading created the daily folder: %v", err)
	}
}

func TestAppendDailyNoteCreatesTheNoteFromTheTemplate(t *testing.T) {
	s, v := dailyServer(t, map[string]string{
		".obsidian/daily-notes.json": dailySettings,
		"Templates/Daily.md":         "---\ntags: [daily]\n---\n# {{title}}\n{{date:dddd}}\n",
	})
	_, out, err := s.appendDailyNote(context.Background(), nil, writeDailyNoteInput{Content: "- first"})
	if err != nil {
		t.Fatal(err)
	}
	if !out.OK || !out.Created || out.Path != "Daily/2026/03/2026-03-01.md" {
		t.Errorf("out = %+v", out)
	}
	want := "---\ntags: [daily]\n---\n# 2026-03-01\nSunday\n- first"
	if got := readFile(t, v, out.Path); got != want {
		t.Errorf("note = %q, want %q", got, want)
	}

	_, out, err = s.appendDailyNote(context.Background(), nil, writeDailyNoteInput{Content: "- second"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Created {
		t.Error("created = true for the existing note")
	}
	_, out, err = s.appendDailyNote(context.Background(), nil, writeDailyNoteInput{Content: " (done)", Inline: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, v, out.Path); got != want+"\n- second (done)" {
		t.Errorf("note = %q", got)
	}
}

func TestAppendDailyNoteForAnotherDayFillsInThatDay(t *testing.T) {
	s, v := dailyServer(t, map[string]string{
		".obsidian/daily-notes.json": dailySettings,
		"Templates/Daily.md":         "{{date}} {{time}}\n",
	})
	_, out, err := s.appendDailyNote(context.Background(), nil, writeDailyNoteInput{Date: "yesterday", Content: "late entry"})
	if err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, v, "Daily/2026/02/2026-02-28.md"); got != "2026-02-28 03:00\nlate entry" || out.Path != "Daily/2026/02/2026-02-28.md" {
		t.Errorf("note = %q at %s", got, out.Path)
	}
}

func TestAppendDailyNoteWithoutATemplateStartsEmpty(t *testing.T) {
	s, v := dailyServer(t, nil)
	if _, _, err := s.appendDailyNote(context.Background(), nil, writeDailyNoteInput{Content: "hello\n"}); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, v, "2026-03-01.md"); got != "hello\n" {
		t.Errorf("note = %q", got)
	}
}

func TestPrependDailyNoteGoesBelowTheFrontmatter(t *testing.T) {
	s, v := dailyServer(t, map[string]string{"2026-03-01.md": "---\ntags: [daily]\n---\n# Today\n"})
	_, out, err := s.prependDailyNote(context.Background(), nil, writeDailyNoteInput{Content: "> pinned"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Created {
		t.Error("created = true for the existing note")
	}
	if got := readFile(t, v, "2026-03-01.md"); got != "---\ntags: [daily]\n---\n> pinned\n# Today\n" {
		t.Errorf("note = %q", got)
	}
}

func TestPrependDailyNoteCreatesTheNoteFromTheTemplate(t *testing.T) {
	s, v := dailyServer(t, map[string]string{
		".obsidian/daily-notes.json": `{"template": "Templates/Daily.md"}`,
		"Templates/Daily.md":         "---\ntags: [daily]\n---\n# {{title}}\n",
	})
	_, out, err := s.prependDailyNote(context.Background(), nil, writeDailyNoteInput{Content: "first"})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Created {
		t.Error("created = false for a new note")
	}
	if got := readFile(t, v, "2026-03-01.md"); got != "---\ntags: [daily]\n---\nfirst\n# 2026-03-01\n" {
		t.Errorf("note = %q", got)
	}
}

func TestDailyNoteWriteWithAMissingTemplateCreatesNothing(t *testing.T) {
	s, v := dailyServer(t, map[string]string{".obsidian/daily-notes.json": dailySettings})
	_, _, err := s.appendDailyNote(context.Background(), nil, writeDailyNoteInput{Content: "x"})
	if err == nil || !strings.Contains(err.Error(), "Templates/Daily.md") {
		t.Fatalf("err = %v, want one naming the template", err)
	}
	if _, err := os.Stat(filepath.Join(v.Root(), "Daily/2026/03/2026-03-01.md")); !os.IsNotExist(err) {
		t.Errorf("note was created: %v", err)
	}
}

func TestDailyNoteWriteWithAnUnsupportedTemplateFormatCreatesNothing(t *testing.T) {
	s, _ := dailyServer(t, map[string]string{
		".obsidian/daily-notes.json": `{"template": "T"}`,
		"T.md":                       "{{date:gggg}}",
	})
	if _, _, err := s.prependDailyNote(context.Background(), nil, writeDailyNoteInput{Content: "x"}); err == nil || !strings.Contains(err.Error(), "{{date:gggg}}") {
		t.Errorf("err = %v, want one naming the variable", err)
	}
}

func TestDailyNoteToolsReportBadSettingsAndDates(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name  string
		files map[string]string
		date  string
		want  string
	}{
		{"bad json", map[string]string{".obsidian/daily-notes.json": "{"}, "", "daily-notes.json"},
		{"unsupported format", map[string]string{".obsidian/daily-notes.json": `{"format": "gggg-ww"}`}, "", "not supported"},
		{"hidden folder", map[string]string{".obsidian/daily-notes.json": `{"folder": ".journal"}`}, "", "hidden"},
		{"bad date", nil, "next week", "YYYY-MM-DD"},
	}
	for _, c := range cases {
		s, _ := dailyServer(t, c.files)
		errs := map[string]error{}
		_, _, errs["daily_note_path"] = s.dailyNotePath(ctx, nil, dailyNoteInput{Date: c.date})
		_, _, errs["read_daily_note"] = s.readDailyNote(ctx, nil, readDailyNoteInput{Date: c.date})
		_, _, errs["append_daily_note"] = s.appendDailyNote(ctx, nil, writeDailyNoteInput{Date: c.date, Content: "x"})
		_, _, errs["prepend_daily_note"] = s.prependDailyNote(ctx, nil, writeDailyNoteInput{Date: c.date, Content: "x"})
		for tool, err := range errs {
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("%s: %s err = %v, want one containing %q", c.name, tool, err, c.want)
			}
		}
	}
}

func TestDailyNoteToolsReportUnreadableFiles(t *testing.T) {
	ctx := context.Background()
	s, v := dailyServer(t, nil)
	if err := os.MkdirAll(filepath.Join(v.Root(), ".obsidian", "daily-notes.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.dailyNotePath(ctx, nil, dailyNoteInput{}); err == nil {
		t.Error("daily_note_path read a directory as the settings")
	}

	s, v = dailyServer(t, nil)
	if err := os.MkdirAll(filepath.Join(v.Root(), "2026-03-01.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.dailyNotePath(ctx, nil, dailyNoteInput{}); err == nil {
		t.Error("daily_note_path read a directory as the note")
	}
	if _, _, err := s.readDailyNote(ctx, nil, readDailyNoteInput{}); err == nil {
		t.Error("read_daily_note read a directory as the note")
	}
	if _, _, err := s.appendDailyNote(ctx, nil, writeDailyNoteInput{Content: "x"}); err == nil {
		t.Error("append_daily_note wrote to a directory")
	}
}

func TestDailyNoteToolsNeedAKnownVault(t *testing.T) {
	ctx := context.Background()
	s, _ := dailyServer(t, nil)
	errs := map[string]error{}
	_, _, errs["daily_note_path"] = s.dailyNotePath(ctx, nil, dailyNoteInput{Vault: "Nope"})
	_, _, errs["read_daily_note"] = s.readDailyNote(ctx, nil, readDailyNoteInput{Vault: "Nope"})
	_, _, errs["append_daily_note"] = s.appendDailyNote(ctx, nil, writeDailyNoteInput{Vault: "Nope"})
	_, _, errs["prepend_daily_note"] = s.prependDailyNote(ctx, nil, writeDailyNoteInput{Vault: "Nope"})
	for tool, err := range errs {
		if err == nil || !strings.Contains(err.Error(), "unknown vault") {
			t.Errorf("%s: err = %v, want unknown vault", tool, err)
		}
	}
}

func TestDailyNoteToolsOverMCP(t *testing.T) {
	s, _ := dailyServer(t, nil)
	session := testSession(t, s)
	ctx := context.Background()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "read_daily_note", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("read_daily_note on a missing note errored: %+v", res.Content)
	}
	if text := res.Content[0].(*mcp.TextContent).Text; !strings.Contains(text, `"exists":false`) {
		t.Errorf("read_daily_note = %s", text)
	}

	res, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "append_daily_note", Arguments: map[string]any{"content": "hi"}})
	if err != nil || res.IsError {
		t.Fatalf("append_daily_note: %v %+v", err, res)
	}
	res, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "read_daily_note", Arguments: map[string]any{}})
	if err != nil || res.IsError {
		t.Fatalf("read_daily_note: %v %+v", err, res)
	}
	if text := res.Content[0].(*mcp.TextContent).Text; !strings.Contains(text, `"content":"hi"`) {
		t.Errorf("read_daily_note = %s", text)
	}
}
