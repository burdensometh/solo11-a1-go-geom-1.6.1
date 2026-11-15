// Package resample resamples time-measured trajectories onto a fixed time grid.
//
// A trajectory is a [geom.LineString] or [geom.MultiLineString] whose layout
// carries an M ordinate that records time, i.e. [geom.XYM] or [geom.XYZM]. The
// grid consists of the times t(k) = origin + k*step for all integers k. Every
// grid time that falls within a continuous fragment of the input produces one
// output point: its X, Y, and (if present) Z ordinates are linearly
// interpolated within the input segment that contains the time, while its M
// ordinate is set to the grid time itself. M is time, not a height, so it is
// never interpolated. Z, if present, is an ordinary coordinate and is
// interpolated linearly like X and Y, as are any ordinates beyond X, Y, Z, and
// M.
//
// Each [Sample] records the input part and input segment from which the output
// point was interpolated, so every output point remains traceable to the
// original trajectory. The input geometry is never modified.
//
// # Continuous fragments and gaps
//
// Consecutive input samples whose measures differ by more than the maximum gap
// ([WithMaxGap]) form a gap. A gap is never filled: grid times strictly inside
// it produce no points, and the output trajectory is split there, so no line is
// ever drawn across missing data. A vertex at the edge of a gap is still
// emitted when its time falls on the grid: it is an observed sample, not an
// interpolation across the gap. The fragments on either side of a gap are
// emitted as separate output parts, even when a fragment contains a single
// point.
//
// # Time direction and duplicate times
//
// A trajectory may run forward or backward in time. Each fragment must be
// monotonic, but different fragments may run in different directions: a
// trajectory that reverses direction across a gap is accepted. The output
// preserves each fragment's traversal order, so a backward fragment produces
// descending output times.
// Duplicate measures describe zero-duration segments. They are permitted: they
// never produce interpolated points of their own, and a grid time that
// coincides with a run of equal measures is emitted once, from the last vertex
// of the run in input order. That vertex is also the one from which the
// following segment continues, so the emitted point and the next interpolated
// point use the same observation. A fragment whose measures are all equal
// yields at most one point, from its last vertex. A fragment whose measures are
// neither non-decreasing nor non-increasing, or that contains a non-finite
// measure, is rejected.
//
// Empty input parts produce no output. Distinct input parts are never joined,
// whatever their measure ranges.
package resample

import (
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/twpayne/go-geom"
)

var (
	// ErrUnsupportedGeometry is returned when the input is not a
	// *geom.LineString or *geom.MultiLineString.
	ErrUnsupportedGeometry = errors.New("resample: geometry must be a *geom.LineString or *geom.MultiLineString")
	// ErrNoMeasure is returned when the trajectory's layout has no M ordinate.
	ErrNoMeasure = errors.New("resample: trajectory layout has no M dimension")
	// ErrInvalidGrid is returned when the origin is not finite or the step is
	// not a finite positive number.
	ErrInvalidGrid = errors.New("resample: origin must be finite and step must be positive and finite")
	// ErrInvalidMaxGap is returned when the maximum gap is negative or NaN.
	ErrInvalidMaxGap = errors.New("resample: max gap must not be negative")
	// ErrNonFiniteMeasure is returned when an input measure is NaN or infinite.
	ErrNonFiniteMeasure = errors.New("resample: input measures must be finite")
	// ErrNonMonotonicMeasure is returned when a fragment's measures are
	// neither non-decreasing nor non-increasing.
	ErrNonMonotonicMeasure = errors.New("resample: input measures must be monotonic within a fragment")
	// ErrGridRange is returned when the grid indices covering an input part
	// are not representable.
	ErrGridRange = errors.New("resample: time grid range is out of range")
	// ErrMalformedGeometry is returned when a geometry's flat coordinates are
	// not a whole number of coordinates.
	ErrMalformedGeometry = errors.New("resample: geometry flat coordinates are misaligned")
)

