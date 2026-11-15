package dbscan

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"testing"

	"github.com/alecthomas/assert/v2"

	"github.com/twpayne/go-geom"
)

var testConfig = Config{EpsMeters: 10, MinPts: 3, CRS: CRSProjectedMeters}

// idsOf returns the IDs of cs.
func idsOf(cs []Classified) []string {
	ids := make([]string, len(cs))
	for i, c := range cs {
		ids[i] = c.ID
	}
	return ids
}

func TestClusterRoles(t *testing.T) {
	// Two well-separated triples plus one lone observation. With MinPts=3 and
	// EpsMeters=10, the observations at distance 1 from each other are core.
	obs := []Observation{
		{ID: "a1", X: 0, Y: 0},
		{ID: "a2", X: 1, Y: 0},
		{ID: "a3", X: 0, Y: 1},
		{ID: "b1", X: 100, Y: 0},
		{ID: "b2", X: 101, Y: 0},
		{ID: "b3", X: 100, Y: 1},
		{ID: "z", X: 50, Y: 50},
	}
	result, err := Cluster(obs, testConfig)
	assert.NoError(t, err)

	assert.Equal(t, []string{"a1", "b1"}, clusterIDsOf(t, result))

	roles := roleByID(result)
	for _, id := range []string{"a1", "a2", "a3"} {
		assert.Equal(t, RoleCore, roles[id], id)
	}
	for _, id := range []string{"b1", "b2", "b3"} {
		assert.Equal(t, RoleCore, roles[id], id)
	}
	assert.Equal(t, RoleNoise, roles["z"])
	assert.Equal(t, []string{"z"}, idsOf(result.Noise))
}

func clusterIDsOf(t *testing.T, result *Result) []string {
	t.Helper()
	ids := make([]string, len(result.Clusters))
	for i, c := range result.Clusters {
		ids[i] = c.ID
	}
	return ids
}

func roleByID(result *Result) map[string]Role {
	roles := make(map[string]Role, len(result.Observations))
	for _, c := range result.Observations {
		roles[c.ID] = c.Role
	}
	return roles
}

func TestClusterBorder(t *testing.T) {
	// Core observations along a line, each within eps of at least three
	// observations. "edge" is within eps of only "d" (and itself), so it is
	// not core, but it is within eps of a core observation and is therefore a
	// border observation of d's cluster.
	obs := []Observation{
		{ID: "a", X: 0, Y: 0},
		{ID: "b", X: 5, Y: 0},
		{ID: "c", X: 10, Y: 0},
		{ID: "d", X: 15, Y: 0},
		{ID: "edge", X: 25, Y: 0}, // 10 from d, 15 from c
	}
	result, err := Cluster(obs, testConfig)
	assert.NoError(t, err)

	roles := roleByID(result)
	for _, id := range []string{"a", "b", "c", "d"} {
		assert.Equal(t, RoleCore, roles[id], id)
	}
	assert.Equal(t, RoleBorder, roles["edge"])
	assert.Equal(t, 1, len(result.Clusters))
	assert.Equal(t, []string{"a", "b", "c", "d"}, idsOf(result.Clusters[0].Core))
	assert.Equal(t, []string{"edge"}, idsOf(result.Clusters[0].Border))
	assert.Equal(t, 0, len(result.Noise))
}

func TestDuplicateCoordinatesCountTowardsDensity(t *testing.T) {
	// Three observations at exactly the same coordinate. Each is a distinct
	// observation and together they satisfy MinPts=3 even though the
	// coordinate is unique.
	obs := []Observation{
		{ID: "dup-1", X: 5, Y: 5},
		{ID: "dup-2", X: 5, Y: 5},
		{ID: "dup-3", X: 5, Y: 5},
	}
	result, err := Cluster(obs, testConfig)
	assert.NoError(t, err)

	assert.Equal(t, 1, len(result.Clusters))
	cluster := result.Clusters[0]
	assert.Equal(t, "dup-1", cluster.ID)
	assert.Equal(t, []string{"dup-1", "dup-2", "dup-3"}, idsOf(cluster.Core))
	assert.Equal(t, []string{}, idsOf(cluster.Border))
	for _, c := range cluster.Core {
		assert.Equal(t, RoleCore, c.Role)
		assert.Equal(t, "dup-1", c.ClusterID)
	}
}

