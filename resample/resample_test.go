package resample_test

import (
	"math"
	"testing"

	"github.com/alecthomas/assert/v2"

	"github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/resample"
)

func mustResample(t *testing.T, g geom.T, origin, step float64, options ...resample.Option) *resample.Result {
	t.Helper()
	result, err := resample.Resample(g, origin, step, options...)
	assert.NoError(t, err)
	return result
}

func assertCoords(t *testing.T, want [][]geom.Coord, g *geom.MultiLineString) {
	t.Helper()
	assert.Equal(t, want, g.Coords())
}

func TestResampleForwardInterpolates(t *testing.T) {
	// X advances by 1 per unit of time and Z is an ordinary coordinate that
	// must be interpolated like X and Y. M is time and is never interpolated.
	ls := geom.NewLineString(geom.XYZM).MustSetCoords([]geom.Coord{
		{0, 0, 100, 100},
		{10, 20, 200, 110},
		{20, 0, 100, 120},
	})
	result := mustResample(t, ls, 100, 5)

	assert.Equal(t, geom.XYZM, result.Geometry.Layout())
	assert.Equal(t, geom.XYZM.Stride(), result.Geometry.Stride())
	assert.Equal(t, 0, result.Geometry.SRID())
	assertCoords(t, [][]geom.Coord{{
		{0, 0, 100, 100},
		{5, 10, 150, 105},
		{10, 20, 200, 110},
		{15, 10, 150, 115},
		{20, 0, 100, 120},
	}}, result.Geometry)

	assert.Equal(t, 5, len(result.Samples))
	for i, sample := range result.Samples {
		assert.Equal(t, float64(100+5*i), sample.Time)
		assert.Equal(t, sample.Time, sample.Coord[3], "M must hold the grid time")
		assert.Equal(t, 0, sample.OutputPart)
		assert.Equal(t, 0, sample.InputPart)
	}
	// The first vertex is between no segments: it is attributed to the segment
	// departing from it.
	assert.Equal(t, 0, result.Samples[0].InputSegment)
	assert.Equal(t, 0.0, result.Samples[0].Fraction)
	// Interpolated halfway along the first segment.
	assert.Equal(t, 0, result.Samples[1].InputSegment)
	assert.Equal(t, 0.5, result.Samples[1].Fraction)
	// An observed vertex is attributed to the segment arriving at it.
	assert.Equal(t, 0, result.Samples[2].InputSegment)
	assert.Equal(t, 1.0, result.Samples[2].Fraction)
	assert.Equal(t, 1, result.Samples[3].InputSegment)
	assert.Equal(t, 0.5, result.Samples[3].Fraction)
	assert.Equal(t, 1, result.Samples[4].InputSegment)
	assert.Equal(t, 1.0, result.Samples[4].Fraction)
}

func TestResampleXYM(t *testing.T) {
	ls := geom.NewLineString(geom.XYM).MustSetCoords([]geom.Coord{
		{0, 0, 0},
		{10, 10, 10},
	})
	result := mustResample(t, ls, 0, 2.5)
	assertCoords(t, [][]geom.Coord{{
		{0, 0, 0},
		{2.5, 2.5, 2.5},
		{5, 5, 5},
		{7.5, 7.5, 7.5},
		{10, 10, 10},
	}}, result.Geometry)
}

func TestResampleGridOriginAndStep(t *testing.T) {
	// The grid is t(k) = origin + k*step; it does not have to be anchored to
	// the input's first vertex.
	ls := geom.NewLineString(geom.XYM).MustSetCoords([]geom.Coord{
		{0, 0, 3},
		{12, 12, 9},
	})
	result := mustResample(t, ls, 4, 2)
	// At time 4 the fraction along the segment is (4-3)/(9-3) = 1/6, so
	// X = Y = 2. At time 6 the fraction is 1/2, so X = Y = 6. At time 8 it
	// is 5/6, so X = Y = 10. M is the grid time itself.
	assertCoords(t, [][]geom.Coord{{
		{2, 2, 4},
		{6, 6, 6},
		{10, 10, 8},
	}}, result.Geometry)
	assert.Equal(t, []float64{4, 6, 8}, []float64{
		result.Samples[0].Time, result.Samples[1].Time, result.Samples[2].Time,
	})
	assert.Equal(t, []float64{1.0 / 6, 0.5, 5.0 / 6}, []float64{
		result.Samples[0].Fraction, result.Samples[1].Fraction, result.Samples[2].Fraction,
	})
}

