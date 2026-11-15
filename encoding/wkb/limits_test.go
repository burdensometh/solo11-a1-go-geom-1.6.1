package wkb

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"sync"
	"testing"

	"github.com/alecthomas/assert/v2"

	"github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/wkbcommon"
)

func assertMultiPoint(t *testing.T, g geom.T) *geom.MultiPoint {
	t.Helper()
	mp, ok := g.(*geom.MultiPoint)
	assert.True(t, ok)
	return mp
}

func assertLineString(t *testing.T, g geom.T) *geom.LineString {
	t.Helper()
	ls, ok := g.(*geom.LineString)
	assert.True(t, ok)
	return ls
}

func assertPoint(t *testing.T, g geom.T) *geom.Point {
	t.Helper()
	p, ok := g.(*geom.Point)
	assert.True(t, ok)
	return p
}

func assertGeometryCollection(t *testing.T, g geom.T) *geom.GeometryCollection {
	t.Helper()
	gc, ok := g.(*geom.GeometryCollection)
	assert.True(t, ok)
	return gc
}

const maxUint32Count = uint32(math.MaxUint32)

// writeHeader writes a WKB byte order and geometry type header.
func writeHeader(buf *bytes.Buffer, order binary.ByteOrder, typeID uint32) {
	switch order {
	case NDR:
		buf.WriteByte(wkbcommon.NDRID)
	case XDR:
		buf.WriteByte(wkbcommon.XDRID)
	}
	var b [4]byte
	order.PutUint32(b[:], typeID)
	buf.Write(b[:])
}

// writeUint32 writes n in the given byte order.
func writeUint32(buf *bytes.Buffer, order binary.ByteOrder, n uint32) {
	var b [4]byte
	order.PutUint32(b[:], n)
	buf.Write(b[:])
}

// writePoint writes a complete Point WKB with the given order and coordinates.
func writePoint(buf *bytes.Buffer, order binary.ByteOrder, coords ...float64) {
	writeHeader(buf, order, wkbcommon.PointID)
	var b [8]byte
	for _, c := range coords {
		order.PutUint64(b[:], math.Float64bits(c))
		buf.Write(b[:])
	}
}

// multiPointWKB builds a MultiPoint containing n points, each sub-geometry
// using its own byte order so mixed byte orders are exercised.
func multiPointWKB(n int, topOrder, subOrder binary.ByteOrder) []byte {
	var buf bytes.Buffer
	writeHeader(&buf, topOrder, wkbcommon.MultiPointID)
	writeUint32(&buf, topOrder, uint32(n))
	for range n {
		writePoint(&buf, subOrder, 1, 2)
	}
	return buf.Bytes()
}

// geometryCollectionWKB builds a GeometryCollection containing n points.
func geometryCollectionWKB(n int, order binary.ByteOrder) []byte {
	var buf bytes.Buffer
	writeHeader(&buf, order, wkbcommon.GeometryCollectionID)
	writeUint32(&buf, order, uint32(n))
	for range n {
		writePoint(&buf, order, 1, 2)
	}
	return buf.Bytes()
}

// nestedCollectionWKB builds depth nested GeometryCollections, the innermost
// containing one Point. Each level may use a different byte order.
func nestedCollectionWKB(depth int, orders ...binary.ByteOrder) []byte {
	var buf bytes.Buffer
	orderAt := func(level int) binary.ByteOrder {
		return orders[level%len(orders)]
	}
	for level := range depth {
		writeHeader(&buf, orderAt(level), wkbcommon.GeometryCollectionID)
		writeUint32(&buf, orderAt(level), 1)
	}
	writePoint(&buf, orderAt(depth), 3, 4)
	return buf.Bytes()
}

// lineStringWKB builds a LineString with n coordinate tuples.
func lineStringWKB(n int, order binary.ByteOrder) []byte {
	var buf bytes.Buffer
	writeHeader(&buf, order, wkbcommon.LineStringID)
	writeUint32(&buf, order, uint32(n))
	var b [8]byte
	for i := range n {
		for range 2 {
			order.PutUint64(b[:], math.Float64bits(float64(i)))
			buf.Write(b[:])
		}
	}
	return buf.Bytes()
}