func TestDuplicatesDoNotMergeDistinctObservations(t *testing.T) {
	// A single coordinate observed twice must not be collapsed: with only two
	// observations, MinPts=3 is not reached and both are noise.
	obs := []Observation{
		{ID: "dup-1", X: 0, Y: 0},
		{ID: "dup-2", X: 0, Y: 0},
	}
	result, err := Cluster(obs, testConfig)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(result.Clusters))
	assert.Equal(t, []string{"dup-1", "dup-2"}, idsOf(result.Noise))
	assert.Equal(t, 2, len(result.Observations))
}

func TestDuplicateNeighbourCounts(t *testing.T) {
	// A lone observation with two duplicates within eps becomes core thanks to
	// the duplicate observations. Without them it would be noise.
	obs := []Observation{
		{ID: "p", X: 0, Y: 0},
		{ID: "q1", X: 1, Y: 0},
		{ID: "q2", X: 1, Y: 0},
	}
	result, err := Cluster(obs, testConfig)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(result.Clusters))
	assert.Equal(t, []string{"p", "q1", "q2"}, idsOf(result.Clusters[0].Core))
}

func TestNeighboursAcrossCellBoundaries(t *testing.T) {
	// Observations at exact multiples of eps sit on cell boundaries. A
	// neighbour exactly eps away must still be found regardless of which side
	// of the boundary the division rounds to.
	cfg := Config{EpsMeters: 10, MinPts: 2, CRS: CRSProjectedMeters}
	for _, coords := range [][2][2]float64{
		{{0, 0}, {10, 0}},
		{{0, 0}, {-10, 0}},
		{{10, 10}, {10, 20}},
		{{10, 10}, {0, 10}},
		{{-10, -10}, {-20, -10}},
	} {
		result, err := Cluster([]Observation{
			{ID: "a", X: coords[0][0], Y: coords[0][1]},
			{ID: "b", X: coords[1][0], Y: coords[1][1]},
		}, cfg)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(result.Clusters), "coords %v", coords)
	}
}

func TestMatchesBruteForce(t *testing.T) {
	// Compare the grid index against an O(n^2) reference implementation on
	// pseudo-random input, including duplicate coordinates.
	const n = 200
	obs := make([]Observation, n)
	// A small deterministic generator, so the test is reproducible without
	// importing math/rand's global state.
	state := uint64(1)
	next := func() float64 {
		state = state*6364136223846793005 + 1442695040888963407
		return float64(state>>11) / float64(1<<53)
	}
	for i := range obs {
		// Coordinates in [-5, 5] with eps 1: many points share cells and some
		// are exact duplicates of each other.
		x := math.Round((next()*10-5)*10) / 10
		y := math.Round((next()*10-5)*10) / 10
		obs[i] = Observation{ID: fmt.Sprintf("o%03d", i), X: x, Y: y}
	}
	cfg := Config{EpsMeters: 1, MinPts: 4, CRS: CRSProjectedMeters}
	result, err := Cluster(obs, cfg)
	assert.NoError(t, err)

	// Reference: compute neighbours by brute force and in the same
	// ID-sorted order, then compare roles and cluster membership.
	sorted := make([]Observation, n)
	copy(sorted, obs)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	core := make([]bool, n)
	counts := make([]int, n)
	for i := range sorted {
		for j := range sorted {
			dx := sorted[i].X - sorted[j].X
			dy := sorted[i].Y - sorted[j].Y
			if dx*dx+dy*dy <= cfg.EpsMeters*cfg.EpsMeters {
				counts[i]++
			}
		}
		core[i] = counts[i] >= cfg.MinPts
	}
	wantRoles := make(map[string]Role, n)
	for i := range sorted {
		switch {
		case core[i]:
			wantRoles[sorted[i].ID] = RoleCore
		default:
			border := false
			for j := range sorted {
				if !core[j] {
					continue
				}
				dx := sorted[i].X - sorted[j].X
				dy := sorted[i].Y - sorted[j].Y
				if dx*dx+dy*dy <= cfg.EpsMeters*cfg.EpsMeters {
					border = true
					break
				}
			}
			if border {
				wantRoles[sorted[i].ID] = RoleBorder
			} else {
				wantRoles[sorted[i].ID] = RoleNoise
			}
		}
	}
	gotRoles := roleByID(result)
	for id, want := range wantRoles {
		assert.Equal(t, want, gotRoles[id], id)
	}
	if len(wantRoles) != len(gotRoles) {
		t.Fatalf("role count mismatch")
	}
}