// gridToleranceULPs is the number of float64 ULPs by which a computed grid time
// may differ from an input measure and still denote the same moment.
const gridToleranceULPs = 8

// An Option configures a call to [Resample].
type Option func(*config)

type config struct {
	maxGap float64
}

// WithMaxGap sets the largest measure difference between consecutive input
// samples that is still considered continuous. A larger difference forms a gap,
// which is never interpolated across. A maximum gap of zero, the default, means
// no limit. A negative or NaN maximum gap makes [Resample] fail with
// [ErrInvalidMaxGap].
func WithMaxGap(maxGap float64) Option {
	return func(c *config) {
		c.maxGap = maxGap
	}
}

// A Sample is one resampled point together with the provenance of the input
// segment it was interpolated from.
type Sample struct {
	// Coord holds the full output coordinate, with the M ordinate set to Time.
	// It aliases the flat coordinates of [Result.Geometry]; treat it as
	// read-only.
	Coord geom.Coord
	// Time is the grid time of the sample, t(k) = origin + k*step. It equals
	// Coord at the layout's M index.
	Time float64
	// OutputPart is the index of the part of [Result.Geometry] containing
	// Coord.
	OutputPart int
	// InputPart is the index of the input part that contains the source
	// segment. It is always zero for a *geom.LineString input.
	InputPart int
	// InputSegment is the index of the source segment, i.e. the segment between
	// input vertices InputSegment and InputSegment+1 of InputPart. It is -1
	// when no input segment provides the point, which happens for an isolated
	// input vertex and for a part whose measures are all equal.
	InputSegment int
	// Fraction is the position of the sample along the source segment, in
	// [0, 1]. A value of 0 means the sample is at the segment's first vertex in
	// input order and 1 means it is at the second.
	Fraction float64
}

// A Result is a resampled trajectory.
type Result struct {
	// Geometry is the resampled trajectory. It has the same layout and SRID as
	// the input and contains one part per continuous fragment of the input that
	// produced at least one point. A fragment that produces a single point is
	// represented by a part with a single coordinate.
	Geometry *geom.MultiLineString
	// Samples has one entry per coordinate of Geometry, in the same order.
	Samples []Sample
}

// Resample resamples the time-measured trajectory g onto the fixed grid with
// the given origin and step. The input geometry is not modified. A new
// [geom.MultiLineString] with the same layout and SRID as g is returned,
// together with the provenance of every output point.
func Resample(g geom.T, origin, step float64, options ...Option) (*Result, error) {
	var cfg config
	for _, option := range options {
		option(&cfg)
	}
	if err := validateGrid(origin, step); err != nil {
		return nil, err
	}
	if math.IsNaN(cfg.maxGap) || cfg.maxGap < 0 {
		return nil, fmt.Errorf("%w: %v", ErrInvalidMaxGap, cfg.maxGap)
	}
	if err := validateInputType(g); err != nil {
		return nil, err
	}
	layout := g.Layout()
	mIndex := layout.MIndex()
	if mIndex < 0 {
		return nil, fmt.Errorf("%w: %s", ErrNoMeasure, layout)
	}
	inputParts, err := splitParts(g)
	if err != nil {
		return nil, err
	}

	r := &resampler{
		maxGap: cfg.maxGap,
		grid:   grid{origin: origin, step: step},
		stride: layout.Stride(),
		mIndex: mIndex,
	}
	var builder outputBuilder
	builder.stride = layout.Stride()
	for _, part := range inputParts {
		runs, err := r.resamplePart(part)
		if err != nil {
			return nil, err
		}
		for _, run := range runs {
			builder.addRun(part.index, run)
		}
	}

	return &Result{
		Geometry: geom.NewMultiLineStringFlat(layout, builder.flatCoords, builder.ends).SetSRID(g.SRID()),
		Samples:  builder.finish(),
	}, nil
}

