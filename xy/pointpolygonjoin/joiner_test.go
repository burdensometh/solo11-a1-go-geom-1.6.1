package pointpolygonjoin

import (
	"errors"
	"math"
	"sync"
	"testing"

	"github.com/alecthomas/assert/v2"

	"github.com/twpayne/go-geom"
)

// squareWithHole is a 10x10 square with a 2x2 hole centered on (5, 5).
func squareWithHole() *geom.Polygon {
	return geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
		{{0, 0}, {10, 0}, {10, 10}, {0, 10}, {0, 0}},
		{{4, 4}, {4, 6}, {6, 6}, {6, 4}, {4, 4}},
	})
}

func TestJoinerHoleAndStats(t *testing.T) {
	regions := []Region{
		{ID: "square", Geometry: squareWithHole()},
	}
	points := []Point{
		{ID: "interior", Coord: geom.Coord{1, 1}, Value: 2},
		{ID: "in-hole", Coord: geom.Coord{5, 5}, Value: 40},
		{ID: "on-boundary", Coord: geom.Coord{10, 5}, Value: 8},
		{ID: "outside", Coord: geom.Coord{20, 20}, Value: 100},
	}

	result, err := Join(regions, points)
	assert.NoError(t, err)

	// The point in the hole must not be captured by the bounding box, and the
	// exterior point is unmatched: only two points are associated.
	assert.Equal(t, 1, len(result.Stats))
	assert.Equal(t, 2, result.Stats[0].Count)
	assert.Equal(t, 10.0, result.Stats[0].Sum)
	assert.Equal(t, 5.0, result.Stats[0].Mean)
	assert.Equal(t, "square", result.Stats[0].RegionID)

	assert.Equal(t, 2, len(result.Associations))
	assert.Equal(t, "interior", result.Associations[0].PointID)
	assert.False(t, result.Associations[0].OnBoundary)
	assert.Equal(t, "on-boundary", result.Associations[1].PointID)
	assert.True(t, result.Associations[1].OnBoundary)

	// Unmatched points keep their full identity.
	assert.Equal(t, []Point{
		{ID: "in-hole", Coord: geom.Coord{5, 5}, Value: 40},
		{ID: "outside", Coord: geom.Coord{20, 20}, Value: 100},
	}, result.Unmatched)
}

func TestJoinerBoundaryPolicy(t *testing.T) {
	regions := []Region{{ID: "square", Geometry: squareWithHole()}}
	points := []Point{
		{ID: "outer-edge", Coord: geom.Coord{10, 5}, Value: 1},
		{ID: "hole-edge", Coord: geom.Coord{4, 5}, Value: 2},
	}

	t.Run("include", func(t *testing.T) {
		result, err := Join(regions, points, WithBoundaryPolicy(BoundaryInclude))
		assert.NoError(t, err)
		assert.Equal(t, 2, result.Stats[0].Count)
		assert.Equal(t, 0, len(result.Unmatched))
		assert.True(t, result.Associations[0].OnBoundary)
		assert.True(t, result.Associations[1].OnBoundary)
	})

	t.Run("exclude", func(t *testing.T) {
		result, err := Join(regions, points, WithBoundaryPolicy(BoundaryExclude))
		assert.NoError(t, err)
		assert.Equal(t, 0, result.Stats[0].Count)
		assert.True(t, math.IsNaN(result.Stats[0].Mean))
		assert.Equal(t, points, result.Unmatched)
		assert.Equal(t, 0, len(result.Associations))
	})
}

func TestJoinerOverlappingRegionsAll(t *testing.T) {
	regions := []Region{
		{ID: "a", Geometry: geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
			{{0, 0}, {10, 0}, {10, 10}, {0, 10}, {0, 0}},
		})},
		{ID: "b", Geometry: geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
			{{5, 0}, {15, 0}, {15, 10}, {5, 10}, {5, 0}},
		})},
	}
	points := []Point{
		{ID: "only-a", Coord: geom.Coord{1, 5}, Value: 1},
		{ID: "overlap", Coord: geom.Coord{7, 5}, Value: 10},
		{ID: "only-b", Coord: geom.Coord{14, 5}, Value: 100},
	}

	result, err := Join(regions, points, WithOverlapMode(OverlapAll))
	assert.NoError(t, err)

	// Region a: only-a and overlap. Region b: overlap and only-b.
	assert.Equal(t, 2, result.Stats[0].Count)
	assert.Equal(t, 11.0, result.Stats[0].Sum)
	assert.Equal(t, 5.5, result.Stats[0].Mean)
	assert.Equal(t, 2, result.Stats[1].Count)
	assert.Equal(t, 110.0, result.Stats[1].Sum)

	// The overlapping point is associated twice, ordered by region index.
	var overlapAssociations []PointAssociation
	for _, association := range result.Associations {
		if association.PointID == "overlap" {
			overlapAssociations = append(overlapAssociations, association)
		}
	}
	assert.Equal(t, []PointAssociation{
		{PointID: "overlap", RegionID: "a"},
		{PointID: "overlap", RegionID: "b"},
	}, overlapAssociations)
	assert.Equal(t, 0, len(result.Unmatched))
}

