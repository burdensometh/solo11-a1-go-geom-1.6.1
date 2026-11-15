package geom

import (
	"fmt"
	"math"
	"strconv"
	"testing"

	"github.com/alecthomas/assert/v2"
)

func TestRingOrientationOf(t *testing.T) {
	for i, tc := range []struct {
		layout Layout
		ring   []float64
		want   RingOrientation
	}{
		// A unit square in counter-clockwise order.
		{
			layout: XY,
			ring:   []float64{0, 0, 1, 0, 1, 1, 0, 1, 0, 0},
			want:   RingOrientationCounterClockwise,
		},
		// The same square in clockwise order.
		{
			layout: XY,
			ring:   []float64{0, 0, 0, 1, 1, 1, 1, 0, 0, 0},
			want:   RingOrientationClockwise,
		},
		// An unclosed counter-clockwise triangle, which is implicitly closed
		// by the first edge of the shoelace formula.
		{
			layout: XY,
			ring:   []float64{0, 0, 1, 0, 0, 1},
			want:   RingOrientationCounterClockwise,
		},
		{
			layout: XY,
			ring:   nil,
			want:   RingOrientationEmpty,
		},
		// Fewer than three coordinates.
		{
			layout: XY,
			ring:   []float64{},
			want:   RingOrientationEmpty,
		},
		{
			layout: XY,
			ring:   []float64{0, 0},
			want:   RingOrientationDegenerate,
		},
		{
			layout: XY,
			ring:   []float64{0, 0, 1, 1},
			want:   RingOrientationDegenerate,
		},
		// A whole number of coordinates, but not enough to enclose an area.
		{
			layout: XY,
			ring:   []float64{0, 0, 1, 1, 2, 2},
			want:   RingOrientationDegenerate,
		},
		// All coordinates identical.
		{
			layout: XY,
			ring:   []float64{1, 1, 1, 1, 1, 1, 1, 1},
			want:   RingOrientationDegenerate,
		},
		// A partially specified coordinate: the length is not a multiple of
		// the stride implied by the layout.
		{
			layout: XY,
			ring:   []float64{0, 0, 1, 0, 1},
			want:   RingOrientationDegenerate,
		},
		// Z and M ordinates do not influence the orientation.
		{
			layout: XYZM,
			ring: []float64{
				0, 0, 1, 2,
				1, 0, 3, 4,
				1, 1, 5, 6,
				0, 1, 7, 8,
				0, 0, 1, 2,
			},
			want: RingOrientationCounterClockwise,
		},
		// Four XYZ coordinates have a flat coordinate length that is also a
		// multiple of the XY stride. The layout, not the length, must decide
		// the stride: read as XY this slice would be a degenerate ring, but
		// read as XYZ it is a valid counter-clockwise triangle.
		{
			layout: XYZ,
			ring: []float64{
				0, 0, 0,
				1, 0, 0,
				1, 1, 0,
				0, 0, 0,
			},
			want: RingOrientationCounterClockwise,
		},
		{
			layout: XYZ,
			ring: []float64{
				0, 0, 0,
				1, 0, 0,
				1, 1, 0,
				0, 1, 0,
				0, 0, 0,
			},
			want: RingOrientationCounterClockwise,
		},
		// Extra dimensions are ignored except for their effect on the stride.
		{
			layout: Layout(5),
			ring: []float64{
				0, 0, 1, 2, 3,
				1, 0, 4, 5, 6,
				1, 1, 7, 8, 9,
				0, 1, 10, 11, 12,
				0, 0, 1, 2, 3,
			},
			want: RingOrientationCounterClockwise,
		},
		// Non-finite coordinates make the signed area undefined, which is
		// reported as degenerate rather than guessed.
		{
			layout: XY,
			ring:   []float64{0, 0, math.Inf(1), 0, 0, 1, 0, 0},
			want:   RingOrientationDegenerate,
		},
		{
			layout: XY,
			ring:   []float64{0, 0, math.NaN(), 0, 0, 1, 0, 0},
			want:   RingOrientationDegenerate,
		},
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			assert.Equal(t, tc.want, RingOrientationOf(tc.layout, tc.ring))
		})
	}
}

func TestRingOrientationOfLinearRing(t *testing.T) {
	ccw := NewLinearRing(XYZ).MustSetCoords([]Coord{
		{0, 0, 0},
		{1, 0, 1},
		{1, 1, 2},
		{0, 1, 3},
		{0, 0, 0},
	})
	cw := NewLinearRing(XYZ).MustSetCoords([]Coord{
		{0, 0, 0},
		{0, 1, 3},
		{1, 1, 2},
		{1, 0, 1},
		{0, 0, 0},
	})
	assert.Equal(t, RingOrientationCounterClockwise, RingOrientationOfLinearRing(ccw))
	assert.Equal(t, RingOrientationClockwise, RingOrientationOfLinearRing(cw))
	assert.Equal(t, RingOrientationEmpty, RingOrientationOfLinearRing(NewLinearRing(XY)))
}