// polygonWKB builds a Polygon whose first ring declares n points (the body is
// left truncated on purpose by callers that only need the header to be read).
func polygonRingHeaderWKB(n uint32, order binary.ByteOrder) []byte {
	var buf bytes.Buffer
	writeHeader(&buf, order, wkbcommon.PolygonID)
	writeUint32(&buf, order, 1) // one ring
	writeUint32(&buf, order, n) // ring point count
	return buf.Bytes()
}

func TestReadLimitsCoordinates(t *testing.T) {
	t.Run("linestring within budget", func(t *testing.T) {
		data := lineStringWKB(3, NDR)
		g, err := Unmarshal(data, wkbcommon.WKBOptionReadLimits(wkbcommon.ReadLimits{MaxCoordinates: 3}))
		assert.NoError(t, err)
		assert.Equal(t, 3, assertLineString(t, g).NumCoords())
	})

	t.Run("linestring over budget is rejected before reading the body", func(t *testing.T) {
		data := lineStringWKB(10, NDR)
		_, err := Unmarshal(data, wkbcommon.WKBOptionReadLimits(wkbcommon.ReadLimits{MaxCoordinates: 9}))
		var limitErr wkbcommon.ErrLimitExceeded
		assert.True(t, errors.As(err, &limitErr))
		assert.Equal(t, wkbcommon.ErrLimitExceeded{
			What:      "coordinates",
			Path:      "",
			Requested: 10,
			Used:      0,
			Limit:     9,
		}, limitErr)
	})

	t.Run("polygon ring over budget identifies the ring", func(t *testing.T) {
		data := polygonRingHeaderWKB(1_000_000, NDR)
		_, err := Unmarshal(data, wkbcommon.WKBOptionReadLimits(wkbcommon.ReadLimits{MaxCoordinates: 100}))
		var limitErr wkbcommon.ErrLimitExceeded
		assert.True(t, errors.As(err, &limitErr))
		assert.Equal(t, "coordinates", limitErr.What)
		assert.Equal(t, "ring 0", limitErr.Path)
		assert.Equal(t, int64(1_000_000), limitErr.Requested)
	})

	t.Run("coordinate budget is shared between nested sub-geometries", func(t *testing.T) {
		// A MultiPoint of 4 points needs 4 coordinate tuples in total.
		data := multiPointWKB(4, NDR, NDR)
		_, err := Unmarshal(data, wkbcommon.WKBOptionReadLimits(wkbcommon.ReadLimits{MaxCoordinates: 3}))
		var limitErr wkbcommon.ErrLimitExceeded
		assert.True(t, errors.As(err, &limitErr))
		assert.Equal(t, "coordinates", limitErr.What)
		// Three points were already read; the fourth one fails.
		assert.Equal(t, "member 3", limitErr.Path)
		assert.Equal(t, int64(3), limitErr.Used)
		assert.Equal(t, int64(1), limitErr.Requested)
	})

	t.Run("no coordinate limit means unlimited", func(t *testing.T) {
		data := multiPointWKB(100, NDR, NDR)
		g, err := Unmarshal(data)
		assert.NoError(t, err)
		assert.Equal(t, 100, assertMultiPoint(t, g).NumPoints())
	})
}

