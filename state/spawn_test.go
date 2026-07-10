package state

import (
	"math/rand"
	"testing"

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
