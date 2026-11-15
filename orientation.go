package geom

import (
	"errors"
	"fmt"
	"math"
)

// A RingOrientation describes the winding order of a linear ring in the XY
// plane.
type RingOrientation int

const (
	// RingOrientationUnknown is the zero value and indicates that no
	// orientation has been determined.
	RingOrientationUnknown RingOrientation = iota
	// RingOrientationEmpty indicates that the ring has no coordinates.
	RingOrientationEmpty
	// RingOrientationDegenerate indicates that the ring has fewer than three
	// coordinates, malformed coordinates, or encloses zero area, so its
	// winding order is undefined.
	RingOrientationDegenerate
	// RingOrientationClockwise indicates that the ring is wound clockwise
	// when viewed in the XY plane with the X axis pointing right and the Y
	// axis pointing up.
	RingOrientationClockwise
	// RingOrientationCounterClockwise indicates that the ring is wound
	// counter-clockwise when viewed in the XY plane with the X axis pointing
	// right and the Y axis pointing up.
	RingOrientationCounterClockwise
)

// ErrInvalidRingOrientation is returned by Orient and related functions when
// the requested orientation is neither RingOrientationClockwise nor
// RingOrientationCounterClockwise.
var ErrInvalidRingOrientation = errors.New("geom: invalid ring orientation")

// IsDefined reports whether o is RingOrientationClockwise or
// RingOrientationCounterClockwise.
func (o RingOrientation) IsDefined() bool {
	return o == RingOrientationClockwise || o == RingOrientationCounterClockwise
}

// Opposite returns the opposite defined orientation of o. It returns o
// unchanged if o is not a defined orientation.
func (o RingOrientation) Opposite() RingOrientation {
	switch o {
	case RingOrientationClockwise:
		return RingOrientationCounterClockwise
	case RingOrientationCounterClockwise:
		return RingOrientationClockwise
	default:
		return o
	}
}

// String returns a human-readable representation of o.
func (o RingOrientation) String() string {
	switch o {
	case RingOrientationUnknown:
		return "Unknown"
	case RingOrientationEmpty:
		return "Empty"
	case RingOrientationDegenerate:
		return "Degenerate"
	case RingOrientationClockwise:
		return "Clockwise"
	case RingOrientationCounterClockwise:
		return "CounterClockwise"
	default:
		return fmt.Sprintf("RingOrientation(%d)", int(o))
	}
}

// RingOrientationOf returns the winding order of the ring described by
// flatCoords with the given layout. Only the X and Y ordinates are considered,
// so Z, M, and any extra ordinates do not affect the result.
//
// The stride is derived from layout, never from len(flatCoords). This matters
// for rings with a layout whose stride divides the length of the coordinate
// slice in more than one way, for example an XYZ ring with four coordinates.
//
// RingOrientationEmpty is returned when flatCoords is empty.
// RingOrientationDegenerate is returned when flatCoords does not contain a
// whole number of coordinates, contains fewer than three coordinates, contains
// a non-finite ordinate, or encloses zero area (for example, when all its
// coordinates are collinear). Unlike [xy.IsRingCounterClockwise],
// RingOrientationOf never panics.
func RingOrientationOf(layout Layout, flatCoords []float64) RingOrientation {
	return ringOrientation(flatCoords, layout.Stride())
}

// RingOrientationOfLinearRing returns the winding order of g.
func RingOrientationOfLinearRing(g *LinearRing) RingOrientation {
	return RingOrientationOf(g.Layout(), g.FlatCoords())
}

