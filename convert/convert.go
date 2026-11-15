// Package convert provides explicit, contract-driven conversion of
// geometries between layouts, for use when moving between encodings with
// different layout expressiveness such as WKT, EWKB, and GeoJSON.
//
// Not all encodings can express all layouts. GeoJSON positions, for
// example, have no measure (M) dimension: a third position element is
// decoded as Z, so an XYM geometry silently round-trips through GeoJSON as
// XYZ. Instead of relying on such implicit guesses, a Contract declares the
// source and target layouts and an SRID policy, and states whether padding
// dimensions, dropping dimensions, or dropping measures is allowed or must
// be rejected. Contract.Convert then reports the actual losses that
// occurred.
//
// The WKT, EWKB, and GeoJSON encoders and decoders keep their default
// behavior; a Contract makes each cross-format conversion explicit at the
// call site. Changing an SRID label never implies reprojection: coordinates
// are only ever copied, synthesized from caller-provided values, or
// dropped, never transformed.
package convert

import (
	"fmt"
	"strconv"

	geom "github.com/twpayne/go-geom"
)

// An Operation is a dimension-changing operation that a DimensionPolicy
// governs.
type Operation int

const (
	// PadZOperation synthesizes Z ordinates required by the target layout
	// but absent from the source layout.
	PadZOperation Operation = iota
	// PadMOperation synthesizes M ordinates required by the target layout
	// but absent from the source layout.
	PadMOperation
	// DropZOperation discards Z ordinates present in the source layout but
	// absent from the target layout.
	DropZOperation
	// DropMOperation discards M ordinates that the target layout cannot
	// express.
	DropMOperation
)

// String returns a human-readable description of o.
func (o Operation) String() string {
	switch o {
	case PadZOperation:
		return "padding Z ordinates"
	case PadMOperation:
		return "padding M ordinates"
	case DropZOperation:
		return "dropping Z ordinates"
	case DropMOperation:
		return "dropping M ordinates"
	default:
		return "Operation(" + strconv.Itoa(int(o)) + ")"
	}
}

// A DimensionPolicy determines whether a dimension-changing Operation is
// permitted during a conversion.
type DimensionPolicy int

const (
	// Reject rejects the operation: conversion fails with a PolicyError.
	Reject DimensionPolicy = iota
	// Allow allows the operation: conversion performs it and records its
	// actual effect in the returned Report.
	Allow
)

// An SRIDPolicy determines how a conversion sets the SRID of the converted
// geometry. Changing an SRID label never implies reprojection.
type SRIDPolicy int

const (
	// SRIDKeep keeps the SRID of the source geometry.
	SRIDKeep SRIDPolicy = iota
	// SRIDSet sets the SRID of the converted geometry to the Contract's
	// SRIDValue. It only relabels the geometry; coordinates are unchanged.
	SRIDSet
	// SRIDClear clears the SRID of the converted geometry to zero.
	SRIDClear
)

// A Contract is an explicit agreement on how a geometry is converted
// between layouts, typically when moving between encodings with different
// layout expressiveness.
//
// The zero value is the strictest useful contract: it keeps the source
// layout and SRID and rejects every dimension-changing Operation, so any
// conversion that would pad or drop ordinates fails with a PolicyError.
type Contract struct {
	// SourceLayout, if not geom.NoLayout, declares the layout of the input
	// geometry, overriding the geometry's own layout. Use it to correct
	// layouts guessed by encodings that cannot express all dimensions, e.g.
	// to declare that the third ordinate of a geometry decoded from GeoJSON
	// is M, not Z. Reinterpreting a non-empty geometry requires the
	// declared layout to have the same stride as the actual layout, so that
	// no ordinate changes position; empty geometries carry no ordinates and
	// can always be reinterpreted. If geom.NoLayout, each (sub)geometry's
	// own layout is used.
	SourceLayout geom.Layout

	// TargetLayout is the layout of the converted geometry. If
	// geom.NoLayout, the (possibly reinterpreted) source layout is kept.
	TargetLayout geom.Layout

	// OnPadZ and OnPadM determine whether Z or M ordinates required by
	// TargetLayout but absent from the source layout may be synthesized
	// from PadZ and PadM respectively.
	OnPadZ DimensionPolicy
	OnPadM DimensionPolicy

	// PadZ and PadM are the values of synthesized Z and M ordinates.
	PadZ float64
	PadM float64

	// OnDropZ determines whether Z ordinates present in the source layout
	// but absent from TargetLayout may be discarded.
	OnDropZ DimensionPolicy

	// OnDropM determines whether M ordinates may be discarded because
	// TargetLayout cannot express measures.
	OnDropM DimensionPolicy

	// SRID determines how the SRID of the converted geometry is set.
	// Changing an SRID label never implies reprojection: coordinates are
	// never transformed.
	SRID SRIDPolicy

	// SRIDValue is the SRID label applied when SRID is SRIDSet.
	SRIDValue int
}