func TestReadLimitsMembers(t *testing.T) {
	t.Run("multipoint member limit", func(t *testing.T) {
		data := multiPointWKB(5, NDR, NDR)
		_, err := Unmarshal(data, wkbcommon.WKBOptionReadLimits(wkbcommon.ReadLimits{MaxMembers: 4}))
		var limitErr wkbcommon.ErrLimitExceeded
		assert.True(t, errors.As(err, &limitErr))
		assert.Equal(t, "members", limitErr.What)
		assert.Equal(t, "", limitErr.Path)
		assert.Equal(t, int64(5), limitErr.Requested)
	})

	t.Run("member budget shared across nesting levels", func(t *testing.T) {
		// GeometryCollection of 3 members, each member is a point. The
		// collection itself reserves 3 members, so a budget of 2 fails.
		data := geometryCollectionWKB(3, NDR)
		_, err := Unmarshal(data, wkbcommon.WKBOptionReadLimits(wkbcommon.ReadLimits{MaxMembers: 2}))
		var limitErr wkbcommon.ErrLimitExceeded
		assert.True(t, errors.As(err, &limitErr))
		assert.Equal(t, "members", limitErr.What)
		assert.Equal(t, int64(3), limitErr.Requested)
	})

	t.Run("polygon rings count as members", func(t *testing.T) {
		// A polygon declaring 3 rings needs 3 members.
		var buf bytes.Buffer
		writeHeader(&buf, NDR, wkbcommon.PolygonID)
		writeUint32(&buf, NDR, 3)
		_, err := Unmarshal(buf.Bytes(), wkbcommon.WKBOptionReadLimits(wkbcommon.ReadLimits{MaxMembers: 2}))
		var limitErr wkbcommon.ErrLimitExceeded
		assert.True(t, errors.As(err, &limitErr))
		assert.Equal(t, "members", limitErr.What)
		assert.Equal(t, int64(3), limitErr.Requested)
	})

	t.Run("member limit error names the offending member", func(t *testing.T) {
		// Outer collection of 2 members, the second being another collection
		// of 10 points. A member budget of 5 is exhausted inside member 1.
		var buf bytes.Buffer
		writeHeader(&buf, NDR, wkbcommon.GeometryCollectionID)
		writeUint32(&buf, NDR, 2)
		writePoint(&buf, NDR, 1, 2) // member 0: costs 1 member? no, points are not members
		buf.Write(geometryCollectionWKB(10, NDR))
		_, err := Unmarshal(buf.Bytes(), wkbcommon.WKBOptionReadLimits(wkbcommon.ReadLimits{MaxMembers: 5}))
		var limitErr wkbcommon.ErrLimitExceeded
		assert.True(t, errors.As(err, &limitErr))
		assert.Equal(t, "members", limitErr.What)
		assert.Equal(t, "member 1", limitErr.Path)
		assert.Equal(t, int64(10), limitErr.Requested)
		assert.Equal(t, int64(2), limitErr.Used)
	})
}

func TestReadLimitsDepth(t *testing.T) {
	t.Run("within depth", func(t *testing.T) {
		data := nestedCollectionWKB(2, NDR)
		g, err := Unmarshal(data, wkbcommon.WKBOptionReadLimits(wkbcommon.ReadLimits{MaxDepth: 4}))
		assert.NoError(t, err)
		assert.True(t, g != nil)
	})

	t.Run("depth exceeded identifies the path", func(t *testing.T) {
		data := nestedCollectionWKB(3, NDR)
		_, err := Unmarshal(data, wkbcommon.WKBOptionReadLimits(wkbcommon.ReadLimits{MaxDepth: 3}))
		var limitErr wkbcommon.ErrLimitExceeded
		assert.True(t, errors.As(err, &limitErr))
		assert.Equal(t, "depth", limitErr.What)
		// Top level is depth 1; the 4th geometry (the point at level 3) is
		// rejected.
		assert.Equal(t, "member 0/member 0/member 0", limitErr.Path)
		assert.Equal(t, int64(4), limitErr.Requested)
		assert.Equal(t, int64(3), limitErr.Limit)
	})
}

func TestReadLimitsAreIndependentPerRead(t *testing.T) {
	opts := wkbcommon.WKBOptionReadLimits(wkbcommon.ReadLimits{MaxCoordinates: 10})

	// Sequential reads each get a fresh budget: the second read is not
	// affected by the first having consumed its budget.
	first, err := Unmarshal(multiPointWKB(10, NDR, NDR), opts)
	assert.NoError(t, err)
	assert.Equal(t, 10, assertMultiPoint(t, first).NumPoints())
	second, err := Unmarshal(multiPointWKB(10, NDR, NDR), opts)
	assert.NoError(t, err)
	assert.Equal(t, 10, assertMultiPoint(t, second).NumPoints())

	// Concurrent reads must not share budget state.
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			g, err := Unmarshal(multiPointWKB(10, NDR, NDR), opts)
			assert.NoError(t, err)
			assert.Equal(t, 10, assertMultiPoint(t, g).NumPoints())
		}()
	}
	wg.Wait()

	// A read with limits and a read without limits on the same input do not
	// interfere.
	_, err = Unmarshal(multiPointWKB(50, NDR, NDR))
	assert.NoError(t, err)
	_, err = Unmarshal(multiPointWKB(11, NDR, NDR), opts)
	var limitErr wkbcommon.ErrLimitExceeded
	assert.True(t, errors.As(err, &limitErr))
}

