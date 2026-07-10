package state

import (
	"testing"

	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
	"github.com/paul-freeman/satisfactory-story/recipes"
	"github.com/paul-freeman/satisfactory-story/resources"
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
}

func Test_sourceSinks_buys_all_available_capacity(t *testing.T) {
	seller := &resources.Resource{
		Production: production.Production{Name: "SpaceElevatorPart_1", Rate: 5},
		Loc:        point.Point{X: 0, Y: 0},
	}
	rs := recipes.Recipes{
		{DisplayName: "A", OutputProducts: production.Products{{Name: "SpaceElevatorPart_1", Rate: 1}}},
	}
	sinks := newSinks(rs, 0, 1000, 0, 1000)

	producers := make([]production.Producer, 0, len(sinks)+1)
	producers = append(producers, seller)
	for _, sk := range sinks {
		producers = append(producers, sk)
	}
	s := &State{producers: producers, market: make(map[string]float64)}

	s.sourceSinks(testLogger())

	if got := seller.RemainingCapacityFor("SpaceElevatorPart_1"); got != 0 {
		t.Errorf("expected the sink to buy all remaining capacity, got %f left", got)
	}
}