// validateInputType checks that g is a non-nil *geom.LineString or
// *geom.MultiLineString. It also catches typed nil pointers.
func validateInputType(g geom.T) error {
	if g == nil {
		return fmt.Errorf("%w: <nil>", ErrUnsupportedGeometry)
	}
	switch g := g.(type) {
	case *geom.LineString:
		if g == nil {
			return fmt.Errorf("%w: typed nil *geom.LineString", ErrUnsupportedGeometry)
		}
	case *geom.MultiLineString:
		if g == nil {
			return fmt.Errorf("%w: typed nil *geom.MultiLineString", ErrUnsupportedGeometry)
		}
	default:
		return fmt.Errorf("%w: %T", ErrUnsupportedGeometry, g)
	}
	return nil
}

func validateGrid(origin, step float64) error {
	if math.IsNaN(origin) || math.IsInf(origin, 0) {
		return fmt.Errorf("%w: origin %v is not finite", ErrInvalidGrid, origin)
	}
	if math.IsNaN(step) || math.IsInf(step, 0) || step <= 0 {
		return fmt.Errorf("%w: step %v is not positive and finite", ErrInvalidGrid, step)
	}
	// The grid must be able to distinguish consecutive points. With a step
	// that is tiny relative to the origin, adjacent grid times round to the
	// same float64 and the grid degenerates.
	if origin+step == origin || origin-step == origin {
		return fmt.Errorf("%w: step %v is too small for origin %v", ErrInvalidGrid, step, origin)
	}
	return nil
}

// inputPart is one non-empty part of the input geometry.
type inputPart struct {
	index      int
	flatCoords []float64
}

// splitParts returns the non-empty parts of g. The index of each part is its
// index in the input geometry, so empty parts are skipped without renumbering
// their successors.
func splitParts(g geom.T) ([]inputPart, error) {
	stride := g.Stride()
	if stride <= 0 || len(g.FlatCoords())%stride != 0 {
		return nil, fmt.Errorf("%w: %T", ErrMalformedGeometry, g)
	}
	switch g := g.(type) {
	case *geom.LineString:
		if g.Empty() {
			return nil, nil
		}
		return []inputPart{{index: 0, flatCoords: g.FlatCoords()}}, nil
	case *geom.MultiLineString:
		flatCoords := g.FlatCoords()
		var parts []inputPart
		offset := 0
		for i, end := range g.Ends() {
			if end < offset || end > len(flatCoords) || end%stride != 0 {
				return nil, fmt.Errorf("%w: %T part %d", ErrMalformedGeometry, g, i)
			}
			if end > offset {
				parts = append(parts, inputPart{
					index:      i,
					flatCoords: flatCoords[offset:end],
				})
			}
			offset = end
		}
		if offset != len(flatCoords) {
			return nil, fmt.Errorf("%w: %T trailing coordinates", ErrMalformedGeometry, g)
		}
		return parts, nil
	default:
		return nil, fmt.Errorf("%w: %T", ErrUnsupportedGeometry, g)
	}
}

type grid struct {
	origin float64
	step   float64
}

func (g grid) at(k int) float64 {
	return g.origin + float64(k)*g.step
}

// maxGridSpan bounds the grid indices a single fragment may cover. It keeps
// loop counters, slice lengths, and float-to-int conversions within range on
// all supported architectures; no real trajectory approaches it.
const maxGridSpan = 1 << 30

// bounds returns the grid indices that cover [lo, hi], widened by one index on
// each side to absorb floating-point rounding. Entries beyond the fragment are
// discarded by the caller.
func (g grid) bounds(lo, hi float64) (int, int, error) {
	qLo := (lo - g.origin) / g.step
	qHi := (hi - g.origin) / g.step
	if math.IsNaN(qLo) || math.IsNaN(qHi) || math.IsInf(qLo, 0) || math.IsInf(qHi, 0) {
		return 0, 0, ErrGridRange
	}
	fLo := math.Floor(qLo) - 1
	fHi := math.Ceil(qHi) + 1
	if fLo < -maxGridSpan || fHi > maxGridSpan || fHi-fLo+1 > maxGridSpan {
		return 0, 0, ErrGridRange
	}
	return int(fLo), int(fHi), nil
}

