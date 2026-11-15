package dbscan

import (
	"fmt"
	"math"

	"github.com/twpayne/go-geom"
)

// validateObservations checks that IDs are present and unique and that
// coordinates are finite.
func validateObservations(obs []Observation) error {
	ids := make(map[string]struct{}, len(obs))
	for _, o := range obs {
		if o.ID == "" {
			return ErrEmptyID
		}
		if _, ok := ids[o.ID]; ok {
			return fmt.Errorf("%w: %q", ErrDuplicateID, o.ID)
		}
		ids[o.ID] = struct{}{}
		if math.IsNaN(o.X) || math.IsInf(o.X, 0) || math.IsNaN(o.Y) || math.IsInf(o.Y, 0) {
			return fmt.Errorf("%w: observation %q at (%v, %v)", ErrNonFiniteCoordinate, o.ID, o.X, o.Y)
		}
	}
	return nil
}

// ClusterMultiPoint clusters the points of mp, pairing point i with ids[i].
//
// The coordinates must be planar metres in a projected CRS; see the package
// documentation. Empty points are an error because they have no position.
//
// Pairing is by index, so reordering the points of mp together with ids
// produces the same result: output is keyed by ID and does not depend on
// index order.
func ClusterMultiPoint(mp *geom.MultiPoint, ids []string, cfg Config) (*Result, error) {
	if mp.NumPoints() != len(ids) {
		return nil, fmt.Errorf("%w: got %d IDs for %d points", ErrIDCountMismatch, len(ids), mp.NumPoints())
	}
	observations := make([]Observation, mp.NumPoints())
	for i := range observations {
		coord := mp.Coord(i)
		if coord == nil {
			return nil, fmt.Errorf("%w: point %d (%q)", ErrEmptyPoint, i, ids[i])
		}
		observations[i] = Observation{ID: ids[i], X: coord.X(), Y: coord.Y()}
	}
	return Cluster(observations, cfg)
}
