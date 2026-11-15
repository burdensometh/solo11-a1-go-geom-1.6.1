package convert_test

import (
	"fmt"

	geom "github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/convert"
	"github.com/twpayne/go-geom/encoding/geojson"
	"github.com/twpayne/go-geom/encoding/wkt"
)

// GeoJSON positions have no measure (M) dimension, so an XYM geometry
// silently round-trips through GeoJSON as XYZ. A convert.Contract makes the
// cross-format conversion explicit.
func ExampleContract_Convert_geoJSONRoundTrip() {
	// Start with a WKT point whose third dimension is M, not Z.
	g, err := wkt.Unmarshal(`POINT M (1 2 3)`)
	if err != nil {
		panic(err)
	}

	// The default GeoJSON encoding writes the M ordinate as a plain third
	// position element...
	data, err := geojson.Marshal(g)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(data))

	// ...and the default decoding guesses it to be Z.
	var decoded geom.T
	if err := geojson.Unmarshal(data, &decoded); err != nil {
		panic(err)
	}
	fmt.Println(decoded.Layout())

	// Declare the source layout explicitly to recover the measure.
	fixed, report, err := convert.Contract{SourceLayout: geom.XYM}.Convert(decoded)
	if err != nil {
		panic(err)
	}
	fmt.Println(fixed.Layout(), report.Lossless())
	recovered, err := wkt.Marshal(fixed)
	if err != nil {
		panic(err)
	}
	fmt.Println(recovered)

	// Output:
	// {"type":"Point","coordinates":[1,2,3]}
	// XYZ
	// XYM true
	// POINT M (1 2 3)
}

// The zero-value contract rejects any loss. Allowing losses explicitly
// returns a report of what was actually lost.
func ExampleContract_Convert_reportLosses() {
	g, err := wkt.Unmarshal(`LINESTRING M (0 0 1, 1 1 2)`)
	if err != nil {
		panic(err)
	}

	// GeoJSON cannot express measures, and the strict default contract
	// refuses to drop them silently.
	_, _, err = convert.Contract{TargetLayout: geom.XYZ}.Convert(g)
	fmt.Println(err)

	// Allow the losses explicitly and inspect what actually happened.
	converted, report, err := convert.Contract{
		TargetLayout: geom.XYZ,
		OnPadZ:       convert.Allow,
		OnDropM:      convert.Allow,
	}.Convert(g)
	if err != nil {
		panic(err)
	}
	fmt.Println("dropped M:", report.DroppedM, "padded Z:", report.PaddedZ)
	encoded, err := wkt.Marshal(converted)
	if err != nil {
		panic(err)
	}
	fmt.Println(encoded)

	// Output:
	// convert: conversion from XYM to XYZ requires dropping M ordinates, rejected by contract
	// dropped M: 2 padded Z: 2
	// LINESTRING Z (0 0 0, 1 1 0)
}

// Changing an SRID label never implies reprojection: only the label
// changes, never the coordinates.
func ExampleContract_Convert_sridRelabelIsNotProjection() {
	g, err := wkt.Unmarshal(`POINT (1 2)`)
	if err != nil {
		panic(err)
	}
	if g, err = geom.SetSRID(g, 4326); err != nil {
		panic(err)
	}
	relabelled, _, err := convert.Contract{
		SRID:      convert.SRIDSet,
		SRIDValue: 3857,
	}.Convert(g)
	if err != nil {
		panic(err)
	}
	fmt.Println(relabelled.SRID(), relabelled.FlatCoords())

	// Output:
	// 3857 [1 2]
}
