package server

import (
	"sort"

	"github.com/dakotahp/crystal-cove/internal/search"
)

// rankFiles orders search results by how well each note answers the query.
// A note named after the query is what a person would have opened, so it
// comes first whatever its body holds; then notes with more matching lines;
// then path order. Ripgrep returns path order alone, which buries the
// obvious note under whatever happens to sort first.
func rankFiles(res *search.Result) {
	sort.Slice(res.Files, func(i, j int) bool {
		a, b := res.Files[i], res.Files[j]
		if a.TitleMatch != b.TitleMatch {
			return a.TitleMatch
		}
		if a.TotalMatches != b.TotalMatches {
			return a.TotalMatches > b.TotalMatches
		}
		return a.Path < b.Path
	})
}