func TestJoinerOverlappingRegionsUniquePriority(t *testing.T) {
	regions := []Region{
		{ID: "low-priority-index-first", Geometry: geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
			{{0, 0}, {10, 0}, {10, 10}, {0, 10}, {0, 0}},
		}), Priority: 5},
		{ID: "high-priority", Geometry: geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
			{{5, 0}, {15, 0}, {15, 10}, {5, 10}, {5, 0}},
		}), Priority: 1},
		{ID: "same-priority-later", Geometry: geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
			{{2, 0}, {12, 0}, {12, 10}, {2, 10}, {2, 0}},
		}), Priority: 1},
	}
	points := []Point{
		// Covered by all three regions: priority 1 wins, then the smallest
		// region index (region b).
		{ID: "three-way", Coord: geom.Coord{7, 5}, Value: 10},
		// Covered by regions a and c: both priority 1 and 5, so c wins.
		{ID: "a-and-c", Coord: geom.Coord{3, 5}, Value: 20},
		// Covered only by region b.
		{ID: "only-b", Coord: geom.Coord{14, 5}, Value: 30},
	}

	result, err := Join(regions, points, WithOverlapMode(OverlapUniquePriority))
	assert.NoError(t, err)

	got := map[any]any{}
	for _, association := range result.Associations {
		got[association.PointID] = association.RegionID
	}
	assert.Equal(t, map[any]any{
		"three-way": "high-priority",
		"a-and-c":   "same-priority-later",
		"only-b":    "high-priority",
	}, got)
	assert.Equal(t, 0, result.Stats[0].Count)
	assert.Equal(t, 2, result.Stats[1].Count)
	assert.Equal(t, 1, result.Stats[2].Count)
	assert.Equal(t, 20.0, result.Stats[2].Sum)
	assert.Equal(t, 0, len(result.Unmatched))
}

func TestJoinerMultiPolygon(t *testing.T) {
	multiPolygon := geom.NewMultiPolygon(geom.XY).MustSetCoords([][][]geom.Coord{
		{
			{{0, 0}, {2, 0}, {2, 2}, {0, 2}, {0, 0}},
		},
		{
			{{8, 8}, {10, 8}, {10, 10}, {8, 10}, {8, 8}},
			// Hole in the second polygon.
			{{8.5, 8.5}, {9.5, 8.5}, {9.5, 9.5}, {8.5, 9.5}, {8.5, 8.5}},
		},
	})
	regions := []Region{{ID: "multi", Geometry: multiPolygon}}
	points := []Point{
		{ID: "part-one", Coord: geom.Coord{1, 1}, Value: 1},
		{ID: "part-two", Coord: geom.Coord{9.8, 9.8}, Value: 2},
		{ID: "part-two-hole", Coord: geom.Coord{9, 9}, Value: 4},
		{ID: "between", Coord: geom.Coord{5, 5}, Value: 8},
	}

	result, err := Join(regions, points)
	assert.NoError(t, err)
	assert.Equal(t, 2, result.Stats[0].Count)
	assert.Equal(t, 3.0, result.Stats[0].Sum)
	assert.Equal(t, []Point{
		{ID: "part-two-hole", Coord: geom.Coord{9, 9}, Value: 4},
		{ID: "between", Coord: geom.Coord{5, 5}, Value: 8},
	}, result.Unmatched)
}

func TestJoinerEmptyRegionsAndNoPoints(t *testing.T) {
	regions := []Region{
		{ID: "empty", Geometry: geom.NewPolygon(geom.XY)},
		{ID: "square", Geometry: geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
			{{0, 0}, {1, 0}, {1, 1}, {0, 1}, {0, 0}},
		})},
		{ID: "empty-multi", Geometry: geom.NewMultiPolygon(geom.XY)},
	}

	t.Run("no points", func(t *testing.T) {
		result, err := Join(regions, nil)
		assert.NoError(t, err)
		assert.Equal(t, 3, len(result.Stats))
		for i, stats := range result.Stats {
			assert.Equal(t, regions[i].ID, stats.RegionID)
			assert.Equal(t, 0, stats.Count)
			assert.Equal(t, 0.0, stats.Sum)
			assert.True(t, math.IsNaN(stats.Mean))
		}
		assert.Equal(t, 0, len(result.Associations))
		assert.Equal(t, 0, len(result.Unmatched))
	})

	t.Run("point near empty region", func(t *testing.T) {
		points := []Point{{ID: "p", Coord: geom.Coord{0.5, 0.5}, Value: 3}}
		result, err := Join(regions, points)
		assert.NoError(t, err)
		assert.Equal(t, 0, result.Stats[0].Count)
		assert.Equal(t, 1, result.Stats[1].Count)
		assert.Equal(t, 3.0, result.Stats[1].Mean)
		assert.Equal(t, 0, result.Stats[2].Count)
	})
}

