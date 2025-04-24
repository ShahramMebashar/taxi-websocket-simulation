package quadtree

import "sync"

// Bounds represents a rectangular area in 2D space.
type Bounds struct {
	MinX, MinY float64
	MaxX, MaxY float64
}

func (b Bounds) contains(x, y float64) bool {
	return x >= b.MinX && x <= b.MaxX && y >= b.MinY && y <= b.MaxY
}

// Point represents a location in 2D space.
type Point struct {
	X, Y float64
}

// Quadtree is a spatial data structure for efficient point storage and retrieval.
type Quadtree struct {
	capacity             int
	nodes                []Point
	bounds               Bounds
	divided              bool
	northWest, northEast *Quadtree
	southWest, southEast *Quadtree
}

// New creates a new Quadtree instance with the given bounds and capacity.
func New(bounds Bounds, capcity int) *Quadtree {
	return &Quadtree{
		bounds:   bounds,
		capacity: capcity,
		nodes:    make([]Point, 0, capcity),
		divided:  false,
	}
}

func (qt *Quadtree) Insert(node Point) bool {
	if !qt.InsideBounds(node.X, node.Y) {
		return false
	}

	// If we have capacity and aren't divided, add the point
	if len(qt.nodes) < qt.capacity && !qt.divided {
		qt.nodes = append(qt.nodes, node)
		return true
	}

	if !qt.divided {
		qt.subDivide()
	}

	// After subdivision, insert into appropriate child
	return qt.insertIntoChild(node)
}

func (qt *Quadtree) insertIntoChild(node Point) bool {
	midX := (qt.bounds.MinX + qt.bounds.MaxX) / 2
	midY := (qt.bounds.MinY + qt.bounds.MaxY) / 2

	if node.X <= midX { // West side
		if node.Y <= midY { // South
			return qt.southWest.Insert(node)
		}
		return qt.northWest.Insert(node) // North
	} else { // East side
		if node.Y <= midY { // South
			return qt.southEast.Insert(node)
		}
		return qt.northEast.Insert(node) // North
	}
}

func (qt *Quadtree) subDivide() {
	midX := (qt.bounds.MinX + qt.bounds.MaxX) / 2
	midY := (qt.bounds.MinY + qt.bounds.MaxY) / 2

	qt.northWest = New(Bounds{
		MinX: qt.bounds.MinX,
		MaxX: midX,
		MinY: midY,
		MaxY: qt.bounds.MaxY,
	}, qt.capacity)

	qt.northEast = New(Bounds{
		MinX: midX,
		MaxX: qt.bounds.MaxX,
		MinY: midY,
		MaxY: qt.bounds.MaxY,
	}, qt.capacity)

	qt.southWest = New(Bounds{
		MinX: qt.bounds.MinX,
		MaxX: midX,
		MinY: qt.bounds.MinY,
		MaxY: midY,
	}, qt.capacity)

	qt.southEast = New(Bounds{
		MinX: midX,
		MaxX: qt.bounds.MaxX,
		MinY: qt.bounds.MinY,
		MaxY: midY,
	}, qt.capacity)

	qt.divided = true

	// Redistribute ALL existing points to children
	for _, n := range qt.nodes {
		if !qt.insertIntoChild(n) {
			panic("failed to redistribute point during subdivision")
		}
	}
	qt.nodes = nil // Clear parent's points
}

// InsertAll inserts multiple points into the quadtree
func (qt *Quadtree) InsertAll(points []Point) {
	for _, p := range points {
		qt.Insert(p)
	}
}

// Query finds all points within the given bounds
func (qt *Quadtree) Query(bounds Bounds, results *[]Point) {
	if !qt.Intersects(bounds) {
		return
	}
	for _, node := range qt.nodes {
		if bounds.contains(node.X, node.Y) {
			*results = append(*results, node)
		}
	}

	if qt.divided {
		qt.northWest.Query(bounds, results)
		qt.northEast.Query(bounds, results)
		qt.southWest.Query(bounds, results)
		qt.southEast.Query(bounds, results)
	}
}

var resultsPool = sync.Pool{
	New: func() interface{} {
		slice := make([]Point, 0, 4)
		return &slice
	},
}

// QueryResults returns all points within the given bounds
func (qt *Quadtree) QueryResults(bounds Bounds) []Point {
	// Get a pre-allocated slice from the pool
	resultsPtr := resultsPool.Get().(*[]Point)
	results := *resultsPtr
	results = results[:0] // Clear but keep capacity

	// Use the pooled slice for the query
	qt.Query(bounds, &results)

	// Create a new slice with exact capacity for the return value
	returnSlice := make([]Point, len(results))
	copy(returnSlice, results)

	// Return the original slice to the pool
	*resultsPtr = results
	resultsPool.Put(resultsPtr)

	return returnSlice
}

// Intersects checks if a given bounds intersects with the quadtree's bounds
// (separating axis theorem)
func (qt *Quadtree) Intersects(b Bounds) bool {
	// If any of these are true, the rectangles definitely don't overlap.
	return !(b.MaxX < qt.bounds.MinX || b.MinX > qt.bounds.MaxX ||
		b.MinY > qt.bounds.MaxY || b.MaxY < qt.bounds.MinY)
}

// InsideBounds check if a point is inside the quadtree's bounds
func (qt *Quadtree) InsideBounds(x, y float64) bool {
	return x >= qt.bounds.MinX && x <= qt.bounds.MaxX &&
		y >= qt.bounds.MinY && y <= qt.bounds.MaxY
}
