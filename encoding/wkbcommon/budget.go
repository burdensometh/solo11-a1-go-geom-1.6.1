package wkbcommon

import (
	"fmt"
	"math"
	"strconv"
)

// ReadLimits limits the resources consumed by a single WKB or EWKB read.
//
// A zero or negative value means that no limit is applied. Each Read or
// Unmarshal call gets its own independent limits: limits are never shared
// between reads and are safe to use concurrently.
type ReadLimits struct {
	// MaxCoordinates is the maximum total number of coordinate tuples (points
	// or vertices) decoded across the whole geometry, including every nested
	// sub-geometry.
	MaxCoordinates int
	// MaxMembers is the maximum total number of collection members decoded,
	// that is points in a MultiPoint, LineStrings in a MultiLineString,
	// Polygons in a MultiPolygon, rings in a Polygon, and geometries in a
	// GeometryCollection, counted across every level of nesting.
	MaxMembers int
	// MaxDepth is the maximum nesting depth of geometries. A top level
	// geometry has depth 1, its immediate sub-geometries have depth 2, and so
	// on.
	MaxDepth int
}

// ErrLimitExceeded is returned when a read exceeds one of its ReadLimits.
type ErrLimitExceeded struct {
	// What names the exhausted resource, for example "coordinates",
	// "members", or "depth".
	What string
	// Path identifies the member being read when the limit was exceeded, for
	// example "member 2/member 0" or "ring 1". It is empty for the top level
	// geometry.
	Path string
	// Requested is the amount that would have been consumed by the current
	// element.
	Requested int64
	// Used is the amount already consumed before the current element.
	Used int64
	// Limit is the configured limit.
	Limit int64
}

func (e ErrLimitExceeded) Error() string {
	if e.Path == "" {
		return fmt.Sprintf("wkb: %s limit exceeded: requested %d, used %d, limit %d", e.What, e.Requested, e.Used, e.Limit)
	}
	return fmt.Sprintf("wkb: %s limit exceeded at %s: requested %d, used %d, limit %d", e.What, e.Path, e.Requested, e.Used, e.Limit)
}

// ErrIntegerOverflow is returned when an element count read from the input, or
// the size derived from it, cannot be represented. The check happens before any
// allocation, so no memory is allocated for the oversized element.
type ErrIntegerOverflow struct {
	// What describes the value that overflows, for example "coordinate count"
	// or "member count".
	What string
	// Path identifies the member being read when the overflow was detected. It
	// is empty for the top level geometry.
	Path string
	// Value is the value that overflows.
	Value int64
}

func (e ErrIntegerOverflow) Error() string {
	if e.Path == "" {
		return fmt.Sprintf("wkb: integer overflow in %s: %d", e.What, e.Value)
	}
	return fmt.Sprintf("wkb: integer overflow in %s at %s: %d", e.What, e.Path, e.Value)
}

// ReadBudget tracks the resources consumed by a single read. It is mutated as
// the geometry is decoded and is shared by every recursive sub-geometry, so
// nested collections draw on the same remaining budget. It is not safe for use
// by concurrent reads; each read must create its own budget with NewReadBudget.
type ReadBudget struct {
	limits      ReadLimits
	coordinates int64
	members     int64
	depth       int
}

// NewReadBudget returns a new ReadBudget enforcing limits.
func NewReadBudget(limits ReadLimits) *ReadBudget {
	return &ReadBudget{limits: limits}
}

// EnterGeometry records that another level of geometry nesting is entered and
// returns an error if the depth limit is exceeded. The returned function must
// be called when the geometry has been read, typically via defer.
func (b *ReadBudget) EnterGeometry(path string) (func(), error) {
	b.depth++
	if b.limits.MaxDepth > 0 && b.depth > b.limits.MaxDepth {
		depth := b.depth
		b.depth--
		return nil, ErrLimitExceeded{
			What:      "depth",
			Path:      path,
			Requested: int64(depth),
			Used:      int64(depth - 1),
			Limit:     int64(b.limits.MaxDepth),
		}
	}
	return func() { b.depth-- }, nil
}

// AddCoordinates reserves n coordinate tuples before they are allocated or
// read. It returns an error without allocating if n cannot be represented as an
// int, if n multiplied by stride overflows, or if the coordinate budget would
// be exceeded.
func (b *ReadBudget) AddCoordinates(n uint32, stride int, path string) error {
	nn := int64(n)
	// Reject counts whose allocation cannot be represented as an int. The
	// division-based check also catches overflow when multiplying by stride.
	if (strconv.IntSize == 32 && nn > math.MaxInt32) || nn > math.MaxInt/int64(stride) {
		return ErrIntegerOverflow{What: "coordinate count", Path: path, Value: nn}
	}
	if b.limits.MaxCoordinates > 0 && b.coordinates+nn > int64(b.limits.MaxCoordinates) {
		return ErrLimitExceeded{
			What:      "coordinates",
			Path:      path,
			Requested: nn,
			Used:      b.coordinates,
			Limit:     int64(b.limits.MaxCoordinates),
		}
	}
	b.coordinates += nn
	return nil
}

// AddMembers reserves n collection members before they are allocated or read.
// It returns an error without allocating if n cannot be represented as an int
// or if the member budget would be exceeded.
func (b *ReadBudget) AddMembers(n uint32, path string) error {
	nn := int64(n)
	// On 32-bit platforms, reject counts that cannot index a slice.
	if strconv.IntSize == 32 && nn > math.MaxInt32 {
		return ErrIntegerOverflow{What: "member count", Path: path, Value: nn}
	}
	if b.limits.MaxMembers > 0 && b.members+nn > int64(b.limits.MaxMembers) {
		return ErrLimitExceeded{
			What:      "members",
			Path:      path,
			Requested: nn,
			Used:      b.members,
			Limit:     int64(b.limits.MaxMembers),
		}
	}
	b.members += nn
	return nil
}

// Coordinates returns the number of coordinate tuples consumed so far.
func (b *ReadBudget) Coordinates() int64 { return b.coordinates }

// Members returns the number of collection members consumed so far.
func (b *ReadBudget) Members() int64 { return b.members }

// Depth returns the current nesting depth.
func (b *ReadBudget) Depth() int { return b.depth }

// JoinPath returns the path of a named member nested within path. An empty path
// identifies the top level geometry.
func JoinPath(path, member string) string {
	if path == "" {
		return member
	}
	return path + "/" + member
}
