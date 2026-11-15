package pointpolygonjoin

import "sort"

// defaultNodeCapacity is the maximum number of entries stored in each R-tree
// node.
const defaultNodeCapacity = 25

// box is an axis-aligned bounding box in the XY plane.
type box struct {
	minX float64
	minY float64
	maxX float64
	maxY float64
}

// containsPoint reports whether the point (x, y) lies within or on the border
// of b.
func (b box) containsPoint(x, y float64) bool {
	return b.minX <= x && x <= b.maxX && b.minY <= y && y <= b.maxY
}

// union returns the smallest box containing both b and other.
func (b box) union(other box) box {
	return box{
		minX: min(b.minX, other.minX),
		minY: min(b.minY, other.minY),
		maxX: max(b.maxX, other.maxX),
		maxY: max(b.maxY, other.maxY),
	}
}

// centerX returns the center of b on the X axis.
func (b box) centerX() float64 {
	return (b.minX + b.maxX) / 2
}

// centerY returns the center of b on the Y axis.
func (b box) centerY() float64 {
	return (b.minY + b.maxY) / 2
}

// rtreeEntry pairs a bounding box with a value.
type rtreeEntry[T any] struct {
	box box
	v   T
}

// rtreeNode is a node of an rTree. Leaf nodes store entries, internal nodes
// store children.
type rtreeNode[T any] struct {
	box      box
	children []*rtreeNode[T]
	entries  []rtreeEntry[T]
}

// isLeaf reports whether n is a leaf node.
func (n *rtreeNode[T]) isLeaf() bool {
	return n.entries != nil
}

// rTree is a static, immutable R-tree built with sort-based bulk loading:
// entries and nodes are sorted by the center of their bounding box before
// being packed into capacity-sized nodes. It only supports point queries.
type rTree[T any] struct {
	root     *rtreeNode[T]
	capacity int
}

// newRTree returns an R-tree bulk loaded from entries. Nodes hold at most
// capacity entries or children. An empty tree has a nil root.
func newRTree[T any](entries []rtreeEntry[T], capacity int) *rTree[T] {
	if capacity < 2 {
		capacity = defaultNodeCapacity
	}
	tree := &rTree[T]{capacity: capacity}
	if len(entries) == 0 {
		return tree
	}

	// Build the leaf level, sorting entries by the center of their box so that
	// spatially close entries end up in the same leaf.
	sort.Slice(entries, func(i, j int) bool {
		ei, ej := entries[i].box, entries[j].box
		if ei.centerX() != ej.centerX() {
			return ei.centerX() < ej.centerX()
		}
		return ei.centerY() < ej.centerY()
	})
	level := tree.packLeaves(entries)

	// Repeatedly pack the nodes of one level into the nodes of the next level
	// until a single root node remains.
	for len(level) > 1 {
		sort.Slice(level, func(i, j int) bool {
			ni, nj := level[i].box, level[j].box
			if ni.centerX() != nj.centerX() {
				return ni.centerX() < nj.centerX()
			}
			return ni.centerY() < nj.centerY()
		})
		level = tree.packNodes(level)
	}
	tree.root = level[0]
	return tree
}

// packLeaves groups entries into leaf nodes of at most t.capacity entries.
func (t *rTree[T]) packLeaves(entries []rtreeEntry[T]) []*rtreeNode[T] {
	nodes := make([]*rtreeNode[T], 0, (len(entries)+t.capacity-1)/t.capacity)
	for start := 0; start < len(entries); start += t.capacity {
		end := min(start+t.capacity, len(entries))
		nodeEntries := entries[start:end]
		node := &rtreeNode[T]{
			box:     nodeEntries[0].box,
			entries: nodeEntries,
		}
		for _, entry := range nodeEntries[1:] {
			node.box = node.box.union(entry.box)
		}
		nodes = append(nodes, node)
	}
	return nodes
}

// packNodes groups nodes into parent nodes of at most t.capacity children.
func (t *rTree[T]) packNodes(nodes []*rtreeNode[T]) []*rtreeNode[T] {
	parents := make([]*rtreeNode[T], 0, (len(nodes)+t.capacity-1)/t.capacity)
	for start := 0; start < len(nodes); start += t.capacity {
		end := min(start+t.capacity, len(nodes))
		children := nodes[start:end]
		parent := &rtreeNode[T]{
			box:      children[0].box,
			children: children,
		}
		for _, child := range children[1:] {
			parent.box = parent.box.union(child.box)
		}
		parents = append(parents, parent)
	}
	return parents
}

// searchPoint calls yield with every value whose box contains the point
// (x, y). Traversal stops early if yield returns false. stack is used as
// scratch storage for the node traversal and may be reused across calls by
// passing the returned slice in again. The R-tree only filters candidates:
// callers must still perform an exact containment test.
func (t *rTree[T]) searchPoint(
	stack []*rtreeNode[T], x, y float64, yield func(T) bool,
) []*rtreeNode[T] {
	if t.root == nil {
		return stack[:0]
	}
	stack = append(stack[:0], t.root)
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if !node.box.containsPoint(x, y) {
			continue
		}
		if node.isLeaf() {
			for i := range node.entries {
				entry := &node.entries[i]
				if entry.box.containsPoint(x, y) && !yield(entry.v) {
					return stack[:0]
				}
			}
			continue
		}
		stack = append(stack, node.children...)
	}
	return stack
}