func TestRingOrientationString(t *testing.T) {
	for i, tc := range []struct {
		orientation RingOrientation
		want        string
	}{
		{orientation: RingOrientationUnknown, want: "Unknown"},
		{orientation: RingOrientationEmpty, want: "Empty"},
		{orientation: RingOrientationDegenerate, want: "Degenerate"},
		{orientation: RingOrientationClockwise, want: "Clockwise"},
		{orientation: RingOrientationCounterClockwise, want: "CounterClockwise"},
		{orientation: RingOrientation(99), want: "RingOrientation(99)"},
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			assert.Equal(t, tc.want, tc.orientation.String())
		})
	}
}

func TestRingOrientationIsDefinedAndOpposite(t *testing.T) {
	assert.False(t, RingOrientationUnknown.IsDefined())
	assert.False(t, RingOrientationEmpty.IsDefined())
	assert.False(t, RingOrientationDegenerate.IsDefined())
	assert.True(t, RingOrientationClockwise.IsDefined())
	assert.True(t, RingOrientationCounterClockwise.IsDefined())

	assert.Equal(t, RingOrientationCounterClockwise, RingOrientationClockwise.Opposite())
	assert.Equal(t, RingOrientationClockwise, RingOrientationCounterClockwise.Opposite())
	assert.Equal(t, RingOrientationUnknown, RingOrientationUnknown.Opposite())
	assert.Equal(t, RingOrientationEmpty, RingOrientationEmpty.Opposite())
	assert.Equal(t, RingOrientationDegenerate, RingOrientationDegenerate.Opposite())
}

func TestOrientInvalidOrientation(t *testing.T) {
	for i, orientation := range []RingOrientation{
		RingOrientationUnknown,
		RingOrientationEmpty,
		RingOrientationDegenerate,
		RingOrientation(99),
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			p := NewPolygon(XY).MustSetCoords([][]Coord{
				{{0, 0}, {1, 0}, {1, 1}, {0, 1}, {0, 0}},
			})
			_, err := Orient(p, orientation)
			assert.Error(t, err)
			_, err = OrientPolygon(p, orientation)
			assert.Error(t, err)
			_, err = OrientMultiPolygon(NewMultiPolygon(XY), orientation)
			assert.Error(t, err)
			_, err = OrientLinearRing(NewLinearRing(XY), orientation)
			assert.Error(t, err)
			_, err = OrientGeometryCollection(NewGeometryCollection(), orientation)
			assert.Error(t, err)
		})
	}
}