// Convert converts g according to c and returns the converted geometry
// together with a report of the actual losses and additions. g is not
// modified. GeometryCollections are converted recursively; empty geometries
// keep their emptiness and adopt the target layout.
func (c Contract) Convert(g geom.T) (geom.T, Report, error) {
	var report Report
	if g == nil {
		return nil, report, nil
	}
	converted, err := c.convert(g, &report)
	if err != nil {
		return nil, Report{}, err
	}
	return converted, report, nil
}

// convert is the recursive implementation of Convert.
func (c Contract) convert(g geom.T, report *Report) (geom.T, error) {
	srcLayout, err := sourceLayout(g, c.SourceLayout)
	if err != nil {
		return nil, err
	}
	if err := checkLayout(srcLayout); err != nil {
		return nil, err
	}
	dstLayout := c.TargetLayout
	if dstLayout == geom.NoLayout {
		dstLayout = srcLayout
	}
	if err := checkLayout(dstLayout); err != nil {
		return nil, err
	}
	plan, err := c.makePlan(srcLayout, dstLayout)
	if err != nil {
		return nil, err
	}

	srcStride := srcLayout.Stride()
	dstStride := dstLayout.Stride()
	var converted geom.T
	switch g := g.(type) {
	case *geom.Point:
		converted = geom.NewPointFlat(dstLayout, convertFlatCoords(g.FlatCoords(), srcStride, plan, report))
	case *geom.LineString:
		converted = geom.NewLineStringFlat(dstLayout, convertFlatCoords(g.FlatCoords(), srcStride, plan, report))
	case *geom.LinearRing:
		converted = geom.NewLinearRingFlat(dstLayout, convertFlatCoords(g.FlatCoords(), srcStride, plan, report))
	case *geom.Polygon:
		converted = geom.NewPolygonFlat(
			dstLayout,
			convertFlatCoords(g.FlatCoords(), srcStride, plan, report),
			scaleEnds(g.Ends(), srcStride, dstStride),
		)
	case *geom.MultiPoint:
		converted = geom.NewMultiPointFlat(
			dstLayout,
			convertFlatCoords(g.FlatCoords(), srcStride, plan, report),
			geom.NewMultiPointFlatOptionWithEnds(scaleEnds(g.Ends(), srcStride, dstStride)),
		)
	case *geom.MultiLineString:
		converted = geom.NewMultiLineStringFlat(
			dstLayout,
			convertFlatCoords(g.FlatCoords(), srcStride, plan, report),
			scaleEnds(g.Ends(), srcStride, dstStride),
		)
	case *geom.MultiPolygon:
		converted = geom.NewMultiPolygonFlat(
			dstLayout,
			convertFlatCoords(g.FlatCoords(), srcStride, plan, report),
			scaleEndss(g.Endss(), srcStride, dstStride),
		)
	case *geom.GeometryCollection:
		gc := geom.NewGeometryCollection()
		geoms := make([]geom.T, len(g.Geoms()))
		for i, sub := range g.Geoms() {
			convertedSub, err := c.convert(sub, report)
			if err != nil {
				return nil, err
			}
			geoms[i] = convertedSub
		}
		if err := gc.Push(geoms...); err != nil {
			return nil, err
		}
		// A collection's layout is derived from its members unless the
		// contract declares one.
		if declaredLayout := c.TargetLayout; declaredLayout != geom.NoLayout {
			if err := gc.SetLayout(declaredLayout); err != nil {
				return nil, err
			}
		} else if c.SourceLayout != geom.NoLayout {
			if err := gc.SetLayout(srcLayout); err != nil {
				return nil, err
			}
		}
		converted = gc
	default:
		return nil, geom.ErrUnsupportedType{Value: g}
	}
	return geom.SetSRID(converted, c.srid(g))
}