func TestResampleGridOutsideInput(t *testing.T) {
	ls := geom.NewLineString(geom.XYM).MustSetCoords([]geom.Coord{
		{0, 0, 0},
		{1, 1, 1},
	})
	result := mustResample(t, ls, 10, 100)
	assert.Equal(t, 0, result.Geometry.NumLineStrings())
	assert.Equal(t, 0, len(result.Samples))
}

func TestResampleBackward(t *testing.T) {
	ls := geom.NewLineString(geom.XYM).MustSetCoords([]geom.Coord{
		{0, 0, 30},
		{30, 30, 10},
	})
	result := mustResample(t, ls, 30, 5)
	assertCoords(t, [][]geom.Coord{{
		{0, 0, 30},
		{7.5, 7.5, 25},
		{15, 15, 20},
		{22.5, 22.5, 15},
		{30, 30, 10},
	}}, result.Geometry)
	// Output keeps the input's traversal order, so M descends.
	assert.Equal(t, []float64{30, 25, 20, 15, 10}, []float64{
		result.Samples[0].Time,
		result.Samples[1].Time,
		result.Samples[2].Time,
		result.Samples[3].Time,
		result.Samples[4].Time,
	})
}

func TestResampleBackwardOffGrid(t *testing.T) {
	ls := geom.NewLineString(geom.XYM).MustSetCoords([]geom.Coord{
		{0, 0, 13},
		{0, 0, 2},
	})
	result := mustResample(t, ls, 0, 3)
	assertCoords(t, [][]geom.Coord{{
		{0, 0, 12},
		{0, 0, 9},
		{0, 0, 6},
		{0, 0, 3},
	}}, result.Geometry)
}

func TestResampleDuplicateTimes(t *testing.T) {
	// Time 5 is observed twice. The instant is emitted once, using the last
	// observation in input order, which is also the start of the next segment.
	ls := geom.NewLineString(geom.XYM).MustSetCoords([]geom.Coord{
		{0, 0, 0},
		{5, 5, 5},
		{50, 5, 5},
		{10, 10, 10},
	})
	result := mustResample(t, ls, 0, 2.5)
	assertCoords(t, [][]geom.Coord{{
		{0, 0, 0},
		{2.5, 2.5, 2.5},
		{50, 5, 5},
		{30, 7.5, 7.5},
		{10, 10, 10},
	}}, result.Geometry)
	assert.Equal(t, 5, len(result.Samples))
	// The zero-duration segment never produces a point, and the emitted
	// observation belongs to the segment departing from it.
	assert.Equal(t, geom.Coord{50, 5, 5}, result.Samples[2].Coord)
	assert.Equal(t, 2, result.Samples[2].InputSegment)
	assert.Equal(t, 0.0, result.Samples[2].Fraction)
}

func TestResampleMalformedGeometry(t *testing.T) {
	// Constructors do not validate; a misaligned flat coordinate slice or a
	// broken ends list must be reported rather than read out of bounds.
	for _, tc := range []struct {
		name string
		g    geom.T
	}{
		{
			name: "misaligned flat coords",
			g:    geom.NewLineStringFlat(geom.XYM, []float64{0, 0, 0, 1, 1}),
		},
		{
			name: "misaligned end",
			g:    geom.NewMultiLineStringFlat(geom.XYM, []float64{0, 0, 0, 1, 1, 1}, []int{2}),
		},
		{
			name: "out of order ends",
			g:    geom.NewMultiLineStringFlat(geom.XYM, []float64{0, 0, 0, 1, 1, 1}, []int{6, 3}),
		},
		{
			name: "end beyond flat coords",
			g:    geom.NewMultiLineStringFlat(geom.XYM, []float64{0, 0, 0}, []int{9}),
		},
		{
			name: "trailing coordinates",
			g:    geom.NewMultiLineStringFlat(geom.XYM, []float64{0, 0, 0, 1, 1, 1}, []int{3}),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := resample.Resample(tc.g, 0, 1)
			assert.IsError(t, err, resample.ErrMalformedGeometry)
		})
	}
}