// Orient returns a new geometry with its areal rings oriented as requested.
//
// For a Polygon, the exterior ring is given orientation and each hole is
// given the opposite orientation. MultiPolygons and GeometryCollections are
// handled recursively. A LinearRing is treated as an exterior ring. Other
// geometry types are returned unchanged and are not copied.
//
// The original geometry is never modified. Rings that already have the
// requested orientation are copied without being reversed, so Orient is
// idempotent and preserves vertex identity.
//
// Orient is not a validity fixer. It never reorders, removes, or adds rings, so
// a hole remains a hole of the same polygon and a self-intersecting ring stays
// self-intersecting. A ring is reversed only when its signed area says that it
// has the opposite orientation. Empty and degenerate rings, whose orientation
// is undefined, are copied unchanged.
//
// The SRID, the layout, and every ordinate of every coordinate, including Z
// and M, are preserved.
//
// An error is returned if orientation is not RingOrientationClockwise or
// RingOrientationCounterClockwise.
func Orient(g T, orientation RingOrientation) (T, error) {
	if !orientation.IsDefined() {
		return nil, ErrInvalidRingOrientation
	}
	return orient(g, orientation), nil
}

// orient returns a new geometry with its areal rings oriented as requested.
// orientation must be a defined orientation.
func orient(g T, orientation RingOrientation) T {
	switch g := g.(type) {
	case *LinearRing:
		return Must(OrientLinearRing(g, orientation))
	case *Polygon:
		return Must(OrientPolygon(g, orientation))
	case *MultiPolygon:
		return Must(OrientMultiPolygon(g, orientation))
	case *GeometryCollection:
		return Must(OrientGeometryCollection(g, orientation))
	default:
		return g
	}
}

// OrientLinearRing returns a copy of g with the given orientation. The
// original ring is not modified. An empty or degenerate ring is copied
// unchanged. An error is returned if orientation is not a defined
// orientation.
func OrientLinearRing(g *LinearRing, orientation RingOrientation) (*LinearRing, error) {
	if !orientation.IsDefined() {
		return nil, ErrInvalidRingOrientation
	}

	var flatCoords []float64
	if g.flatCoords != nil {
		flatCoords = make([]float64, 0, len(g.flatCoords))
	}
	flatCoords = appendOrientedRing(flatCoords, g.flatCoords, g.Layout(), orientation)

	return NewLinearRingFlat(g.Layout(), flatCoords).SetSRID(g.SRID()), nil
}

// OrientPolygon returns a copy of g with its exterior ring oriented to
// orientation and its holes oriented in the opposite direction. The original
// polygon is not modified. The number and order of the rings are preserved:
// rings are never reassigned as holes, and empty or degenerate rings are
// copied unchanged. An error is returned if orientation is not a defined
// orientation.
func OrientPolygon(g *Polygon, orientation RingOrientation) (*Polygon, error) {
	if !orientation.IsDefined() {
		return nil, ErrInvalidRingOrientation
	}

	var flatCoords []float64
	if g.flatCoords != nil {
		flatCoords = make([]float64, 0, len(g.flatCoords))
	}
	flatCoords, ends := appendOrientedRings(flatCoords, g.flatCoords, 0, g.ends, g.Layout(), orientation)

	return NewPolygonFlat(g.Layout(), flatCoords, ends).SetSRID(g.SRID()), nil
}

// OrientMultiPolygon returns a copy of g with the exterior ring of every
// Polygon oriented to orientation and every hole oriented in the opposite
// direction. The original multipolygon is not modified. The number and order
// of the polygons and their rings are preserved, including empty polygons and
// empty rings. An error is returned if orientation is not a defined
// orientation.
func OrientMultiPolygon(g *MultiPolygon, orientation RingOrientation) (*MultiPolygon, error) {
	if !orientation.IsDefined() {
		return nil, ErrInvalidRingOrientation
	}

	layout := g.Layout()
	var flatCoords []float64
	if g.flatCoords != nil {
		flatCoords = make([]float64, 0, len(g.flatCoords))
	}
	endss := make([][]int, len(g.endss))

	offset := 0
	for i, ends := range g.endss {
		var polygonEnds []int
		flatCoords, polygonEnds = appendOrientedRings(flatCoords, g.flatCoords, offset, ends, layout, orientation)
		if len(ends) > 0 {
			offset = ends[len(ends)-1]
		}
		endss[i] = polygonEnds
	}
	if g.endss == nil {
		endss = nil
	}

	return NewMultiPolygonFlat(layout, flatCoords, endss).SetSRID(g.SRID()), nil
}

