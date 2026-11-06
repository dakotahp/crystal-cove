package semantic

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
)

const (
	topicsVersion      = "1"
	minGroupSections   = 4
	minTopicNotes      = 2
	maxTopics          = 30
	outlierSpread      = 3
	minSpread          = 0.02
	maxIterations      = 25
	minClusterSections = 2
	reseedRounds       = 3
	overcluster        = 2
)

// grouping is the result of group: assign[i] is section i's topic, or -1
// when it is far from every topic, closeness[i] is its dot product with its
// nearest center, and margin[i] how much closer that is than the next
// center, which is small for a section that mixes two topics.
type grouping struct {
	assign    []int
	centers   [][]float32
	closeness []float32
	margin    []float32
}

// topicCount is how many topics to make from this many sections.
func topicCount(sections int) int {
	k := int(math.Round(math.Sqrt(float64(sections) / 8)))
	return min(max(k, 2), maxTopics)
}

// group sorts unit vectors into k topics. Spherical k-means from a seeded
// k-means++ start first makes twice as many groups, and the closest groups
// are then merged down to k: k-means alone favors groups of equal size, so
// one large blob, such as many mixed daily notes, would split in two and
// swallow a smaller real topic. Last, it leaves out the vectors much
// farther from their center than the rest.
func group(vecs [][]float32, k int, seed uint64) grouping {
	k = min(k, len(vecs))
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)) // #nosec G404 -- deterministic grouping seed, not security
	g := grouping{centers: seedCenters(vecs, min(overcluster*k, len(vecs)), rng), assign: make([]int, len(vecs)), closeness: make([]float32, len(vecs)), margin: make([]float32, len(vecs))}
	g.converge(vecs)
	for range reseedRounds {
		if !g.reseedTiny(vecs) {
			break
		}
		g.converge(vecs)
	}
	g.merge(vecs, k)
	g.cutOutliers()
	return g
}

// merge joins groups by Ward's rule until k remain, then assigns every
// vector to its nearest merged center. Ward's cost, sizes a*b/(a+b) times
// the squared distance between centers, merges a small stray group into its
// neighbor before it merges two large real topics.
func (g *grouping) merge(vecs [][]float32, k int) {
	size := make([]float64, len(g.centers))
	for _, c := range g.assign {
		size[c]++
	}
	sums := make([][]float32, len(g.centers))
	for c := range sums {
		sums[c] = make([]float32, len(vecs[0]))
	}
	for i, v := range vecs {
		for d, x := range v {
			sums[g.assign[i]][d] += x
		}
	}
	active := make([]int, 0, len(g.centers))
	for c, sum := range sums {
		if normalize(slices.Clone(sum)) == nil {
			active = append(active, c)
		}
	}
	for len(active) > k {
		bi, bj, best := 0, 1, math.Inf(1)
		for i := range active {
			for j := i + 1; j < len(active); j++ {
				a, b := active[i], active[j]
				cost := size[a] * size[b] / (size[a] + size[b]) * (2 - 2*float64(dot(g.centers[a], g.centers[b])))
				if cost < best {
					bi, bj, best = i, j, cost
				}
			}
		}
		a, b := active[bi], active[bj]
		size[a] += size[b]
		for d, x := range sums[b] {
			sums[a][d] += x
		}
		merged := slices.Clone(sums[a])
		if normalize(merged) == nil {
			g.centers[a] = merged
		}
		active = slices.Delete(active, bj, bj+1)
	}
	centers := make([][]float32, len(active))
	for i, c := range active {
		centers[i] = g.centers[c]
	}
	g.centers = centers
	for i, v := range vecs {
		var score, second float32
		g.assign[i], score, second = nearest(g.centers, v)
		g.closeness[i], g.margin[i] = score, score-second
	}
}

// seedCenters is greedy k-means++: for each new center it draws a few
// candidates weighted by squared distance and keeps the one that brings the
// sections closest overall, which avoids most of the poor starts that leave
// two real topics merged.
func seedCenters(vecs [][]float32, k int, rng *rand.Rand) [][]float32 {
	first := rng.IntN(len(vecs))
	centers := [][]float32{slices.Clone(vecs[first])}
	far := distancesTo(vecs, vecs[first], nil)
	tries := 2 + int(math.Log(float64(k)))
	for len(centers) < k {
		total := 0.0
		for _, f := range far {
			total += f
		}
		best, bestFar, bestTotal := 0, far, math.Inf(1)
		for range tries {
			pick := weightedPick(far, total, rng)
			next := distancesTo(vecs, vecs[pick], far)
			sum := 0.0
			for _, f := range next {
				sum += f
			}
			if sum < bestTotal {
				best, bestFar, bestTotal = pick, next, sum
			}
		}
		centers = append(centers, slices.Clone(vecs[best]))
		far = bestFar
	}
	return centers
}

