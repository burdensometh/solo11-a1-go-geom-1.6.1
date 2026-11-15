// Package pointpolygonjoin performs indexed point-in-polygon joins.
//
// Sample points are assigned to regions and their values aggregated into
// count, sum, and mean statistics. Regions are [geom.Polygon]s or
// [geom.MultiPolygon]s, possibly with holes. A point in a hole is not
// contained in the region even though it lies inside the region's bounding
// box, because the R-tree is used only to select candidate regions and every
// candidate is still tested exactly with a ray-crossing algorithm.
//
// Overlapping regions can either match a point to all regions
// ([OverlapAll]) or to a single region chosen by priority
// ([OverlapUniquePriority]). Points that match no region are returned
// unchanged so their identity is preserved. Every region, including empty
// regions and regions with no matching points, appears in the results.
//
// The spatial index is a static R-tree. A point query descends only through
// nodes whose boxes contain the point, so each point is tested against a
// handful of candidate regions rather than every region.
package pointpolygonjoin

import (
	"errors"
	"fmt"
	"math"

	"github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/xy"
	"github.com/twpayne/go-geom/xy/location"
)

// OverlapMode controls how points are assigned when regions overlap.
type OverlapMode int

const (
	// OverlapAll associates a point with every region that contains it.
	OverlapAll OverlapMode = iota
	// OverlapUniquePriority associates a point with a single region. When
	// several regions contain the point, the region with the smallest Priority
	// wins; ties are broken deterministically by region index.
	OverlapUniquePriority
)

// BoundaryPolicy controls whether points on a region's boundary are
// considered contained.
type BoundaryPolicy int

const (
	// BoundaryInclude treats points on a region's boundary as contained.
	BoundaryInclude BoundaryPolicy = iota
	// BoundaryExclude treats points on a region's boundary as not contained.
	BoundaryExclude
)

// A Region is an indexed join target. Geometry must be either a
// [geom.Polygon] or a [geom.MultiPolygon].
type Region struct {
	// ID is returned unchanged in the results.
	ID any
	// Geometry is the polygon or multipolygon defining the region.
	Geometry geom.T
	// Priority is used by OverlapUniquePriority only; smaller values win.
	Priority int
}

// A Point is a sample point carrying an application-defined ID and the value
// aggregated into region statistics.
type Point struct {
	// ID is returned unchanged in the results and in unmatched points.
	ID any
	// Coord holds at least the X and Y ordinates of the point.
	Coord geom.Coord
	// Value is summed and averaged over the points assigned to each region.
	Value float64
}

// RegionStats holds the aggregate statistics of the points associated with a
// region. Mean is math.NaN when Count is zero.
type RegionStats struct {
	// RegionID is the ID of the corresponding Region.
	RegionID any
	// Count is the number of points associated with the region.
	Count int
	// Sum is the sum of the points' values.
	Sum float64
	// Mean is the arithmetic mean of the points' values, or math.NaN when the
	// region has no points.
	Mean float64
}

// A PointAssociation records that a point is associated with a region.
type PointAssociation struct {
	// PointID is the ID of the associated Point.
	PointID any
	// RegionID is the ID of the associated Region.
	RegionID any
	// OnBoundary is true if the point lies on the region's boundary.
	OnBoundary bool
}

// A Result is the output of a join.
type Result struct {
	// Stats holds one entry per input region, including empty regions and
	// regions without points, in input order.
	Stats []RegionStats
	// Associations holds every point-region association, ordered by point
	// index and then by region index. In OverlapUniquePriority mode each point
	// appears at most once.
	Associations []PointAssociation
	// Unmatched holds the points associated with no region, in input order and
	// unchanged.
	Unmatched []Point
}

var (
	errNoRegions            = errors.New("pointpolygonjoin: at least one region is required")
	errNilRegionGeometry    = errors.New("pointpolygonjoin: region geometry is nil")
	errUnsupportedGeometry  = errors.New("pointpolygonjoin: region geometry must be *geom.Polygon or *geom.MultiPolygon")
	errConflictingLayouts   = errors.New("pointpolygonjoin: all region geometries must share one layout")
	errUnsupportedLayout    = errors.New("pointpolygonjoin: region layout must have at least two ordinates")
	errInvalidNodeCapacity  = errors.New("pointpolygonjoin: node capacity must be at least 2")
	errPointWithoutOrdinals = errors.New("pointpolygonjoin: point coordinate must have at least two ordinates")
	errNonFiniteCoordinate  = errors.New("pointpolygonjoin: coordinates must be finite")
	errMalformedRing        = errors.New("pointpolygonjoin: ring has fewer coordinates than the layout stride")
)