// exactIndex returns the grid index k for which g.at(k) is closest to t, when
// that index's time is within tolerance of t.
func (g grid) exactIndex(t, tolerance float64) (int, bool) {
	q := (t - g.origin) / g.step
	if math.IsNaN(q) || math.IsInf(q, 0) || math.Abs(q) > maxGridSpan {
		return 0, false
	}
	k := int(math.Round(q))
	if math.Abs(g.at(k)-t) <= tolerance {
		return k, true
	}
	return 0, false
}

// fragment is a maximal run of vertices whose consecutive measures do not
// contain a gap. Measures in a fragment are monotonic.
type fragment struct {
	start int
	end   int
	lo    float64
	hi    float64
}

// resampler resamples one input geometry.
type resampler struct {
	maxGap float64
	grid   grid
	stride int
	mIndex int
}

// emitted is one output coordinate before it is packed into the result.
type emitted struct {
	coord    []float64
	time     float64
	segment  int
	fraction float64
}

func (r *resampler) resamplePart(part inputPart) ([][]emitted, error) {
	numCoords := len(part.flatCoords) / r.stride
	if numCoords == 0 {
		return nil, nil
	}
	times := make([]float64, numCoords)
	for i := range times {
		t := part.flatCoords[i*r.stride+r.mIndex]
		if math.IsNaN(t) || math.IsInf(t, 0) {
			return nil, fmt.Errorf("%w: part %d, vertex %d", ErrNonFiniteMeasure, part.index, i)
		}
		times[i] = t
	}

	var runs [][]emitted
	for _, frag := range splitFragments(times, r.maxGap) {
		direction, err := fragmentDirection(times, frag)
		if err != nil {
			return nil, fmt.Errorf("%w: part %d, vertex %d", err, part.index, frag.start+1)
		}
		run, err := r.resampleFragment(part, times, frag, direction)
		if err != nil {
			return nil, err
		}
		if len(run) > 0 {
			runs = append(runs, run)
		}
	}
	return runs, nil
}

// fragmentDirection returns the direction of a fragment: 1 for non-decreasing
// measures, -1 for non-increasing measures, and 0 for a fragment whose measures
// are all equal. Zero differences are neutral. Measures that both increase and
// decrease within a fragment make it invalid.
func fragmentDirection(times []float64, frag fragment) (int, error) {
	direction := 0
	for i := frag.start; i < frag.end; i++ {
		switch {
		case times[i+1] > times[i]:
			if direction < 0 {
				return 0, ErrNonMonotonicMeasure
			}
			direction = 1
		case times[i+1] < times[i]:
			if direction > 0 {
				return 0, ErrNonMonotonicMeasure
			}
			direction = -1
		}
	}
	return direction, nil
}

// splitFragments splits consecutive vertices at gaps. A gap separates the
// vertices on either side; every vertex belongs to exactly one fragment.
func splitFragments(times []float64, maxGap float64) []fragment {
	fragments := make([]fragment, 0, 1)
	start := 0
	for i := 0; i+1 < len(times); i++ {
		if maxGap > 0 && math.Abs(times[i+1]-times[i]) > maxGap {
			fragments = append(fragments, newFragment(times, start, i))
			start = i + 1
		}
	}
	return append(fragments, newFragment(times, start, len(times)-1))
}

func newFragment(times []float64, start, end int) fragment {
	lo, hi := times[start], times[end]
	if lo > hi {
		lo, hi = hi, lo
	}
	return fragment{start: start, end: end, lo: lo, hi: hi}
}

