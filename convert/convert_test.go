package convert_test

import (
	"errors"
	"testing"

	"github.com/alecthomas/assert/v2"

	geom "github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/convert"
	"github.com/twpayne/go-geom/encoding/geojson"
	"github.com/twpayne/go-geom/encoding/wkt"
)

func TestContractConvertLayouts(t *testing.T) {
	allowAll := convert.Contract{
		OnPadZ:  convert.Allow,
		OnPadM:  convert.Allow,
		OnDropZ: convert.Allow,
		OnDropM: convert.Allow,
		PadZ:    -1,
		PadM:    -2,
	}
	for _, tc := range []struct {
		name       string
		srcLayout  geom.Layout
		flatCoords []float64
		dstLayout  geom.Layout
		wantCoords []float64
		wantReport convert.Report
	}{
		{
			name:       "XY to XY",
			srcLayout:  geom.XY,
			flatCoords: []float64{1, 2, 3, 4},
			dstLayout:  geom.XY,
			wantCoords: []float64{1, 2, 3, 4},
		},
		{
			name:       "XY to XYZ",
			srcLayout:  geom.XY,
			flatCoords: []float64{1, 2, 3, 4},
			dstLayout:  geom.XYZ,
			wantCoords: []float64{1, 2, -1, 3, 4, -1},
			wantReport: convert.Report{PaddedZ: 2},
		},
		{
			name:       "XY to XYM",
			srcLayout:  geom.XY,
			flatCoords: []float64{1, 2, 3, 4},
			dstLayout:  geom.XYM,
			wantCoords: []float64{1, 2, -2, 3, 4, -2},
			wantReport: convert.Report{PaddedM: 2},
		},
		{
			name:       "XY to XYZM",
			srcLayout:  geom.XY,
			flatCoords: []float64{1, 2, 3, 4},
			dstLayout:  geom.XYZM,
			wantCoords: []float64{1, 2, -1, -2, 3, 4, -1, -2},
			wantReport: convert.Report{PaddedZ: 2, PaddedM: 2},
		},
		{
			name:       "XYZ to XY",
			srcLayout:  geom.XYZ,
			flatCoords: []float64{1, 2, 10, 3, 4, 20},
			dstLayout:  geom.XY,
			wantCoords: []float64{1, 2, 3, 4},
			wantReport: convert.Report{DroppedZ: 2},
		},
		{
			name:       "XYZ to XYZ",
			srcLayout:  geom.XYZ,
			flatCoords: []float64{1, 2, 10, 3, 4, 20},
			dstLayout:  geom.XYZ,
			wantCoords: []float64{1, 2, 10, 3, 4, 20},
		},
		{
			name:       "XYZ to XYM",
			srcLayout:  geom.XYZ,
			flatCoords: []float64{1, 2, 10, 3, 4, 20},
			dstLayout:  geom.XYM,
			wantCoords: []float64{1, 2, -2, 3, 4, -2},
			wantReport: convert.Report{DroppedZ: 2, PaddedM: 2},
		},
		{
			name:       "XYZ to XYZM",
			srcLayout:  geom.XYZ,
			flatCoords: []float64{1, 2, 10, 3, 4, 20},
			dstLayout:  geom.XYZM,
			wantCoords: []float64{1, 2, 10, -2, 3, 4, 20, -2},
			wantReport: convert.Report{PaddedM: 2},
		},
		{
			name:       "XYM to XY",
			srcLayout:  geom.XYM,
			flatCoords: []float64{1, 2, 100, 3, 4, 200},
			dstLayout:  geom.XY,
			wantCoords: []float64{1, 2, 3, 4},
			wantReport: convert.Report{DroppedM: 2},
		},
		{
			name:       "XYM to XYZ",
			srcLayout:  geom.XYM,
			flatCoords: []float64{1, 2, 100, 3, 4, 200},
			dstLayout:  geom.XYZ,
			wantCoords: []float64{1, 2, -1, 3, 4, -1},
			wantReport: convert.Report{DroppedM: 2, PaddedZ: 2},
		},
		{
			name:       "XYM to XYM",
			srcLayout:  geom.XYM,
			flatCoords: []float64{1, 2, 100, 3, 4, 200},
			dstLayout:  geom.XYM,
			wantCoords: []float64{1, 2, 100, 3, 4, 200},
		},
		{
			name:       "XYM to XYZM",
			srcLayout:  geom.XYM,
			flatCoords: []float64{1, 2, 100, 3, 4, 200},
			dstLayout:  geom.XYZM,
			wantCoords: []float64{1, 2, -1, 100, 3, 4, -1, 200},
			wantReport: convert.Report{PaddedZ: 2},
		},
		{
			name:       "XYZM to XY",
			srcLayout:  geom.XYZM,
			flatCoords: []float64{1, 2, 10, 100, 3, 4, 20, 200},
			dstLayout:  geom.XY,
			wantCoords: []float64{1, 2, 3, 4},
			wantReport: convert.Report{DroppedZ: 2, DroppedM: 2},
		},
		{
			name:       "XYZM to XYZ",
			srcLayout:  geom.XYZM,
			flatCoords: []float64{1, 2, 10, 100, 3, 4, 20, 200},
			dstLayout:  geom.XYZ,
			wantCoords: []float64{1, 2, 10, 3, 4, 20},
			wantReport: convert.Report{DroppedM: 2},
		},
		{
			name:       "XYZM to XYM",
			srcLayout:  geom.XYZM,
			flatCoords: []float64{1, 2, 10, 100, 3, 4, 20, 200},
			dstLayout:  geom.XYM,
			wantCoords: []float64{1, 2, 100, 3, 4, 200},
			wantReport: convert.Report{DroppedZ: 2},
		},
		{
			name:       "XYZM to XYZM",
			srcLayout:  geom.XYZM,
			flatCoords: []float64{1, 2, 10, 100, 3, 4, 20, 200},
			dstLayout:  geom.XYZM,
			wantCoords: []float64{1, 2, 10, 100, 3, 4, 20, 200},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			contract := allowAll
			contract.TargetLayout = tc.dstLayout
			src := geom.NewLineStringFlat(tc.srcLayout, tc.flatCoords)
			converted, report, err := contract.Convert(src)
			assert.NoError(t, err)
			assert.Equal(t, tc.dstLayout, converted.Layout())
			assert.Equal(t, tc.wantCoords, converted.FlatCoords())
			assert.Equal(t, tc.wantReport, report)
			assert.Equal(t, tc.wantReport.Lossless(), tc.wantReport.DroppedZ == 0 && tc.wantReport.DroppedM == 0)
		})
	}
}

