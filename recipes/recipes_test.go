package recipes

import (
	"testing"

	"github.com/paul-freeman/satisfactory-story/point"
)

func Test_UnitTransportCost(t *testing.T) {
	a := point.Point{X: 0, Y: 0}
	// Same-spot collision guard survives from TransportCost.
	if got := UnitTransportCost(a, point.Point{X: 1, Y: 0}); got != 1e12 {
		t.Fatalf("collision guard = %v, want 1e12", got)
	}
	// 10000 units of distance costs 0.1 fixed + 1.0 distance.
	got := UnitTransportCost(a, point.Point{X: 10000, Y: 0})
	if got < 1.0999 || got > 1.1001 {
		t.Fatalf("UnitTransportCost(10000) = %v, want ~1.1", got)
	}
}
