package daily

import (
	"strings"
	"testing"
	"time"
)

var day = time.Date(2026, time.March, 1, 14, 5, 9, 0, time.UTC)

func TestFormatDate(t *testing.T) {
	cases := []struct {
		layout, want string
	}{
		{"YYYY-MM-DD", "2026-03-01"},
		{"YY/M/D", "26/3/1"},
		{"Y", "2026"},
		{"MMMM MMM", "March Mar"},
		{"dddd ddd dd d E", "Sunday Sun Su 0 7"},
		{"Do", "1st"},
		{"DDD DDDD", "60 060"},
		{"Q", "1"},
		{"GGGG-[W]WW", "2026-W09"},
		{"GG W", "26 9"},
		{"HH:mm:ss", "14:05:09"},
		{"H h hh m s", "14 2 02 5 9"},
		{"A a", "PM pm"},
		{"[YYYY] YYYY", "YYYY 2026"},
		{`\YYYY-MM`, "YYYY-03"},
		{`[a[b]`, "[pmb"},
		{"YYYY/MM/YYYY-MM-DD", "2026/03/2026-03-01"},
		{"T_b-", "T_b-"},
		{"[_", "[_"},
		{`YYYY\`, "2026"},
		{"", ""},
	}
	for _, c := range cases {
		got, err := FormatDate(day, c.layout)
		if err != nil {
			t.Errorf("FormatDate(%q): %v", c.layout, err)
			continue
		}
		if got != c.want {
			t.Errorf("FormatDate(%q) = %q, want %q", c.layout, got, c.want)
		}
	}
}

func TestFormatDateOrdinals(t *testing.T) {
	for d, want := range map[int]string{2: "2nd", 3: "3rd", 4: "4th", 11: "11th", 12: "12th", 13: "13th", 21: "21st", 22: "22nd", 23: "23rd"} {
		got, err := FormatDate(time.Date(2026, time.January, d, 0, 0, 0, 0, time.UTC), "Do")
		if err != nil || got != want {
			t.Errorf("Do on day %d = %q, %v; want %q", d, got, err, want)
		}
	}
}

func TestFormatDateISOWeekdayCountsFromMonday(t *testing.T) {
	got, err := FormatDate(day.AddDate(0, 0, 1), "E dddd")
	if err != nil || got != "1 Monday" {
		t.Errorf("got %q, %v; want 1 Monday", got, err)
	}
}

func TestFormatDateMidnightAndNoonInTwelveHourClock(t *testing.T) {
	for hour, want := range map[int]string{0: "12 AM", 12: "12 PM"} {
		got, err := FormatDate(time.Date(2026, time.January, 1, hour, 0, 0, 0, time.UTC), "h A")
		if err != nil || got != want {
			t.Errorf("hour %d = %q, %v; want %q", hour, got, err, want)
		}
	}
}

func TestFormatDateRefusesTokensItCannotMatch(t *testing.T) {
	for _, layout := range []string{"gggg-ww", "w", "e", "X", "x", "Z", "zz", "kk", "SSS", "Mo", "YYYYY", "N", "yyyy"} {
		_, err := FormatDate(day, layout)
		if err == nil || !strings.Contains(err.Error(), "not supported") {
			t.Errorf("FormatDate(%q) err = %v, want not supported", layout, err)
		}
	}
}

func TestParseSettings(t *testing.T) {
	s, err := ParseSettings([]byte(`{"folder": "/Journal/Daily/", "format": "YYYY/MM/DD", "template": "Templates/Daily"}`))
	if err != nil {
		t.Fatal(err)
	}
	if s.Folder != "Journal/Daily" || s.Format != "YYYY/MM/DD" || s.Template != "Templates/Daily" {
		t.Errorf("settings = %+v", s)
	}
}

func TestParseSettingsDefaults(t *testing.T) {
	for _, data := range []string{"", "{}", `{"folder": "/", "format": "  "}`} {
		s, err := ParseSettings([]byte(data))
		if err != nil {
			t.Fatalf("%q: %v", data, err)
		}
		if s.Folder != "" || s.Format != DefaultFormat || s.Template != "" {
			t.Errorf("%q: settings = %+v", data, s)
		}
	}
}

func TestParseSettingsRejectsBadJSON(t *testing.T) {
	if _, err := ParseSettings([]byte("{")); err == nil || !strings.Contains(err.Error(), SettingsFile) {
		t.Errorf("err = %v, want one naming %s", err, SettingsFile)
	}
}

func TestPath(t *testing.T) {
	cases := []struct {
		s    Settings
		want string
	}{
		{Settings{Format: DefaultFormat}, "2026-03-01.md"},
		{Settings{Folder: "Daily", Format: DefaultFormat}, "Daily/2026-03-01.md"},
		{Settings{Folder: "Daily", Format: "YYYY/MMMM/Do"}, "Daily/2026/March/1st.md"},
	}
	for _, c := range cases {
		got, err := c.s.Path(day)
		if err != nil || got != c.want {
			t.Errorf("%+v: Path = %q, %v; want %q", c.s, got, err, c.want)
		}
	}
}

func TestPathRejectsAnUnsupportedFormat(t *testing.T) {
	if _, err := (Settings{Format: "gggg"}).Path(day); err == nil {
		t.Error("Path accepted an unsupported format")
	}
}

func TestPathRejectsAFormatThatNamesNoFile(t *testing.T) {
	for _, format := range []string{"[ ]", "[/]", "YYYY/"} {
		if _, err := (Settings{Format: format}).Path(day); err == nil {
			t.Errorf("format %q: Path succeeded", format)
		}
	}
}

func TestTemplatePath(t *testing.T) {
	cases := map[string]string{
		"":                       "",
		"Templates/Daily":        "Templates/Daily.md",
		"Templates/v1.2 Daily":   "Templates/v1.2 Daily.md",
		"Templates/Daily.MD":     "Templates/Daily.MD",
		"Templates/Daily.canvas": "Templates/Daily.canvas.md",
	}
	for in, want := range cases {
		if got := (Settings{Template: in}).TemplatePath(); got != want {
			t.Errorf("TemplatePath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRenderTemplate(t *testing.T) {
	tmpl := "# {{title}}\ncreated {{date}} {{time}}\n{{date:dddd, MMMM Do}} at {{time:h A}}\n{{DATE}} {{Title}}\n{{title:YYYY}} {{other}}\n"
	got, err := RenderTemplate(tmpl, "2026-03-01", day)
	if err != nil {
		t.Fatal(err)
	}
	want := "# 2026-03-01\ncreated 2026-03-01 14:05\nSunday, March 1st at 2 PM\n2026-03-01 2026-03-01\n{{title:YYYY}} {{other}}\n"
	if got != want {
		t.Errorf("RenderTemplate =\n%q\nwant\n%q", got, want)
	}
}

func TestRenderTemplateRefusesAnUnsupportedFormat(t *testing.T) {
	if _, err := RenderTemplate("{{date:gggg}}", "x", day); err == nil || !strings.Contains(err.Error(), "{{date:gggg}}") {
		t.Errorf("err = %v, want one naming the variable", err)
	}
}

func TestDay(t *testing.T) {
	loc := time.FixedZone("UTC-7", -7*3600)
	now := time.Date(2026, time.March, 1, 3, 0, 0, 0, time.UTC)
	cases := map[string]string{
		"":           "2026-02-28 20:00",
		"today":      "2026-02-28 20:00",
		" Today ":    "2026-02-28 20:00",
		"yesterday":  "2026-02-27 20:00",
		"tomorrow":   "2026-03-01 20:00",
		"2025-12-31": "2025-12-31 20:00",
	}
	for in, want := range cases {
		got, err := Day(in, now, loc)
		if err != nil {
			t.Errorf("Day(%q): %v", in, err)
			continue
		}
		if s := got.Format("2006-01-02 15:04"); s != want || got.Location() != loc {
			t.Errorf("Day(%q) = %s in %v, want %s in %v", in, s, got.Location(), want, loc)
		}
	}
}

func TestDayRejectsOtherText(t *testing.T) {
	for _, in := range []string{"2026-02-30", "03/01/2026", "last week"} {
		if _, err := Day(in, day, time.UTC); err == nil || !strings.Contains(err.Error(), "YYYY-MM-DD") {
			t.Errorf("Day(%q) err = %v, want one naming YYYY-MM-DD", in, err)
		}
	}
}
