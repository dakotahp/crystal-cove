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
	outlierSpread      = 1.5
	maxIterations      = 25
	minClusterSections = 2
	reseedRounds       = 3
)

// grouping is the result of group: assign[i] is section i's topic, or -1
// when it is far from every topic, and closeness[i] is its dot product with
// its nearest center.
type grouping struct {
	assign    []int
	centers   [][]float32
	closeness []float32
}

// topicCount is how many topics to make from this many sections.
func topicCount(sections int) int {
	k := int(math.Round(math.Sqrt(float64(sections) / 8)))
	return min(max(k, 2), maxTopics)
}

// group sorts unit vectors into k topics by spherical k-means from a seeded
// k-means++ start, then leaves out the vectors much farther from their
// center than the rest.
func group(vecs [][]float32, k int, seed uint64) grouping {
	k = min(k, len(vecs))
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)) // #nosec G404 -- deterministic grouping seed, not security
	g := grouping{centers: seedCenters(vecs, k, rng), assign: make([]int, len(vecs)), closeness: make([]float32, len(vecs))}
	g.converge(vecs)
	for range reseedRounds {
		if !g.reseedTiny(vecs) {
			break
		}
		g.converge(vecs)
	}
	g.cutOutliers()
	return g
}

func seedCenters(vecs [][]float32, k int, rng *rand.Rand) [][]float32 {
	centers := [][]float32{slices.Clone(vecs[rng.IntN(len(vecs))])}
	far := make([]float64, len(vecs))
	for i := range far {
		far[i] = math.Inf(1)
	}
	for len(centers) < k {
		last := centers[len(centers)-1]
		total := 0.0
		for i, v := range vecs {
			d := 1 - float64(dot(v, last))
			far[i] = min(far[i], d*d)
			total += far[i]
		}
		pick := 0
		if total > 0 {
			r := rng.Float64() * total
			for i, f := range far {
				r -= f
				if r <= 0 {
					pick = i
					break
				}
			}
		} else {
			pick = rng.IntN(len(vecs))
		}
		centers = append(centers, slices.Clone(vecs[pick]))
	}
	return centers
}

func (g *grouping) converge(vecs [][]float32) {
	for i := range g.assign {
		g.assign[i] = -1
	}
	for range maxIterations {
		changed := false
		for i, v := range vecs {
			best, score := nearest(g.centers, v)
			if best != g.assign[i] {
				g.assign[i], changed = best, true
			}
			g.closeness[i] = score
		}
		if !changed {
			return
		}
		g.recenter(vecs)
	}
}

func nearest(centers [][]float32, v []float32) (int, float32) {
	best, score := 0, float32(math.Inf(-1))
	for c, center := range centers {
		if s := dot(center, v); s > score {
			best, score = c, s
		}
	}
	return best, score
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

func (g *grouping) cutOutliers() {
	n := float64(len(g.closeness))
	var sum, sq float64
	for _, c := range g.closeness {
		sum += float64(c)
	}
	mean := sum / n
	for _, c := range g.closeness {
		sq += (float64(c) - mean) * (float64(c) - mean)
	}
	cutoff := mean - outlierSpread*math.Sqrt(sq/n)
	for i, c := range g.closeness {
		if float64(c) < cutoff {
			g.assign[i] = -1
		}
	}
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