func TestOrientLinearRing(t *testing.T) {
	for i, tc := range []struct {
		ring        *LinearRing
		orientation RingOrientation
		want        *LinearRing
	}{
		{
			ring:        NewLinearRing(XY).MustSetCoords([]Coord{{0, 0}, {1, 0}, {1, 1}, {0, 1}, {0, 0}}),
			orientation: RingOrientationCounterClockwise,
			want:        NewLinearRing(XY).MustSetCoords([]Coord{{0, 0}, {1, 0}, {1, 1}, {0, 1}, {0, 0}}),
		},
		{
			ring:        NewLinearRing(XY).MustSetCoords([]Coord{{0, 0}, {1, 0}, {1, 1}, {0, 1}, {0, 0}}),
			orientation: RingOrientationClockwise,
			want:        NewLinearRing(XY).MustSetCoords([]Coord{{0, 0}, {0, 1}, {1, 1}, {1, 0}, {0, 0}}),
		},
		{
			// Every ordinate of a coordinate moves with the vertex, so Z
			// values stay attached to their X and Y values.
			ring: NewLinearRing(XYZ).MustSetCoords([]Coord{
				{0, 0, 1},
				{1, 0, 2},
				{1, 1, 3},
				{0, 1, 4},
				{0, 0, 1},
			}),
			orientation: RingOrientationClockwise,
			want: NewLinearRing(XYZ).MustSetCoords([]Coord{
				{0, 0, 1},
				{0, 1, 4},
				{1, 1, 3},
				{1, 0, 2},
				{0, 0, 1},
			}),
		},
		{
			// The closing endpoint is reversed like every other coordinate,
			// so it appears exactly twice, once at each end of the ring.
			ring: NewLinearRing(XY).MustSetCoords([]Coord{
				{0, 0},
				{0, 1},
				{1, 1},
				{1, 0},
				{0, 0},
			}),
			orientation: RingOrientationCounterClockwise,
			want: NewLinearRing(XY).MustSetCoords([]Coord{
				{0, 0},
				{1, 0},
				{1, 1},
				{0, 1},
				{0, 0},
			}),
		},
		{
			ring: NewLinearRing(XY).MustSetCoords([]Coord{
				{0, 0},
				{1, 0},
				{1, 1},
				{0, 1},
			}),
			orientation: RingOrientationClockwise,
			want: NewLinearRing(XY).MustSetCoords([]Coord{
				{0, 1},
				{1, 1},
				{1, 0},
				{0, 0},
			}),
		},
		{
			// Empty rings are copied unchanged.
			ring:        NewLinearRing(XY),
			orientation: RingOrientationClockwise,
			want:        NewLinearRing(XY),
		},
		{
			// Degenerate rings are copied unchanged rather than repaired.
			ring:        NewLinearRing(XY).MustSetCoords([]Coord{{0, 0}, {1, 1}, {2, 2}}),
			orientation: RingOrientationClockwise,
			want:        NewLinearRing(XY).MustSetCoords([]Coord{{0, 0}, {1, 1}, {2, 2}}),
		},
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			orig := tc.ring.Clone()
			got, err := OrientLinearRing(tc.ring, tc.orientation)
			assert.NoError(t, err)
			assert.NoError(t, got.verify())
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.ring.SRID(), got.SRID())
			if o := RingOrientationOfLinearRing(got); o.IsDefined() {
				assert.Equal(t, tc.orientation, o)
			}
			assert.Equal(t, orig, tc.ring)
			assert.False(t, aliases(tc.ring.FlatCoords(), got.FlatCoords()))
		})
	}
}

func TestOrientLinearRingPreservesSRID(t *testing.T) {
	g := NewLinearRing(XY).
		MustSetCoords([]Coord{{0, 0}, {1, 0}, {0, 1}, {0, 0}}).
		SetSRID(4326)
	got, err := OrientLinearRing(g, RingOrientationClockwise)
	assert.NoError(t, err)
	assert.Equal(t, 4326, got.SRID())
}