func TestResampleConstantTimePart(t *testing.T) {
	ls := geom.NewLineString(geom.XYM).MustSetCoords([]geom.Coord{
		{1, 2, 4},
		{3, 4, 4},
		{5, 6, 4},
	})
	// A part with constant time yields exactly one point when its time is on
	// the grid, taken from its last vertex.
	result := mustResample(t, ls, 0, 2)
	assertCoords(t, [][]geom.Coord{{{5, 6, 4}}}, result.Geometry)
	assert.Equal(t, 1, len(result.Samples))
	assert.Equal(t, 4.0, result.Samples[0].Time)
	assert.Equal(t, -1, result.Samples[0].InputSegment)
	assert.Equal(t, 0.0, result.Samples[0].Fraction)

	// Off the grid, it yields nothing.
	result = mustResample(t, ls, 0, 3)
	assert.Equal(t, 0, result.Geometry.NumLineStrings())
	assert.Equal(t, 0, len(result.Samples))
}

func TestResampleSingleVertex(t *testing.T) {
	ls := geom.NewLineString(geom.XYM).MustSetCoords([]geom.Coord{{7, 8, 3}})
	result := mustResample(t, ls, 0, 3)
	assertCoords(t, [][]geom.Coord{{{7, 8, 3}}}, result.Geometry)
	assert.Equal(t, 1, len(result.Samples))
	assert.Equal(t, 3.0, result.Samples[0].Time)
	assert.Equal(t, -1, result.Samples[0].InputSegment)
}

func TestResampleGap(t *testing.T) {
	// The samples at times 5 and 100 are 95 apart, far more than the maximum
	// gap, so the trajectory is broken there.
	ls := geom.NewLineString(geom.XYM).MustSetCoords([]geom.Coord{
		{0, 0, 0},
		{5, 5, 5},
		{100, 100, 100},
		{105, 105, 105},
	})
	result := mustResample(t, ls, 0, 5, resample.WithMaxGap(10))

	// The output is two parts, so no line is drawn across the gap.
	assertCoords(t, [][]geom.Coord{
		{{0, 0, 0}, {5, 5, 5}},
		{{100, 100, 100}, {105, 105, 105}},
	}, result.Geometry)
	assert.Equal(t, 4, len(result.Samples))
	assert.Equal(t, []int{0, 0, 1, 1}, []int{
		result.Samples[0].OutputPart,
		result.Samples[1].OutputPart,
		result.Samples[2].OutputPart,
		result.Samples[3].OutputPart,
	})
	assert.Equal(t, []int{0, 0, 2, 2}, []int{
		result.Samples[0].InputSegment,
		result.Samples[1].InputSegment,
		result.Samples[2].InputSegment,
		result.Samples[3].InputSegment,
	})
}

func TestResampleGapInteriorNotFilled(t *testing.T) {
	// Times 0 and 100 are separated by a gap far larger than the maximum, so
	// the grid times 10, 20, ..., 90 lie strictly inside it. They are not
	// emitted, and the two observations form separate parts.
	ls := geom.NewLineString(geom.XYM).MustSetCoords([]geom.Coord{
		{0, 0, 0},
		{10, 10, 100},
	})
	result := mustResample(t, ls, 0, 10, resample.WithMaxGap(20))
	assertCoords(t, [][]geom.Coord{
		{{0, 0, 0}},
		{{10, 10, 100}},
	}, result.Geometry)
	assert.Equal(t, []float64{0, 100}, []float64{
		result.Samples[0].Time,
		result.Samples[1].Time,
	})
}

func TestResampleGapEdgeOnGrid(t *testing.T) {
	// The vertex at time 10 ends the leading fragment and lies on the grid, so
	// it is emitted even though the next sample is far away. The two
	// fragments remain separate.
	ls := geom.NewLineString(geom.XYM).MustSetCoords([]geom.Coord{
		{0, 0, 0},
		{5, 5, 10},
		{10, 10, 100},
	})
	result := mustResample(t, ls, 0, 10, resample.WithMaxGap(20))
	assertCoords(t, [][]geom.Coord{
		{{0, 0, 0}, {5, 5, 10}},
		{{10, 10, 100}},
	}, result.Geometry)
	assert.Equal(t, 1.0, result.Samples[1].Fraction)
	assert.Equal(t, 0, result.Samples[1].InputSegment)
}

