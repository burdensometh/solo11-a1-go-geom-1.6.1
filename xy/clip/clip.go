// Package clip clips lines to an axis-aligned rectangle in the XY plane.
//
// The typical use case is map display and map-based analysis, where only the
// parts of a trajectory that fall inside the current window are of interest.
// A line that leaves and re-enters the window is clipped into multiple
// pieces, and pieces are never connected across a gap: two parts of the
// input that are not contiguous are never joined into a single piece.
//
// Clipping is exact, not sampling-based: intersection points are computed
// parametrically from the original segments with the Liang-Barsky algorithm.
// The following deterministic rules apply:
//
//   - The rectangle boundary is inclusive. Points and segments lying exactly
//     on the boundary, including segments coincident with an edge of the
//     rectangle, are kept.
//   - Intersection points are computed by linear interpolation along the
//     original segment, and all ordinates, including Z and M, are
//     interpolated with the same parameter. Original vertices are copied
//     verbatim, without rounding.
//   - A segment that merely touches the rectangle in a single point, for
//     example one that cuts across a corner or touches an edge tangentially,
//     produces no output of its own.
//   - A zero-length segment (two consecutive vertices with identical X and
//     Y) is kept if and only if its point lies inside or on the rectangle.
//     When kept, both of its vertices are preserved, including their Z and M
//     values.
//   - Pieces are emitted in the order in which they occur along the input,
//     and the coordinates of each piece keep the original direction of
//     travel.
//
// The input geometry is never modified and the output never aliases the
// input's coordinate data.
package clip

import (
	"math"

	"github.com/twpayne/go-geom"
)

// A Rect is an axis-aligned rectangle in the XY plane. Its boundary is
// inclusive: points on the boundary are considered to be inside.
type Rect struct {
	MinX, MinY, MaxX, MaxY float64
}

// NewRect returns a Rect with the given corner coordinates, normalized so
// that MinX <= MaxX and MinY <= MaxY.
func NewRect(x1, y1, x2, y2 float64) Rect {
	return Rect{
		MinX: math.Min(x1, x2),
		MinY: math.Min(y1, y2),
		MaxX: math.Max(x1, x2),
		MaxY: math.Max(y1, y2),
	}
}

// RectFromBounds returns the rectangle covered by the first two dimensions
// of b.
func RectFromBounds(b *geom.Bounds) Rect {
	return Rect{
		MinX: b.Min(0),
		MinY: b.Min(1),
		MaxX: b.Max(0),
		MaxY: b.Max(1),
	}
}

// A SegmentRef identifies a segment of a clipped input geometry: the
// Segment'th segment of the LineString'th LineString of the input. For
// LineString input, LineString is always zero.
type SegmentRef struct {
	LineString int
	Segment    int
}

// A Piece is one maximal connected part of a clipped line.
type Piece struct {
	// LineString is the preserved part of the input, in the original
	// direction of travel. Its first and last vertices are original input
	// vertices if they lie inside or on the rectangle, and computed
	// intersection points otherwise.
	LineString *geom.LineString

	// Source identifies the input segment that produced each segment of
	// LineString: Source[i] is the source of the segment from vertex i to
	// vertex i+1, so len(Source) == LineString.NumCoords()-1.
	Source []SegmentRef
}

// Pieces is the result of clipping a line to a rectangle: the preserved
// parts of the input, in the order in which they occur along it.
type Pieces []Piece

// MultiLineString collects ps into a single *geom.MultiLineString,
// discarding the source segment information. layout is only used when ps is
// empty; otherwise the layout and SRID of the pieces are used.
func (ps Pieces) MultiLineString(layout geom.Layout) *geom.MultiLineString {
	if len(ps) == 0 {
		return geom.NewMultiLineString(layout)
	}
	var flatCoords []float64
	ends := make([]int, 0, len(ps))
	for _, piece := range ps {
		flatCoords = append(flatCoords, piece.LineString.FlatCoords()...)
		ends = append(ends, len(flatCoords))
	}
	mls := geom.NewMultiLineStringFlat(ps[0].LineString.Layout(), flatCoords, ends)
	mls.SetSRID(ps[0].LineString.SRID())
	return mls
}

// LineString clips ls to the axis-aligned rectangle r and returns the
// preserved pieces in the order in which they occur along ls. It returns nil
// if no part of ls lies inside or on r. ls is not modified.
func LineString(ls *geom.LineString, r Rect) Pieces {
	c := clipper{
		rect:   r,
		layout: ls.Layout(),
		stride: ls.Stride(),
	}
	c.clipLineString(ls.FlatCoords(), 0)
	return c.finish(ls.SRID())
}

// MultiLineString clips each LineString of mls to the axis-aligned rectangle
// r and returns the preserved pieces in the order in which they occur along
// mls. Pieces from different LineStrings of mls are never merged, even if
// their endpoints coincide. It returns nil if no part of mls lies inside or
// on r. mls is not modified.
func MultiLineString(mls *geom.MultiLineString, r Rect) Pieces {
	c := clipper{
		rect:   r,
		layout: mls.Layout(),
		stride: mls.Stride(),
	}
	flatCoords := mls.FlatCoords()
	offset := 0
	for i, end := range mls.Ends() {
		c.clipLineString(flatCoords[offset:end], i)
		offset = end
	}
	return c.finish(mls.SRID())
}