func TestOrientPolygon(t *testing.T) {
	for i, tc := range []struct {
		polygon     *Polygon
		orientation RingOrientation
		want        *Polygon
	}{
		{
			// The exterior ring is already counter-clockwise and the hole is
			// already clockwise.
			polygon: NewPolygon(XY).MustSetCoords([][]Coord{
				{{0, 0}, {4, 0}, {4, 4}, {0, 4}, {0, 0}},
				{{1, 1}, {1, 3}, {3, 3}, {3, 1}, {1, 1}},
			}),
			orientation: RingOrientationCounterClockwise,
			want: NewPolygon(XY).MustSetCoords([][]Coord{
				{{0, 0}, {4, 0}, {4, 4}, {0, 4}, {0, 0}},
				{{1, 1}, {1, 3}, {3, 3}, {3, 1}, {1, 1}},
			}),
		},
		{
			// Both rings are declared counter-clockwise. The exterior ring is
			// kept and the hole is reversed. The hole stays a hole.
			polygon: NewPolygon(XY).MustSetCoords([][]Coord{
				{{0, 0}, {4, 0}, {4, 4}, {0, 4}, {0, 0}},
				{{1, 1}, {3, 1}, {3, 3}, {1, 3}, {1, 1}},
			}),
			orientation: RingOrientationCounterClockwise,
			want: NewPolygon(XY).MustSetCoords([][]Coord{
				{{0, 0}, {4, 0}, {4, 4}, {0, 4}, {0, 0}},
				{{1, 1}, {1, 3}, {3, 3}, {3, 1}, {1, 1}},
			}),
		},
		{
			// Both rings are declared clockwise. The exterior ring is
			// reversed and the hole is kept.
			polygon: NewPolygon(XY).MustSetCoords([][]Coord{
				{{0, 0}, {0, 4}, {4, 4}, {4, 0}, {0, 0}},
				{{1, 1}, {1, 3}, {3, 3}, {3, 1}, {1, 1}},
			}),
			orientation: RingOrientationCounterClockwise,
			want: NewPolygon(XY).MustSetCoords([][]Coord{
				{{0, 0}, {4, 0}, {4, 4}, {0, 4}, {0, 0}},
				{{1, 1}, {1, 3}, {3, 3}, {3, 1}, {1, 1}},
			}),
		},
		{
			// The opposite declaration reverses the roles of the exterior
			// ring and holes without changing their order.
			polygon: NewPolygon(XY).MustSetCoords([][]Coord{
				{{0, 0}, {4, 0}, {4, 4}, {0, 4}, {0, 0}},
				{{1, 1}, {1, 3}, {3, 3}, {3, 1}, {1, 1}},
			}),
			orientation: RingOrientationClockwise,
			want: NewPolygon(XY).MustSetCoords([][]Coord{
				{{0, 0}, {0, 4}, {4, 4}, {4, 0}, {0, 0}},
				{{1, 1}, {3, 1}, {3, 3}, {1, 3}, {1, 1}},
			}),
		},
		{
			// Z and M values stay with their vertices.
			polygon: NewPolygon(XYZM).MustSetCoords([][]Coord{
				{{0, 0, 1, 1}, {2, 0, 2, 2}, {2, 2, 3, 3}, {0, 2, 4, 4}, {0, 0, 1, 1}},
				{{1, 1, 5, 5}, {1, 3, 6, 6}, {3, 3, 7, 7}, {3, 1, 8, 8}, {1, 1, 5, 5}},
			}),
			orientation: RingOrientationClockwise,
			want: NewPolygon(XYZM).MustSetCoords([][]Coord{
				{{0, 0, 1, 1}, {0, 2, 4, 4}, {2, 2, 3, 3}, {2, 0, 2, 2}, {0, 0, 1, 1}},
				{{1, 1, 5, 5}, {3, 1, 8, 8}, {3, 3, 7, 7}, {1, 3, 6, 6}, {1, 1, 5, 5}},
			}),
		},
		{
			// Empty and degenerate rings are kept, with their positions
			// preserved.
			polygon: NewPolygon(XY).MustSetCoords([][]Coord{
				{{0, 0}, {4, 0}, {4, 4}, {0, 4}, {0, 0}},
				{},
				{{1, 1}, {2, 2}, {3, 3}},
			}),
			orientation: RingOrientationCounterClockwise,
			want: NewPolygon(XY).MustSetCoords([][]Coord{
				{{0, 0}, {4, 0}, {4, 4}, {0, 4}, {0, 0}},
				{},
				{{1, 1}, {2, 2}, {3, 3}},
			}),
		},
		{
			polygon:     NewPolygon(XY),
			orientation: RingOrientationClockwise,
			want:        NewPolygon(XY),
		},
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			orig := tc.polygon.Clone()

			got, err := OrientPolygon(tc.polygon, tc.orientation)
			assert.NoError(t, err)
			assert.NoError(t, got.verify())
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.polygon.SRID(), got.SRID())
			assert.Equal(t, tc.polygon.NumLinearRings(), got.NumLinearRings())
			assert.Equal(t, orig, tc.polygon)
			assert.False(t, aliases(tc.polygon.FlatCoords(), got.FlatCoords()))

			for j := range got.NumLinearRings() {
				ring := got.LinearRing(j)
				o := RingOrientationOfLinearRing(ring)
				if o.IsDefined() {
					if j == 0 {
						assert.Equal(t, tc.orientation, o)
					} else {
						assert.Equal(t, tc.orientation.Opposite(), o)
					}
				}
			}
		})
	}
}

// ringOrientationForPolygon returns the orientation of the ith ring of p, or
// RingOrientationUnknown if the ring is empty.
func ringOrientationForPolygon(t *testing.T, p *Polygon, i int) RingOrientation {
	t.Helper()
	if i >= p.NumLinearRings() {
		return RingOrientationUnknown
	}
	return RingOrientationOfLinearRing(p.LinearRing(i))
}

func TestOrientPolygonIdempotent(t *testing.T) {
	for i, orientation := range []RingOrientation{
		RingOrientationClockwise,
		RingOrientationCounterClockwise,
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			p := NewPolygon(XYZ).MustSetCoords([][]Coord{
				{{0, 0, 1}, {4, 0, 2}, {4, 4, 3}, {0, 4, 4}, {0, 0, 1}},
				{{1, 1, 5}, {3, 1, 6}, {3, 3, 7}, {1, 3, 8}, {1, 1, 5}},
				{{1, 1, 9}, {1, 3, 10}, {3, 3, 11}, {3, 1, 12}, {1, 1, 9}},
			})
			first, err := OrientPolygon(p, orientation)
			assert.NoError(t, err)
			second, err := OrientPolygon(first, orientation)
			assert.NoError(t, err)
			assert.Equal(t, first.FlatCoords(), second.FlatCoords())
			assert.Equal(t, first.Ends(), second.Ends())
			assert.Equal(t, first.Layout(), second.Layout())
			assert.Equal(t, first.SRID(), second.SRID())

			// Degenerate rings are also stable across repeated calls.
			degenerate, err := OrientPolygon(NewPolygon(XY).MustSetCoords([][]Coord{
				{{0, 0}, {1, 1}, {2, 2}},
			}), orientation)
			assert.NoError(t, err)
			assert.Equal(t, NewPolygon(XY).MustSetCoords([][]Coord{
				{{0, 0}, {1, 1}, {2, 2}},
			}), degenerate)
		})
	}
}