func TestJoinerXYZLayout(t *testing.T) {
	polygon := geom.NewPolygon(geom.XYZ).MustSetCoords([][]geom.Coord{
		{{0, 0, 100}, {10, 0, 100}, {10, 10, 100}, {0, 10, 100}, {0, 0, 100}},
	})
	regions := []Region{{ID: "xyz", Geometry: polygon}}
	points := []Point{
		{ID: "inside", Coord: geom.Coord{5, 5, -999}, Value: 1},
		{ID: "outside", Coord: geom.Coord{50, 50, 0}, Value: 2},
	}

	result, err := Join(regions, points)
	assert.NoError(t, err)
	assert.Equal(t, 1, result.Stats[0].Count)
	assert.Equal(t, 1, len(result.Unmatched))
	assert.Equal(t, "outside", result.Unmatched[0].ID)
}

func TestJoinerErrors(t *testing.T) {
	square := geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
		{{0, 0}, {1, 0}, {1, 1}, {0, 1}, {0, 0}},
	})

	t.Run("no regions", func(t *testing.T) {
		_, err := Join(nil, nil)
		assert.Error(t, err)
		assert.True(t, errorIs(err, errNoRegions))
	})

	t.Run("nil geometry", func(t *testing.T) {
		_, err := Join([]Region{{ID: "nil"}}, nil)
		assert.Error(t, err)
		assert.True(t, errorIs(err, errNilRegionGeometry))
	})

	t.Run("unsupported geometry", func(t *testing.T) {
		_, err := Join([]Region{{ID: "point", Geometry: geom.NewPoint(geom.XY)}}, nil)
		assert.Error(t, err)
		assert.True(t, errorIs(err, errUnsupportedGeometry))
	})

	t.Run("layout mismatch", func(t *testing.T) {
		_, err := Join([]Region{
			{ID: "xy", Geometry: square},
			{ID: "xyz", Geometry: geom.NewPolygon(geom.XYZ)},
		}, nil)
		assert.Error(t, err)
		assert.True(t, errorIs(err, errConflictingLayouts))
	})

	t.Run("bad node capacity", func(t *testing.T) {
		_, err := Join([]Region{{ID: "square", Geometry: square}}, nil, WithNodeCapacity(1))
		assert.Error(t, err)
		assert.True(t, errorIs(err, errInvalidNodeCapacity))
	})

	t.Run("point without ordinates", func(t *testing.T) {
		_, err := Join([]Region{{ID: "square", Geometry: square}},
			[]Point{{ID: "bad", Coord: geom.Coord{1}}})
		assert.Error(t, err)
		assert.True(t, errorIs(err, errPointWithoutOrdinals))
	})

	t.Run("non-finite point", func(t *testing.T) {
		_, err := Join([]Region{{ID: "square", Geometry: square}},
			[]Point{{ID: "nan", Coord: geom.Coord{math.NaN(), 0.5}}})
		assert.Error(t, err)
		assert.True(t, errorIs(err, errNonFiniteCoordinate))
	})
}

func TestJoinerReusable(t *testing.T) {
	regions := []Region{{ID: "square", Geometry: squareWithHole()}}
	joiner, err := NewJoiner(regions)
	assert.NoError(t, err)

	first, err := joiner.Join([]Point{{ID: "p1", Coord: geom.Coord{1, 1}, Value: 4}})
	assert.NoError(t, err)
	assert.Equal(t, 1, first.Stats[0].Count)

	second, err := joiner.Join([]Point{{ID: "p2", Coord: geom.Coord{5, 5}, Value: 9}})
	assert.NoError(t, err)
	assert.Equal(t, 0, second.Stats[0].Count)
	assert.Equal(t, 1, len(second.Unmatched))
}

func TestJoinerConcurrent(t *testing.T) {
	// An XYZ region forces the reusable query-coordinate path.
	polygon := geom.NewPolygon(geom.XYZ).MustSetCoords([][]geom.Coord{
		{{0, 0, 0}, {10, 0, 0}, {10, 10, 0}, {0, 10, 0}, {0, 0, 0}},
		{{4, 4, 0}, {6, 4, 0}, {6, 6, 0}, {4, 6, 0}, {4, 4, 0}},
	})
	joiner, err := NewJoiner([]Region{{ID: "region", Geometry: polygon}})
	assert.NoError(t, err)
	points := []Point{
		{ID: "inside", Coord: geom.Coord{1, 1, 0}, Value: 1},
		{ID: "hole", Coord: geom.Coord{5, 5, 0}, Value: 2},
		{ID: "outside", Coord: geom.Coord{50, 50, 0}, Value: 3},
	}

	const goroutines = 32
	var waitGroup sync.WaitGroup
	waitGroup.Add(goroutines)
	for range goroutines {
		go func() {
			defer waitGroup.Done()
			for range 100 {
				result, joinErr := joiner.Join(points)
				assert.NoError(t, joinErr)
				assert.Equal(t, 1, result.Stats[0].Count)
				assert.Equal(t, 2, len(result.Unmatched))
			}
		}()
	}
	waitGroup.Wait()
}

// errorIs wraps errors.Is so the assertion reads naturally.
func errorIs(err, target error) bool {
	return errors.Is(err, target)
}