func TestStringMethods(t *testing.T) {
	assert.Equal(t, "unknown", CRSUnknown.String())
	assert.Equal(t, "projected-metres", CRSProjectedMeters.String())
	assert.Equal(t, "geographic-degrees", CRSGeographicDegrees.String())
	assert.Equal(t, "CRS(99)", CRS(99).String())

	assert.Equal(t, "noise", RoleNoise.String())
	assert.Equal(t, "border", RoleBorder.String())
	assert.Equal(t, "core", RoleCore.String())
	assert.Equal(t, "Role(99)", Role(99).String())
}

func TestClusterIDIsSmallestCoreID(t *testing.T) {
	// The core observations form a single chain, so all four are core. The
	// cluster ID must be the lexicographically smallest core ID ("m1"), not
	// the ID of whichever core was visited first.
	obs := []Observation{
		{ID: "m2", X: 0, Y: 0},
		{ID: "m1", X: 5, Y: 0},
		{ID: "m4", X: 10, Y: 0},
		{ID: "m3", X: 15, Y: 0},
		{ID: "zz", X: 25, Y: 0},
	}
	result, err := Cluster(obs, testConfig)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(result.Clusters))
	assert.Equal(t, "m1", result.Clusters[0].ID)
	assert.Equal(t, []string{"m1", "m2", "m3", "m4"}, idsOf(result.Clusters[0].Core))
	assert.Equal(t, []string{"zz"}, idsOf(result.Clusters[0].Border))
}

func TestBorderTieBreakBySmallestCoreID(t *testing.T) {
	// Two clusters of four core observations, each cluster a vertical line.
	// "aaa" (smallest ID) is at x=+10 and "zzz" at x=-10, so the spatial index
	// visits zzz's cell before aaa's: a naive "first nearest wins" would pick
	// zzz. The border "mid" is exactly 10 m from core "aaa" and exactly 10 m
	// from core "zzz" and is within eps of no other core, so it is a border.
	// The distance tie must be broken by the lexicographically smallest core
	// ID, which is "aaa", not by iteration order.
	cfg := Config{EpsMeters: 10, MinPts: 4, CRS: CRSProjectedMeters}
	obs := []Observation{
		{ID: "aaa", X: 10, Y: 0},
		{ID: "aaa2", X: 10, Y: 3},
		{ID: "aaa3", X: 10, Y: 6},
		{ID: "aaa4", X: 10, Y: 9},
		{ID: "zzz", X: -10, Y: 0},
		{ID: "zzz2", X: -10, Y: 3},
		{ID: "zzz3", X: -10, Y: 6},
		{ID: "zzz4", X: -10, Y: 9},
		{ID: "mid", X: 0, Y: 0},
	}
	result, err := Cluster(obs, cfg)
	assert.NoError(t, err)
	assert.Equal(t, []string{"aaa", "zzz"}, clusterIDsOf(t, result))

	roles := roleByID(result)
	assert.Equal(t, RoleBorder, roles["mid"])
	assert.Equal(t, "aaa", clusterIDFor(result, "mid"))
	assert.Equal(t, []string{"mid"}, idsOf(result.Clusters[0].Border))
	assert.Equal(t, []string{}, idsOf(result.Clusters[1].Border))
}

func TestBorderPrefersNearestCore(t *testing.T) {
	// The border is nearer to the "zzz" cluster even though "aaa" sorts first,
	// so nearest wins before the ID tie-break is consulted.
	cfg := Config{EpsMeters: 10, MinPts: 4, CRS: CRSProjectedMeters}
	obs := []Observation{
		{ID: "aaa", X: -30, Y: 0},
		{ID: "aaa2", X: -30, Y: 3},
		{ID: "aaa3", X: -30, Y: 6},
		{ID: "aaa4", X: -30, Y: 9},
		{ID: "zzz", X: 10, Y: 0},
		{ID: "zzz2", X: 10, Y: 3},
		{ID: "zzz3", X: 10, Y: 6},
		{ID: "zzz4", X: 10, Y: 9},
		{ID: "near-zzz", X: 1, Y: 0},
	}
	result, err := Cluster(obs, cfg)
	assert.NoError(t, err)
	assert.Equal(t, RoleBorder, roleByID(result)["near-zzz"])
	assert.Equal(t, "zzz", clusterIDFor(result, "near-zzz"))
}

