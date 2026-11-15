package dbscan

import (
	"fmt"
	"math"
)

// maxCellOrdinate bounds the cell ordinates so that they can be represented as
// int64. Coordinates whose cell ordinate would exceed this are rejected rather
// than silently truncated.
const maxCellOrdinate = float64(1 << 62)

// gridCell identifies a square cell of the uniform grid.
type gridCell struct {
	x int64
	y int64
}

// gridIndex is a uniform grid spatial index. Each cell is EpsMeters wide and
// high. A query scans the cells that can contain an observation within
// EpsMeters of the query point.
//
// Duplicate coordinates are appended as separate entries: the index stores one
// entry per observation, never one entry per distinct coordinate.
type gridIndex struct {
	eps   float64
	eps2  float64
	cells map[gridCell][]int
	obs   []Observation
}

// newGridIndex indexes observations with cells of side eps.
func newGridIndex(obs []Observation, eps float64) (*gridIndex, error) {
	idx := &gridIndex{
		eps:   eps,
		eps2:  eps * eps,
		cells: make(map[gridCell][]int, len(obs)),
		obs:   obs,
	}
	for i, o := range obs {
		x := o.X / eps
		y := o.Y / eps
		if math.Abs(x) >= maxCellOrdinate || math.Abs(y) >= maxCellOrdinate {
			return nil, fmt.Errorf(
				"%w: observation %q at (%v, %v) with EpsMeters %v",
				ErrCoordinateOutOfRange, o.ID, o.X, o.Y, eps,
			)
		}
		c := gridCell{x: cellOrdinate(o.X, eps), y: cellOrdinate(o.Y, eps)}
		idx.cells[c] = append(idx.cells[c], i)
	}
	return idx, nil
}

// cellOrdinate returns the grid cell ordinate containing v.
func cellOrdinate(v, eps float64) int64 {
	return int64(math.Floor(v / eps))
}

// forEachNeighbor calls fn for every observation within EpsMeters of
// observation i, including i itself if it is within EpsMeters of itself.
//
// The cells to scan are derived from the coordinate interval [x-eps, x+eps]
// rather than from a fixed three-by-three block centred on i's own cell. The
// interval is padded by one cell on each side to absorb floating-point error
// in the division and subtraction at cell boundaries, so a neighbour exactly
// eps away is never missed because of rounding.
//
// The call order is unspecified. Callers that must be independent of input
// order must not depend on it.
func (idx *gridIndex) forEachNeighbor(i int, fn func(j int)) {
	o := idx.obs[i]
	xlo := cellOrdinate(o.X-idx.eps, idx.eps) - 1
	xhi := cellOrdinate(o.X+idx.eps, idx.eps) + 1
	ylo := cellOrdinate(o.Y-idx.eps, idx.eps) - 1
	yhi := cellOrdinate(o.Y+idx.eps, idx.eps) + 1
	for x := xlo; x <= xhi; x++ {
		for y := ylo; y <= yhi; y++ {
			for _, j := range idx.cells[gridCell{x: x, y: y}] {
				other := idx.obs[j]
				ddx := other.X - o.X
				ddy := other.Y - o.Y
				if ddx*ddx+ddy*ddy <= idx.eps2 {
					fn(j)
				}
			}
		}
	}
}