func TestResampleIsolatedVertex(t *testing.T) {
	// Time 50 is isolated by gaps on both sides. It is kept on its own, as a
	// one-coordinate part, without a line to either neighbour.
	ls := geom.NewLineString(geom.XYM).MustSetCoords([]geom.Coord{
		{0, 0, 0},
		{1, 1, 1},
		{50, 50, 50},
		{100, 100, 100},
		{101, 101, 101},
	})
	result := mustResample(t, ls, 0, 50, resample.WithMaxGap(10))
	assertCoords(t, [][]geom.Coord{
		{{0, 0, 0}},
		{{50, 50, 50}},
		{{100, 100, 100}},
	}, result.Geometry)
	assert.Equal(t, []int{0, 1, 2}, []int{
		result.Samples[0].OutputPart,
		result.Samples[1].OutputPart,
		result.Samples[2].OutputPart,
	})
}

func TestResampleMultiLineString(t *testing.T) {
	mls := geom.NewMultiLineString(geom.XYM).MustSetCoords([][]geom.Coord{
		{{0, 0, 0}, {1, 1, 1}},
		{},
		{{10, 10, 10}, {11, 11, 13}, {12, 12, 15}},
	})
	result := mustResample(t, mls, 0, 2)
	assert.Equal(t, 2, result.Geometry.NumLineStrings())
	assertCoords(t, [][]geom.Coord{
		{{0, 0, 0}},
		{{10, 10, 10}, {10.666666666666666, 10.666666666666666, 12}, {11.5, 11.5, 14}},
	}, result.Geometry)
	// The empty middle part is skipped without renumbering its successor.
	assert.Equal(t, []int{0, 2, 2, 2}, []int{
		result.Samples[0].InputPart,
		result.Samples[1].InputPart,
		result.Samples[2].InputPart,
		result.Samples[3].InputPart,
	})
}

func TestResamplePartsNeverJoined(t *testing.T) {
	// Parts with contiguous times are still separate output parts.
	mls := geom.NewMultiLineString(geom.XYM).MustSetCoords([][]geom.Coord{
		{{0, 0, 0}, {1, 1, 2}},
		{{2, 2, 2}, {3, 3, 4}},
	})
	result := mustResample(t, mls, 0, 1)
	assertCoords(t, [][]geom.Coord{
		{{0, 0, 0}, {0.5, 0.5, 1}, {1, 1, 2}},
		{{2, 2, 2}, {2.5, 2.5, 3}, {3, 3, 4}},
	}, result.Geometry)
	assert.Equal(t, 2, result.Geometry.NumLineStrings())
}

func TestResampleTypedNil(t *testing.T) {
	var ls *geom.LineString
	_, err := resample.Resample(ls, 0, 1)
	assert.IsError(t, err, resample.ErrUnsupportedGeometry)

	var mls *geom.MultiLineString
	_, err = resample.Resample(mls, 0, 1)
	assert.IsError(t, err, resample.ErrUnsupportedGeometry)
}

func TestResampleStepTooSmallForOrigin(t *testing.T) {
	ls := geom.NewLineString(geom.XYM).MustSetCoords([]geom.Coord{{0, 0, 0}, {1, 1, 1}})
	_, err := resample.Resample(ls, math.MaxFloat64, 1)
	assert.IsError(t, err, resample.ErrInvalidGrid)
}

func TestResampleGridRange(t *testing.T) {
	// A fragment whose grid index would overflow is rejected rather than
	// silently truncated.
	ls := geom.NewLineString(geom.XYM).MustSetCoords([]geom.Coord{
		{0, 0, math.MaxFloat64 / 2}, {1, 1, math.MaxFloat64},
	})
	_, err := resample.Resample(ls, 0, 1)
	assert.IsError(t, err, resample.ErrGridRange)
}

