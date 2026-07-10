package state

import (
	"math"
	"math/rand"
	"testing"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
	"github.com/paul-freeman/satisfactory-story/recipes"
	"github.com/paul-freeman/satisfactory-story/resources"
)

func newTestState(rs recipes.Recipes, producers []production.Producer) *State {
	return &State{
		recipes:   rs,
		producers: producers,
		market:    make(map[string]float64),
		unmet:     make(map[string]float64),
		randSrc:   rand.New(rand.NewSource(1)),
		xmin:      0, xmax: 1000, ymin: 0, ymax: 1000,
	}
}

func Test_spawnNewProducer_spawns_when_viable(t *testing.T) {
	ore := &resources.Resource{
		Production: production.Production{Name: "Ore", Rate: 100},
		Loc:        point.Point{X: 500, Y: 500},
	}
	rs := recipes.Recipes{
		{
			DisplayName:    "Smelt Ore",
			Active:         true,
			InputProducts:  production.Products{{Name: "Ore", Rate: 5}},
			OutputProducts: production.Products{{Name: "Ingot", Rate: 5}},
		},
	}
	s := newTestState(rs, []production.Producer{ore})

	s.spawnNewProducer(testLogger())

	if len(s.producers) != 2 {
		t.Fatalf("expected a new factory to spawn, got %d producers", len(s.producers))
	}

	// Find the newly spawned factory (the non-ore producer).
	var f *factory.Factory
	for _, p := range s.producers {
		if _, ok := p.(*resources.Resource); !ok {
			f = p.(*factory.Factory)
			break
		}
	}
	if f == nil {
		t.Fatalf("could not find spawned factory in producers")
	}

	// Compute expected seed capital from fixture data.
	// seedCapitalBufferTicks is the constant from spawn.go.
	const seedCapitalBufferTicks = 5.0
	transportCost := recipes.TransportCost(ore.Loc, f.Loc)
	order := production.Production{Name: "Ore", Rate: 5}
	salesPrice := ore.SalesPriceFor(order, transportCost)
	expectedSeedCapital := (salesPrice + transportCost) * seedCapitalBufferTicks

	// Assert seed capital matches expected value (with float tolerance).
	if math.Abs(f.Wallet.Balance-expectedSeedCapital) >= 0.0001 {
		t.Errorf("expected seed capital %f, got %f", expectedSeedCapital, f.Wallet.Balance)
	}
}

func Test_spawnNewProducer_skips_when_uneconomical(t *testing.T) {
	ore := &resources.Resource{
		Production: production.Production{Name: "Ore", Rate: 100},
		Loc:        point.Point{X: 500, Y: 500},
	}
	rs := recipes.Recipes{
		{
			DisplayName:    "Smelt Ore",
			Active:         true,
			InputProducts:  production.Products{{Name: "Ore", Rate: 5}},
			OutputProducts: production.Products{{Name: "Ingot", Rate: 5}},
		},
	}
	s := newTestState(rs, []production.Producer{ore})
	// Someone is already selling Ingot far below what this recipe could
	// ever recoup from its input costs -- spawning here would guarantee a
	// loss, so it should be skipped.
	s.market["Ingot"] = 0.0001

	s.spawnNewProducer(testLogger())

	if len(s.producers) != 1 {
		t.Fatalf("expected no new factory to spawn, got %d producers", len(s.producers))
	}
}