func TestContractConvertRejectsByDefault(t *testing.T) {
	for _, tc := range []struct {
		name      string
		srcLayout geom.Layout
		dstLayout geom.Layout
		wantOp    convert.Operation
	}{
		{"XY to XYZ", geom.XY, geom.XYZ, convert.PadZOperation},
		{"XY to XYM", geom.XY, geom.XYM, convert.PadMOperation},
		{"XY to XYZM", geom.XY, geom.XYZM, convert.PadZOperation},
		{"XYZ to XY", geom.XYZ, geom.XY, convert.DropZOperation},
		{"XYZ to XYM", geom.XYZ, geom.XYM, convert.DropZOperation},
		{"XYZ to XYZM", geom.XYZ, geom.XYZM, convert.PadMOperation},
		{"XYM to XY", geom.XYM, geom.XY, convert.DropMOperation},
		{"XYM to XYZ", geom.XYM, geom.XYZ, convert.DropMOperation},
		{"XYM to XYZM", geom.XYM, geom.XYZM, convert.PadZOperation},
		{"XYZM to XY", geom.XYZM, geom.XY, convert.DropZOperation},
		{"XYZM to XYZ", geom.XYZM, geom.XYZ, convert.DropMOperation},
		{"XYZM to XYM", geom.XYZM, geom.XYM, convert.DropZOperation},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := geom.NewLineStringFlat(tc.srcLayout, make([]float64, 2*tc.srcLayout.Stride()))
			converted, report, err := convert.Contract{TargetLayout: tc.dstLayout}.Convert(src)
			assert.True(t, converted == nil)
			assert.Equal(t, convert.Report{}, report)
			var policyErr convert.PolicyError
			assert.True(t, errors.As(err, &policyErr))
			assert.Equal(t, tc.wantOp, policyErr.Op)
			assert.Equal(t, tc.srcLayout, policyErr.SourceLayout)
			assert.Equal(t, tc.dstLayout, policyErr.TargetLayout)
		})
	}
}

