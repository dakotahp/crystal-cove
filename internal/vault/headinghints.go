package vault

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// maxHeadingHints caps the headings a missing-section error lists, so a
// note with hundreds of headings cannot flood the reply.
const maxHeadingHints = 50

// headingLookalikes folds the characters that differ between a heading as
// written and as a model retypes it: dashes, curly quotes and spacing.
var headingLookalikes = strings.NewReplacer(
	"—", "-", "–", "-", "‘", "'", "’", "'", "“", `"`, "”", `"`,
)

func foldHeading(title string) string {
	return strings.Join(strings.Fields(headingLookalikes.Replace(strings.ToLower(title))), " ")
}

// headingHints lists the note's heading paths, written the way heading_path
// takes them, with the ones closest to want first, so a caller that
// misremembered a heading can retry without reading the whole note.
func headingHints(sections []section, want string) string {
	if len(sections) == 0 {
		return "the note has no headings"
	}
	folded := foldHeading(want)
	closeness := func(s section) int {
		title := foldHeading(s.path[len(s.path)-1])
		switch {
		case title == folded:
			return 0
		case folded != "" && (strings.Contains(title, folded) || strings.Contains(folded, title)):
			return 1
		}
		return 2
	}
	ranked := slices.Clone(sections)
	slices.SortStableFunc(ranked, func(a, b section) int { return closeness(a) - closeness(b) })

	paths := make([]string, 0, min(len(ranked), maxHeadingHints))
	for _, s := range ranked[:min(len(ranked), maxHeadingHints)] {
		quoted := make([]string, len(s.path))
		for i, title := range s.path {
			quoted[i] = strconv.Quote(title)
		}
		paths = append(paths, "["+strings.Join(quoted, ",")+"]")
	}
	hint := "headings in this note, closest first: " + strings.Join(paths, ", ")
	if rest := len(ranked) - len(paths); rest > 0 {
		hint += fmt.Sprintf(" (%d more)", rest)
	}
	return hint
}