func TestOrientDoesNotModifySelfIntersectingInput(t *testing.T) {
	// A figure eight intersects itself and has zero signed area. It is
	// degenerate, so it must be copied unchanged rather than repaired.
	p := NewPolygon(XY).MustSetCoords([][]Coord{
		{{0, 0}, {2, 2}, {2, 0}, {0, 2}, {0, 0}},
	})
	orig := p.Clone()
	got, err := OrientPolygon(p, RingOrientationClockwise)
	assert.NoError(t, err)
	assert.Equal(t, orig, p)
	assert.Equal(t, orig, got)

	// A self-intersecting ring with non-zero signed area is reversed to
	// satisfy the declared orientation, but is not repaired: the set of
	// vertices is unchanged and only their order differs.
	p2 := NewPolygon(XY).MustSetCoords([][]Coord{
		{{0, 0}, {4, 0}, {1, 3}, {3, 3}, {0, 0}},
	})
	orig2 := p2.Clone()
	got2, err := OrientPolygon(p2, RingOrientationClockwise)
	assert.NoError(t, err)
	assert.Equal(t, orig2, p2)
	assert.Equal(t, len(p2.FlatCoords()), len(got2.FlatCoords()))
	assert.Equal(t, reverseFlatCoords(orig2.FlatCoords(), orig2.Stride()), got2.FlatCoords())
}

func reverseFlatCoords(flatCoords []float64, stride int) []float64 {
	reversed := make([]float64, 0, len(flatCoords))
	for i := len(flatCoords) - stride; i >= 0; i -= stride {
		reversed = append(reversed, flatCoords[i:i+stride]...)
	}
	return reversed
}

func TestOrientMultiPolygon(t *testing.T) {
	mp := NewMultiPolygon(XY).MustSetCoords([][][]Coord{
		{
			{{0, 0}, {2, 0}, {2, 2}, {0, 2}, {0, 0}},
			{{0, 0}, {0, 1}, {1, 1}, {1, 0}, {0, 0}},
		},
		{},
		{
			{{0, 0}, {0, 3}, {3, 3}, {3, 0}, {0, 0}},
		},
		{},
		{
			{{0, 0}, {1, 0}, {0, 1}, {0, 0}},
		},
	}).SetSRID(3857)
	orig := mp.Clone()

	got, err := OrientMultiPolygon(mp, RingOrientationCounterClockwise)
	assert.NoError(t, err)
	assert.NoError(t, got.verify())
	assert.Equal(t, 3857, got.SRID())
	assert.Equal(t, mp.NumPolygons(), got.NumPolygons())
	assert.Equal(t, orig, mp)
	assert.False(t, aliases(mp.FlatCoords(), got.FlatCoords()))

	want := NewMultiPolygon(XY).MustSetCoords([][][]Coord{
		{
			{{0, 0}, {2, 0}, {2, 2}, {0, 2}, {0, 0}},
			{{0, 0}, {0, 1}, {1, 1}, {1, 0}, {0, 0}},
		},
		{},
		{
			{{0, 0}, {3, 0}, {3, 3}, {0, 3}, {0, 0}},
		},
		{},
		{
			{{0, 0}, {1, 0}, {0, 1}, {0, 0}},
		},
	}).SetSRID(3857)
	assert.Equal(t, want, got)
	assert.Equal(t, want.Endss(), got.Endss())

	for i := range got.NumPolygons() {
		for j := range got.Polygon(i).NumLinearRings() {
			o := RingOrientationOfLinearRing(got.Polygon(i).LinearRing(j))
			if !o.IsDefined() {
				continue
			}
			if j == 0 {
				assert.Equal(t, RingOrientationCounterClockwise, o)
			} else {
				assert.Equal(t, RingOrientationClockwise, o)
			}
		}
	}

	// Repeated conversion is idempotent.
	gotAgain, err := OrientMultiPolygon(got, RingOrientationCounterClockwise)
	assert.NoError(t, err)
	assert.Equal(t, got, gotAgain)

	// The empty multipolygon stays empty and keeps its nil endss.
	empty := NewMultiPolygon(XY)
	emptyGot, err := OrientMultiPolygon(empty, RingOrientationClockwise)
	assert.NoError(t, err)
	assert.Equal(t, empty, emptyGot)
	assert.Equal(t, empty.Endss(), emptyGot.Endss())
}

