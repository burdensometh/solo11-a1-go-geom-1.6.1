package geom_test

import (
	"strconv"
	"testing"

	"github.com/alecthomas/assert/v2"

	"github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/xy"
)

// TestRingOrientationMatchesXY checks that geom's layout-driven orientation
// calculation agrees with xy.IsRingCounterClockwise for valid rings, including
// rings with Z, M, and extra ordinates.
func TestRingOrientationMatchesXY(t *testing.T) {
	for i, tc := range []struct {
		layout geom.Layout
		ring   []float64
	}{
		{
			layout: geom.XY,
			ring:   []float64{0, 0, 4, 0, 4, 4, 0, 4, 0, 0},
		},
		{
			layout: geom.XY,
			ring:   []float64{0, 0, 0, 4, 4, 4, 4, 0, 0, 0},
		},
		{
			layout: geom.XYZ,
			ring: []float64{
				0, 0, 1,
				4, 0, 2,
				4, 4, 3,
				0, 4, 4,
				0, 0, 1,
			},
		},
		{
			layout: geom.XYZM,
			ring: []float64{
				0, 0, 1, 1,
				4, 0, 2, 2,
				4, 4, 3, 3,
				0, 4, 4, 4,
				0, 0, 1, 1,
			},
		},
		{
			layout: geom.Layout(5),
			ring: []float64{
				0, 0, 1, 2, 3,
				4, 0, 4, 5, 6,
				4, 4, 7, 8, 9,
				0, 4, 10, 11, 12,
				0, 0, 1, 2, 3,
			},
		},
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			want := geom.RingOrientationClockwise
			if xy.IsRingCounterClockwise(tc.layout, tc.ring) {
				want = geom.RingOrientationCounterClockwise
			}
			assert.Equal(t, want, geom.RingOrientationOf(tc.layout, tc.ring))
		})
	}
}

// TestOrientPreservesZAndM checks that orienting a polygon keeps every ordinate
// attached to its vertex and does not modify the receiver.
func TestOrientPreservesZAndM(t *testing.T) {
	original := [][]geom.Coord{
		{{0, 0, 1, 10}, {2, 0, 2, 20}, {2, 2, 3, 30}, {0, 2, 4, 40}, {0, 0, 1, 10}},
		{{1, 1, 5, 50}, {1, 3, 6, 60}, {3, 3, 7, 70}, {3, 1, 8, 80}, {1, 1, 5, 50}},
	}
	p := geom.NewPolygon(geom.XYZM).MustSetCoords(original)

	got, err := geom.OrientPolygon(p, geom.RingOrientationClockwise)
	assert.NoError(t, err)
	assert.Equal(t, geom.XYZM, got.Layout())
	assert.Equal(t, 4, got.Stride())
	assert.Equal(t, [][]geom.Coord{
		{{0, 0, 1, 10}, {0, 2, 4, 40}, {2, 2, 3, 30}, {2, 0, 2, 20}, {0, 0, 1, 10}},
		{{1, 1, 5, 50}, {3, 1, 8, 80}, {3, 3, 7, 70}, {1, 3, 6, 60}, {1, 1, 5, 50}},
	}, got.Coords())
	assert.Equal(t, original, p.Coords())
}
