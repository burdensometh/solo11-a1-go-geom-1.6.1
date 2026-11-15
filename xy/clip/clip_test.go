package clip_test

import (
	"testing"

	"github.com/alecthomas/assert/v2"

	"github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/xy/clip"
)

type expectedPiece struct {
	flatCoords []float64
	source     []clip.SegmentRef
}

func assertPieces(t *testing.T, expected []expectedPiece, pieces clip.Pieces) {
	t.Helper()
	assert.Equal(t, len(expected), len(pieces))
	for i, e := range expected {
		assert.Equal(t, e.flatCoords, pieces[i].LineString.FlatCoords(), "piece %d flatCoords", i)
		assert.Equal(t, e.source, pieces[i].Source, "piece %d source", i)
		if len(e.flatCoords) > 0 {
			stride := pieces[i].LineString.Stride()
			assert.Equal(t, len(e.flatCoords)/stride-1, len(pieces[i].Source), "piece %d source length", i)
		}
	}
}

func TestLineString(t *testing.T) {
	r := clip.NewRect(0, 0, 10, 10)
	for _, tc := range []struct {
		name       string
		layout     geom.Layout
		flatCoords []float64
		expected   []expectedPiece
	}{
		{
			name:       "empty",
			layout:     geom.XY,
			flatCoords: nil,
			expected:   nil,
		},
		{
			name:       "single point",
			layout:     geom.XY,
			flatCoords: []float64{5, 5},
			expected:   nil,
		},
		{
			name:       "all inside",
			layout:     geom.XY,
			flatCoords: []float64{1, 1, 9, 1, 9, 9},
			expected: []expectedPiece{
				{
					flatCoords: []float64{1, 1, 9, 1, 9, 9},
					source:     []clip.SegmentRef{{LineString: 0, Segment: 0}, {LineString: 0, Segment: 1}},
				},
			},
		},
		{
			name:       "all outside",
			layout:     geom.XY,
			flatCoords: []float64{11, 1, 12, 2},
			expected:   nil,
		},
		{
			name:       "crossing",
			layout:     geom.XY,
			flatCoords: []float64{-5, 5, 15, 5},
			expected: []expectedPiece{
				{
					flatCoords: []float64{0, 5, 10, 5},
					source:     []clip.SegmentRef{{LineString: 0, Segment: 0}},
				},
			},
		},
		{
			// The line leaves and re-enters the rectangle: the two
			// preserved parts must not be connected.
			name:       "exit and re-enter",
			layout:     geom.XY,
			flatCoords: []float64{5, 2, 15, 2, 15, 8, 5, 8},
			expected: []expectedPiece{
				{
					flatCoords: []float64{5, 2, 10, 2},
					source:     []clip.SegmentRef{{LineString: 0, Segment: 0}},
				},
				{
					flatCoords: []float64{10, 8, 5, 8},
					source:     []clip.SegmentRef{{LineString: 0, Segment: 2}},
				},
			},
		},
		{
			name:       "Z and M are interpolated with the segment parameter",
			layout:     geom.XYZM,
			flatCoords: []float64{-5, 5, 100, 1000, 15, 5, 200, 2000},
			expected: []expectedPiece{
				{
					flatCoords: []float64{0, 5, 125, 1250, 10, 5, 175, 1750},
					source:     []clip.SegmentRef{{LineString: 0, Segment: 0}},
				},
			},
		},
		{
			name:       "Z is interpolated with the segment parameter",
			layout:     geom.XYZ,
			flatCoords: []float64{-5, 5, 1, 15, 5, 3},
			expected: []expectedPiece{
				{
					flatCoords: []float64{0, 5, 1.5, 10, 5, 2.5},
					source:     []clip.SegmentRef{{LineString: 0, Segment: 0}},
				},
			},
		},
		{
			name:       "segment coincident with boundary",
			layout:     geom.XY,
			flatCoords: []float64{0, 2, 0, 8},
			expected: []expectedPiece{
				{
					flatCoords: []float64{0, 2, 0, 8},
					source:     []clip.SegmentRef{{LineString: 0, Segment: 0}},
				},
			},
		},
		{
			name:       "segment along boundary extending beyond",
			layout:     geom.XY,
			flatCoords: []float64{-5, 0, 15, 0},
			expected: []expectedPiece{
				{
					flatCoords: []float64{0, 0, 10, 0},
					source:     []clip.SegmentRef{{LineString: 0, Segment: 0}},
				},
			},
		},
		{
			// The line passes exactly through the corner (10, 0) but never
			// enters the rectangle.
			name:       "corner touch",
			layout:     geom.XY,
			flatCoords: []float64{5, -5, 15, 5},
			expected:   nil,
		},
		{
			// The line cuts across the corner of the rectangle, entering
			// through one edge and leaving through the adjacent one, with
			// no vertex inside.
			name:       "corner cut",
			layout:     geom.XY,
			flatCoords: []float64{4, -2, 12, 6},
			expected: []expectedPiece{
				{
					flatCoords: []float64{6, 0, 10, 4},
					source:     []clip.SegmentRef{{LineString: 0, Segment: 0}},
				},
			},
		},
		{
			// A zero-length segment inside the rectangle is kept,
			// preserving both vertices including their M values.
			name:       "zero-length segment inside",
			layout:     geom.XYM,
			flatCoords: []float64{5, 5, 1, 5, 5, 2, 8, 5, 3},
			expected: []expectedPiece{
				{
					flatCoords: []float64{5, 5, 1, 5, 5, 2, 8, 5, 3},
					source:     []clip.SegmentRef{{LineString: 0, Segment: 0}, {LineString: 0, Segment: 1}},
				},
			},
		},
		{
			name:       "zero-length segment outside",
			layout:     geom.XY,
			flatCoords: []float64{20, 20, 20, 20},
			expected:   nil,
		},
		{
			// A vertex exactly on the boundary is inside, so the two
			// segments are joined into a single piece.
			name:       "vertex on boundary joins segments",
			layout:     geom.XY,
			flatCoords: []float64{5, 5, 0, 5, 5, 0},
			expected: []expectedPiece{
				{
					flatCoords: []float64{5, 5, 0, 5, 5, 0},
					source:     []clip.SegmentRef{{LineString: 0, Segment: 0}, {LineString: 0, Segment: 1}},
				},
			},
		},
		{
			// The line touches the corner (0, 0) at a vertex and heads back
			// outside: the piece ends at the corner vertex.
			name:       "vertex on corner",
			layout:     geom.XY,
			flatCoords: []float64{5, 5, 0, 0, 5, -5},
			expected: []expectedPiece{
				{
					flatCoords: []float64{5, 5, 0, 0},
					source:     []clip.SegmentRef{{LineString: 0, Segment: 0}},
				},
			},
		},
		{
			// The line enters the rectangle exactly through the corner
			// (0, 0) at a vertex.
			name:       "enter through corner vertex",
			layout:     geom.XY,
			flatCoords: []float64{-5, 0, 0, 0, 5, 5},
			expected: []expectedPiece{
				{
					flatCoords: []float64{0, 0, 5, 5},
					source:     []clip.SegmentRef{{LineString: 0, Segment: 1}},
				},
			},
		},
		{
			// An isolated touch on the boundary produces no output.
			name:       "isolated touch",
			layout:     geom.XY,
			flatCoords: []float64{0, 5, -5, 5, 0, 6, 5, 6},
			expected: []expectedPiece{
				{
					flatCoords: []float64{0, 6, 5, 6},
					source:     []clip.SegmentRef{{LineString: 0, Segment: 2}},
				},
			},
		},
		{
			// The entry point's X is snapped to the boundary even when
			// floating-point interpolation lands slightly outside.
			name:       "entry point snapped to boundary",
			layout:     geom.XY,
			flatCoords: []float64{-1, 5, 2, 5},
			expected: []expectedPiece{
				{
					flatCoords: []float64{0, 5, 2, 5},
					source:     []clip.SegmentRef{{LineString: 0, Segment: 0}},
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ls := geom.NewLineStringFlat(tc.layout, append([]float64(nil), tc.flatCoords...))
			pieces := clip.LineString(ls, r)
			assertPieces(t, tc.expected, pieces)
			// The input must not be modified.
			assert.Equal(t, tc.flatCoords, ls.FlatCoords())
		})
	}
}

func TestLineStringPreservesOriginalVertices(t *testing.T) {
	// Original vertices inside the rectangle must be copied verbatim, with
	// no floating-point drift in any ordinate.
	r := clip.NewRect(0, 0, 10, 10)
	ls := geom.NewLineStringFlat(geom.XYZM, []float64{
		-5, 5, 100, 1000,
		1.0000000000000002, 5, 123.456, 1e9 + 0.5,
		15, 5, 200, 2000,
	})
	pieces := clip.LineString(ls, r)
	assert.Equal(t, 1, len(pieces))
	got := pieces[0].LineString.FlatCoords()
	assert.Equal(t, 1.0000000000000002, got[4])
	assert.Equal(t, 123.456, got[6])
	assert.Equal(t, 1e9+0.5, got[7])
}

func TestLineStringOutputDoesNotAliasInput(t *testing.T) {
	r := clip.NewRect(0, 0, 10, 10)
	ls := geom.NewLineStringFlat(geom.XY, []float64{1, 1, 9, 1, 9, 9})
	pieces := clip.LineString(ls, r)
	assert.Equal(t, 1, len(pieces))
	pieces[0].LineString.FlatCoords()[0] = 100
	assert.Equal(t, 1.0, ls.FlatCoords()[0])
}

func TestMultiLineString(t *testing.T) {
	r := clip.NewRect(0, 0, 10, 10)
	mls := geom.NewMultiLineStringFlat(geom.XY, []float64{
		2, 2, 5, 2, 8, 2,
		8, 2, 8, 5, 8, 8,
		20, 20, 30, 30,
	}, []int{6, 12, 16})
	original := append([]float64(nil), mls.FlatCoords()...)
	pieces := clip.MultiLineString(mls, r)
	// Pieces from different LineStrings are never merged, even though the
	// end of the first LineString coincides with the start of the second.
	assertPieces(t, []expectedPiece{
		{
			flatCoords: []float64{2, 2, 5, 2, 8, 2},
			source:     []clip.SegmentRef{{LineString: 0, Segment: 0}, {LineString: 0, Segment: 1}},
		},
		{
			flatCoords: []float64{8, 2, 8, 5, 8, 8},
			source:     []clip.SegmentRef{{LineString: 1, Segment: 0}, {LineString: 1, Segment: 1}},
		},
	}, pieces)
	assert.Equal(t, original, mls.FlatCoords())
}

func TestMultiLineStringEmpty(t *testing.T) {
	r := clip.NewRect(0, 0, 10, 10)
	assert.Equal(t, 0, len(clip.MultiLineString(geom.NewMultiLineString(geom.XY), r)))
	mls := geom.NewMultiLineStringFlat(geom.XY, []float64{20, 20, 30, 30}, []int{4})
	assert.Equal(t, 0, len(clip.MultiLineString(mls, r)))
}

func TestNoLayout(t *testing.T) {
	r := clip.NewRect(0, 0, 10, 10)
	assert.Equal(t, 0, len(clip.LineString(geom.NewLineString(geom.NoLayout), r)))
	assert.Equal(t, 0, len(clip.MultiLineString(geom.NewMultiLineString(geom.NoLayout), r)))
}

func TestPiecesMultiLineString(t *testing.T) {
	r := clip.NewRect(0, 0, 10, 10)
	ls := geom.NewLineStringFlat(geom.XY, []float64{5, 2, 15, 2, 15, 8, 5, 8})
	pieces := clip.LineString(ls, r)
	mls := pieces.MultiLineString(geom.XY)
	assert.Equal(t, geom.XY, mls.Layout())
	assert.Equal(t, []float64{5, 2, 10, 2, 10, 8, 5, 8}, mls.FlatCoords())
	assert.Equal(t, []int{4, 8}, mls.Ends())

	empty := clip.Pieces(nil).MultiLineString(geom.XYM)
	assert.Equal(t, geom.XYM, empty.Layout())
	assert.Equal(t, 0, empty.NumLineStrings())
}

func TestNewRect(t *testing.T) {
	assert.Equal(t, clip.Rect{MinX: 0, MinY: 0, MaxX: 10, MaxY: 10}, clip.NewRect(0, 0, 10, 10))
	assert.Equal(t, clip.Rect{MinX: 0, MinY: 0, MaxX: 10, MaxY: 10}, clip.NewRect(10, 10, 0, 0))
}

func TestRectFromBounds(t *testing.T) {
	bounds := geom.NewBounds(geom.XY).Set(1, 2, 3, 4)
	assert.Equal(t, clip.Rect{MinX: 1, MinY: 2, MaxX: 3, MaxY: 4}, clip.RectFromBounds(bounds))
}

func TestSRID(t *testing.T) {
	r := clip.NewRect(0, 0, 10, 10)
	ls := geom.NewLineStringFlat(geom.XY, []float64{1, 1, 9, 9}).SetSRID(4326)
	pieces := clip.LineString(ls, r)
	assert.Equal(t, 1, len(pieces))
	assert.Equal(t, 4326, pieces[0].LineString.SRID())
	assert.Equal(t, 4326, pieces.MultiLineString(geom.XY).SRID())
}