func TestOrientGeometryCollection(t *testing.T) {
	ccwSquare := [][]Coord{
		{{0, 0}, {1, 0}, {1, 1}, {0, 1}, {0, 0}},
	}
	cwSquare := [][]Coord{
		{{0, 0}, {0, 1}, {1, 1}, {1, 0}, {0, 0}},
	}

	polygon := NewPolygon(XY).MustSetCoords(ccwSquare).SetSRID(4326)
	multiPolygon := NewMultiPolygon(XY).MustSetCoords([][][]Coord{cwSquare, {}}).SetSRID(4326)
	point := NewPoint(XY).MustSetCoords(Coord{1, 2})
	lineString := NewLineString(XY).MustSetCoords([]Coord{{0, 0}, {1, 1}})

	gc := NewGeometryCollection().
		MustPush(polygon, multiPolygon, point, lineString).
		MustSetLayout(XY).
		SetSRID(4326)
	orig := NewGeometryCollection().
		MustPush(polygon.Clone(), multiPolygon.Clone(), point, lineString).
		MustSetLayout(XY).
		SetSRID(4326)

	got, err := OrientGeometryCollection(gc, RingOrientationCounterClockwise)
	assert.NoError(t, err)
	assert.Equal(t, gc.Layout(), got.Layout())
	assert.Equal(t, 4326, got.SRID())
	assert.Equal(t, gc.NumGeoms(), got.NumGeoms())
	assert.Equal(t, orig, gc)
	assert.Equal(t, orig.Geoms()[2], got.Geoms()[2])
	assert.Equal(t, orig.Geoms()[3], got.Geoms()[3])

	gotPolygon, ok := got.Geom(0).(*Polygon)
	assert.True(t, ok)
	assert.Equal(t, NewPolygon(XY).MustSetCoords(ccwSquare).SetSRID(4326), gotPolygon)
	gotMultiPolygon, ok := got.Geom(1).(*MultiPolygon)
	assert.True(t, ok)
	assert.Equal(t, NewMultiPolygon(XY).MustSetCoords([][][]Coord{ccwSquare, {}}).SetSRID(4326), gotMultiPolygon)

	// Repeated conversion is idempotent.
	gotAgain, err := OrientGeometryCollection(got, RingOrientationCounterClockwise)
	assert.NoError(t, err)
	assert.Equal(t, got, gotAgain)

	// An empty collection remains empty.
	emptyGot, err := OrientGeometryCollection(NewGeometryCollection(), RingOrientationClockwise)
	assert.NoError(t, err)
	assert.Equal(t, NewGeometryCollection(), emptyGot)
}

func TestOrientNonArealGeometries(t *testing.T) {
	for i, g := range []T{
		NewPoint(XY),
		NewLineString(XY),
		NewMultiPoint(XY),
		NewMultiLineString(XY),
		NewLinearRing(XY),
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			got, err := Orient(g, RingOrientationClockwise)
			assert.NoError(t, err)
			assert.Equal(t, g, got)
		})
	}
}

func TestOrientReturnsNonArealGeometryUnchanged(t *testing.T) {
	point := NewPoint(XY).MustSetCoords(Coord{1, 2})
	got, err := Orient(point, RingOrientationClockwise)
	assert.NoError(t, err)
	assert.True(t, point == got)
}

func TestOrientClosingEndpointAppearsOnceInReversal(t *testing.T) {
	// A closed ring repeats its first coordinate at the end. Reversing the
	// ring must neither drop nor duplicate that coordinate: the reversed ring
	// is still closed and has the same number of coordinates.
	for i, orientation := range []RingOrientation{
		RingOrientationClockwise,
		RingOrientationCounterClockwise,
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			ring := NewLinearRing(XY).MustSetCoords([]Coord{
				{0, 0},
				{2, 0},
				{2, 2},
				{0, 2},
				{0, 0},
			})
			got, err := OrientLinearRing(ring, orientation)
			assert.NoError(t, err)
			gotCoords := got.Coords()
			assert.Equal(t, len(ring.Coords()), len(gotCoords))
			assert.Equal(t, gotCoords[0], gotCoords[len(gotCoords)-1])
			assert.Equal(t, ring.Coord(0), got.Coord(len(gotCoords)-1))
			assert.Equal(t, ring.Coord(len(ring.Coords())-1), got.Coord(0))
		})
	}
}