// A layoutPlan describes how to convert coordinates between two layouts.
type layoutPlan struct {
	dims  []dimPlan
	dropZ bool
	dropM bool
}

// A dimPlan describes how to produce one target ordinate: copy it from
// srcIndex of the source coordinate, or, when srcIndex is -1, synthesize it
// as padValue.
type dimPlan struct {
	srcIndex int
	padValue float64
	pad      padKind
}

// A padKind identifies the Report counter for a synthesized ordinate.
type padKind int

const (
	padNone padKind = iota
	padZ
	padM
)

// makePlan returns the plan for converting coordinates from srcLayout to
// dstLayout, checking every required operation against the Contract's
// dimension policies.
func (c Contract) makePlan(srcLayout, dstLayout geom.Layout) (layoutPlan, error) {
	srcZIndex, srcMIndex := srcLayout.ZIndex(), srcLayout.MIndex()
	dstZIndex, dstMIndex := dstLayout.ZIndex(), dstLayout.MIndex()

	plan := layoutPlan{
		dims:  make([]dimPlan, dstLayout.Stride()),
		dropZ: srcZIndex != -1 && dstZIndex == -1,
		dropM: srcMIndex != -1 && dstMIndex == -1,
	}
	if plan.dropZ && c.OnDropZ != Allow {
		return layoutPlan{}, PolicyError{Op: DropZOperation, SourceLayout: srcLayout, TargetLayout: dstLayout}
	}
	if plan.dropM && c.OnDropM != Allow {
		return layoutPlan{}, PolicyError{Op: DropMOperation, SourceLayout: srcLayout, TargetLayout: dstLayout}
	}
	for i := range plan.dims {
		switch i {
		case dstZIndex:
			if srcZIndex != -1 {
				plan.dims[i] = dimPlan{srcIndex: srcZIndex}
			} else {
				if c.OnPadZ != Allow {
					return layoutPlan{}, PolicyError{Op: PadZOperation, SourceLayout: srcLayout, TargetLayout: dstLayout}
				}
				plan.dims[i] = dimPlan{srcIndex: -1, padValue: c.PadZ, pad: padZ}
			}
		case dstMIndex:
			if srcMIndex != -1 {
				plan.dims[i] = dimPlan{srcIndex: srcMIndex}
			} else {
				if c.OnPadM != Allow {
					return layoutPlan{}, PolicyError{Op: PadMOperation, SourceLayout: srcLayout, TargetLayout: dstLayout}
				}
				plan.dims[i] = dimPlan{srcIndex: -1, padValue: c.PadM, pad: padM}
			}
		default:
			plan.dims[i] = dimPlan{srcIndex: i}
		}
	}
	return plan, nil
}

// srid returns the SRID label to apply to the conversion of g.
func (c Contract) srid(g geom.T) int {
	switch c.SRID {
	case SRIDSet:
		return c.SRIDValue
	case SRIDClear:
		return 0
	default:
		return g.SRID()
	}
}

// A Report describes the actual losses and additions that occurred during a
// conversion. Counts are in ordinates, not geometries: converting a
// LineString with three coordinates from XYZ to XY reports DroppedZ == 3.
// Empty geometries contain no ordinates and therefore never contribute to a
// Report, even when their layout changes.
type Report struct {
	// DroppedZ is the number of discarded Z ordinates.
	DroppedZ int
	// DroppedM is the number of M ordinates discarded because the target
	// layout cannot express measures.
	DroppedM int
	// PaddedZ is the number of synthesized Z ordinates.
	PaddedZ int
	// PaddedM is the number of synthesized M ordinates.
	PaddedM int
}

