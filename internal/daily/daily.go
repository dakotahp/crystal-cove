// Package daily finds and fills in daily notes the way Obsidian's Daily
// notes core plugin does, from the plugin's settings file in the vault.
package daily

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"
)

// SettingsFile is where the Daily notes plugin keeps its settings. Obsidian
// Sync carries it by default, as core plugin settings.
const SettingsFile = ".obsidian/daily-notes.json"

// DefaultFormat is the note name format Obsidian uses when none is set.
const DefaultFormat = "YYYY-MM-DD"

// Settings are the Daily notes plugin's options.
type Settings struct {
	// Folder holds new daily notes; empty means the vault root.
	Folder string
	// Format is the moment.js format of a note's name.
	Format string
	// Template is the note whose text a new daily note starts with.
	Template string
}

// ParseSettings reads the plugin's settings file. Empty data, such as from a
// vault that never changed the settings, gives Obsidian's defaults.
func ParseSettings(data []byte) (Settings, error) {
	var raw struct {
		Folder   string `json:"folder"`
		Format   string `json:"format"`
		Template string `json:"template"`
	}
	if len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, &raw); err != nil {
			return Settings{}, fmt.Errorf("reading %s: %w", SettingsFile, err)
		}
	}
	s := Settings{
		Folder:   normalize(raw.Folder),
		Format:   strings.TrimSpace(raw.Format),
		Template: normalize(raw.Template),
	}
	if s.Format == "" {
		s.Format = DefaultFormat
	}
	return s, nil
}

func normalize(p string) string {
	return strings.Trim(path.Clean("/"+strings.TrimSpace(p)), "/")
}

// Path returns the vault-relative path of the daily note for day.
func (s Settings) Path(day time.Time) (string, error) {
	name, err := FormatDate(day, s.Format)
	if err != nil {
		return "", err
	}
	name = strings.TrimSpace(name)
	if name == "" || strings.HasSuffix(name, "/") {
		return "", fmt.Errorf("daily note format %q gives %q, which names no note: change the format in Obsidian's Daily notes settings", s.Format, name)
	}
	return path.Join(s.Folder, name) + ".md", nil
}

// TemplatePath returns the vault-relative path of the template note, or ""
// when no template is set. Like Obsidian, it adds .md when it is missing.
func (s Settings) TemplatePath() string {
	if s.Template == "" || strings.EqualFold(path.Ext(s.Template), ".md") {
		return s.Template
	}
	return s.Template + ".md"
}

var templateVar = regexp.MustCompile(`(?i)\{\{(date|time|title)(?::([^}]*))?\}\}`)

// RenderTemplate fills in Obsidian's template variables: {{title}},
// {{date}} and {{time}}, which take a moment.js format after a colon, such
// as {{date:dddd}}. Other text, plugin syntax included, is kept as written.
func RenderTemplate(tmpl, title string, t time.Time) (string, error) {
	var b strings.Builder
	last := 0
	for _, m := range templateVar.FindAllStringSubmatchIndex(tmpl, -1) {
		b.WriteString(tmpl[last:m[0]])
		last = m[1]
		whole := tmpl[m[0]:m[1]]
		name := strings.ToLower(tmpl[m[2]:m[3]])
		hasFormat := m[4] >= 0
		if name == "title" {
			if hasFormat {
				b.WriteString(whole)
			} else {
				b.WriteString(title)
			}
			continue
		}
		layout := map[string]string{"date": DefaultFormat, "time": "HH:mm"}[name]
		if hasFormat {
			layout = tmpl[m[4]:m[5]]
		}
		text, err := FormatDate(t, layout)
		if err != nil {
			return "", fmt.Errorf("template variable %s: %w", whole, err)
		}
		b.WriteString(text)
	}
	b.WriteString(tmpl[last:])
	return b.String(), nil
}

// Day returns the day a tool asked for, in loc, at now's time of day: date
// is empty or "today", "yesterday", "tomorrow", or a YYYY-MM-DD date.
func Day(date string, now time.Time, loc *time.Location) (time.Time, error) {
	now = now.In(loc)
	switch strings.ToLower(strings.TrimSpace(date)) {
	case "", "today":
		return now, nil
	case "yesterday":
		return now.AddDate(0, 0, -1), nil
	case "tomorrow":
		return now.AddDate(0, 0, 1), nil
	}
	d, err := time.ParseInLocation(time.DateOnly, strings.TrimSpace(date), loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("date %q is not a YYYY-MM-DD date, today, yesterday or tomorrow", date)
	}
	return time.Date(d.Year(), d.Month(), d.Day(), now.Hour(), now.Minute(), now.Second(), now.Nanosecond(), loc), nil
}
