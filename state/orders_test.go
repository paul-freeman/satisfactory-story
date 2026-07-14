package state

import (
	"testing"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
	"github.com/paul-freeman/satisfactory-story/recipes"
	"github.com/paul-freeman/satisfactory-story/resources"
)

func Test_publishOrders_resource_ask_and_factory_orders(t *testing.T) {
	ore := &resources.Resource{
		Production: production.Production{Name: "Ore", Rate: 100},
		Loc:        point.Point{X: 500, Y: 500},
	}
	idle := factory.New("Smelt", "Recipe_Smelt_C", point.Point{X: 600, Y: 600}, 0,
		production.Products{{Name: "Ore", Rate: 5}},
		production.Products{{Name: "Ingot", Rate: 5}}, 1000)

	s := newTestState(recipes.Recipes{}, []production.Producer{ore, idle})
	s.publishOrders(testLogger())

	if asks := s.book.Asks("Ore"); len(asks) != 1 || asks[0].Remaining != 100 {
		t.Fatalf("expected one Ore ask at rate 100, got %+v", asks)
	}
	if bids := s.book.Bids("Ore"); len(bids) != 1 || bids[0].Remaining != 5 {
		t.Fatalf("expected one Ore bid at rate 5, got %+v", bids)
	}
	// The factory is idle (no input contracts) so it must not offer output.
	if asks := s.book.Asks("Ingot"); len(asks) != 0 {
		t.Fatalf("an idle factory must not publish asks, got %+v", asks)
	}
}

func Test_matchOrders_signs_contract_and_records_trade(t *testing.T) {
	ore := &resources.Resource{
		Production: production.Production{Name: "Ore", Rate: 100},
		Loc:        point.Point{X: 500, Y: 500},
	}
	buyer := factory.New("Smelt", "Recipe_Smelt_C", point.Point{X: 600, Y: 600}, 0,
		production.Products{{Name: "Ore", Rate: 5}},
		production.Products{{Name: "Ingot", Rate: 5}}, 1000)
	buyer.SetBidPrice("Ore", 50.0) // rich enough to cover ask + transport

	s := newTestState(recipes.Recipes{}, []production.Producer{ore, buyer})
	s.publishOrders(testLogger())
	s.matchOrders(testLogger())

	if !buyer.Producing() {
		t.Fatal("expected the buyer's Ore input to be fully contracted")
	}
	if got := ore.RemainingCapacityFor("Ore"); got != 95 {
		t.Errorf("expected 95 capacity left on the ore node, got %f", got)
	}
	if len(buyer.Purchases) != 1 {
		t.Fatalf("expected 1 purchase, got %d", len(buyer.Purchases))
	}
	p := buyer.Purchases[0]
	wantCost := production.DefaultUnitPrice * 5 // ask price * rate
	if p.ProductCost != wantCost {
		t.Errorf("expected ProductCost %f, got %f", wantCost, p.ProductCost)
	}
	wantTransport := recipes.UnitTransportCost(ore.Loc, buyer.Loc) * p.Order.Rate
	if p.TransportCost != wantTransport {
		t.Errorf("expected TransportCost %f, got %f", wantTransport, p.TransportCost)
	}
	if got := s.lastTrade["Ore"]; got != production.DefaultUnitPrice {
		t.Errorf("expected lastTrade recorded at %f, got %f", production.DefaultUnitPrice, got)
	}
}

func Test_matchOrders_goal_sink_buys_available_capacity(t *testing.T) {
	// A resource node standing in for a finished-part producer, matching
	// the fixture style of the old sourceSinks test.
	seller := &resources.Resource{
		Production: production.Production{Name: "SpaceElevatorPart_1", Rate: 5},
		Loc:        point.Point{X: 0, Y: 0},
	}
	rs := recipes.Recipes{
		{DisplayName: "A", OutputProducts: production.Products{{Name: "SpaceElevatorPart_1", Rate: 1}}},
	}
	sinks := newSinks(rs, 0, 1000, 0, 1000)

	producers := []production.Producer{seller}
	for _, sk := range sinks {
		producers = append(producers, sk)
	}
	s := newTestState(rs, producers)

	s.publishOrders(testLogger())
	s.matchOrders(testLogger())

	if got := seller.RemainingCapacityFor("SpaceElevatorPart_1"); got != 0 {
		t.Errorf("expected the goal sink to buy all capacity, got %f left", got)
	}
}
