package state

import (
	"testing"

	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
	"github.com/paul-freeman/satisfactory-story/resources"
)

func testResourceAt(x, y int) *resources.Resource {
	return &resources.Resource{
		Production: production.Production{Name: "OreIron", Rate: 1},
		Loc:        point.Point{X: x, Y: y},
	}
}

func Test_tradeLedger(t *testing.T) {
	a := testResourceAt(0, 0)
	b := testResourceAt(10, 0)
	c := testResourceAt(20, 0)

	tl := &tradeLedger{}
	tl.record(100, a, b, "OreIron", 2, 1.5)
	tl.record(101, a, b, "OreIron", 3, 1.5)
	tl.record(102, c, b, "OreIron", 1, 2.0)

	edges := tl.edges()
	if len(edges) != 2 {
		t.Fatalf("edges = %d, want 2 (a->b aggregated, c->b)", len(edges))
	}
	if edges[0].seller != production.Producer(a) || edges[0].qty != 5 {
		t.Fatalf("first edge = %+v, want a->b qty 5", edges[0])
	}
	if edges[1].seller != production.Producer(c) || edges[1].qty != 1 {
		t.Fatalf("second edge = %+v, want c->b qty 1", edges[1])
	}

	if !tl.recentSellers()[production.Producer(a)] || tl.recentSellers()[production.Producer(b)] {
		t.Fatal("recentSellers should contain a (sold) and not b (only bought)")
	}

	tl.prune(700, 600) // drops trades older than tick 100
	if len(tl.trades) != 3 {
		t.Fatalf("prune(700, 600) kept %d, want 3 (all within window)", len(tl.trades))
	}
	tl.prune(1000, 500) // window now starts at 500: everything dropped
	if len(tl.trades) != 0 {
		t.Fatalf("prune(1000, 500) kept %d, want 0", len(tl.trades))
	}
}
