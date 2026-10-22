package daily

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"
)

var formatTokens = map[string]func(time.Time) string{
	"YYYY": func(t time.Time) string { return pad(t.Year(), 4) },
	"Y":    func(t time.Time) string { return pad(t.Year(), 4) },
	"YY":   func(t time.Time) string { return pad(t.Year()%100, 2) },
	"Q":    func(t time.Time) string { return strconv.Itoa((int(t.Month())-1)/3 + 1) },
	"MMMM": func(t time.Time) string { return t.Month().String() },
	"MMM":  func(t time.Time) string { return t.Month().String()[:3] },
	"MM":   func(t time.Time) string { return pad(int(t.Month()), 2) },
	"M":    func(t time.Time) string { return strconv.Itoa(int(t.Month())) },
	"DDDD": func(t time.Time) string { return pad(t.YearDay(), 3) },
	"DDD":  func(t time.Time) string { return strconv.Itoa(t.YearDay()) },
	"DD":   func(t time.Time) string { return pad(t.Day(), 2) },
	"D":    func(t time.Time) string { return strconv.Itoa(t.Day()) },
	"Do":   func(t time.Time) string { return ordinal(t.Day()) },
	"dddd": func(t time.Time) string { return t.Weekday().String() },
	"ddd":  func(t time.Time) string { return t.Weekday().String()[:3] },
	"dd":   func(t time.Time) string { return t.Weekday().String()[:2] },
	"d":    func(t time.Time) string { return strconv.Itoa(int(t.Weekday())) },
	"E":    func(t time.Time) string { return strconv.Itoa(isoWeekday(t)) },
	"GGGG": func(t time.Time) string { y, _ := t.ISOWeek(); return pad(y, 4) },
	"GG":   func(t time.Time) string { y, _ := t.ISOWeek(); return pad(y%100, 2) },
	"WW":   func(t time.Time) string { _, w := t.ISOWeek(); return pad(w, 2) },
	"W":    func(t time.Time) string { _, w := t.ISOWeek(); return strconv.Itoa(w) },
	"HH":   func(t time.Time) string { return pad(t.Hour(), 2) },
	"H":    func(t time.Time) string { return strconv.Itoa(t.Hour()) },
	"hh":   func(t time.Time) string { return pad(hour12(t), 2) },
	"h":    func(t time.Time) string { return strconv.Itoa(hour12(t)) },
	"mm":   func(t time.Time) string { return pad(t.Minute(), 2) },
	"m":    func(t time.Time) string { return strconv.Itoa(t.Minute()) },
	"ss":   func(t time.Time) string { return pad(t.Second(), 2) },
	"s":    func(t time.Time) string { return strconv.Itoa(t.Second()) },
	"A":    func(t time.Time) string { return meridiem(t) },
	"a":    func(t time.Time) string { return strings.ToLower(meridiem(t)) },
}

// unsupportedTokens are moment.js tokens whose output depends on a locale or
// on precision this server does not reproduce. Treating them as plain text
// would give a different file name than Obsidian, so they are refused.
var unsupportedTokens = []string{
	"YYYYYY", "YYYYY", "yyyy", "yyy", "yy", "yo", "y",
	"NNNNN", "NNNN", "NNN", "NN", "N",
	"ggggg", "gggg", "gg", "GGGGG",
	"Mo", "DDDo", "do", "Qo", "wo", "Wo", "ww", "w", "e",
	"kk", "k", "SSSSSSSSS", "SSSSSSSS", "SSSSSSS", "SSSSSS", "SSSSS", "SSSS", "SSS", "SS", "S",
	"x", "X", "zz", "z", "ZZ", "Z",
	"LTS", "LT", "LLLL", "LLL", "LL", "L", "llll", "lll", "ll", "l",
}

// allTokens holds every token, longest first, so a scan takes "YYYY" before "YY".
var allTokens = func() []string {
	all := slices.Concat(slices.Collect(maps.Keys(formatTokens)), unsupportedTokens)
	slices.SortFunc(all, func(a, b string) int { return cmp.Compare(len(b), len(a)) })
	return all
}()

// FormatDate formats t with a moment.js format string, as Obsidian names
// daily notes and fills in {{date:...}}. Text in [brackets] and a token
// after a backslash are kept as written; other characters that are not
// tokens pass through. Names of months and weekdays are in English.
func FormatDate(t time.Time, layout string) (string, error) {
	var b strings.Builder
	for rest := layout; rest != ""; {
		if rest[0] == '[' {
			if end := strings.IndexAny(rest[1:], "[]"); end >= 0 && rest[1+end] == ']' {
				b.WriteString(rest[1 : 1+end])
				rest = rest[end+2:]
				continue
			}
		}
		escaped := rest[0] == '\\'
		if escaped {
			rest = rest[1:]
			if rest == "" {
				break
			}
		}
		tok := tokenAt(rest)
		switch {
		case tok == "":
			tok = rest[:1]
			b.WriteString(tok)
		case escaped:
			b.WriteString(tok)
		case formatTokens[tok] != nil:
			b.WriteString(formatTokens[tok](t))
		default:
			return "", fmt.Errorf("date format %q uses %q, which is not supported: use tokens such as YYYY, MM, DD, ddd and WW", layout, tok)
		}
		rest = rest[len(tok):]
	}
	return b.String(), nil
}

func tokenAt(s string) string {
	for _, tok := range allTokens {
		if strings.HasPrefix(s, tok) {
			return tok
		}
	}
	return ""
}

func pad(n, width int) string {
	return fmt.Sprintf("%0*d", width, n)
}

func ordinal(n int) string {
	suffix := "th"
	if n%100 < 11 || n%100 > 13 {
		switch n % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return strconv.Itoa(n) + suffix
}

func isoWeekday(t time.Time) int {
	if t.Weekday() == time.Sunday {
		return 7
	}
	return int(t.Weekday())
}

func hour12(t time.Time) int {
	if h := t.Hour() % 12; h != 0 {
		return h
	}
	return 12
}

func meridiem(t time.Time) string {
	if t.Hour() < 12 {
		return "AM"
	}
	return "PM"
}