// A clipper accumulates the pieces produced by clipping one or more
// polylines to a rectangle.
type clipper struct {
	rect   Rect
	layout geom.Layout
	stride int

	pieces     []Piece
	flatCoords []float64    // flat coordinates of the current piece
	source     []SegmentRef // source segments of the current piece
	open       bool         // whether the current piece ends exactly at the last processed vertex
}

// clipLineString clips the polyline with the given flat coordinates to the
// rectangle, appending the preserved pieces to c.pieces. lineStringIndex
// identifies the polyline within the input geometry for source tracking.
func (c *clipper) clipLineString(flatCoords []float64, lineStringIndex int) {
	if c.stride == 0 {
		return
	}
	n := len(flatCoords) / c.stride
	for i := 0; i+1 < n; i++ {
		p0 := flatCoords[i*c.stride : (i+1)*c.stride]
		p1 := flatCoords[(i+1)*c.stride : (i+2)*c.stride]
		t0, t1, ok := clipSegment(p0[0], p0[1], p1[0], p1[1], c.rect)
		if !ok {
			// The segment lies entirely outside the rectangle. The current
			// piece, if any, ends here and must not be connected to
			// whatever comes next.
			c.open = false
			continue
		}
		if !c.open || t0 != 0 {
			// The segment is not contiguous with the current piece: either
			// there is no current piece, or the line left the rectangle
			// between the two. Start a new piece at the entry point.
			c.startPiece(c.pointAt(p0, p1, t0))
		}
		if t1 > t0 {
			c.flatCoords = append(c.flatCoords, c.pointAt(p0, p1, t1)...)
			c.source = append(c.source, SegmentRef{LineString: lineStringIndex, Segment: i})
		}
		// The next segment can only extend the current piece if this
		// segment's end vertex lies inside or on the rectangle, in which
		// case it was appended verbatim above.
		c.open = t1 == 1
	}
	c.finishPiece()
}

// clipSegment returns the parametric interval [t0, t1] over which the
// segment from (x0, y0) to (x1, y1) lies inside or on r, computed with the
// Liang-Barsky algorithm. ok is false if the segment lies entirely outside
// r. The boundary is inclusive: a segment coincident with an edge of r is
// kept, and a segment touching r in a single point is kept with t0 == t1.
func clipSegment(x0, y0, x1, y1 float64, r Rect) (t0, t1 float64, ok bool) {
	dx := x1 - x0
	dy := y1 - y0
	p := [4]float64{-dx, dx, -dy, dy}
	q := [4]float64{x0 - r.MinX, r.MaxX - x0, y0 - r.MinY, r.MaxY - y0}
	t0, t1 = 0, 1
	for i := range 4 {
		if p[i] == 0 {
			// The segment is parallel to this edge of r. It is only
			// rejected if it lies strictly outside this edge.
			if q[i] < 0 {
				return 0, 0, false
			}
			continue
		}
		t := q[i] / p[i]
		if p[i] < 0 {
			if t > t1 {
				return 0, 0, false
			}
			if t > t0 {
				t0 = t
			}
		} else {
			if t < t0 {
				return 0, 0, false
			}
			if t < t1 {
				t1 = t
			}
		}
	}
	return t0, t1, true
}

// pointAt returns the point at parameter t along the segment from p0 to p1.
// All ordinates, including Z and M, are linearly interpolated with t. t == 0
// and t == 1 return verbatim copies of the segment's endpoints, so original
// vertices are never rounded. The X and Y of interpolated points are clamped
// to the rectangle to absorb floating-point error.
func (c *clipper) pointAt(p0, p1 []float64, t float64) []float64 {
	p := make([]float64, c.stride)
	switch t {
	case 0:
		copy(p, p0)
	case 1:
		copy(p, p1)
	default:
		for i := range c.stride {
			p[i] = p0[i] + t*(p1[i]-p0[i])
		}
		p[0] = math.Min(math.Max(p[0], c.rect.MinX), c.rect.MaxX)
		p[1] = math.Min(math.Max(p[1], c.rect.MinY), c.rect.MaxY)
	}
	return p
}

// startPiece finishes the current piece, if any, and starts a new piece at
// p, which must not be aliased afterwards.
func (c *clipper) startPiece(p []float64) {
	c.finishPiece()
	c.flatCoords = p
	c.source = nil
}

// finishPiece appends the current piece to the result if it contains at
// least one segment. Pieces that degenerate to a single point, produced by
// segments that merely touch the rectangle, are discarded.
func (c *clipper) finishPiece() {
	if len(c.flatCoords) >= 2*c.stride {
		c.pieces = append(c.pieces, Piece{
			LineString: geom.NewLineStringFlat(c.layout, c.flatCoords),
			Source:     c.source,
		})
	}
	c.flatCoords = nil
	c.source = nil
	c.open = false
}

// finish sets the SRID on all pieces and returns them.
func (c *clipper) finish(srid int) Pieces {
	for i := range c.pieces {
		c.pieces[i].LineString.SetSRID(srid)
	}
	return c.pieces
}