func TestOrientUnclosedRing(t *testing.T) {
	// LinearRings are not required to be closed by go-geom's representation,
	// so an unclosed ring must still be reversed correctly.
	ring := NewLinearRing(XY).MustSetCoords([]Coord{
		{0, 0},
		{1, 0},
		{0, 1},
	})
	assert.Equal(t, RingOrientationCounterClockwise, RingOrientationOfLinearRing(ring))
	got, err := OrientLinearRing(ring, RingOrientationClockwise)
	assert.NoError(t, err)
	assert.Equal(t, []Coord{{0, 1}, {1, 0}, {0, 0}}, got.Coords())
	assert.Equal(t, RingOrientationClockwise, RingOrientationOfLinearRing(got))
}

func TestOrientNoLayout(t *testing.T) {
	// A geometry with no layout has a stride of zero. Nothing can be
	// interpreted, so the ring is treated as degenerate and copied unchanged.
	ring := NewLinearRing(NoLayout)
	got, err := OrientLinearRing(ring, RingOrientationClockwise)
	assert.NoError(t, err)
	assert.Equal(t, ring, got)

	polygon := NewPolygon(NoLayout)
	gotPolygon, err := OrientPolygon(polygon, RingOrientationClockwise)
	assert.NoError(t, err)
	assert.Equal(t, polygon, gotPolygon)
	assert.Equal(t, RingOrientationEmpty, RingOrientationOf(NoLayout, nil))
}

func TestOrientNestedGeometryCollection(t *testing.T) {
	inner := NewGeometryCollection().
		MustPush(NewPolygon(XY).MustSetCoords([][]Coord{
			{{0, 0}, {1, 0}, {1, 1}, {0, 1}, {0, 0}},
			{{0, 0}, {0, 1}, {1, 1}, {1, 0}, {0, 0}},
		}))
	outer := NewGeometryCollection().MustPush(inner)
	got, err := OrientGeometryCollection(outer, RingOrientationClockwise)
	assert.NoError(t, err)
	gotInner, ok := got.Geom(0).(*GeometryCollection)
	assert.True(t, ok)
	gotPolygon, ok := gotInner.Geom(0).(*Polygon)
	assert.True(t, ok)
	assert.Equal(t, RingOrientationClockwise, ringOrientationForPolygon(t, gotPolygon, 0))
	assert.Equal(t, RingOrientationCounterClockwise, ringOrientationForPolygon(t, gotPolygon, 1))
}

func TestOrientEmptyRings(t *testing.T) {
	// A polygon with empty rings keeps the same number of rings and the same
	// end offsets. Empty rings have no orientation, so they are copied as-is.
	polygon := NewPolygon(XY).MustSetCoords([][]Coord{{}, {}})
	got, err := OrientPolygon(polygon, RingOrientationCounterClockwise)
	assert.NoError(t, err)
	assert.Equal(t, polygon, got)
	assert.Equal(t, []int{0, 0}, got.Ends())
	assert.Equal(t, RingOrientationEmpty, RingOrientationOfLinearRing(got.LinearRing(0)))
	assert.Equal(t, RingOrientationEmpty, RingOrientationOfLinearRing(got.LinearRing(1)))

	// The nil-ness of the ends slice is preserved.
	assert.Zero(t, NewPolygon(XY).Ends())
	emptyPolygon, err := OrientPolygon(NewPolygon(XY), RingOrientationCounterClockwise)
	assert.NoError(t, err)
	assert.Zero(t, emptyPolygon.Ends())
	assert.True(t, emptyPolygon.FlatCoords() == nil)

	explicitlyEmpty, err := OrientPolygon(NewPolygonFlat(XY, nil, []int{}), RingOrientationCounterClockwise)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(explicitlyEmpty.Ends()))
	assert.False(t, explicitlyEmpty.Ends() == nil)
	assert.True(t, explicitlyEmpty.FlatCoords() == nil)

	// The same applies to a multipolygon containing empty polygons.
	mp := NewMultiPolygon(XY).MustSetCoords([][][]Coord{{}, {}})
	gotMP, err := OrientMultiPolygon(mp, RingOrientationCounterClockwise)
	assert.NoError(t, err)
	assert.Equal(t, mp, gotMP)
	assert.Equal(t, mp.Endss(), gotMP.Endss())
	assert.Equal(t, 2, gotMP.NumPolygons())
	assert.True(t, gotMP.FlatCoords() == nil)
}

