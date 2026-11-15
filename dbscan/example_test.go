package dbscan_test

import (
	"fmt"

	"github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/dbscan"
)

func ExampleCluster() {
	// Coordinates are projected planar metres, e.g. UTM eastings and
	// northings. They are not degrees of longitude and latitude.
	observations := []dbscan.Observation{
		{ID: "a1", X: 0, Y: 0},
		{ID: "a2", X: 1, Y: 0},
		{ID: "a3", X: 0, Y: 1},
		{ID: "b1", X: 100, Y: 0},
		{ID: "b2", X: 101, Y: 0},
		{ID: "b3", X: 100, Y: 1},
		{ID: "lone", X: 50, Y: 50},
	}
	result, err := dbscan.Cluster(observations, dbscan.Config{
		EpsMeters: 10,
		MinPts:    3,
		CRS:       dbscan.CRSProjectedMeters,
	})
	if err != nil {
		panic(err)
	}
	for _, cluster := range result.Clusters {
		fmt.Printf("cluster %s\n", cluster.ID)
		for _, core := range cluster.Core {
			fmt.Printf("  core   %s\n", core.ID)
		}
		for _, border := range cluster.Border {
			fmt.Printf("  border %s\n", border.ID)
		}
	}
	for _, noise := range result.Noise {
		fmt.Printf("noise  %s\n", noise.ID)
	}
	// Output:
	// cluster a1
	//   core   a1
	//   core   a2
	//   core   a3
	// cluster b1
	//   core   b1
	//   core   b2
	//   core   b3
	// noise  lone
}

func ExampleCluster_duplicates() {
	// Three observations at the same coordinate are three observations, not
	// one. Together they reach MinPts and form a cluster.
	observations := []dbscan.Observation{
		{ID: "obs-1", X: 7, Y: 7},
		{ID: "obs-2", X: 7, Y: 7},
		{ID: "obs-3", X: 7, Y: 7},
	}
	result, err := dbscan.Cluster(observations, dbscan.Config{
		EpsMeters: 10,
		MinPts:    3,
		CRS:       dbscan.CRSProjectedMeters,
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(len(result.Clusters), len(result.Clusters[0].Core))
	// Output: 1 3
}

func ExampleClusterMultiPoint() {
	// Points are planar metres; the IDs are business IDs paired by index.
	points := geom.NewMultiPointFlat(geom.XY, []float64{
		0, 0,
		1, 0,
		0, 1,
		500, 500,
	})
	result, err := dbscan.ClusterMultiPoint(points, []string{"p1", "p2", "p3", "p4"}, dbscan.Config{
		EpsMeters: 10,
		MinPts:    3,
		CRS:       dbscan.CRSProjectedMeters,
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(result.Clusters[0].ID, result.Noise[0].ID)
	// Output: p1 p4
}
