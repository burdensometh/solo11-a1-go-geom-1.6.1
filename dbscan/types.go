package dbscan

import (
	"errors"
	"fmt"
	"math"
)

var (
	// ErrNonMetricCRS is returned when Config.CRS does not declare projected,
	// metre-based planar coordinates. Longitude and latitude are angles and
	// must be projected to a metric coordinate reference system before use.
	ErrNonMetricCRS = errors.New("dbscan: coordinates must be projected planar metres")

	// ErrGeographicCRS is returned when Config.CRS declares geographic
	// longitude/latitude degrees. Degrees are angular, not metric planar,
	// units: project the observations to a metre-based CRS first, then cluster
	// them.
	ErrGeographicCRS = errors.New("dbscan: geographic degrees must be projected to metres before clustering")

	// ErrInvalidEpsMeters is returned when Config.EpsMeters is not a positive,
	// finite distance in metres.
	ErrInvalidEpsMeters = errors.New("dbscan: invalid EpsMeters")

	// ErrInvalidMinPts is returned when Config.MinPts is less than one.
	ErrInvalidMinPts = errors.New("dbscan: invalid MinPts")

	// ErrEmptyID is returned when an observation has an empty ID.
	ErrEmptyID = errors.New("dbscan: empty observation ID")

	// ErrDuplicateID is returned when two observations share an ID.
	ErrDuplicateID = errors.New("dbscan: duplicate observation ID")

	// ErrNonFiniteCoordinate is returned when an observation coordinate is
	// NaN or an infinity.
	ErrNonFiniteCoordinate = errors.New("dbscan: non-finite coordinate")

	// ErrCoordinateOutOfRange is returned when a coordinate is too far from
	// the origin for Config.EpsMeters to be used to build a spatial index.
	ErrCoordinateOutOfRange = errors.New("dbscan: coordinate out of range for EpsMeters")

	// ErrIDCountMismatch is returned when the number of IDs does not match the
	// number of points in a MultiPoint.
	ErrIDCountMismatch = errors.New("dbscan: ID count does not match point count")

	// ErrEmptyPoint is returned when a MultiPoint contains an empty point,
	// which has no position to cluster.
	ErrEmptyPoint = errors.New("dbscan: empty point has no position")
)

// A CRS declares the coordinate reference system of the observations.
//
// Only projected, metre-based planar coordinates can be clustered.
// CRSProjectedMeters accepts input. CRSGeographicDegrees exists so that a
// caller who knows its input is longitude/latitude receives a specific error
// instead of having degrees silently treated as metres; it is never accepted.
// Any other value is rejected as non-metric. Projecting to a metric CRS is the
// caller's responsibility.
type CRS int

const (
	// CRSUnknown is the zero value and is rejected by Cluster.
	CRSUnknown CRS = iota
	// CRSProjectedMeters declares coordinates in a projected coordinate
	// reference system whose linear unit is the metre.
	CRSProjectedMeters
	// CRSGeographicDegrees declares coordinates as longitude/latitude in
	// degrees. It exists so that callers who name their input honestly get a
	// specific error instead of silently treating degrees as metres. It is
	// always rejected: project the observations to a metric CRS first.
	CRSGeographicDegrees
)

// String returns a human-readable name for c.
func (c CRS) String() string {
	switch c {
	case CRSUnknown:
		return "unknown"
	case CRSProjectedMeters:
		return "projected-metres"
	case CRSGeographicDegrees:
		return "geographic-degrees"
	default:
		return fmt.Sprintf("CRS(%d)", int(c))
	}
}

// A Role is the density-based classification of an observation.
type Role int

const (
	// RoleNoise identifies an observation that is neither a core observation
	// nor within EpsMeters of one.
	RoleNoise Role = iota
	// RoleBorder identifies a non-core observation within EpsMeters of at
	// least one core observation. Border observations belong to a cluster.
	RoleBorder
	// RoleCore identifies an observation with at least MinPts observations,
	// including itself and any duplicates of itself, within EpsMeters.
	RoleCore
)

// String returns a human-readable name for r.
func (r Role) String() string {
	switch r {
	case RoleNoise:
		return "noise"
	case RoleBorder:
		return "border"
	case RoleCore:
		return "core"
	default:
		return fmt.Sprintf("Role(%d)", int(r))
	}
}

// An Observation is a single point observation at planar coordinates (X, Y)
// measured in metres in a projected CRS, carrying a business ID.
//
// Observations with equal coordinates are distinct observations: each one is
// counted separately when measuring neighbourhood density.
type Observation struct {
	ID string
	X  float64
	Y  float64
}

// A Config configures Cluster.
type Config struct {
	// EpsMeters is the neighbourhood radius in metres. It must be positive
	// and finite.
	EpsMeters float64
	// MinPts is the minimum number of observations, including the observation
	// itself and any duplicates of it, for an observation to be core. It must
	// be at least one.
	MinPts int
	// CRS must be CRSProjectedMeters. This makes the unit of the coordinates
	// explicit and prevents raw longitude/latitude from being treated as
	// metric planar data. CRSGeographicDegrees is recognized but rejected.
	CRS CRS
}

func (c Config) validate() error {
	switch c.CRS {
	case CRSProjectedMeters:
	case CRSGeographicDegrees:
		return fmt.Errorf("%w: got %s", ErrGeographicCRS, c.CRS)
	default:
		return fmt.Errorf("%w: got %s", ErrNonMetricCRS, c.CRS)
	}
	if !(c.EpsMeters > 0) || math.IsInf(c.EpsMeters, 1) {
		return fmt.Errorf("%w: %v", ErrInvalidEpsMeters, c.EpsMeters)
	}
	if c.MinPts < 1 {
		return fmt.Errorf("%w: %d", ErrInvalidMinPts, c.MinPts)
	}
	return nil
}

// A Classified is an observation together with its density-based role and the
// ID of the cluster it belongs to. ClusterID is empty for noise.
type Classified struct {
	Observation
	Role      Role
	ClusterID string
}

// A ResultCluster is a set of core observations connected through other core
// observations, together with the border observations assigned to it.
//
// ID is the lexicographically smallest ID among the cluster's core
// observations. Core and Border are sorted by observation ID.
type ResultCluster struct {
	ID     string
	Core   []Classified
	Border []Classified
}

// A Result is the output of Cluster.
//
// Clusters is sorted by cluster ID, Noise is sorted by observation ID, and
// Observations contains every input observation sorted by observation ID.
// No slice is nil for a successful call, including a cluster's Core and
// Border, so results can be compared directly with reflect.DeepEqual or
// similar.
type Result struct {
	Clusters     []ResultCluster
	Noise        []Classified
	Observations []Classified
}