func TestResampleFragmentWithoutGridTimes(t *testing.T) {
	// A fragment shorter than the step produces nothing, but a later fragment
	// can still produce points.
	ls := geom.NewLineString(geom.XYM).MustSetCoords([]geom.Coord{
		{0, 0, 1},
		{1, 1, 2},
		{100, 100, 100},
	})
	result := mustResample(t, ls, 0, 100)
	assertCoords(t, [][]geom.Coord{{{100, 100, 100}}}, result.Geometry)
}

func TestResampleEmpty(t *testing.T) {
	for _, g := range []geom.T{
		geom.NewLineString(geom.XYM),
		geom.NewMultiLineString(geom.XYM),
		geom.NewMultiLineString(geom.XYM).MustSetCoords([][]geom.Coord{{}, {}}),
	} {
		result := mustResample(t, g, 0, 1)
		assert.Equal(t, 0, result.Geometry.NumLineStrings())
		assert.Equal(t, 0, len(result.Samples))
		assert.Equal(t, g.Layout(), result.Geometry.Layout())
		assert.Equal(t, g.SRID(), result.Geometry.SRID())
	}
}

func TestResamplePreservesInputAndSRID(t *testing.T) {
	ls := geom.NewLineString(geom.XYZM).MustSetCoords([]geom.Coord{
		{0, 0, 0, 0},
		{4, 4, 8, 4},
	}).SetSRID(4326)
	result := mustResample(t, ls, 0, 1)
	assert.Equal(t, 4326, result.Geometry.SRID())

	// The input keeps its coordinates and the output does not alias them.
	assert.Equal(t, []geom.Coord{{0, 0, 0, 0}, {4, 4, 8, 4}}, ls.Coords())
	assert.False(t, aliases(result.Geometry.FlatCoords(), ls.FlatCoords()))
}

func TestResampleSampleCoordAliasesGeometry(t *testing.T) {
	result := mustResample(t, geom.NewLineString(geom.XYM).MustSetCoords([]geom.Coord{
		{0, 0, 0}, {2, 2, 2},
	}), 0, 1)
	// Sample.Coord is documented as a read-only view of the result's
	// coordinates, so writing through it is visible in the geometry.
	result.Samples[1].Coord[0] = 99
	assert.Equal(t, 99.0, result.Geometry.FlatCoords()[3])
}

func TestResampleErrors(t *testing.T) {
	xy := geom.NewLineString(geom.XY).MustSetCoords([]geom.Coord{{0, 0}, {1, 1}})
	xyz := geom.NewLineString(geom.XYZ).MustSetCoords([]geom.Coord{{0, 0, 0}, {1, 1, 1}})
	point := geom.NewPoint(geom.XYM).MustSetCoords(geom.Coord{0, 0, 0})
	polygon := geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
		{{0, 0}, {1, 0}, {1, 1}, {0, 0}},
	})
	nonMonotonic := geom.NewLineString(geom.XYM).MustSetCoords([]geom.Coord{
		{0, 0, 0}, {1, 1, 10}, {2, 2, 5},
	})
	nonMonotonicBackward := geom.NewLineString(geom.XYM).MustSetCoords([]geom.Coord{
		{0, 0, 10}, {1, 1, 0}, {2, 2, 5},
	})
	nanMeasure := geom.NewLineString(geom.XYM).MustSetCoords([]geom.Coord{
		{0, 0, 0}, {1, 1, math.NaN()},
	})
	infMeasure := geom.NewLineString(geom.XYM).MustSetCoords([]geom.Coord{
		{0, 0, 0}, {1, 1, math.Inf(1)},
	})

	for _, tc := range []struct {
		name    string
		g       geom.T
		origin  float64
		step    float64
		options []resample.Option
		want    error
	}{
		{name: "nil", g: nil, origin: 0, step: 1, want: resample.ErrUnsupportedGeometry},
		{name: "point", g: point, origin: 0, step: 1, want: resample.ErrUnsupportedGeometry},
		{name: "polygon", g: polygon, origin: 0, step: 1, want: resample.ErrUnsupportedGeometry},
		{name: "xy", g: xy, origin: 0, step: 1, want: resample.ErrNoMeasure},
		{name: "xyz", g: xyz, origin: 0, step: 1, want: resample.ErrNoMeasure},
		{name: "nan origin", g: xy, origin: math.NaN(), step: 1, want: resample.ErrInvalidGrid},
		{name: "inf origin", g: xy, origin: math.Inf(1), step: 1, want: resample.ErrInvalidGrid},
		{name: "zero step", g: xy, origin: 0, step: 0, want: resample.ErrInvalidGrid},
		{name: "negative step", g: xy, origin: 0, step: -1, want: resample.ErrInvalidGrid},
		{name: "nan step", g: xy, origin: 0, step: math.NaN(), want: resample.ErrInvalidGrid},
		{name: "inf step", g: xy, origin: 0, step: math.Inf(1), want: resample.ErrInvalidGrid},
		{
			name: "negative max gap", g: xy, origin: 0, step: 1,
			options: []resample.Option{resample.WithMaxGap(-1)}, want: resample.ErrInvalidMaxGap,
		},
		{
			name: "nan max gap", g: xy, origin: 0, step: 1,
			options: []resample.Option{resample.WithMaxGap(math.NaN())}, want: resample.ErrInvalidMaxGap,
		},
		{
			name: "non-monotonic forward", g: nonMonotonic, origin: 0, step: 1,
			want: resample.ErrNonMonotonicMeasure,
		},
		{
			name: "non-monotonic backward", g: nonMonotonicBackward, origin: 0, step: 1,
			want: resample.ErrNonMonotonicMeasure,
		},
		{name: "nan measure", g: nanMeasure, origin: 0, step: 1, want: resample.ErrNonFiniteMeasure},
		{name: "inf measure", g: infMeasure, origin: 0, step: 1, want: resample.ErrNonFiniteMeasure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := resample.Resample(tc.g, tc.origin, tc.step, tc.options...)
			assert.IsError(t, err, tc.want)
		})
	}
}