func clusterIDFor(result *Result, id string) string {
	for _, c := range result.Observations {
		if c.ID == id {
			return c.ClusterID
		}
	}
	return "<missing>"
}

func TestBorderDoesNotBridgeClusters(t *testing.T) {
	// A chain of observations leads from one cluster towards another. The
	// observations adjacent to a cluster are borders, but the middle of the
	// chain is noise. Clusters are connected through core observations only,
	// so the two clusters must remain separate.
	cfg := Config{EpsMeters: 10, MinPts: 3, CRS: CRSProjectedMeters}
	obs := []Observation{
		// Cluster "a" is a compact vertical triple, all core.
		{ID: "a1", X: 0, Y: 0},
		{ID: "a2", X: 0, Y: 3},
		{ID: "a3", X: 0, Y: 6},
		// Chain: p1 is 9.9 m from a1 only; p2 is out of reach of everything;
		// p3 is 9.9 m from b1 only.
		{ID: "p1", X: 9.9, Y: 0},
		{ID: "p2", X: 20, Y: 0},
		{ID: "p3", X: 30.1, Y: 0},
		// Cluster "b" is a compact vertical triple, all core.
		{ID: "b1", X: 40, Y: 0},
		{ID: "b2", X: 40, Y: 3},
		{ID: "b3", X: 40, Y: 6},
	}
	result, err := Cluster(obs, cfg)
	assert.NoError(t, err)
	assert.Equal(t, []string{"a1", "b1"}, clusterIDsOf(t, result))

	roles := roleByID(result)
	assert.Equal(t, RoleBorder, roles["p1"])
	assert.Equal(t, RoleNoise, roles["p2"])
	assert.Equal(t, RoleBorder, roles["p3"])
	assert.Equal(t, "a1", clusterIDFor(result, "p1"))
	assert.Equal(t, "b1", clusterIDFor(result, "p3"))
	assert.Equal(t, []string{"p2"}, idsOf(result.Noise))
	assert.Equal(t, []string{"p1"}, idsOf(result.Clusters[0].Border))
	assert.Equal(t, []string{"p3"}, idsOf(result.Clusters[1].Border))
}

func TestPermutationInvariance(t *testing.T) {
	obs := []Observation{
		{ID: "a1", X: 0, Y: 0},
		{ID: "a2", X: 1, Y: 0},
		{ID: "a3", X: 0, Y: 1},
		{ID: "b1", X: 100, Y: 0},
		{ID: "b2", X: 101, Y: 0},
		{ID: "b3", X: 100, Y: 1},
		{ID: "shared", X: 50, Y: 0},
		{ID: "near-a", X: 10, Y: 0},
		{ID: "near-b", X: 90, Y: 0},
		{ID: "noise", X: 500, Y: 500},
		{ID: "dup-1", X: 99, Y: 0},
		{ID: "dup-2", X: 99, Y: 0},
	}
	want, err := Cluster(obs, testConfig)
	assert.NoError(t, err)

	// Try a number of deterministic permutations.
	perms := [][]int{
		reverseIndexes(len(obs)),
		rotatedIndexes(len(obs), 3),
		oddsThenEvens(len(obs)),
	}
	for _, perm := range perms {
		permuted := make([]Observation, len(obs))
		for i, j := range perm {
			permuted[i] = obs[j]
		}
		got, err := Cluster(permuted, testConfig)
		assert.NoError(t, err)
		assert.Equal(t, want, got)
	}

	// A full shuffle of a copy must also agree.
	shuffled := append([]Observation(nil), obs...)
	sort.Slice(shuffled, func(i, j int) bool { return shuffled[i].ID > shuffled[j].ID })
	got, err := Cluster(shuffled, testConfig)
	assert.NoError(t, err)
	assert.Equal(t, want, got)
}

func reverseIndexes(n int) []int {
	perm := make([]int, n)
	for i := range perm {
		perm[i] = n - 1 - i
	}
	return perm
}

func rotatedIndexes(n, by int) []int {
	perm := make([]int, n)
	for i := range perm {
		perm[i] = (i + by) % n
	}
	return perm
}

func oddsThenEvens(n int) []int {
	var perm []int
	for i := 1; i < n; i += 2 {
		perm = append(perm, i)
	}
	for i := 0; i < n; i += 2 {
		perm = append(perm, i)
	}
	return perm
}