// distancesTo returns each vector's squared distance to center, or to the
// nearer of center and its distance in prev when prev is given.
func distancesTo(vecs [][]float32, center []float32, prev []float64) []float64 {
	out := make([]float64, len(vecs))
	for i, v := range vecs {
		d := 1 - float64(dot(v, center))
		out[i] = d * d
		if prev != nil {
			out[i] = min(out[i], prev[i])
		}
	}
	return out
}

func weightedPick(weights []float64, total float64, rng *rand.Rand) int {
	if total <= 0 {
		return rng.IntN(len(weights))
	}
	r := rng.Float64() * total
	for i, w := range weights {
		r -= w
		if r <= 0 {
			return i
		}
	}
	return len(weights) - 1
}

func (g *grouping) converge(vecs [][]float32) {
	for i := range g.assign {
		g.assign[i] = -1
	}
	for range maxIterations {
		changed := false
		for i, v := range vecs {
			best, score, second := nearest(g.centers, v)
			if best != g.assign[i] {
				g.assign[i], changed = best, true
			}
			g.closeness[i], g.margin[i] = score, score-second
		}
		if !changed {
			return
		}
		g.recenter(vecs)
	}
}

// nearest returns the closest center, its closeness, and the closeness of
// the runner-up (the same as the best when there is one center).
func nearest(centers [][]float32, v []float32) (int, float32, float32) {
	best, score, second := 0, float32(math.Inf(-1)), float32(math.Inf(-1))
	for c, center := range centers {
		switch s := dot(center, v); {
		case s > score:
			best, score, second = c, s, score
		case s > second:
			second = s
		}
	}
	if len(centers) < 2 {
		second = score
	}
	return best, score, second
}

func (g *grouping) recenter(vecs [][]float32) {
	sums := make([][]float32, len(g.centers))
	for i, v := range vecs {
		c := g.assign[i]
		if sums[c] == nil {
			sums[c] = make([]float32, len(v))
		}
		for d, x := range v {
			sums[c][d] += x
		}
	}
	for c, sum := range sums {
		if sum != nil && normalize(sum) == nil {
			g.centers[c] = sum
		}
	}
}

// reseedTiny moves each center holding fewer than minClusterSections
// vectors onto the loosest member of the largest topic. A lone far vector
// often wins a k-means++ seed and leaves two real topics merged; this splits
// them again. It reports whether it moved any center.
func (g *grouping) reseedTiny(vecs [][]float32) bool {
	sizes := make([]int, len(g.centers))
	for _, c := range g.assign {
		sizes[c]++
	}
	largest := 0
	for c, n := range sizes {
		if n > sizes[largest] {
			largest = c
		}
	}
	var members []int
	for i, c := range g.assign {
		if c == largest {
			members = append(members, i)
		}
	}
	slices.SortStableFunc(members, func(a, b int) int { return cmp.Compare(g.closeness[a], g.closeness[b]) })
	moved := false
	for c, n := range sizes {
		if n >= minClusterSections || len(members) < 2*minClusterSections {
			continue
		}
		g.centers[c] = slices.Clone(vecs[members[0]])
		members = members[1:]
		moved = true
	}
	return moved
}

// cutOutliers leaves out each section much less close to its center than
// the rest of its topic: below the topic's median by outlierSpread robust
// standard deviations (1.4826 times the median absolute deviation), with
// minSpread as the floor for a topic whose sections are all alike.
func (g *grouping) cutOutliers() {
	byTopic := make([][]float64, len(g.centers))
	for i, c := range g.assign {
		byTopic[c] = append(byTopic[c], float64(g.closeness[i]))
	}
	cutoff := make([]float64, len(g.centers))
	for c, values := range byTopic {
		if len(values) == 0 {
			continue
		}
		med := median(values)
		dev := make([]float64, len(values))
		for i, v := range values {
			dev[i] = math.Abs(v - med)
		}
		cutoff[c] = med - outlierSpread*max(1.4826*median(dev), minSpread)
	}
	for i, c := range g.assign {
		if float64(g.closeness[i]) < cutoff[c] {
			g.assign[i] = -1
		}
	}
}

func median(values []float64) float64 {
	s := slices.Clone(values)
	slices.Sort(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

// buildID names the sections in scope, so the same content and scope always
// give the same ID and any change gives a new one.
func buildID(scope string, sections []entry) string {
	keys := make([]string, len(sections))
	for i, e := range sections {
		keys[i] = e.path + "\x1f" + strings.Join(e.chunk.HeadingPath, "\x1f") + "\x1f" + e.chunk.Text
	}
	slices.Sort(keys)
	h := sha256.New()
	h.Write([]byte(topicsVersion + "\x00" + scope))
	for _, k := range keys {
		h.Write([]byte("\x00" + k))
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}