func TestResampleValidatesGridFirst(t *testing.T) {
	// Grid validation happens before the geometry is inspected, so a bad grid
	// is reported the same way whatever the input is.
	_, err := resample.Resample(geom.NewLineString(geom.XY), math.NaN(), 0)
	assert.IsError(t, err, resample.ErrInvalidGrid)
}

func TestResampleDecimalStep(t *testing.T) {
	// A fractional step must not accumulate floating-point drift: every grid
	// time is computed as origin + k*step, and M holds exactly that value.
	ls := geom.NewLineString(geom.XYM).MustSetCoords([]geom.Coord{
		{0, 0, 0},
		{10, 10, 1},
	})
	result := mustResample(t, ls, 0, 0.1)
	assert.Equal(t, 11, len(result.Samples))
	for i, sample := range result.Samples {
		assert.Equal(t, float64(i)*0.1, sample.Time)
		assert.Equal(t, sample.Time, sample.Coord[2])
		assert.Equal(t, 0, sample.OutputPart)
	}
}

func TestResampleDirectionReversesAcrossGap(t *testing.T) {
	// Each fragment is monotonic but the trajectory reverses direction across
	// the gap, which is allowed.
	ls := geom.NewLineString(geom.XYM).MustSetCoords([]geom.Coord{
		{0, 0, 0},
		{10, 10, 10},
		{30, 30, 30},
		{20, 20, 20},
	})
	result := mustResample(t, ls, 0, 10, resample.WithMaxGap(15))
	assertCoords(t, [][]geom.Coord{
		{{0, 0, 0}, {10, 10, 10}},
		{{30, 30, 30}, {20, 20, 20}},
	}, result.Geometry)
}

func TestResampleDuplicateTimesBackward(t *testing.T) {
	ls := geom.NewLineString(geom.XYM).MustSetCoords([]geom.Coord{
		{0, 0, 10},
		{10, 10, 5},
		{100, 100, 5},
		{200, 200, 0},
	})
	result := mustResample(t, ls, 0, 5)
	// Times descend; time 5 is observed twice and resolved to the last
	// observation in input order.
	assertCoords(t, [][]geom.Coord{{
		{0, 0, 10},
		{100, 100, 5},
		{200, 200, 0},
	}}, result.Geometry)
	assert.Equal(t, []float64{10, 5, 0}, []float64{
		result.Samples[0].Time,
		result.Samples[1].Time,
		result.Samples[2].Time,
	})
	assert.Equal(t, 2, result.Samples[1].InputSegment)
	assert.Equal(t, 0.0, result.Samples[1].Fraction)
}

