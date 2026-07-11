package state

import (
	"testing"

	"github.com/paul-freeman/satisfactory-story/production"
	"github.com/paul-freeman/satisfactory-story/recipes"
)

func Test_newSinks_finds_distinct_space_elevator_parts(t *testing.T) {
	rs := recipes.Recipes{
		{DisplayName: "A", OutputProducts: production.Products{{Name: "SpaceElevatorPart_1", Rate: 1}}},
		{DisplayName: "B", OutputProducts: production.Products{{Name: "SpaceElevatorPart_1", Rate: 1}}}, // duplicate product
		{DisplayName: "C", OutputProducts: production.Products{{Name: "SpaceElevatorPart_2", Rate: 1}}},
		{DisplayName: "D", OutputProducts: production.Products{{Name: "IronPlate", Rate: 1}}}, // not a sink product
	}

	sinks := newSinks(rs, 0, 1000, 0, 1000)
	if len(sinks) != 2 {
		t.Fatalf("expected 2 distinct sinks, got %d", len(sinks))
	}
	names := map[string]bool{}
	for _, sk := range sinks {
		names[sk.Name] = true
	}
	if !names["SpaceElevatorPart_1"] || !names["SpaceElevatorPart_2"] {
		t.Errorf("expected both space elevator parts represented, got %+v", names)
	}
	for _, sk := range sinks {
		if sk.BidUnitPrice != goalBidUnitPrice {
			t.Errorf("goal sink %s should bid %f, got %f", sk.Name, goalBidUnitPrice, sk.BidUnitPrice)
		}
	}
}
