package ewkb

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

const (
	ewkbTestZ    = 0x80000000
	ewkbTestM    = 0x40000000
	ewkbTestSRID = 0x20000000
)

func ewkbHeader(buf *bytes.Buffer, order binary.ByteOrder, typeID uint32, srid uint32) {
	switch order {
	case NDR:
		buf.WriteByte(wkbcommon.NDRID)
	case XDR:
		buf.WriteByte(wkbcommon.XDRID)
	}
	if srid != 0 {
		typeID |= ewkbTestSRID
	}
	var b [4]byte
	order.PutUint32(b[:], typeID)
	buf.Write(b[:])
	if srid != 0 {
		order.PutUint32(b[:], srid)
		buf.Write(b[:])
	}
}

func ewkbUint32(buf *bytes.Buffer, order binary.ByteOrder, n uint32) {
	var b [4]byte
	order.PutUint32(b[:], n)
	buf.Write(b[:])
}

func ewkbPoint(buf *bytes.Buffer, order binary.ByteOrder, srid uint32, x, y float64) {
	ewkbHeader(buf, order, wkbcommon.PointID, srid)
	var b [8]byte
	order.PutUint64(b[:], math.Float64bits(x))
	buf.Write(b[:])
	order.PutUint64(b[:], math.Float64bits(y))
	buf.Write(b[:])
}

func ewkbMultiPoint(n int, topOrder, subOrder binary.ByteOrder) []byte {
	var buf bytes.Buffer
	ewkbHeader(&buf, topOrder, wkbcommon.MultiPointID, 4326)
	ewkbUint32(&buf, topOrder, uint32(n))
	for range n {
		// Sub-geometries do not repeat the SRID flag in EWKB.
		ewkbPoint(&buf, subOrder, 0, 1, 2)
	}
	return buf.Bytes()
}

func ewkbNestedCollections(depth int, order binary.ByteOrder) []byte {
	var buf bytes.Buffer
	for range depth {
		ewkbHeader(&buf, order, wkbcommon.GeometryCollectionID, 0)
		ewkbUint32(&buf, order, 1)
	}
	ewkbPoint(&buf, order, 0, 3, 4)
	return buf.Bytes()
}

func TestEWKBReadLimits(t *testing.T) {
	t.Run("coordinate budget shared across members", func(t *testing.T) {
		data := ewkbMultiPoint(4, NDR, NDR)
		_, err := Unmarshal(data, wkbcommon.WKBOptionReadLimits(wkbcommon.ReadLimits{MaxCoordinates: 3}))
		var limitErr wkbcommon.ErrLimitExceeded
		assert.True(t, errors.As(err, &limitErr))
		assert.Equal(t, "coordinates", limitErr.What)
		assert.Equal(t, "member 3", limitErr.Path)
		assert.Equal(t, int64(3), limitErr.Used)
	})

	t.Run("member budget", func(t *testing.T) {
		data := ewkbMultiPoint(5, NDR, NDR)
		_, err := Unmarshal(data, wkbcommon.WKBOptionReadLimits(wkbcommon.ReadLimits{MaxMembers: 4}))
		var limitErr wkbcommon.ErrLimitExceeded
		assert.True(t, errors.As(err, &limitErr))
		assert.Equal(t, "members", limitErr.What)
		assert.Equal(t, int64(5), limitErr.Requested)
	})

	t.Run("depth budget with path", func(t *testing.T) {
		data := ewkbNestedCollections(3, NDR)
		_, err := Unmarshal(data, wkbcommon.WKBOptionReadLimits(wkbcommon.ReadLimits{MaxDepth: 3}))
		var limitErr wkbcommon.ErrLimitExceeded
		assert.True(t, errors.As(err, &limitErr))
		assert.Equal(t, "depth", limitErr.What)
		assert.Equal(t, "member 0/member 0/member 0", limitErr.Path)
	})

	t.Run("limits do not affect valid reads without options", func(t *testing.T) {
		g, err := Unmarshal(ewkbMultiPoint(100, NDR, NDR))
		assert.NoError(t, err)
		mp, ok := g.(*geom.MultiPoint)
		assert.True(t, ok)
		assert.Equal(t, 100, mp.NumPoints())
		assert.Equal(t, 4326, mp.SRID())
	})
}

func TestEWKBReadLimitsIndependent(t *testing.T) {
	opts := wkbcommon.WKBOptionReadLimits(wkbcommon.ReadLimits{MaxCoordinates: 10, MaxMembers: 10})
	for range 10 {
		g, err := Unmarshal(ewkbMultiPoint(10, NDR, NDR), opts)
		assert.NoError(t, err)
		mp, ok := g.(*geom.MultiPoint)
		assert.True(t, ok)
		assert.Equal(t, 10, mp.NumPoints())
	}
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := Unmarshal(ewkbMultiPoint(10, NDR, NDR), opts)
			assert.NoError(t, err)
		}()
	}
	wg.Wait()
}

func TestEWKBMixedByteOrder(t *testing.T) {
	g, err := Unmarshal(ewkbMultiPoint(3, XDR, NDR), wkbcommon.WKBOptionReadLimits(wkbcommon.ReadLimits{
		MaxCoordinates: 3,
		MaxMembers:     3,
		MaxDepth:       3,
	}))
	assert.NoError(t, err)
	mp, ok := g.(*geom.MultiPoint)
	assert.True(t, ok)
	assert.Equal(t, 3, mp.NumPoints())
	for i := range 3 {
		assert.Equal(t, []float64{1, 2}, mp.Point(i).FlatCoords())
	}
	assert.Equal(t, 4326, mp.SRID())
}

func TestEWKBEmptyGeometries(t *testing.T) {
	t.Run("empty geometry collection round trip", func(t *testing.T) {
		original := geom.NewGeometryCollection()
		assert.NoError(t, original.SetLayout(geom.XY))
		data, err := Marshal(original, NDR)
		assert.NoError(t, err)
		got, err := Unmarshal(data, wkbcommon.WKBOptionReadLimits(wkbcommon.ReadLimits{
			MaxMembers: 0,
			MaxDepth:   1,
		}))
		assert.NoError(t, err)
		assert.Equal[geom.T](t, original, got)
	})

	t.Run("empty point as NaN round trip", func(t *testing.T) {
		original := geom.NewPointEmpty(geom.XY).SetSRID(4326)
		data, err := Marshal(original, NDR)
		assert.NoError(t, err)
		got, err := Unmarshal(data, wkbcommon.WKBOptionReadLimits(wkbcommon.ReadLimits{MaxCoordinates: 1}))
		assert.NoError(t, err)
		p, ok := got.(*geom.Point)
		assert.True(t, ok)
		assert.True(t, p.Empty())
		assert.Equal(t, 4326, p.SRID())
	})
}