// Lossless reports whether the conversion discarded no ordinates.
func (r Report) Lossless() bool {
	return r.DroppedZ == 0 && r.DroppedM == 0
}

// A PolicyError is returned when a conversion requires an Operation that
// the Contract's DimensionPolicy rejects.
type PolicyError struct {
	Op           Operation
	SourceLayout geom.Layout
	TargetLayout geom.Layout
}

// Error implements error.
func (e PolicyError) Error() string {
	return fmt.Sprintf(
		"convert: conversion from %s to %s requires %s, rejected by contract",
		e.SourceLayout, e.TargetLayout, e.Op,
	)
}

// An ErrSourceLayoutMismatch is returned when a Contract's declared
// SourceLayout is incompatible with a non-empty geometry's actual layout.
type ErrSourceLayoutMismatch struct {
	Declared geom.Layout
	Actual   geom.Layout
}

// Error implements error.
func (e ErrSourceLayoutMismatch) Error() string {
	return fmt.Sprintf(
		"convert: declared source layout %s is incompatible with actual layout %s",
		e.Declared, e.Actual,
	)
}

// sourceLayout returns the layout to use for g: the declared layout if it
// is compatible with g, otherwise g's own layout.
func sourceLayout(g geom.T, declared geom.Layout) (geom.Layout, error) {
	actual := g.Layout()
	if declared == geom.NoLayout || declared == actual {
		return actual, nil
	}
	// Empty geometries carry no ordinates and can be reinterpreted freely;
	// non-empty geometries require matching strides so that no ordinate
	// changes position.
	if g.Empty() || declared.Stride() == actual.Stride() {
		return declared, nil
	}
	return geom.NoLayout, ErrSourceLayoutMismatch{Declared: declared, Actual: actual}
}

// checkLayout returns an error if layout is not supported by this package.
func checkLayout(layout geom.Layout) error {
	switch layout {
	case geom.NoLayout, geom.XY, geom.XYZ, geom.XYM, geom.XYZM:
		return nil
	default:
		return geom.ErrUnsupportedLayout(layout)
	}
}

// convertFlatCoords converts flat coordinates from a source layout with
// stride srcStride according to plan, recording actual losses and additions
// in report.
func convertFlatCoords(src []float64, srcStride int, plan layoutPlan, report *Report) []float64 {
	if len(src) == 0 || srcStride == 0 {
		return nil
	}
	dst := make([]float64, 0, len(src)/srcStride*len(plan.dims))
	numCoords := 0
	for i := 0; i < len(src); i += srcStride {
		for _, dim := range plan.dims {
			if dim.srcIndex >= 0 {
				dst = append(dst, src[i+dim.srcIndex])
				continue
			}
			dst = append(dst, dim.padValue)
			switch dim.pad {
			case padZ:
				report.PaddedZ++
			case padM:
				report.PaddedM++
			case padNone:
			}
		}
		numCoords++
	}
	if plan.dropZ {
		report.DroppedZ += numCoords
	}
	if plan.dropM {
		report.DroppedM += numCoords
	}
	return dst
}

// scaleEnds converts end indexes from srcStride to dstStride coordinates.
func scaleEnds(ends []int, srcStride, dstStride int) []int {
	if len(ends) == 0 {
		return nil
	}
	scaled := make([]int, len(ends))
	for i, end := range ends {
		scaled[i] = end / srcStride * dstStride
	}
	return scaled
}

// scaleEndss converts nested end indexes from srcStride to dstStride
// coordinates.
func scaleEndss(endss [][]int, srcStride, dstStride int) [][]int {
	if len(endss) == 0 {
		return nil
	}
	scaled := make([][]int, len(endss))
	for i, ends := range endss {
		scaled[i] = scaleEnds(ends, srcStride, dstStride)
	}
	return scaled
}