func TestMixedByteOrder(t *testing.T) {
	// Top level big endian, sub-geometries little endian.
	data := multiPointWKB(3, XDR, NDR)
	g, err := Unmarshal(data, wkbcommon.WKBOptionReadLimits(wkbcommon.ReadLimits{
		MaxCoordinates: 3,
		MaxMembers:     3,
		MaxDepth:       3,
	}))
	assert.NoError(t, err)
	mp := assertMultiPoint(t, g)
	assert.Equal(t, 3, mp.NumPoints())
	for i := range 3 {
		assert.Equal(t, geom.XY, mp.Point(i).Layout())
		assert.Equal(t, []float64{1, 2}, mp.Point(i).FlatCoords())
	}

	// The reverse ordering also decodes and stays within budget.
	data = multiPointWKB(3, NDR, XDR)
	g, err = Unmarshal(data, wkbcommon.WKBOptionReadLimits(wkbcommon.ReadLimits{MaxCoordinates: 3}))
	assert.NoError(t, err)
	assert.Equal(t, 3, assertMultiPoint(t, g).NumPoints())

	// Alternating byte orders at every nesting level.
	data = nestedCollectionWKB(3, NDR, XDR)
	g, err = Unmarshal(data, wkbcommon.WKBOptionReadLimits(wkbcommon.ReadLimits{MaxDepth: 5}))
	assert.NoError(t, err)
	assert.True(t, g != nil)
}

func TestEmptyGeometriesKeepValuesUnderLimits(t *testing.T) {
	nanOpts := []wkbcommon.WKBOption{
		wkbcommon.WKBOptionEmptyPointHandling(wkbcommon.EmptyPointHandlingNaN),
		wkbcommon.WKBOptionReadLimits(wkbcommon.ReadLimits{
			MaxCoordinates: 1,
			MaxMembers:     0,
			MaxDepth:       2,
		}),
	}

	t.Run("empty point stays empty", func(t *testing.T) {
		original := geom.NewPointEmpty(geom.XY)
		data, err := Marshal(original, NDR, nanOpts...)
		assert.NoError(t, err)
		got, err := Unmarshal(data, nanOpts...)
		assert.NoError(t, err)
		assert.Equal[geom.T](t, original, got)
		assert.True(t, assertPoint(t, got).Empty())
	})

	t.Run("empty linestring", func(t *testing.T) {
		original := geom.NewLineString(geom.XY)
		data, err := Marshal(original, NDR)
		assert.NoError(t, err)
		got, err := Unmarshal(data, wkbcommon.WKBOptionReadLimits(wkbcommon.ReadLimits{MaxCoordinates: 0}))
		assert.NoError(t, err)
		assert.Equal[geom.T](t, original, got)
	})

	t.Run("empty multipoint", func(t *testing.T) {
		original := geom.NewMultiPoint(geom.XY)
		data, err := Marshal(original, NDR)
		assert.NoError(t, err)
		got, err := Unmarshal(data, wkbcommon.WKBOptionReadLimits(wkbcommon.ReadLimits{MaxMembers: 0}))
		assert.NoError(t, err)
		assert.Equal[geom.T](t, original, got)
	})

	t.Run("empty geometry collection keeps its layout", func(t *testing.T) {
		original := geom.NewGeometryCollection()
		assert.NoError(t, original.SetLayout(geom.XYZ))
		data, err := Marshal(original, NDR)
		assert.NoError(t, err)
		got, err := Unmarshal(data, wkbcommon.WKBOptionReadLimits(wkbcommon.ReadLimits{
			MaxMembers: 0,
			MaxDepth:   1,
		}))
		assert.NoError(t, err)
		assert.Equal[geom.T](t, original, got)
		assert.Equal(t, geom.XYZ, assertGeometryCollection(t, got).Layout())
	})

	t.Run("collection containing an empty point", func(t *testing.T) {
		original := geom.NewGeometryCollection().MustPush(geom.NewPointEmpty(geom.XY))
		data, err := Marshal(original, NDR, nanOpts...)
		assert.NoError(t, err)
		got, err := Unmarshal(data, nanOpts...)
		assert.NoError(t, err)
		assert.Equal[geom.T](t, original, got)
	})
}