func TestContractConvertGeoJSONRoundTrip(t *testing.T) {
	// A WKT point whose third dimension is M, not Z.
	g, err := wkt.Unmarshal(`POINT M (1 2 3)`)
	assert.NoError(t, err)
	assert.Equal(t, geom.XYM, g.Layout())

	// The default GeoJSON APIs are unchanged: encoding writes M as a plain
	// third position element, and decoding guesses it to be Z.
	data, err := geojson.Marshal(g)
	assert.NoError(t, err)
	assert.Equal(t, `{"type":"Point","coordinates":[1,2,3]}`, string(data))
	var decoded geom.T
	assert.NoError(t, geojson.Unmarshal(data, &decoded))
	assert.Equal(t, geom.XYZ, decoded.Layout())

	// Declaring the source layout explicitly recovers the measure.
	fixed, report, err := convert.Contract{SourceLayout: geom.XYM}.Convert(decoded)
	assert.NoError(t, err)
	assert.Equal(t, geom.XYM, fixed.Layout())
	assert.Equal(t, []float64{1, 2, 3}, fixed.FlatCoords())
	assert.True(t, report.Lossless())
	encoded, err := wkt.Marshal(fixed)
	assert.NoError(t, err)
	assert.Equal(t, `POINT M (1 2 3)`, encoded)
}

func TestContractConvertSourceLayout(t *testing.T) {
	// A non-empty geometry cannot be reinterpreted to a layout with a
	// different stride.
	src := geom.NewPointFlat(geom.XYZ, []float64{1, 2, 3})
	_, _, err := convert.Contract{SourceLayout: geom.XY}.Convert(src)
	var mismatch convert.ErrSourceLayoutMismatch
	assert.True(t, errors.As(err, &mismatch))
	assert.Equal(t, geom.XY, mismatch.Declared)
	assert.Equal(t, geom.XYZ, mismatch.Actual)

	// An empty geometry with no declared layout can be reinterpreted
	// freely, e.g. a point decoded from a null GeoJSON coordinates member.
	empty := geom.NewPointEmpty(geom.NoLayout)
	converted, report, err := convert.Contract{SourceLayout: geom.XYM}.Convert(empty)
	assert.NoError(t, err)
	assert.Equal(t, geom.XYM, converted.Layout())
	assert.True(t, converted.Empty())
	assert.Equal(t, convert.Report{}, report)

	// An empty geometry with a matching-stride layout can be reinterpreted.
	emptyXYZ := geom.NewPointEmpty(geom.XYZ)
	converted, _, err = convert.Contract{SourceLayout: geom.XYM}.Convert(emptyXYZ)
	assert.NoError(t, err)
	assert.Equal(t, geom.XYM, converted.Layout())
	assert.True(t, converted.Empty())
}

func TestContractConvertGeometryCollection(t *testing.T) {
	nested := geom.NewGeometryCollection().MustPush(
		geom.NewPointFlat(geom.XY, []float64{5, 6}),
	)
	gc := geom.NewGeometryCollection().MustPush(
		geom.NewPointFlat(geom.XYZ, []float64{1, 2, 3}),
		geom.NewLineStringFlat(geom.XYM, []float64{0, 0, 1, 1, 1, 2}),
		nested,
		geom.NewPointEmpty(geom.XYM),
	)
	converted, report, err := convert.Contract{
		TargetLayout: geom.XYZM,
		OnPadZ:       convert.Allow,
		OnPadM:       convert.Allow,
		PadZ:         -1,
		PadM:         -2,
	}.Convert(gc)
	assert.NoError(t, err)
	assert.Equal(t, geom.XYZM, converted.Layout())
	assert.Equal(t, convert.Report{PaddedZ: 3, PaddedM: 2}, report)
	assert.True(t, report.Lossless())

	convertedGC, ok := converted.(*geom.GeometryCollection)
	assert.True(t, ok)
	geoms := convertedGC.Geoms()
	assert.Equal(t, geom.XYZM, geoms[0].Layout())
	assert.Equal(t, []float64{1, 2, 3, -2}, geoms[0].FlatCoords())
	assert.Equal(t, geom.XYZM, geoms[1].Layout())
	assert.Equal(t, []float64{0, 0, -1, 1, 1, 1, -1, 2}, geoms[1].FlatCoords())
	nestedGC, ok := geoms[2].(*geom.GeometryCollection)
	assert.True(t, ok)
	assert.Equal(t, geom.XYZM, nestedGC.Layout())
	assert.Equal(t, []float64{5, 6, -1, -2}, nestedGC.Geom(0).FlatCoords())
	assert.Equal(t, geom.XYZM, geoms[3].Layout())
	assert.True(t, geoms[3].Empty())

	// The strict default contract rejects the same conversion.
	_, _, err = convert.Contract{TargetLayout: geom.XYZM}.Convert(gc)
	var policyErr convert.PolicyError
	assert.True(t, errors.As(err, &policyErr))

	// An empty collection adopts the target layout.
	emptyGC := geom.NewGeometryCollection()
	converted, _, err = convert.Contract{TargetLayout: geom.XY}.Convert(emptyGC)
	assert.NoError(t, err)
	assert.Equal(t, geom.XY, converted.Layout())
	assert.True(t, converted.Empty())
}