func TestResultIsStableAcrossDuplicatePermutations(t *testing.T) {
	// Duplicate coordinates exercise map iteration more than distinct ones
	// because several observations share a grid cell.
	obs := []Observation{
		{ID: "x1", X: 0, Y: 0},
		{ID: "x2", X: 0, Y: 0},
		{ID: "x3", X: 0, Y: 0},
		{ID: "y1", X: 1, Y: 0},
		{ID: "y2", X: 1, Y: 0},
		{ID: "y3", X: 1, Y: 0},
	}
	want, err := Cluster(obs, testConfig)
	assert.NoError(t, err)
	for _, perm := range [][]int{reverseIndexes(len(obs)), rotatedIndexes(len(obs), 2)} {
		permuted := make([]Observation, len(obs))
		for i, j := range perm {
			permuted[i] = obs[j]
		}
		got, err := Cluster(permuted, testConfig)
		assert.NoError(t, err)
		assert.Equal(t, want, got)
	}
}

func TestNoCrossClusterBorderAssignment(t *testing.T) {
	// A chain of duplicates at one coordinate counts as one dense region; make
	// sure each border belongs to exactly one cluster and every cluster's Core
	// contains only RoleCore observations.
	obs := []Observation{
		{ID: "a1", X: 0, Y: 0},
		{ID: "a2", X: 0, Y: 0},
		{ID: "a3", X: 0, Y: 0},
		{ID: "b1", X: 20, Y: 0},
		{ID: "b2", X: 20, Y: 0},
		{ID: "b3", X: 20, Y: 0},
		{ID: "b4", X: 20, Y: 0},
	}
	result, err := Cluster(obs, testConfig)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(result.Clusters))
	for _, c := range result.Clusters {
		for _, core := range c.Core {
			assert.Equal(t, RoleCore, core.Role)
			assert.Equal(t, c.ID, core.ClusterID)
		}
		for _, border := range c.Border {
			assert.Equal(t, RoleBorder, border.Role)
			assert.Equal(t, c.ID, border.ClusterID)
		}
	}
}

func TestObservationsContainsEveryInput(t *testing.T) {
	obs := []Observation{
		{ID: "b", X: 100, Y: 0},
		{ID: "a", X: 0, Y: 0},
		{ID: "c", X: 50, Y: 50},
	}
	result, err := Cluster(obs, testConfig)
	assert.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, idsOf(result.Observations))
	assert.Equal(t, []string{"a", "b", "c"}, idsOf(result.Noise))
}

func TestValidation(t *testing.T) {
	valid := []Observation{{ID: "a", X: 0, Y: 0}}

	for _, tc := range []struct {
		name string
		obs  []Observation
		cfg  Config
		want error
	}{
		{
			name: "unknown CRS",
			obs:  valid,
			cfg:  Config{EpsMeters: 10, MinPts: 3},
			want: ErrNonMetricCRS,
		},
		{
			name: "geographic degrees CRS",
			obs: []Observation{
				{ID: "a", X: -0.1278, Y: 51.5074}, // London in degrees
				{ID: "b", X: -0.1277, Y: 51.5073},
			},
			cfg:  Config{EpsMeters: 0.001, MinPts: 2, CRS: CRSGeographicDegrees},
			want: ErrGeographicCRS,
		},
		{
			name: "zero eps",
			obs:  valid,
			cfg:  Config{EpsMeters: 0, MinPts: 3, CRS: CRSProjectedMeters},
			want: ErrInvalidEpsMeters,
		},
		{
			name: "negative eps",
			obs:  valid,
			cfg:  Config{EpsMeters: -1, MinPts: 3, CRS: CRSProjectedMeters},
			want: ErrInvalidEpsMeters,
		},
		{
			name: "NaN eps",
			obs:  valid,
			cfg:  Config{EpsMeters: math.NaN(), MinPts: 3, CRS: CRSProjectedMeters},
			want: ErrInvalidEpsMeters,
		},
		{
			name: "infinite eps",
			obs:  valid,
			cfg:  Config{EpsMeters: math.Inf(1), MinPts: 3, CRS: CRSProjectedMeters},
			want: ErrInvalidEpsMeters,
		},
		{
			name: "zero minpts",
			obs:  valid,
			cfg:  Config{EpsMeters: 10, MinPts: 0, CRS: CRSProjectedMeters},
			want: ErrInvalidMinPts,
		},
		{
			name: "empty id",
			obs:  []Observation{{ID: "", X: 0, Y: 0}},
			cfg:  testConfig,
			want: ErrEmptyID,
		},
		{
			name: "duplicate id",
			obs:  []Observation{{ID: "a", X: 0, Y: 0}, {ID: "a", X: 1, Y: 1}},
			cfg:  testConfig,
			want: ErrDuplicateID,
		},
		{
			name: "NaN coordinate",
			obs:  []Observation{{ID: "a", X: math.NaN(), Y: 0}},
			cfg:  testConfig,
			want: ErrNonFiniteCoordinate,
		},
		{
			name: "infinite coordinate",
			obs:  []Observation{{ID: "a", X: 0, Y: math.Inf(-1)}},
			cfg:  testConfig,
			want: ErrNonFiniteCoordinate,
		},
		{
			name: "coordinate too large for cell grid",
			obs:  []Observation{{ID: "a", X: 1e300, Y: 0}},
			cfg:  testConfig,
			want: ErrCoordinateOutOfRange,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Cluster(tc.obs, tc.cfg)
			assert.IsError(t, err, tc.want)
			assert.True(t, errors.Is(err, tc.want))
		})
	}
}