func TestOrientDegenerateExteriorDoesNotReassignRings(t *testing.T) {
	// The first ring is always treated as the exterior ring, even when it is
	// degenerate and cannot be oriented. The second ring stays a hole, and the
	// ring order is preserved.
	polygon := NewPolygon(XY).MustSetCoords([][]Coord{
		{{0, 0}, {1, 1}, {2, 2}},
		{{0, 0}, {4, 0}, {4, 4}, {0, 4}, {0, 0}},
	})
	orig := polygon.Clone()
	got, err := OrientPolygon(polygon, RingOrientationCounterClockwise)
	assert.NoError(t, err)
	assert.Equal(t, orig, polygon)
	assert.Equal(t, 2, got.NumLinearRings())
	assert.Equal(t, RingOrientationDegenerate, RingOrientationOfLinearRing(got.LinearRing(0)))
	assert.Equal(t, []Coord{{0, 0}, {1, 1}, {2, 2}}, got.LinearRing(0).Coords())
	// The second ring is a hole, so it must be oriented clockwise.
	assert.Equal(t, RingOrientationClockwise, RingOrientationOfLinearRing(got.LinearRing(1)))
}

func ExampleOrient() {
	g := NewGeometryCollection().MustPush(
		NewPolygon(XY).MustSetCoords([][]Coord{
			{{0, 0}, {1, 0}, {1, 1}, {0, 1}, {0, 0}},
			{{0, 0}, {0, 1}, {1, 1}, {1, 0}, {0, 0}},
		}),
		NewPoint(XY).MustSetCoords(Coord{0, 0}),
	)
	oriented, err := Orient(g, RingOrientationClockwise)
	if err != nil {
		fmt.Println(err)
		return
	}
	orientedCollection, ok := oriented.(*GeometryCollection)
	if !ok {
		fmt.Println("unexpected geometry type")
		return
	}
	polygon, ok := orientedCollection.Geom(0).(*Polygon)
	if !ok {
		fmt.Println("unexpected geometry type")
		return
	}
	fmt.Println(RingOrientationOfLinearRing(polygon.LinearRing(0)))
	fmt.Println(RingOrientationOfLinearRing(polygon.LinearRing(1)))
	point, ok := orientedCollection.Geom(1).(*Point)
	if !ok {
		fmt.Println("unexpected geometry type")
		return
	}
	fmt.Println(point.Coords())
	// Output:
	// Clockwise
	// CounterClockwise
	// [0 0]
}

func TestOrientInvalidOrientationReturnsNil(t *testing.T) {
	got, err := Orient(NewPolygon(XY), RingOrientationUnknown)
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidRingOrientation, err)
	assert.Zero(t, got)
}

func TestOrientAreaSign(t *testing.T) {
	// Orienting changes the signed area sign, not its magnitude.
	p := NewPolygon(XY).MustSetCoords([][]Coord{
		{{0, 0}, {2, 0}, {2, 2}, {0, 2}, {0, 0}},
		{{0, 0}, {0, 1}, {1, 1}, {1, 0}, {0, 0}},
	})
	cw, err := OrientPolygon(p, RingOrientationClockwise)
	assert.NoError(t, err)
	ccw, err := OrientPolygon(p, RingOrientationCounterClockwise)
	assert.NoError(t, err)
	assert.Equal(t, p.Area(), -cw.Area())
	assert.Equal(t, p.Area(), ccw.Area())
	assert.Equal(t, p.Length(), cw.Length())
	assert.Equal(t, p.Length(), ccw.Length())
}

func ExampleOrientPolygon() {
	ccw := NewPolygon(XY).MustSetCoords([][]Coord{
		{{0, 0}, {4, 0}, {4, 4}, {0, 4}, {0, 0}},
		{{1, 1}, {3, 1}, {3, 3}, {1, 3}, {1, 1}},
	})
	cw, err := OrientPolygon(ccw, RingOrientationClockwise)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(RingOrientationOfLinearRing(cw.LinearRing(0)))
	fmt.Println(RingOrientationOfLinearRing(cw.LinearRing(1)))
	fmt.Println(ccw.LinearRing(0).FlatCoords())
	fmt.Println(cw.LinearRing(0).FlatCoords())
	// Output:
	// Clockwise
	// CounterClockwise
	// [0 0 4 0 4 4 0 4 0 0]
	// [0 0 0 4 4 4 4 0 0 0]
}

func BenchmarkOrientPolygon(b *testing.B) {
	p := NewPolygon(XY).MustSetCoords([][]Coord{
		{{0, 0}, {4, 0}, {4, 4}, {0, 4}, {0, 0}},
		{{1, 1}, {3, 1}, {3, 3}, {1, 3}, {1, 1}},
	})
	b.ResetTimer()
	for range b.N {
		if _, err := OrientPolygon(p, RingOrientationClockwise); err != nil {
			b.Fatal(err)
		}
	}
}