// resampleFragment returns the output points of one fragment in traversal
// order. A fragment whose measures are all equal yields at most one point.
func (r *resampler) resampleFragment(part inputPart, times []float64, frag fragment, direction int) ([]emitted, error) {
	tolerance := timeTolerance(r.grid.step, r.grid.origin, frag.lo, frag.hi)
	if direction == 0 {
		k, ok := r.grid.exactIndex(times[frag.start], tolerance)
		if !ok {
			return nil, nil
		}
		// One observation per instant: the last vertex in input order is the
		// state of the trajectory at that time.
		time := r.grid.at(k)
		return []emitted{{
			coord:    r.coordinateAt(part, frag.end, time),
			time:     time,
			segment:  -1,
			fraction: 0,
		}}, nil
	}

	kLo, kHi, err := r.grid.bounds(frag.lo, frag.hi)
	if err != nil {
		return nil, err
	}
	run := make([]emitted, 0, min(kHi-kLo+1, 1024))
	if direction > 0 {
		for k := kLo; k <= kHi; k++ {
			if point, ok := r.evalAt(part, times, frag, direction, k, tolerance); ok {
				run = append(run, point)
			}
		}
	} else {
		for k := kHi; k >= kLo; k-- {
			if point, ok := r.evalAt(part, times, frag, direction, k, tolerance); ok {
				run = append(run, point)
			}
		}
	}
	return run, nil
}

// timeTolerance returns how far a computed grid time may be from an input
// measure and still denote the same moment. It is a small number of ULPs of the
// largest value involved, so it scales with the magnitudes in play, capped at a
// quarter of the step so that distinct grid points are never collapsed into one
// another.
func timeTolerance(step float64, values ...float64) float64 {
	magnitude := 0.0
	for _, value := range values {
		if a := math.Abs(value); a > magnitude {
			magnitude = a
		}
	}
	ulp := math.Nextafter(magnitude, math.Inf(1)) - magnitude
	return math.Min(gridToleranceULPs*ulp, step/4)
}

// evalAt evaluates the fragment at grid index k. It reports false when the grid
// time lies outside the fragment or strictly inside a gap.
func (r *resampler) evalAt(part inputPart, times []float64, frag fragment, direction, k int, tolerance float64) (emitted, bool) {
	target := r.grid.at(k)
	if target < frag.lo-tolerance || target > frag.hi+tolerance {
		return emitted{}, false
	}
	// Clamp so that a grid time at an endpoint is recognized as that endpoint
	// rather than as a rounding artifact.
	eval := target
	if eval < frag.lo {
		eval = frag.lo
	} else if eval > frag.hi {
		eval = frag.hi
	}

	index := r.locate(times, frag, direction, eval)
	if index > frag.start && math.Abs(times[index-1]-eval) <= tolerance {
		return r.vertexSample(part, times, frag, index-1, target), true
	}
	if index <= frag.end && math.Abs(times[index]-eval) <= tolerance {
		return r.vertexSample(part, times, frag, index, target), true
	}
	if index <= frag.start || index > frag.end {
		// The grid time is at an extreme of the fragment but does not coincide
		// with the vertex there, so there is no segment to interpolate within.
		return emitted{}, false
	}
	return r.interpolate(part, times, index-1, target, eval), true
}

// locate returns the first vertex index in the fragment whose measure has
// reached eval in the direction of traversal, or frag.end+1 when no vertex has.
func (r *resampler) locate(times []float64, frag fragment, direction int, eval float64) int {
	count := frag.end - frag.start + 1
	offset := sort.Search(count, func(i int) bool {
		if direction > 0 {
			return times[frag.start+i] >= eval
		}
		return times[frag.start+i] <= eval
	})
	return frag.start + offset
}

// vertexSample returns the output point for an observed vertex. A run of equal
// measures is resolved to its last vertex in input order, which is both the
// observation from which the following segment continues and, for a forward
// trajectory, the latest observation at that instant.
func (r *resampler) vertexSample(part inputPart, times []float64, frag fragment, index int, target float64) emitted {
	index = lastOfRun(times, frag, index)
	segment, fraction := vertexProvenance(times, frag, index)
	return emitted{
		coord:    r.coordinateAt(part, index, target),
		time:     target,
		segment:  segment,
		fraction: fraction,
	}
}