// OrientGeometryCollection returns a copy of g in which every areal geometry
// has been oriented as described by [Orient]. The original collection is not
// modified. Non-areal geometries are returned unchanged and are not copied.
// The layout and SRID of the collection are preserved. An error is returned if
// orientation is not a defined orientation.
func OrientGeometryCollection(g *GeometryCollection, orientation RingOrientation) (*GeometryCollection, error) {
	if !orientation.IsDefined() {
		return nil, ErrInvalidRingOrientation
	}

	var geoms []T
	if g.geoms != nil {
		geoms = make([]T, len(g.geoms))
	}
	for i, geom := range g.geoms {
		geoms[i] = orient(geom, orientation)
	}

	return &GeometryCollection{
		layout: g.layout,
		geoms:  geoms,
		srid:   g.srid,
	}, nil
}

// appendOrientedRings appends the rings described by flatCoords[offset:end]
// for each end in ends to dst, orienting the first ring to orientation and
// the remaining rings to its opposite. It returns the extended dst and the new
// end offsets, which have the same nil-ness as ends.
func appendOrientedRings(
	dst, flatCoords []float64, offset int, ends []int, layout Layout, orientation RingOrientation,
) ([]float64, []int) {
	newEnds := make([]int, 0, len(ends))
	for i, end := range ends {
		o := orientation
		if i > 0 {
			o = orientation.Opposite()
		}
		dst = appendOrientedRing(dst, flatCoords[offset:end], layout, o)
		newEnds = append(newEnds, len(dst))
		offset = end
	}
	if ends == nil {
		return dst, nil
	}
	return dst, newEnds
}

// appendOrientedRing appends ring to dst, reversing it if its current
// orientation is the opposite of orientation. Rings with an undefined
// orientation are copied unchanged.
func appendOrientedRing(dst, ring []float64, layout Layout, orientation RingOrientation) []float64 {
	current := RingOrientationOf(layout, ring)
	if current == orientation || !current.IsDefined() {
		return append(dst, ring...)
	}
	return appendReversedRing(dst, ring, layout.Stride())
}

// appendReversedRing appends ring to dst in reverse coordinate order. Every
// ordinate of a coordinate is moved together, so Z and M values stay
// associated with their vertex. For a closed ring, the closing coordinate is
// part of the ring and is reversed like any other coordinate, which keeps the
// ring closed without duplicating the closing coordinate.
func appendReversedRing(dst, ring []float64, stride int) []float64 {
	for i := len(ring) - stride; i >= 0; i -= stride {
		dst = append(dst, ring[i:i+stride]...)
	}
	return dst
}

// ringOrientation returns the winding order of the ring described by
// flatCoords with coordinates of the given stride.
func ringOrientation(flatCoords []float64, stride int) RingOrientation {
	switch {
	case len(flatCoords) == 0:
		return RingOrientationEmpty
	case stride < 2 || len(flatCoords)%stride != 0:
		return RingOrientationDegenerate
	}

	numCoords := len(flatCoords) / stride
	if numCoords < 3 {
		return RingOrientationDegenerate
	}

	// Compute twice the signed area using the shoelace formula, translated to
	// the first coordinate to reduce cancellation. The implicit closing edge
	// contributes nothing when the coordinates are translated to the first
	// coordinate, so only the edges between consecutive coordinates are
	// summed. The result is positive for a counter-clockwise ring.
	x0, y0 := flatCoords[0], flatCoords[1]
	var doubleArea float64
	prevX, prevY := x0, y0
	for i := 1; i < numCoords; i++ {
		x, y := flatCoords[i*stride], flatCoords[i*stride+1]
		doubleArea += (prevX-x0)*(y-y0) - (x-x0)*(prevY-y0)
		prevX, prevY = x, y
	}

	switch {
	// A non-finite signed area means the winding order cannot be determined,
	// for example because a coordinate is infinite or NaN.
	case math.IsNaN(doubleArea) || math.IsInf(doubleArea, 0):
		return RingOrientationDegenerate
	case doubleArea > 0:
		return RingOrientationCounterClockwise
	case doubleArea < 0:
		return RingOrientationClockwise
	default:
		return RingOrientationDegenerate
	}
}
