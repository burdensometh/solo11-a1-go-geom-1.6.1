package wkbcommon

// EmptyPointHandling is the mechanism to handle an empty point.
type EmptyPointHandling uint8

const (
	// EmptyPointHandlingError will error if an empty point is found.
	EmptyPointHandlingError EmptyPointHandling = iota
	// EmptyPointHandlingNaN will decipher empty points with NaN as coordinates.
	// This is in line with Requirement 152 of the GeoPackage spec (http://www.geopackage.org/spec/).
	EmptyPointHandlingNaN
)

// WKBParams are parameters for encoding and decoding WKB items.
type WKBParams struct {
	EmptyPointHandling EmptyPointHandling
	ReadLimits         ReadLimits
}

// WKBOption is an option to set on WKBParams.
type WKBOption func(WKBParams) WKBParams

// WKBOptionEmptyPointHandling sets the params to the specified EmptyPointHandling.
func WKBOptionEmptyPointHandling(h EmptyPointHandling) WKBOption {
	return func(p WKBParams) WKBParams {
		p.EmptyPointHandling = h
		return p
	}
}

// WKBOptionReadLimits sets the per-read resource limits. The limits apply to a
// single Read or Unmarshal call and are independent of every other call.
func WKBOptionReadLimits(limits ReadLimits) WKBOption {
	return func(p WKBParams) WKBParams {
		p.ReadLimits = limits
		return p
	}
}

// InitWKBParams initializes WKBParams from an initial parameter and some options.
func InitWKBParams(params WKBParams, opts ...WKBOption) WKBParams {
	for _, opt := range opts {
		params = opt(params)
	}
	return params
}