// lastOfRun returns the last vertex of the run of equal measures containing
// index.
func lastOfRun(times []float64, frag fragment, index int) int {
	value := times[index]
	for index < frag.end && times[index+1] == value {
		index++
	}
	return index
}

// vertexProvenance returns the input segment that provides an observed vertex,
// and the fraction along it. The segment must lie within the fragment, so
// provenance never points across a gap, and must be one whose coordinate at the
// reported fraction reproduces the emitted vertex exactly, i.e. a segment that
// actually has the vertex as an endpoint. A non-degenerate arriving segment is
// preferred, then a non-degenerate departing one, so a segment's two endpoints
// are reported consistently as fractions 0 and 1; only when both are
// degenerate (duplicate measures) is a zero-duration segment reported, and then
// its endpoint fraction still reproduces the vertex exactly. Only a vertex with
// no neighbour within its fragment, i.e. an isolated vertex or the sole vertex
// of a part, has no segment identity.
func vertexProvenance(times []float64, frag fragment, index int) (int, float64) {
	if index > frag.start && times[index-1] != times[index] {
		return index - 1, 1
	}
	if index < frag.end && times[index] != times[index+1] {
		return index, 0
	}
	if index > frag.start {
		return index - 1, 1
	}
	if index < frag.end {
		return index, 0
	}
	return -1, 0
}

// coordinateAt returns a copy of a vertex with its M ordinate replaced by time.
func (r *resampler) coordinateAt(part inputPart, index int, time float64) []float64 {
	coord := make([]float64, r.stride)
	copy(coord, part.flatCoords[index*r.stride:(index+1)*r.stride])
	coord[r.mIndex] = time
	return coord
}

// interpolate returns the point at eval on the segment that starts at index,
// with every ordinate except M interpolated linearly and M set to target.
func (r *resampler) interpolate(part inputPart, times []float64, index int, target, eval float64) emitted {
	t0, t1 := times[index], times[index+1]
	fraction := (eval - t0) / (t1 - t0)
	if fraction < 0 {
		fraction = 0
	} else if fraction > 1 {
		fraction = 1
	}
	a := part.flatCoords[index*r.stride : (index+1)*r.stride]
	b := part.flatCoords[(index+1)*r.stride : (index+2)*r.stride]
	coord := make([]float64, r.stride)
	for d := range coord {
		if d == r.mIndex {
			coord[d] = target
		} else {
			coord[d] = a[d] + (b[d]-a[d])*fraction
		}
	}
	return emitted{
		coord:    coord,
		time:     target,
		segment:  index,
		fraction: fraction,
	}
}

// outputBuilder packs emitted points into the flat coordinates of the output
// geometry and records their provenance.
type outputBuilder struct {
	stride     int
	flatCoords []float64
	ends       []int
	samples    []Sample
}

func (b *outputBuilder) addRun(inputPart int, run []emitted) {
	if len(run) == 0 {
		return
	}
	outputPart := len(b.ends)
	for _, point := range run {
		b.flatCoords = append(b.flatCoords, point.coord...)
		b.samples = append(b.samples, Sample{
			Time:         point.time,
			OutputPart:   outputPart,
			InputPart:    inputPart,
			InputSegment: point.segment,
			Fraction:     point.fraction,
		})
	}
	b.ends = append(b.ends, len(b.flatCoords))
}

// finish points every Sample at its coordinate in the result and returns the
// samples.
func (b *outputBuilder) finish() []Sample {
	if b.samples == nil {
		return nil
	}
	for i := range b.samples {
		b.samples[i].Coord = b.flatCoords[i*b.stride : (i+1)*b.stride]
	}
	return b.samples
}
