// Package dbscan implements density-based spatial clustering of applications
// with noise (DBSCAN) for planar point observations.
//
// # Planar metric input only
//
// The algorithm operates on projected, planar coordinates whose linear unit is
// the metre. Longitude and latitude are angles, not a linear unit, and must be
// projected to a metric coordinate reference system (for example a local UTM
// zone or another suitable local projection) before being passed to Cluster.
// Config.CRS must explicitly declare CRSProjectedMeters and Config.EpsMeters
// names the unit of the neighbourhood radius, so a radius in degrees cannot be
// mistaken for a radius in metres. Declaring CRSGeographicDegrees is
// recognized and rejected with ErrGeographicCRS rather than silently
// interpreted as metres, and any other CRS is rejected with ErrNonMetricCRS.
//
// A degree of longitude spans a different ground distance depending on
// latitude, so clustering raw longitude/latitude with a distance threshold in
// degrees does not measure ground distance at all. Always project first.
//
// # Duplicate observations
//
// Every element of the input is a distinct observation. Two observations may
// share the same coordinate; they are not merged. Duplicate observations are
// indexed separately and each of them counts towards the density of a
// neighbourhood, so observing the same location several times makes that
// location denser in exactly the same way as observing nearby distinct
// locations.
//
// # Roles
//
// Each observation is classified as RoleCore, RoleBorder, or RoleNoise:
//
//   - A core observation has at least Config.MinPts observations within
//     Config.EpsMeters, including itself and any duplicates of itself.
//   - A border observation is not core but has at least one core observation
//     within Config.EpsMeters.
//   - A noise observation is neither core nor a neighbour of a core
//     observation.
//
// Core observations are grouped into clusters by connectivity through other
// core observations. A border observation that is a neighbour of core
// observations belonging to more than one cluster is assigned to the cluster
// of its nearest core observation; ties are broken by choosing the core
// observation with the lexicographically smallest ID.
//
// # Stability
//
// The result does not depend on the order of the input. Cluster IDs and the
// ordering of every returned slice are derived from observation IDs, so
// permuting the input changes neither the clusters nor the roles attached to
// each business ID.
package dbscan

import (
	"sort"
)

// Cluster runs DBSCAN over observations using cfg.
//
// Every observation is treated as a distinct point, so duplicate coordinates
// increase the observed density. The returned Result depends on the set of
// observations only, not on their order in observations.
func Cluster(observations []Observation, cfg Config) (*Result, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if err := validateObservations(observations); err != nil {
		return nil, err
	}

	// Work on a copy sorted by ID, so that every iteration order below is
	// determined by business IDs rather than by the caller's input order.
	obs := make([]Observation, len(observations))
	copy(obs, observations)
	sort.Slice(obs, func(i, j int) bool { return obs[i].ID < obs[j].ID })

	idx, err := newGridIndex(obs, cfg.EpsMeters)
	if err != nil {
		return nil, err
	}

	n := len(obs)
	roles := make([]Role, n)
	neighbors := make([][]int, n)
	for i := range obs {
		var ns []int
		idx.forEachNeighbor(i, func(j int) {
			ns = append(ns, j)
		})
		neighbors[i] = ns
		if len(ns) >= cfg.MinPts {
			roles[i] = RoleCore
		}
	}

	// label[i] is the index of the representative core observation of i's
	// cluster, or -1 if i is unassigned. The representative is the first core
	// observation of the component encountered in ID order, which because obs
	// is ID-sorted is the lexicographically smallest core ID in the component.
	label := make([]int, n)
	for i := range label {
		label[i] = -1
	}

	// Build core-connected components in ID order. Because obs is sorted by
	// ID, the first core index encountered in a component is the smallest one.
	var stack []int
	for i := range obs {
		if roles[i] != RoleCore || label[i] >= 0 {
			continue
		}
		label[i] = i
		stack = append(stack[:0], i)
		for len(stack) > 0 {
			k := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			for _, j := range neighbors[k] {
				if roles[j] == RoleCore && label[j] < 0 {
					label[j] = i
					stack = append(stack, j)
				}
			}
		}
	}

	// Assign border observations. A border observation may touch several
	// clusters; choose the cluster of its nearest core observation, breaking
	// distance ties by the lexicographically smallest core ID.
	//
	// The winner is selected by an explicit (distance, ID) comparison rather
	// than by taking the first neighbour encountered, because neighbours are
	// returned in spatial-index order, which is not stable.
	for i := range obs {
		if roles[i] == RoleCore {
			continue
		}
		bestDist := 0.0
		bestRep := -1
		for _, j := range neighbors[i] {
			if roles[j] != RoleCore {
				continue
			}
			dx := obs[j].X - obs[i].X
			dy := obs[j].Y - obs[i].Y
			d := dx*dx + dy*dy
			nearest := bestRep < 0 ||
				d < bestDist ||
				(d == bestDist && obs[j].ID < obs[bestRep].ID)
			if nearest {
				bestDist = d
				bestRep = j
			}
		}
		if bestRep >= 0 {
			roles[i] = RoleBorder
			label[i] = label[bestRep]
		}
	}

	// Collect clusters keyed by the representative core index.
	clusterIndex := make(map[int]int, n)
	result := &Result{
		Clusters:     []ResultCluster{},
		Noise:        []Classified{},
		Observations: make([]Classified, n),
	}
	for i := range obs {
		c := Classified{
			Observation: obs[i],
			Role:        roles[i],
		}
		switch roles[i] {
		case RoleCore, RoleBorder:
			rep := label[i]
			ci, ok := clusterIndex[rep]
			if !ok {
				ci = len(result.Clusters)
				clusterIndex[rep] = ci
				result.Clusters = append(result.Clusters, ResultCluster{ID: obs[rep].ID})
			}
			c.ClusterID = obs[rep].ID
			if roles[i] == RoleCore {
				result.Clusters[ci].Core = append(result.Clusters[ci].Core, c)
			} else {
				result.Clusters[ci].Border = append(result.Clusters[ci].Border, c)
			}
		default:
			result.Noise = append(result.Noise, c)
		}
		result.Observations[i] = c
	}

	// obs is ID-sorted, so each cluster's Core and Border and the Noise slice
	// are already ID-sorted. Sort clusters by ID to remove reliance on the
	// order in which representatives first appeared.
	sort.Slice(result.Clusters, func(a, b int) bool {
		return result.Clusters[a].ID < result.Clusters[b].ID
	})
	// A cluster can consist of core observations only, so initialize the
	// slices here to keep the zero value of a ResultCluster's slices out of the
	// output and to make results directly comparable.
	for i := range result.Clusters {
		if result.Clusters[i].Core == nil {
			result.Clusters[i].Core = []Classified{}
		}
		if result.Clusters[i].Border == nil {
			result.Clusters[i].Border = []Classified{}
		}
	}
	return result, nil
}