func TestContractConvertEmptyGeometries(t *testing.T) {
	// Empty geometries adopt the target layout but contribute no ordinates
	// to the report.
	emptyPoint := geom.NewPointEmpty(geom.XYZ)
	converted, report, err := convert.Contract{TargetLayout: geom.XY, OnDropZ: convert.Allow}.Convert(emptyPoint)
	assert.NoError(t, err)
	assert.Equal(t, geom.XY, converted.Layout())
	assert.True(t, converted.Empty())
	assert.Equal(t, convert.Report{}, report)
	assert.True(t, report.Lossless())

	// Policies are checked against the declared layouts even for empty
	// geometries.
	_, _, err = convert.Contract{TargetLayout: geom.XY}.Convert(emptyPoint)
	var policyErr convert.PolicyError
	assert.True(t, errors.As(err, &policyErr))
	assert.Equal(t, convert.DropZOperation, policyErr.Op)

	// An empty multi-polygon keeps its emptiness.
	emptyMultiPolygon := geom.NewMultiPolygonFlat(geom.XYZM, nil, nil)
	converted, _, err = convert.Contract{
		TargetLayout: geom.XYM,
		OnDropZ:      convert.Allow,
	}.Convert(emptyMultiPolygon)
	assert.NoError(t, err)
	assert.Equal(t, geom.XYM, converted.Layout())
	assert.True(t, converted.Empty())
}

func TestContractConvertMultiPointEmptyElements(t *testing.T) {
	// A MultiPoint with an empty first element keeps its ends structure.
	src := geom.NewMultiPointFlat(
		geom.XYZ,
		[]float64{1, 2, 3},
		geom.NewMultiPointFlatOptionWithEnds([]int{0, 3}),
	)
	converted, report, err := convert.Contract{
		TargetLayout: geom.XYZM,
		OnPadM:       convert.Allow,
		PadM:         -2,
	}.Convert(src)
	assert.NoError(t, err)
	mp, ok := converted.(*geom.MultiPoint)
	assert.True(t, ok)
	assert.Equal(t, []float64{1, 2, 3, -2}, mp.FlatCoords())
	assert.Equal(t, []int{0, 4}, mp.Ends())
	assert.Equal(t, 2, mp.NumPoints())
	assert.True(t, mp.Point(0).Empty())
	assert.Equal(t, convert.Report{PaddedM: 1}, report)
}

func TestContractConvertEndsScaling(t *testing.T) {
	// A Polygon's ends are scaled by the stride ratio.
	poly := geom.NewPolygonFlat(geom.XYZ, []float64{
		0, 0, 1, 1, 0, 2, 1, 1, 3,
		2, 2, 4, 3, 2, 5, 3, 3, 6,
	}, []int{9, 18})
	converted, report, err := convert.Contract{TargetLayout: geom.XY, OnDropZ: convert.Allow}.Convert(poly)
	assert.NoError(t, err)
	convertedPoly, ok := converted.(*geom.Polygon)
	assert.True(t, ok)
	assert.Equal(t, []float64{0, 0, 1, 0, 1, 1, 2, 2, 3, 2, 3, 3}, convertedPoly.FlatCoords())
	assert.Equal(t, []int{6, 12}, convertedPoly.Ends())
	assert.Equal(t, convert.Report{DroppedZ: 6}, report)

	// A MultiPolygon's endss are scaled likewise.
	multiPolygon := geom.NewMultiPolygonFlat(geom.XYZM, []float64{
		0, 0, 1, 7, 1, 0, 2, 8, 1, 1, 3, 9,
	}, [][]int{{12}})
	converted, report, err = convert.Contract{TargetLayout: geom.XYM, OnDropZ: convert.Allow}.Convert(multiPolygon)
	assert.NoError(t, err)
	convertedMP, ok := converted.(*geom.MultiPolygon)
	assert.True(t, ok)
	assert.Equal(t, []float64{0, 0, 7, 1, 0, 8, 1, 1, 9}, convertedMP.FlatCoords())
	assert.Equal(t, [][]int{{9}}, convertedMP.Endss())
	assert.Equal(t, convert.Report{DroppedZ: 3}, report)

	// A MultiLineString's ends are scaled likewise.
	mls := geom.NewMultiLineStringFlat(geom.XYZ, []float64{
		0, 0, 1, 1, 1, 2, 2, 2, 3,
	}, []int{6, 9})
	converted, report, err = convert.Contract{TargetLayout: geom.XY, OnDropZ: convert.Allow}.Convert(mls)
	assert.NoError(t, err)
	assert.Equal(t, []float64{0, 0, 1, 1, 2, 2}, converted.FlatCoords())
	assert.Equal(t, []int{4, 6}, converted.Ends())
	assert.Equal(t, convert.Report{DroppedZ: 3}, report)
}

