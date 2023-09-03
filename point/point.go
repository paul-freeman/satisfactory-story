package point

import (
	"fmt"
	"math"
)

type Point struct {
	X int
	Y int
}

func (p Point) String() string {
	return fmt.Sprintf("(%d, %d)", p.X, p.Y)
}

func (p Point) Distance(q Point) float64 {
	a := float64(p.X - q.X)
	b := float64(p.Y - q.Y)
	return math.Sqrt(
		math.Abs(math.Pow(a, 2)) + math.Abs(math.Pow(b, 2)),
	)
}

func (p Point) Up() Point {
	return Point{X: p.X, Y: p.Y - 1}
}

func (p Point) Down() Point {
	return Point{X: p.X, Y: p.Y + 1}
}

func (p Point) Left() Point {
	return Point{X: p.X - 1, Y: p.Y}
}

func (p Point) Right() Point {
	return Point{X: p.X + 1, Y: p.Y}
}