func TestEmptyInput(t *testing.T) {
	result, err := Cluster(nil, testConfig)
	assert.NoError(t, err)
	assert.Equal(t, &Result{
		Clusters:     []ResultCluster{},
		Noise:        []Classified{},
		Observations: []Classified{},
	}, result)
}

func TestSingleObservationIsNoise(t *testing.T) {
	result, err := Cluster([]Observation{{ID: "only", X: 0, Y: 0}}, testConfig)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(result.Clusters))
	assert.Equal(t, []string{"only"}, idsOf(result.Noise))
}

func TestMinPtsOneMakesEverythingCore(t *testing.T) {
	cfg := Config{EpsMeters: 10, MinPts: 1, CRS: CRSProjectedMeters}
	obs := []Observation{
		{ID: "a", X: 0, Y: 0},
		{ID: "b", X: 100, Y: 0},
	}
	result, err := Cluster(obs, cfg)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(result.Clusters))
	for _, c := range result.Clusters {
		assert.Equal(t, 1, len(c.Core))
		assert.Equal(t, 0, len(c.Border))
	}
}

func TestEpsBoundaryIsInclusive(t *testing.T) {
	// A neighbour exactly EpsMeters away counts as inside the neighbourhood.
	cfg := Config{EpsMeters: 1, MinPts: 2, CRS: CRSProjectedMeters}
	obs := []Observation{
		{ID: "a", X: 0, Y: 0},
		{ID: "b", X: 1, Y: 0},
	}
	result, err := Cluster(obs, cfg)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(result.Clusters))
	assert.Equal(t, []string{"a", "b"}, idsOf(result.Clusters[0].Core))
}

func TestEpsIsEuclideanInMeters(t *testing.T) {
	// Neighbourhood membership uses Euclidean distance over the planar
	// coordinates. The "diagonal" observation at (9, 9) is 12.73 m from the
	// origin: within 10 m under the Chebyshev (max-norm) metric but outside
	// the 10 m Euclidean neighbourhood, so it must be noise.
	cfg := Config{EpsMeters: 10, MinPts: 3, CRS: CRSProjectedMeters}
	obs := []Observation{
		{ID: "origin", X: 0, Y: 0},
		{ID: "origin-2", X: 1, Y: 0},
		{ID: "origin-3", X: 0, Y: 1},
		{ID: "diagonal", X: 9, Y: 9},
	}
	result, err := Cluster(obs, cfg)
	assert.NoError(t, err)
	assert.Equal(t, RoleCore, roleByID(result)["origin"])
	assert.Equal(t, RoleNoise, roleByID(result)["diagonal"])
	assert.Equal(t, []string{"diagonal"}, idsOf(result.Noise))

	// A point exactly EpsMeters away along both axes is at 3-4-5 distance and
	// is a neighbour.
	result, err = Cluster([]Observation{
		{ID: "origin", X: 0, Y: 0},
		{ID: "origin-2", X: -1, Y: 0},
		{ID: "origin-3", X: 0, Y: -1},
		{ID: "three-four", X: 3, Y: 4},
	}, cfg)
	assert.NoError(t, err)
	assert.Equal(t, "origin", clusterIDFor(result, "three-four"))
}

