package factory

import (
	"testing"

	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
)

func Test_Factory_Cash(t *testing.T) {
	f := New("Test Factory", "Recipe_Test_C", point.Point{X: 0, Y: 0}, 0, production.Products{}, production.Products{}, 250)
	if f.Cash() != 250 {
		t.Errorf("got %f, want 250", f.Cash())
	}
}

func Test_Factory_RemainingCapacityFor(t *testing.T) {
	output := production.Products{{Name: "Widget", Rate: 10}}
	f := New("Test Factory", "Recipe_Test_C", point.Point{X: 0, Y: 0}, 0, production.Products{}, output, 0)

	if got := f.RemainingCapacityFor("Widget"); got != 10 {
		t.Errorf("got %f, want 10", got)
	}
	if got := f.RemainingCapacityFor("NotProduced"); got != 0 {
		t.Errorf("got %f, want 0", got)
	}

	f.Sales = append(f.Sales, &production.Contract{
		Order: production.Production{Name: "Widget", Rate: 6},
	})
	if got := f.RemainingCapacityFor("Widget"); got != 4 {
		t.Errorf("got %f, want 4", got)
	}

	if err := f.HasCapacityFor(production.Production{Name: "Widget", Rate: 4}); err != nil {
		t.Errorf("expected capacity for 4, got error: %v", err)
	}
	if err := f.HasCapacityFor(production.Production{Name: "Widget", Rate: 5}); err == nil {
		t.Errorf("expected an error oversubscribing capacity, got nil")
	}
}

func Test_Factory_Profit(t *testing.T) {
	f := New("Test Factory", "Recipe_Test_C", point.Point{X: 0, Y: 0}, 0, production.Products{}, production.Products{}, 0)
	f.Sales = append(f.Sales, &production.Contract{
		ProductCost:   30,
		TransportCost: 5,
	})
	f.Purchases = append(f.Purchases, &production.Contract{
		ProductCost:   10,
		TransportCost: 2,
	})
	// (30 - 5) - (10 + 2) = 13
	got := f.Profit()
	want := 13.0
	if got != want {
		t.Errorf("got %f, want %f", got, want)
	}
}

func Test_Factory_Profit_ignores_cancelled_contracts(t *testing.T) {
	f := New("Test Factory", "Recipe_Test_C", point.Point{X: 0, Y: 0}, 0, production.Products{}, production.Products{}, 0)
	f.Sales = append(f.Sales, &production.Contract{
		ProductCost:   30,
		TransportCost: 5,
		Cancelled:     true,
	})
	f.Purchases = append(f.Purchases, &production.Contract{
		ProductCost:   10,
		TransportCost: 2,
	})
	// cancelled sale contributes nothing: 0 - (10 + 2) = -12
	got := f.Profit()
	want := -12.0
	if got != want {
		t.Errorf("got %f, want %f", got, want)
	}
	if len(f.Sales) != 0 {
		t.Errorf("cancelled sale should have been pruned, got %d sales left", len(f.Sales))
	}
}

func Test_Factory_ask_and_bid_prices_default_and_set(t *testing.T) {
	f := New("Test", "Recipe_Test_C", point.Point{X: 0, Y: 0}, 0,
		production.Products{{Name: "Ore", Rate: 5}},
		production.Products{{Name: "Ingot", Rate: 5}}, 0)

	if got := f.AskPriceFor("Ingot"); got != production.DefaultUnitPrice {
		t.Errorf("unquoted ask should default to %f, got %f", production.DefaultUnitPrice, got)
	}
	f.SetAskPrice("Ingot", 2.5)
	if got := f.AskPriceFor("Ingot"); got != 2.5 {
		t.Errorf("got %f, want 2.5", got)
	}

	if got := f.BidPriceFor("Ore"); got != production.DefaultUnitPrice {
		t.Errorf("unquoted bid should default to %f, got %f", production.DefaultUnitPrice, got)
	}
	f.SetBidPrice("Ore", 0.7)
	if got := f.BidPriceFor("Ore"); got != 0.7 {
		t.Errorf("got %f, want 0.7", got)
	}
}

func Test_Factory_UnmetInputRate_and_Producing(t *testing.T) {
	f := New("Test", "Recipe_Test_C", point.Point{X: 0, Y: 0}, 0,
		production.Products{{Name: "Ore", Rate: 5}, {Name: "Coal", Rate: 2}},
		production.Products{{Name: "Ingot", Rate: 5}}, 0)

	if got := f.UnmetInputRate("Ore"); got != 5 {
		t.Errorf("got %f, want 5", got)
	}
	if f.Producing() {
		t.Error("a factory with no input contracts must not be producing")
	}

	// Two partial contracts covering Ore, one covering Coal.
	f.Purchases = append(f.Purchases,
		&production.Contract{Order: production.Production{Name: "Ore", Rate: 3}},
		&production.Contract{Order: production.Production{Name: "Ore", Rate: 2}},
	)
	if got := f.UnmetInputRate("Ore"); got != 0 {
		t.Errorf("got %f, want 0", got)
	}
	if f.Producing() {
		t.Error("Coal is still unsourced -- must not be producing")
	}

	coal := &production.Contract{Order: production.Production{Name: "Coal", Rate: 2}}
	f.Purchases = append(f.Purchases, coal)
	if !f.Producing() {
		t.Error("all inputs covered -- must be producing")
	}

	// Cancelled contracts stop counting.
	coal.Cancel()
	if f.Producing() {
		t.Error("a cancelled contract must not count as coverage")
	}
	if got := f.UnmetInputRate("NotAnInput"); got != 0 {
		t.Errorf("unknown input should report 0 unmet, got %f", got)
	}
}

func Test_Factory_MarginalUnitCost(t *testing.T) {
	f := New("Test", "Recipe_Test_C", point.Point{X: 0, Y: 0}, 0,
		production.Products{{Name: "Ore", Rate: 5}},
		production.Products{{Name: "Ingot", Rate: 4}}, 0)
	f.Purchases = append(f.Purchases, &production.Contract{
		Order:         production.Production{Name: "Ore", Rate: 5},
		ProductCost:   10,
		TransportCost: 2,
	})

	// (10 + 2 purchases + 0.5 upkeep) / 4 output rate = 3.125
	if got := f.MarginalUnitCost(0.5); got != 3.125 {
		t.Errorf("got %f, want 3.125", got)
	}
}
