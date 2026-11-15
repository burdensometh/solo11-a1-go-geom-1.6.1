package pointpolygonjoin_test

import (
	"fmt"
	"math"

	"github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/xy/pointpolygonjoin"
)

// ExampleJoin assigns sample points to regions with holes and aggregates
// count, sum, and mean while preserving unmatched points.
func ExampleJoin() {
	regions := []pointpolygonjoin.Region{
		{
			ID: "park",
			// A square with a lake as a hole.
			Geometry: geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
				{{0, 0}, {10, 0}, {10, 10}, {0, 10}, {0, 0}},
				{{4, 4}, {4, 6}, {6, 6}, {6, 4}, {4, 4}},
			}),
		},
	}
	points := []pointpolygonjoin.Point{
		{ID: "tree-1", Coord: geom.Coord{1, 1}, Value: 2},
		{ID: "tree-2", Coord: geom.Coord{9, 9}, Value: 4},
		{ID: "boat", Coord: geom.Coord{5, 5}, Value: 8}, // inside the hole
		{ID: "bench", Coord: geom.Coord{50, 50}, Value: 16},
	}

	result, err := pointpolygonjoin.Join(regions, points)
	if err != nil {
		panic(err)
	}

	stats := result.Stats[0]
	fmt.Printf("region=%s count=%d sum=%.0f mean=%.1f\n",
		stats.RegionID, stats.Count, stats.Sum, stats.Mean)
	unmatchedIDs := make([]any, 0, len(result.Unmatched))
	for _, point := range result.Unmatched {
		unmatchedIDs = append(unmatchedIDs, point.ID)
	}
	fmt.Printf("unmatched=%v\n", unmatchedIDs)
	// Output:
	// region=park count=2 sum=6 mean=3.0
	// unmatched=[boat bench]
}

// ExampleJoin_uniquePriority shows overlapping regions with priority-based
// assignment.
func ExampleJoin_uniquePriority() {
	regions := []pointpolygonjoin.Region{
		{
			ID:       "city",
			Priority: 10,
			Geometry: geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
				{{0, 0}, {10, 0}, {10, 10}, {0, 10}, {0, 0}},
			}),
		},
		{
			ID:       "district",
			Priority: 1,
			Geometry: geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
				{{4, 4}, {6, 4}, {6, 6}, {4, 6}, {4, 4}},
			}),
		},
	}
	points := []pointpolygonjoin.Point{
		{ID: "in-district", Coord: geom.Coord{5, 5}, Value: 1},
		{ID: "in-city-only", Coord: geom.Coord{1, 1}, Value: 2},
	}

	result, err := pointpolygonjoin.Join(regions, points,
		pointpolygonjoin.WithOverlapMode(pointpolygonjoin.OverlapUniquePriority))
	if err != nil {
		panic(err)
	}
	for _, stats := range result.Stats {
		fmt.Printf("region=%s count=%d\n", stats.RegionID, stats.Count)
	}
	// Output:
	// region=city count=1
	// region=district count=1
}

// ExampleJoin_emptyRegion shows that regions without points still appear in
// the statistics.
func ExampleJoin_emptyRegion() {
	regions := []pointpolygonjoin.Region{
		{ID: "empty", Geometry: geom.NewPolygon(geom.XY)},
	}
	result, err := pointpolygonjoin.Join(regions, nil)
	if err != nil {
		panic(err)
	}
	stats := result.Stats[0]
	fmt.Printf("region=%s count=%d sum=%.0f mean-is-NaN=%t\n",
		stats.RegionID, stats.Count, stats.Sum, math.IsNaN(stats.Mean))
	// Output:
	// region=empty count=0 sum=0 mean-is-NaN=true
}