func TestClusterMultiPoint(t *testing.T) {
	mp := geom.NewMultiPointFlat(geom.XY, []float64{
		0, 0,
		1, 0,
		0, 1,
		100, 0,
		101, 0,
		100, 1,
	})
	ids := []string{"a1", "a2", "a3", "b1", "b2", "b3"}
	result, err := ClusterMultiPoint(mp, ids, testConfig)
	assert.NoError(t, err)
	assert.Equal(t, []string{"a1", "b1"}, clusterIDsOf(t, result))

	// Reordering both the points and the IDs must give the same answer.
	perm := rotatedIndexes(len(ids), 2)
	flat := mp.FlatCoords()
	permutedFlat := make([]float64, 0, len(flat))
	permutedIDs := make([]string, len(ids))
	for i, j := range perm {
		permutedFlat = append(permutedFlat, flat[2*j], flat[2*j+1])
		permutedIDs[i] = ids[j]
	}
	permutedMP := geom.NewMultiPointFlat(geom.XY, permutedFlat)
	permutedResult, err := ClusterMultiPoint(permutedMP, permutedIDs, testConfig)
	assert.NoError(t, err)
	assert.Equal(t, result, permutedResult)
}

func TestClusterMultiPointErrors(t *testing.T) {
	mp := geom.NewMultiPointFlat(geom.XY, []float64{0, 0, 1, 0})
	_, err := ClusterMultiPoint(mp, []string{"only-one"}, testConfig)
	assert.IsError(t, err, ErrIDCountMismatch)

	emptyMP := geom.NewMultiPointFlat(
		geom.XY,
		[]float64{0, 0},
		geom.NewMultiPointFlatOptionWithEnds([]int{2, 2}),
	)
	_, err = ClusterMultiPoint(emptyMP, []string{"a", "b"}, testConfig)
	assert.IsError(t, err, ErrEmptyPoint)
}

func TestClusterMultiPointUsesXYOnly(t *testing.T) {
	// XYM and XYZ layouts must still cluster on the first two ordinates.
	for _, layout := range []geom.Layout{geom.XYZ, geom.XYM, geom.XYZM} {
		stride := layout.Stride()
		flat := make([]float64, 0, 3*stride)
		for _, xy := range [][2]float64{{0, 0}, {1, 0}, {0, 1}} {
			coord := make([]float64, stride)
			coord[0] = xy[0]
			coord[1] = xy[1]
			for k := 2; k < stride; k++ {
				coord[k] = 999 // irrelevant extra ordinate
			}
			flat = append(flat, coord...)
		}
		mp := geom.NewMultiPointFlat(layout, flat)
		result, err := ClusterMultiPoint(mp, []string{"a", "b", "c"}, testConfig)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(result.Clusters))
	}
}

func TestResultSlicesAreSorted(t *testing.T) {
	obs := []Observation{
		{ID: "c1", X: 0, Y: 0},
		{ID: "c2", X: 1, Y: 0},
		{ID: "c3", X: 0, Y: 1},
		{ID: "d1", X: 100, Y: 0},
		{ID: "d2", X: 101, Y: 0},
		{ID: "d3", X: 100, Y: 1},
		{ID: "z", X: 50, Y: 50},
	}
	result, err := Cluster(obs, testConfig)
	assert.NoError(t, err)

	assert.True(t, sort.SliceIsSorted(result.Observations, func(i, j int) bool {
		return result.Observations[i].ID < result.Observations[j].ID
	}))
	assert.True(t, sort.SliceIsSorted(result.Noise, func(i, j int) bool {
		return result.Noise[i].ID < result.Noise[j].ID
	}))
	assert.True(t, sort.SliceIsSorted(result.Clusters, func(i, j int) bool {
		return result.Clusters[i].ID < result.Clusters[j].ID
	}))
	for _, c := range result.Clusters {
		assert.True(t, sort.SliceIsSorted(c.Core, func(i, j int) bool {
			return c.Core[i].ID < c.Core[j].ID
		}))
	}
}

func TestNoInputMutation(t *testing.T) {
	obs := []Observation{
		{ID: "b", X: 1, Y: 0},
		{ID: "a", X: 0, Y: 0},
	}
	before := append([]Observation(nil), obs...)
	_, err := Cluster(obs, testConfig)
	assert.NoError(t, err)
	assert.Equal(t, before, obs)
}