// An Option configures a Joiner.
type Option func(*Joiner)

// WithOverlapMode sets how points are assigned to overlapping regions. The
// default is OverlapAll.
func WithOverlapMode(mode OverlapMode) Option {
	return func(j *Joiner) {
		j.overlapMode = mode
	}
}

// WithBoundaryPolicy sets whether points on a region's boundary are
// considered contained. The default is BoundaryInclude.
func WithBoundaryPolicy(policy BoundaryPolicy) Option {
	return func(j *Joiner) {
		j.boundaryPolicy = policy
	}
}

// WithNodeCapacity sets the maximum number of entries per R-tree node. The
// default is 25; values below 2 are rejected when the Joiner is created.
func WithNodeCapacity(capacity int) Option {
	return func(j *Joiner) {
		j.nodeCapacity = capacity
	}
}

// indexedRing is a linear ring together with its bounding box.
type indexedRing struct {
	flatCoords []float64
	box        box
}

// indexedPolygon is a single polygon of a region: its outer ring followed by
// its holes, each with a bounding box.
type indexedPolygon struct {
	rings []indexedRing
	box   box
}

// indexedRegion is the preprocessed representation of a Region.
type indexedRegion struct {
	index    int
	id       any
	priority int
	layout   geom.Layout
	stride   int
	empty    bool
	polygons []indexedPolygon
	box      box
}

// A Joiner performs point-in-polygon joins against a fixed set of regions.
// It is safe to call Join concurrently. The zero value is not usable; create
// one with NewJoiner.
type Joiner struct {
	regions        []*indexedRegion
	tree           *rTree[*indexedRegion]
	overlapMode    OverlapMode
	boundaryPolicy BoundaryPolicy
	nodeCapacity   int
	// stride is the shared layout stride of the regions.
	stride int
}

// NewJoiner preprocesses regions and builds the spatial index. The returned
// Joiner can be reused for any number of point sets.
func NewJoiner(regions []Region, options ...Option) (*Joiner, error) {
	if len(regions) == 0 {
		return nil, errNoRegions
	}
	joiner := &Joiner{
		overlapMode:    OverlapAll,
		boundaryPolicy: BoundaryInclude,
		nodeCapacity:   defaultNodeCapacity,
	}
	for _, option := range options {
		option(joiner)
	}
	if joiner.nodeCapacity < 2 {
		return nil, errInvalidNodeCapacity
	}

	indexedRegions := make([]*indexedRegion, len(regions))
	var commonLayout geom.Layout
	entries := make([]rtreeEntry[*indexedRegion], 0, len(regions))
	for i, region := range regions {
		if region.Geometry == nil {
			return nil, fmt.Errorf("%w: region %d", errNilRegionGeometry, i)
		}
		if region.Geometry.Layout().Stride() < 2 {
			return nil, fmt.Errorf("%w: region %d has layout %s", errUnsupportedLayout, i, region.Geometry.Layout())
		}
		if i == 0 {
			commonLayout = region.Geometry.Layout()
		} else if region.Geometry.Layout() != commonLayout {
			return nil, fmt.Errorf("%w: region %d has layout %s, want %s",
				errConflictingLayouts, i, region.Geometry.Layout(), commonLayout)
		}
		indexed, err := newIndexedRegion(i, region)
		if err != nil {
			return nil, err
		}
		indexedRegions[i] = indexed
		if !indexed.empty {
			entries = append(entries, rtreeEntry[*indexedRegion]{
				box: indexed.box,
				v:   indexed,
			})
		}
	}

	joiner.regions = indexedRegions
	joiner.tree = newRTree(entries, joiner.nodeCapacity)
	joiner.stride = commonLayout.Stride()
	return joiner, nil
}

