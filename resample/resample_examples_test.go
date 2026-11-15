package resample_test

import (
	"fmt"

	"github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/resample"
)

// ExampleResample samples a two-dimensional trajectory, measuring time in M,
// onto a five-second grid. M is set to the grid time and is never
// interpolated, while X and Y are linearly interpolated.
func ExampleResample() {
	track := geom.NewLineString(geom.XYM).MustSetCoords([]geom.Coord{
		{0, 0, 100},
		{10, 20, 110},
		{20, 0, 120},
	})
	result, err := resample.Resample(track, 100, 5)
	if err != nil {
		panic(err)
	}
	for _, sample := range result.Samples {
		fmt.Printf("t=%.0f x=%.0f y=%.0f segment=%d fraction=%.1f\n",
			sample.Time, sample.Coord[0], sample.Coord[1],
			sample.InputSegment, sample.Fraction)
	}
	// Output:
	// t=100 x=0 y=0 segment=0 fraction=0.0
	// t=105 x=5 y=10 segment=0 fraction=0.5
	// t=110 x=10 y=20 segment=0 fraction=1.0
	// t=115 x=15 y=10 segment=1 fraction=0.5
	// t=120 x=20 y=0 segment=1 fraction=1.0
}

// ExampleResample_gap shows that a maximum gap breaks the output into separate
// parts instead of drawing a line across missing data.
func ExampleResample_gap() {
	track := geom.NewLineString(geom.XYM).MustSetCoords([]geom.Coord{
		{0, 0, 0},
		{10, 10, 10},
		{1000, 1000, 1000},
		{1010, 1010, 1010},
	})
	result, err := resample.Resample(track, 0, 10, resample.WithMaxGap(60))
	if err != nil {
		panic(err)
	}
	fmt.Printf("parts=%d\n", result.Geometry.NumLineStrings())
	for i, part := range result.Geometry.Coords() {
		fmt.Printf("part %d: %v\n", i, part)
	}
	// Output:
	// parts=2
	// part 0: [[0 0 0] [10 10 10]]
	// part 1: [[1000 1000 1000] [1010 1010 1010]]
}

// ExampleResample_backward shows that a trajectory recorded backwards in time
// keeps its traversal order: its output measures descend.
func ExampleResample_backward() {
	track := geom.NewLineString(geom.XYM).MustSetCoords([]geom.Coord{
		{0, 0, 30},
		{30, 30, 10},
	})
	result, err := resample.Resample(track, 30, 10)
	if err != nil {
		panic(err)
	}
	for _, sample := range result.Samples {
		fmt.Printf("t=%.0f x=%.1f\n", sample.Time, sample.Coord[0])
	}
	// Output:
	// t=30 x=0.0
	// t=20 x=15.0
	// t=10 x=30.0
}
