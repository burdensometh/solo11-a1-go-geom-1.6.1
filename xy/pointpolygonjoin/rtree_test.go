package pointpolygonjoin

import (
	"fmt"
	"testing"

	"github.com/alecthomas/assert/v2"

	"github.com/twpayne/go-geom"
)

func TestRTreeSearchPoint(t *testing.T) {
	entries := make([]rtreeEntry[int], 100)
	for i := range entries {
		x := float64(i % 10)
		y := float64(i / 10)
		entries[i] = rtreeEntry[int]{
			box: box{minX: x, minY: y, maxX: x + 0.5, maxY: y + 0.5},
			v:   i,
		}
	}
	tree := newRTree(entries, 4)

	var got []int
	stack := tree.searchPoint(nil, 2.25, 3.25, func(v int) bool {
		got = append(got, v)
		return true
	})
	// Only the single box containing (2.25, 3.25) is returned.
	assert.Equal(t, []int{32}, got)

	// A point outside every box matches nothing.
	tree.searchPoint(stack, 100, 100, func(int) bool {
		t.Fatal("no entry should match")
		return true
	})
}

func TestRTreeEmpty(t *testing.T) {
	tree := newRTree[rtreeEntry[int]](nil, 4)
	called := false
	tree.searchPoint(nil, 0, 0, func(rtreeEntry[int]) bool {
		called = true
		return true
	})
	assert.False(t, called)
}

func TestRTreeDegenerateBoxes(t *testing.T) {
	// Zero-area boxes still contain points on their border.
	entries := []rtreeEntry[string]{
		{box: box{minX: 1, minY: 1, maxX: 1, maxY: 1}, v: "point-box"},
		{box: box{minX: 0, minY: 2, maxX: 4, maxY: 2}, v: "line-box"},
	}
	tree := newRTree(entries, 8)

	var got []string
	tree.searchPoint(nil, 1, 1, func(v string) bool {
		got = append(got, v)
		return true
	})
	assert.Equal(t, []string{"point-box"}, got)

	got = nil
	tree.searchPoint(nil, 3, 2, func(v string) bool {
		got = append(got, v)
		return true
	})
	assert.Equal(t, []string{"line-box"}, got)
}

func TestRTreeEarlyStop(t *testing.T) {
	entries := make([]rtreeEntry[int], 20)
	for i := range entries {
		entries[i] = rtreeEntry[int]{
			box: box{minX: 0, minY: 0, maxX: 1, maxY: 1},
			v:   i,
		}
	}
	tree := newRTree(entries, 4)

	count := 0
	tree.searchPoint(nil, 0.5, 0.5, func(int) bool {
		count++
		return count < 3
	})
	assert.Equal(t, 3, count)
}

// TestIndexPrunesCandidates verifies the core performance guarantee: a point
// is never tested against all regions, only against the few whose indexed box
// contains it.
func TestIndexPrunesCandidates(t *testing.T) {
	// 400 disjoint unit squares laid out on a 20x20 grid.
	const gridSize = 20
	regions := make([]Region, gridSize*gridSize)
	for i := range regions {
		x := float64(i % gridSize)
		y := float64(i / gridSize)
		regions[i] = Region{
			ID:       i,
			Geometry: mustSquare(x, y, x+1, y+1),
		}
	}
	joiner, err := NewJoiner(regions, WithNodeCapacity(8))
	assert.NoError(t, err)

	// A point in the middle of one square yields exactly one candidate even
	// though there are hundreds of regions.
	candidates := joiner.treeCandidates(5.5, 5.5)
	assert.Equal(t, 1, len(candidates))
	// Region index is row-major: y=5, x=5 gives 5*20+5.
	assert.Equal(t, 105, candidates[0].index)

	// Points outside every region yield no candidates.
	assert.Equal(t, 0, len(joiner.treeCandidates(100, 100)))
}

// TestExactTestCountBelowRegionCount proves the R-tree prunes regions rather
// than delegating a full scan: the candidate set for every point is strictly
// smaller than the total number of regions.
func TestExactTestCountBelowRegionCount(t *testing.T) {
	const regionCount = 100
	regions := make([]Region, regionCount)
	for i := range regions {
		x := float64(i)
		regions[i] = Region{
			ID:       i,
			Geometry: mustSquare(x*2, 0, x*2+1, 1),
		}
	}
	joiner, err := NewJoiner(regions)
	assert.NoError(t, err)

	result, err := joiner.Join([]Point{
		{ID: "in-first", Coord: geomCoord(0.5, 0.5), Value: 1},
		{ID: "in-last", Coord: geomCoord(float64(regionCount-1)*2+0.5, 0.5), Value: 2},
		{ID: "outside", Coord: geomCoord(-5, 5), Value: 3},
	})
	assert.NoError(t, err)
	assert.Equal(t, 1, result.Stats[0].Count)
	assert.Equal(t, 1, result.Stats[regionCount-1].Count)
	assert.Equal(t, 1, len(result.Unmatched))

	// Each point visited far fewer than all regions as index candidates.
	for _, pointCandidates := range []int{
		len(joiner.treeCandidates(0.5, 0.5)),
		len(joiner.treeCandidates(float64(regionCount-1)*2+0.5, 0.5)),
		len(joiner.treeCandidates(-5, 5)),
	} {
		assert.True(t, pointCandidates < regionCount)
	}
}

func BenchmarkJoin(b *testing.B) {
	const gridSize = 50
	regions := make([]Region, gridSize*gridSize)
	for i := range regions {
		x := float64(i % gridSize)
		y := float64(i / gridSize)
		regions[i] = Region{
			ID:       i,
			Geometry: mustSquare(x, y, x+1, y+1),
		}
	}
	joiner, err := NewJoiner(regions)
	if err != nil {
		b.Fatal(err)
	}
	points := make([]Point, gridSize*gridSize)
	for i := range points {
		x := float64(i % gridSize)
		y := float64(i / gridSize)
		points[i] = Point{ID: i, Coord: geomCoord(x+0.5, y+0.5), Value: float64(i)}
	}

	b.ResetTimer()
	for range b.N {
		result, err := joiner.Join(points)
		if err != nil {
			b.Fatal(err)
		}
		if len(result.Stats) != len(regions) {
			b.Fatalf("got %d stats, want %d", len(result.Stats), len(regions))
		}
	}
}

// treeCandidates returns the indexed candidate regions for a point.
func (j *Joiner) treeCandidates(x, y float64) []*indexedRegion {
	var candidates []*indexedRegion
	j.tree.searchPoint(nil, x, y, func(region *indexedRegion) bool {
		candidates = append(candidates, region)
		return true
	})
	return candidates
}

// mustSquare builds an axis-aligned square polygon for tests.
func mustSquare(minX, minY, maxX, maxY float64) *geom.Polygon {
	return geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
		{{minX, minY}, {maxX, minY}, {maxX, maxY}, {minX, maxY}, {minX, minY}},
	})
}

// geomCoord builds an XY coordinate for tests.
func geomCoord(x, y float64) geom.Coord {
	return geom.Coord{x, y}
}

func Example_rTree() {
	entries := []rtreeEntry[string]{
		{box: box{minX: 0, minY: 0, maxX: 1, maxY: 1}, v: "a"},
		{box: box{minX: 5, minY: 5, maxX: 6, maxY: 6}, v: "b"},
	}
	tree := newRTree(entries, 8)
	tree.searchPoint(nil, 0.5, 0.5, func(v string) bool {
		fmt.Println(v)
		return true
	})
	// Output: a
}