// Join assigns every point to the regions containing it and aggregates the
// results. Points need not share a layout with the regions; only their first
// two ordinates are used.
func (j *Joiner) Join(points []Point) (*Result, error) {
	accumulators := make([]accumulator, len(j.regions))
	associations := make([]PointAssociation, 0, len(points))
	unmatched := make([]Point, 0)
	candidates := make([]*indexedRegion, 0)
	matches := make([]match, 0)
	stack := make([]*rtreeNode[*indexedRegion], 0)
	// Allocated once per Join: layouts with more than two ordinates need a
	// full-length query coordinate, and keeping it local preserves the
	// Joiner's concurrency safety.
	var queryCoord geom.Coord
	if j.stride > 2 {
		queryCoord = make(geom.Coord, j.stride)
	}

	for pointIndex, point := range points {
		if len(point.Coord) < 2 {
			return nil, fmt.Errorf("%w: point %d", errPointWithoutOrdinals, pointIndex)
		}
		x, y := point.Coord[0], point.Coord[1]
		if !isFinite(x) || !isFinite(y) {
			return nil, fmt.Errorf("%w: point %d", errNonFiniteCoordinate, pointIndex)
		}

		// The index only narrows the search to candidate regions; each
		// candidate is still tested exactly below.
		candidates = candidates[:0]
		stack = j.tree.searchPoint(stack, x, y, func(region *indexedRegion) bool {
			candidates = append(candidates, region)
			return true
		})

		matches = matches[:0]
		for _, region := range candidates {
			loc := region.locate(x, y, queryCoord)
			if j.accept(loc) {
				matches = append(matches, match{
					region:     region,
					onBoundary: loc == location.Boundary,
				})
			}
		}
		if len(matches) == 0 {
			unmatched = append(unmatched, point)
			continue
		}

		if j.overlapMode == OverlapUniquePriority {
			winner := pickWinner(matches)
			matches[0] = matches[winner]
			matches = matches[:1]
		}
		sortMatches(matches)
		for _, m := range matches {
			accumulators[m.region.index].add(point.Value)
			associations = append(associations, PointAssociation{
				PointID:    point.ID,
				RegionID:   m.region.id,
				OnBoundary: m.onBoundary,
			})
		}
	}

	result := &Result{
		Stats:        make([]RegionStats, len(j.regions)),
		Associations: associations,
		Unmatched:    unmatched,
	}
	for i, region := range j.regions {
		result.Stats[i] = accumulators[i].stats(region.id)
	}
	return result, nil
}

// Join creates a one-off Joiner from regions and joins it against points.
// Reuse a Joiner with NewJoiner when joining several point sets against the
// same regions so the index is built only once.
func Join(regions []Region, points []Point, options ...Option) (*Result, error) {
	joiner, err := NewJoiner(regions, options...)
	if err != nil {
		return nil, err
	}
	return joiner.Join(points)
}

// accept reports whether a point at loc is considered contained according to
// the configured boundary policy.
func (j *Joiner) accept(loc location.Type) bool {
	switch loc {
	case location.Interior:
		return true
	case location.Boundary:
		return j.boundaryPolicy == BoundaryInclude
	default:
		return false
	}
}

// A match is a region containing the point being processed.
type match struct {
	region     *indexedRegion
	onBoundary bool
}

// pickWinner returns the index of the match selected by
// OverlapUniquePriority: the smallest priority, then the smallest region
// index.
func pickWinner(matches []match) int {
	winner := 0
	for i := 1; i < len(matches); i++ {
		current, best := matches[i].region, matches[winner].region
		if current.priority < best.priority ||
			(current.priority == best.priority && current.index < best.index) {
			winner = i
		}
	}
	return winner
}

// sortMatches orders matches by region index so association output is
// deterministic.
func sortMatches(matches []match) {
	for i := 1; i < len(matches); i++ {
		for j := i; j > 0 && matches[j-1].region.index > matches[j].region.index; j-- {
			matches[j-1], matches[j] = matches[j], matches[j-1]
		}
	}
}

// newIndexedRegion preprocesses a region into its indexed representation.
func newIndexedRegion(index int, region Region) (*indexedRegion, error) {
	indexed := &indexedRegion{
		index:    index,
		id:       region.ID,
		priority: region.Priority,
		layout:   region.Geometry.Layout(),
		stride:   region.Geometry.Layout().Stride(),
	}
	switch geometry := region.Geometry.(type) {
	case *geom.Polygon:
		if geometry.Empty() {
			indexed.empty = true
			return indexed, nil
		}
		polygon, err := newIndexedPolygon(geometry.FlatCoords(), geometry.Ends(), indexed.stride)
		if err != nil {
			return nil, err
		}
		indexed.polygons = []indexedPolygon{polygon}
		indexed.box = polygon.box
	case *geom.MultiPolygon:
		if geometry.Empty() {
			indexed.empty = true
			return indexed, nil
		}
		polygons := make([]indexedPolygon, 0, geometry.NumPolygons())
		offset := 0
		for _, ends := range geometry.Endss() {
			if len(ends) == 0 {
				continue
			}
			end := ends[len(ends)-1]
			polygon, err := newIndexedPolygon(geometry.FlatCoords()[offset:end], relativeEnds(offset, ends), indexed.stride)
			if err != nil {
				return nil, err
			}
			polygons = append(polygons, polygon)
			offset = end
		}
		if len(polygons) == 0 {
			indexed.empty = true
			return indexed, nil
		}
		indexed.polygons = polygons
		indexed.box = polygons[0].box
		for _, polygon := range polygons[1:] {
			indexed.box = indexed.box.union(polygon.box)
		}
	default:
		return nil, fmt.Errorf("%w: got %T", errUnsupportedGeometry, region.Geometry)
	}
	return indexed, nil
}

