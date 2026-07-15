package state

import (
	"testing"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
	"github.com/paul-freeman/satisfactory-story/recipes"
	"github.com/paul-freeman/satisfactory-story/resources"
	"github.com/paul-freeman/satisfactory-story/sink"
)

// delivered reports whether any sink in the world has ever received a
// delivery.
func delivered(s *State) bool {
	for _, p := range s.producers {
		if sk, ok := p.(*sink.Sink); ok && sk.TotalDelivered() > 0 {
			return true
		}
	}
	return false
}

// anyFactoryProducing reports whether any factory in the world produced
// output on its most recent tick.
func anyFactoryProducing(s *State) bool {
	for _, p := range s.producers {
		if f, ok := p.(*factory.Factory); ok && f.ProducedLastTick {
			return true
		}
	}
	return false
}

// Test_cascade_single_tier: a goal sink bids for Ingot; only an Ore node
// exists. A smelter must spawn, source Ore through the book, produce,
// and deliver to the sink -- demand becomes supply with no tree-reading.
func Test_cascade_single_tier(t *testing.T) {
	ore := &resources.Resource{
		Production: production.Production{Name: "Ore", Rate: 100},
		Loc:        point.Point{X: 400, Y: 400},
	}
	rs := recipes.Recipes{
		{
			ClassName:      "Recipe_Smelt_C",
			DisplayName:    "Smelt Ore",
			Active:         true,
			InputProducts:  production.Products{{Name: "Ore", Rate: 5}},
			OutputProducts: production.Products{{Name: "Ingot", Rate: 5}},
		},
	}
	goal := sink.New("Ingot", point.Point{X: 600, Y: 600},
		production.Products{{Name: "Ingot", Rate: 1}}, goalBidUnitPrice)

	s := newTestStateWithProducers(rs, []production.Producer{ore, goal})

	const budget = 2000
	sawProducing := false
	i := 0
	for ; i < budget && !delivered(s); i++ {
		if err := s.Tick(testLogger()); err != nil {
			t.Fatalf("tick %d failed: %v", i, err)
		}
		if anyFactoryProducing(s) {
			sawProducing = true
		}
	}

	if !delivered(s) {
		t.Fatalf("no Ingot delivered to the goal sink within %d ticks", budget)
	}
	t.Logf("Ingot delivered after %d ticks", i)

	if !sawProducing {
		t.Fatal("no factory ever reported ProducedLastTick == true before delivery")
	}
	if len(s.ledger.trades) == 0 {
		t.Fatal("trade ledger is empty at delivery time; no trades were recorded")
	}
}

// Test_cascade_two_tier: the goal sink bids for Plate, which needs
// Ingot, which needs Ore. The Plate factory must spawn idle, its
// escalating Ingot bid must make smelting look profitable, a smelter
// must spawn and connect to Ore, and the full chain must flow.
func Test_cascade_two_tier(t *testing.T) {
	ore := &resources.Resource{
		Production: production.Production{Name: "Ore", Rate: 100},
		Loc:        point.Point{X: 400, Y: 400},
	}
	rs := recipes.Recipes{
		{
			ClassName:      "Recipe_Smelt_C",
			DisplayName:    "Smelt Ore",
			Active:         true,
			InputProducts:  production.Products{{Name: "Ore", Rate: 5}},
			OutputProducts: production.Products{{Name: "Ingot", Rate: 5}},
		},
		{
			ClassName:      "Recipe_Plate_C",
			DisplayName:    "Roll Plate",
			Active:         true,
			InputProducts:  production.Products{{Name: "Ingot", Rate: 3}},
			OutputProducts: production.Products{{Name: "Plate", Rate: 2}},
		},
	}
	goal := sink.New("Plate", point.Point{X: 600, Y: 600},
		production.Products{{Name: "Plate", Rate: 1}}, goalBidUnitPrice)

	s := newTestStateWithProducers(rs, []production.Producer{ore, goal})

	const budget = 5000
	sawProducing := false
	i := 0
	for ; i < budget && !delivered(s); i++ {
		if err := s.Tick(testLogger()); err != nil {
			t.Fatalf("tick %d failed: %v", i, err)
		}
		if anyFactoryProducing(s) {
			sawProducing = true
		}
	}

	if !delivered(s) {
		t.Fatalf("no Plate delivered to the goal sink within %d ticks", budget)
	}
	t.Logf("Plate delivered after %d ticks", i)

	if !sawProducing {
		t.Fatal("no factory ever reported ProducedLastTick == true before delivery")
	}
	if len(s.ledger.trades) == 0 {
		t.Fatal("trade ledger is empty at delivery time; no trades were recorded")
	}
}