func TestContractConvertLinearRing(t *testing.T) {
	ring := geom.NewLinearRingFlat(geom.XYM, []float64{0, 0, 1, 1, 0, 2, 1, 1, 3, 0, 0, 1})
	converted, report, err := convert.Contract{TargetLayout: geom.XY, OnDropM: convert.Allow}.Convert(ring)
	assert.NoError(t, err)
	_, ok := converted.(*geom.LinearRing)
	assert.True(t, ok)
	assert.Equal(t, []float64{0, 0, 1, 0, 1, 1, 0, 0}, converted.FlatCoords())
	assert.Equal(t, convert.Report{DroppedM: 4}, report)
}

func TestContractConvertSRID(t *testing.T) {
	src := geom.Must(geom.SetSRID(geom.NewPointFlat(geom.XY, []float64{1, 2}), 4326))

	// The default contract keeps the SRID.
	converted, _, err := convert.Contract{}.Convert(src)
	assert.NoError(t, err)
	assert.Equal(t, 4326, converted.SRID())

	// SRIDSet relabels the geometry without reprojecting it: the
	// coordinates are unchanged.
	converted, _, err = convert.Contract{SRID: convert.SRIDSet, SRIDValue: 3857}.Convert(src)
	assert.NoError(t, err)
	assert.Equal(t, 3857, converted.SRID())
	assert.Equal(t, []float64{1, 2}, converted.FlatCoords())

	// SRIDClear clears the SRID.
	converted, _, err = convert.Contract{SRID: convert.SRIDClear}.Convert(src)
	assert.NoError(t, err)
	assert.Equal(t, 0, converted.SRID())

	// The SRID policy applies recursively to collection members.
	gc := geom.NewGeometryCollection().MustPush(
		geom.NewPointFlat(geom.XY, []float64{1, 2}).SetSRID(4326),
		geom.NewPointFlat(geom.XY, []float64{3, 4}).SetSRID(4326),
	).SetSRID(4326)
	converted, _, err = convert.Contract{SRID: convert.SRIDSet, SRIDValue: 3857}.Convert(gc)
	assert.NoError(t, err)
	assert.Equal(t, 3857, converted.SRID())
	convertedGC, ok := converted.(*geom.GeometryCollection)
	assert.True(t, ok)
	geoms := convertedGC.Geoms()
	assert.Equal(t, 3857, geoms[0].SRID())
	assert.Equal(t, 3857, geoms[1].SRID())
}

func TestContractConvertDoesNotModifyInput(t *testing.T) {
	src := geom.NewLineStringFlat(geom.XYM, []float64{1, 2, 100, 3, 4, 200})
	converted, _, err := convert.Contract{TargetLayout: geom.XY, OnDropM: convert.Allow}.Convert(src)
	assert.NoError(t, err)
	assert.Equal(t, []float64{1, 2, 3, 4}, converted.FlatCoords())
	assert.Equal(t, geom.XYM, src.Layout())
	assert.Equal(t, []float64{1, 2, 100, 3, 4, 200}, src.FlatCoords())

	// Even an identity conversion returns a copy that does not alias the
	// input.
	identity, _, err := convert.Contract{}.Convert(src)
	assert.NoError(t, err)
	line, ok := identity.(*geom.LineString)
	assert.True(t, ok)
	line.FlatCoords()[0] = 99
	assert.Equal(t, 1.0, src.FlatCoords()[0])
}

func TestContractConvertUnsupported(t *testing.T) {
	// nil converts to nil.
	converted, report, err := convert.Contract{}.Convert(nil)
	assert.NoError(t, err)
	assert.True(t, converted == nil)
	assert.Equal(t, convert.Report{}, report)

	// Layouts with more than four dimensions are unsupported, both as
	// source and as target.
	g5 := geom.NewPointFlat(geom.Layout(5), []float64{1, 2, 3, 4, 5})
	_, _, err = convert.Contract{}.Convert(g5)
	var unsupported geom.ErrUnsupportedLayout
	assert.True(t, errors.As(err, &unsupported))
	assert.Equal(t, geom.Layout(5), geom.Layout(unsupported))
	_, _, err = convert.Contract{TargetLayout: geom.Layout(5)}.Convert(
		geom.NewPointFlat(geom.XY, []float64{1, 2}),
	)
	assert.True(t, errors.As(err, &unsupported))
	assert.Equal(t, geom.Layout(5), geom.Layout(unsupported))
}
