package clip_test

import (
	"fmt"

	"github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/xy/clip"
)

func ExampleLineString() {
	ls := geom.NewLineStringFlat(geom.XYM, []float64{
		-5, 5, 1,
		5, 5, 2,
		15, 5, 3,
	})
	pieces := clip.LineString(ls, clip.NewRect(0, 0, 10, 10))
	for _, piece := range pieces {
		fmt.Println(piece.LineString.FlatCoords())
		fmt.Println(piece.Source)
	}
	// Output:
	// [0 5 1.5 5 5 2 10 5 2.5]
	// [{0 0} {0 1}]
}
