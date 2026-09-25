package server

import (
	"sort"
	"strings"

	"github.com/dakotahp/crystal-cove/internal/search"
)

// Ranking weights. A note named after the query is what a person would have
// opened, so it outranks any amount of body text; covering more of the
// query's words comes next; the number of matching lines only breaks ties.
const (
	titleWeight = 1000
	wordWeight  = 100
	lineCap     = 50
)

// rankFiles orders search results by how well each note answers the query.
// Ripgrep returns path order, which buries the obvious note under whatever
// happens to sort first.
func rankFiles(res *search.Result, query string, words []string) {
	if len(words) == 0 {
		words = []string{query}
	}
	score := make(map[string]int, len(res.Files))
	for _, f := range res.Files {
		var text strings.Builder
		text.WriteString(strings.ToLower(f.Path))
		text.WriteByte('\n')
		matched := 0
		for _, l := range f.Lines {
			if l.Match {
				matched++
				text.WriteString(strings.ToLower(l.Text))
				text.WriteByte('\n')
			}
		}
		body := text.String()

		s := 0
		if f.TitleMatch {
			s += titleWeight
		}
		for _, w := range words {
			if strings.Contains(body, strings.ToLower(w)) {
				s += wordWeight
			}
		}
		s += min(matched, lineCap)
		score[f.Path] = s
	}

	sort.SliceStable(res.Files, func(i, j int) bool {
		a, b := res.Files[i], res.Files[j]
		if score[a.Path] != score[b.Path] {
			return score[a.Path] > score[b.Path]
		}
		return a.Path < b.Path
	})
}