func TestRejectionDoesNotTruncate(t *testing.T) {
	// When a limit is exceeded the whole read must fail rather than silently
	// dropping sub-geometries to fit the budget.
	data := multiPointWKB(6, NDR, NDR)
	g, err := Unmarshal(data, wkbcommon.WKBOptionReadLimits(wkbcommon.ReadLimits{
		MaxCoordinates: 5,
		MaxMembers:     6,
	}))
	assert.Error(t, err)
	assert.True(t, g == nil)
	var limitErr wkbcommon.ErrLimitExceeded
	assert.True(t, errors.As(err, &limitErr))
	assert.Contains(t, err.Error(), "member 5")
}

func TestHugeCountRejectedBeforeAllocation(t *testing.T) {
	// A LineString header declaring MaxUint32 coordinates: even with no
	// coordinate limit the stride multiplication overflows on 32-bit
	// platforms and the count itself exceeds any sensible budget on 64-bit
	// platforms. Either way it must fail before allocating.
	var buf bytes.Buffer
	writeHeader(&buf, NDR, wkbcommon.LineStringID)
	writeUint32(&buf, NDR, maxUint32Count)
	_, err := Unmarshal(buf.Bytes(), wkbcommon.WKBOptionReadLimits(wkbcommon.ReadLimits{
		MaxCoordinates: 1 << 20,
	}))
	assert.Error(t, err)
	var limitErr wkbcommon.ErrLimitExceeded
	assert.True(t, errors.As(err, &limitErr))
	assert.Equal(t, "coordinates", limitErr.What)
	assert.Equal(t, int64(maxUint32Count), limitErr.Requested)
}

func TestNestedMemberErrorPath(t *testing.T) {
	// MultiPolygon of 2 Polygons; the second polygon declares more rings
	// than the member budget allows. The error path must name the nested
	// member.
	var buf bytes.Buffer
	writeHeader(&buf, NDR, wkbcommon.MultiPolygonID)
	writeUint32(&buf, NDR, 2) // two polygon members
	// Polygon member 0 with one empty ring (zero points).
	writeHeader(&buf, NDR, wkbcommon.PolygonID)
	writeUint32(&buf, NDR, 1)
	writeUint32(&buf, NDR, 0)
	// Polygon member 1 declaring 10 rings.
	writeHeader(&buf, NDR, wkbcommon.PolygonID)
	writeUint32(&buf, NDR, 10)
	_, err := Unmarshal(buf.Bytes(), wkbcommon.WKBOptionReadLimits(wkbcommon.ReadLimits{
		MaxMembers: 5,
	}))
	var limitErr wkbcommon.ErrLimitExceeded
	assert.True(t, errors.As(err, &limitErr))
	assert.Equal(t, "members", limitErr.What)
	assert.Equal(t, "member 1", limitErr.Path)
	assert.Equal(t, int64(10), limitErr.Requested)
}

func TestFailingReadDoesNotAffectLaterReads(t *testing.T) {
	opts := wkbcommon.WKBOptionReadLimits(wkbcommon.ReadLimits{MaxCoordinates: 5})
	// A read that fails...
	_, err := Unmarshal(multiPointWKB(10, NDR, NDR), opts)
	assert.Error(t, err)
	// ...must leave the next read with a fresh budget.
	g, err := Unmarshal(multiPointWKB(5, NDR, NDR), opts)
	assert.NoError(t, err)
	assert.Equal(t, 5, assertMultiPoint(t, g).NumPoints())
}