// newIndexedPolygon preprocesses the rings of one polygon given its flat
// coordinates and ring end offsets.
func newIndexedPolygon(flatCoords []float64, ends []int, stride int) (indexedPolygon, error) {
	polygon := indexedPolygon{
		rings: make([]indexedRing, len(ends)),
	}
	ringStart := 0
	for i, end := range ends {
		ringCoords := flatCoords[ringStart:end]
		ringBox, err := boxOfFlatCoords(ringCoords, stride)
		if err != nil {
			return indexedPolygon{}, err
		}
		polygon.rings[i] = indexedRing{
			flatCoords: ringCoords,
			box:        ringBox,
		}
		if i == 0 {
			polygon.box = ringBox
		} else {
			polygon.box = polygon.box.union(ringBox)
		}
		ringStart = end
	}
	return polygon, nil
}

// relativeEnds converts multipolygon-absolute ring ends into polygon-relative
// ends by subtracting offset.
func relativeEnds(offset int, ends []int) []int {
	relative := make([]int, len(ends))
	for i, end := range ends {
		relative[i] = end - offset
	}
	return relative
}

// boxOfFlatCoords returns the bounding box of all coordinates in flatCoords.
func boxOfFlatCoords(flatCoords []float64, stride int) (box, error) {
	if len(flatCoords) < stride {
		return box{}, errMalformedRing
	}
	b := box{
		minX: flatCoords[0],
		minY: flatCoords[1],
		maxX: flatCoords[0],
		maxY: flatCoords[1],
	}
	for i := stride; i < len(flatCoords); i += stride {
		x, y := flatCoords[i], flatCoords[i+1]
		if !isFinite(x) || !isFinite(y) {
			return box{}, errNonFiniteCoordinate
		}
		b.minX = min(b.minX, x)
		b.minY = min(b.minY, y)
		b.maxX = max(b.maxX, x)
		b.maxY = max(b.maxY, y)
	}
	if !isFinite(b.minX) || !isFinite(b.minY) {
		return box{}, errNonFiniteCoordinate
	}
	return b, nil
}

// locate determines where the point (x, y) lies relative to the region,
// honoring holes. Boundary of any ring wins over interior or exterior
// results, matching the OGC boundary of a polygon. queryCoord is a reusable
// buffer of the layout stride, or nil for two-ordinate layouts.
func (r *indexedRegion) locate(x, y float64, queryCoord geom.Coord) location.Type {
	result := location.Exterior
	for i := range r.polygons {
		polygon := &r.polygons[i]
		if !polygon.box.containsPoint(x, y) {
			continue
		}
		insideOuter := false
		for ringIndex := range polygon.rings {
			ring := &polygon.rings[ringIndex]
			if !ring.box.containsPoint(x, y) {
				continue
			}
			loc := locateInRing(r.layout, x, y, ring.flatCoords, r.stride, queryCoord)
			if loc == location.Boundary {
				return location.Boundary
			}
			if ringIndex == 0 {
				insideOuter = loc == location.Interior
			} else if loc == location.Interior {
				// The point is inside a hole: toggle the containment state so
				// nested rings (island inside a hole) are handled correctly.
				insideOuter = !insideOuter
			}
		}
		if insideOuter {
			result = location.Interior
		}
	}
	return result
}

// locateInRing runs the exact ray-crossing test. queryCoord is a reusable
// buffer of length stride, or nil for the XY layout.
func locateInRing(layout geom.Layout, x, y float64, ring []float64, stride int, queryCoord geom.Coord) location.Type {
	if stride == 2 {
		return xy.LocatePointInRing(layout, geom.Coord{x, y}, ring)
	}
	queryCoord[0], queryCoord[1] = x, y
	return xy.LocatePointInRing(layout, queryCoord, ring)
}

// isFinite reports whether f is neither NaN nor an infinity.
func isFinite(f float64) bool {
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}

// An accumulator builds the statistics of one region.
type accumulator struct {
	count int
	sum   float64
}

// add records one point value.
func (a *accumulator) add(value float64) {
	a.count++
	a.sum += value
}

// stats returns the aggregated statistics for regionID.
func (a *accumulator) stats(regionID any) RegionStats {
	mean := math.NaN()
	if a.count > 0 {
		mean = a.sum / float64(a.count)
	}
	return RegionStats{
		RegionID: regionID,
		Count:    a.count,
		Sum:      a.sum,
		Mean:     mean,
	}
}
