package semantic

import (
	"reflect"
	"testing"
)

func axis(dims, i int) []float32 {
	v := make([]float32, dims)
	v[i] = 1
	return v
}

func clusteredVectors(clusters, per, dims int) [][]float32 {
	var out [][]float32
	for c := range clusters {
		for i := range per {
			v := make([]float32, dims)
			v[c] = 1
			for d := range dims {
				if d != c && d < clusters {
					v[d] = 0.05 * float32((i+d)%3)
				}
			}
			if err := normalize(v); err != nil {
				panic(err)
			}
			out = append(out, v)
		}
	}
	return out
}

func TestTopicCount(t *testing.T) {
	for _, c := range []struct{ sections, want int }{
		{4, 2}, {30, 2}, {100, 4}, {800, 10}, {2000, 16}, {8000, 30}, {100000, 30},
	} {
		if got := topicCount(c.sections); got != c.want {
			t.Errorf("topicCount(%d) = %d, want %d", c.sections, got, c.want)
		}
	}
}

func TestGroupSeparatesClusters(t *testing.T) {
	vecs := clusteredVectors(3, 10, 8)
	vecs = append(vecs, axis(8, 7))
	for seed := range uint64(20) {
		checkSeparated(t, group(vecs, 3, seed), len(vecs))
	}
}

func checkSeparated(t *testing.T, g grouping, n int) {
	t.Helper()
	seen := map[int]bool{}
	for c := range 3 {
		first := g.assign[c*10]
		if first < 0 {
			t.Fatalf("cluster %d left out", c)
		}
		for i := range 10 {
			if g.assign[c*10+i] != first {
				t.Errorf("cluster %d split: %v", c, g.assign[c*10:c*10+10])
			}
		}
		if seen[first] {
			t.Errorf("clusters share topic %d", first)
		}
		seen[first] = true
	}
	if last := g.assign[n-1]; last != -1 {
		t.Errorf("far vector assigned to %d", last)
	}
	if len(g.centers) != 3 || len(g.closeness) != n {
		t.Errorf("centers %d, closeness %d", len(g.centers), len(g.closeness))
	}
}

func TestGroupIsDeterministic(t *testing.T) {
	vecs := clusteredVectors(3, 10, 8)
	if !reflect.DeepEqual(group(vecs, 3, 7), group(vecs, 3, 7)) {
		t.Error("same input and seed gave different groupings")
	}
}

func TestGroupMoreTopicsThanVectors(t *testing.T) {
	vecs := clusteredVectors(1, 2, 4)
	g := group(vecs, 5, 1)
	if len(g.centers) > len(vecs) {
		t.Errorf("%d centers for %d vectors", len(g.centers), len(vecs))
	}
}

func TestBuildIDTracksContentAndScope(t *testing.T) {
	a := []entry{
		{path: "A.md", chunk: Chunk{Text: "one"}},
		{path: "B.md", chunk: Chunk{HeadingPath: []string{"H"}, Text: "two"}},
	}
	id := buildID("", a)
	if len(id) != 12 {
		t.Fatalf("len = %d", len(id))
	}
	if got := buildID("", []entry{a[1], a[0]}); got != id {
		t.Error("order changed the build")
	}
	for name, other := range map[string]string{
		"text":    buildID("", []entry{a[0], {path: "B.md", chunk: Chunk{HeadingPath: []string{"H"}, Text: "three"}}}),
		"heading": buildID("", []entry{a[0], {path: "B.md", chunk: Chunk{HeadingPath: []string{"G"}, Text: "two"}}}),
		"scope":   buildID("+Garden", a),
	} {
		if other == id {
			t.Errorf("changing the %s kept the build", name)
		}
	}
}
