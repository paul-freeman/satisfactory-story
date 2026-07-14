package state

import (
	"math"
	"testing"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
	"github.com/paul-freeman/satisfactory-story/recipes"
	"github.com/paul-freeman/satisfactory-story/resources"
)

func Test_adjustPrices_ask_dynamics(t *testing.T) {
	t.Run("unsold ask is lowered toward the resource floor", func(t *testing.T) {
		ore := &resources.Resource{
			Production: production.Production{Name: "Ore", Rate: 100},
			Loc:        point.Point{X: 500, Y: 500},
		}
		ore.Stock = 5 // stock-backed ask
		s := newTestStateWithProducers(recipes.Recipes{}, []production.Producer{ore})
		s.publishOrders(testLogger()) // ask goes unmatched -- nobody bids

		s.adjustPrices(testLogger())

		want := production.DefaultUnitPrice * (1 - askLowerPct)
		if got := ore.AskPriceFor("Ore"); math.Abs(got-want) > 1e-9 {
			t.Errorf("expected lowered ask %f, got %f", want, got)
		}

		// Lowering never crosses the floor.
		ore.SetAskPrice("Ore", production.MinUnitPrice)
		ore.Stock = 5 // replenish stock for next publishOrders
		s.publishOrders(testLogger())
		s.adjustPrices(testLogger())
		if got := ore.AskPriceFor("Ore"); got < production.MinUnitPrice {
			t.Errorf("ask fell below the floor: %f", got)
		}
	})

	t.Run("sold-out ask is raised", func(t *testing.T) {
		ore := &resources.Resource{
			Production: production.Production{Name: "Ore", Rate: 10},
			Loc:        point.Point{X: 500, Y: 500},
		}
		ore.Stock = 10 // stock-backed ask to be consumed by buyer
		buyer := factory.New("Smelt", "Recipe_Smelt_C", point.Point{X: 501, Y: 501}, 0,
			production.Products{{Name: "Ore", Rate: 10}},
			production.Products{{Name: "Ingot", Rate: 5}}, 1000)
		buyer.SetBidPrice("Ore", 100.0)

		s := newTestStateWithProducers(recipes.Recipes{}, []production.Producer{ore, buyer})
		s.publishOrders(testLogger())
		s.matchOrders(testLogger()) // consumes the whole ask

		s.adjustPrices(testLogger())

		want := production.DefaultUnitPrice * (1 + askRaisePct)
		if got := ore.AskPriceFor("Ore"); math.Abs(got-want) > 1e-9 {
			t.Errorf("expected raised ask %f, got %f", want, got)
		}
	})

	t.Run("factory ask is floored at marginal cost", func(t *testing.T) {
		f := factory.New("Smelt", "Recipe_Smelt_C", point.Point{X: 0, Y: 0}, 0,
			production.Products{{Name: "Ore", Rate: 5}},
			production.Products{{Name: "Ingot", Rate: 4}}, 1000)
		f.Purchases = append(f.Purchases, &production.Contract{
			Order:         production.Production{Name: "Ore", Rate: 5},
			ProductCost:   10,
			TransportCost: 2,
		})
		// marginal cost = (12 + upkeep) / 4 = 3.125 > current default ask 1.0
		f.SetAskPrice("Ingot", 1.0)
		f.OutputStock.Add("Ingot", 5) // stock-backed ask

		s := newTestStateWithProducers(recipes.Recipes{}, []production.Producer{f})
		s.publishOrders(testLogger()) // producing, unsold ask

		s.adjustPrices(testLogger())

		want := f.MarginalUnitCost(upkeepPerTick)
		if got := f.AskPriceFor("Ingot"); math.Abs(got-want) > 1e-9 {
			t.Errorf("expected ask floored at marginal cost %f, got %f", want, got)
		}
	})
}

func Test_adjustPrices_bid_dynamics(t *testing.T) {
	t.Run("unfilled bid escalates when downstream demand affords it", func(t *testing.T) {
		f := factory.New("Smelt", "Recipe_Smelt_C", point.Point{X: 0, Y: 0}, 0,
			production.Products{{Name: "Ore", Rate: 5}},
			production.Products{{Name: "Ingot", Rate: 5}}, 1000)
		rich := factory.New("Down", "Recipe_Down_C", point.Point{X: 1, Y: 1}, 0,
			production.Products{{Name: "Ingot", Rate: 5}},
			production.Products{{Name: "Plate", Rate: 5}}, 1000)

		s := newTestStateWithProducers(recipes.Recipes{}, []production.Producer{f, rich})
		rich.SetBidPrice("Ingot", 40.0) // huge downstream willingness
		s.publishOrders(testLogger())
		// No Ore seller exists: f's bid stays unfilled.

		s.adjustPrices(testLogger())

		want := production.DefaultUnitPrice * (1 + bidRaisePct)
		if got := f.BidPriceFor("Ore"); math.Abs(got-want) > 1e-9 {
			t.Errorf("expected escalated bid %f, got %f", want, got)
		}
	})

	t.Run("bid freezes at the affordability cap", func(t *testing.T) {
		f := factory.New("Smelt", "Recipe_Smelt_C", point.Point{X: 0, Y: 0}, 0,
			production.Products{{Name: "Ore", Rate: 5}},
			production.Products{{Name: "Ingot", Rate: 5}}, 1000)

		s := newTestStateWithProducers(recipes.Recipes{}, []production.Producer{f})
		// Nobody bids on Ingot: achievable revenue is only the salvage
		// floor (5 * 0.1 = 0.5), nowhere near the planned spend of an
		// escalated 3.0-and-up bid on 5 units of Ore.
		f.SetBidPrice("Ore", 3.0)
		s.publishOrders(testLogger())

		s.adjustPrices(testLogger())

		if got := f.BidPriceFor("Ore"); got != 3.0 {
			t.Errorf("bid should freeze without downstream revenue to support it, got %f", got)
		}
	})

	t.Run("sink bids never adjust", func(t *testing.T) {
		rs := recipes.Recipes{
			{DisplayName: "A", OutputProducts: production.Products{{Name: "SpaceElevatorPart_1", Rate: 1}}},
		}
		sinks := newSinks(rs, 0, 1000, 0, 1000)
		producers := make([]production.Producer, 0, len(sinks))
		for _, sk := range sinks {
			producers = append(producers, sk)
		}
		s := newTestStateWithProducers(rs, producers)
		s.publishOrders(testLogger())

		s.adjustPrices(testLogger())

		if sinks[0].BidUnitPrice != goalBidUnitPrice {
			t.Errorf("sink bid must stay fixed at %f, got %f", goalBidUnitPrice, sinks[0].BidUnitPrice)
		}
	})
}