func TestResampleExtraDimensions(t *testing.T) {
	// Ordinates beyond X, Y, Z, and M are interpolated linearly like Z.
	ls := geom.NewLineString(geom.Layout(5)).MustSetCoords([]geom.Coord{
		{0, 0, 0, 5, 0},
		{10, 10, 10, 15, 10},
	})
	result := mustResample(t, ls, 5, 5)
	assertCoords(t, [][]geom.Coord{
		{{0, 0, 0, 5, 0}, {5, 5, 5, 10, 5}, {10, 10, 10, 15, 10}},
	}, result.Geometry)
	assert.Equal(t, []float64{5, 10, 15}, []float64{
		result.Samples[0].Time,
		result.Samples[1].Time,
		result.Samples[2].Time,
	})
}

func TestResampleInterpolatedProvenance(t *testing.T) {
	ls := geom.NewLineString(geom.XYM).MustSetCoords([]geom.Coord{
		{0, 0, 0}, {2, 2, 2}, {6, 6, 6},
	})
	result := mustResample(t, ls, 1, 2)
	// Grid times 1, 3, and 5 lie on the first, second, and second segments.
	assert.Equal(t, []int{0, 1, 1}, []int{
		result.Samples[0].InputSegment,
		result.Samples[1].InputSegment,
		result.Samples[2].InputSegment,
	})
	assert.Equal(t, []float64{0.5, 0.25, 0.75}, []float64{
		result.Samples[0].Fraction,
		result.Samples[1].Fraction,
		result.Samples[2].Fraction,
	})
}

func TestResampleAllGridPointsInFragment(t *testing.T) {
	// Every grid time inside the fragment produces a point, and M is always
	// the grid time even though the input's measured rate varies.
	ls := geom.NewLineString(geom.XYM).MustSetCoords([]geom.Coord{
		{0, 0, 0},
		{10, 10, 7},
		{20, 20, 14},
	})
	result := mustResample(t, ls, 0, 1)
	assert.Equal(t, 15, len(result.Samples))
	for i, sample := range result.Samples {
		assert.Equal(t, float64(i), sample.Time)
		// X and Y advance by 10 per 7 units of time in both segments.
		want := 10.0 / 7 * float64(i)
		assert.True(t, math.Abs(want-sample.Coord[0]) < 1e-12, "coord %v, want %v", sample.Coord[0], want)
		assert.Equal(t, sample.Coord[0], sample.Coord[1])
		assert.Equal(t, sample.Time, sample.Coord[2])
	}
}

func TestResampleVeryLargeStep(t *testing.T) {
	// The grid's quantized index overflows when the step is tiny relative to
	// values near the float64 limit; this is reported rather than truncated.
	ls := geom.NewLineString(geom.XYM).MustSetCoords([]geom.Coord{
		{0, 0, 1e300},
		{1, 1, 1e300 + 1e290},
	})
	_, err := resample.Resample(ls, 0, 1e-300)
	assert.IsError(t, err, resample.ErrGridRange)
}

func TestResampleTolerance(t *testing.T) {
	// Times around a Unix epoch are large and a grid time cannot be computed
	// exactly. A grid time within a few ULPs of an observed time still counts
	// as that observation rather than as a spurious interpolation.
	const origin = 1_700_000_000.0
	input := geom.NewLineString(geom.XYM).MustSetCoords([]geom.Coord{
		{origin, 0, origin},
		{origin + 1024, 1024, origin + 1024},
	})
	// A step that does not divide the segment times evenly still produces an
	// integer number of points and a final point at the segment's end.
	result := mustResample(t, input, origin, 1024.0/7)
	assert.Equal(t, 8, len(result.Samples))
	assert.Equal(t, origin, result.Samples[0].Time)
	assert.Equal(t, origin, result.Samples[0].Coord[0])
	assert.Equal(t, origin+1024, result.Samples[7].Time)
	assert.Equal(t, origin+1024, result.Samples[7].Coord[0])
}

func aliases(a, b []float64) bool {
	return len(a) > 0 && len(b) > 0 && &a[0] == &b[0]
}
