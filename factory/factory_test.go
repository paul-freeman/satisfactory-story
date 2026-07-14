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

func Test_Factory_Move_contractless_factory_holds_still(t *testing.T) {
	start := point.Point{X: 500, Y: 500}
	f := New("Test", "Recipe_Test_C", start, 0,
		production.Products{{Name: "Ore", Rate: 5}},
		production.Products{{Name: "Ingot", Rate: 5}}, 0)

	// A factory with no purchases and no sales has no transport-cost
	// gradient to climb -- it must hold still, not drift in a fixed
	// direction every tick (the bug this test guards against: with no
	// contracts, transportCostsAt is 0 in every direction, so a naive
	// "move toward the lowest-cost neighbor" tie-break always picks the
	// same direction and the factory marches off the map forever).
	for i := 0; i < 10; i++ {
		if err := f.Move(); err != nil {
			t.Fatalf("Move returned an error: %v", err)
		}
	}
	if f.Loc != start {
		t.Errorf("expected a contractless factory to stay at %v, moved to %v", start, f.Loc)
	}
}

func Test_Factory_Move_climbs_toward_lower_transport_cost(t *testing.T) {
	start := point.Point{X: 500, Y: 500}
	f := New("Test", "Recipe_Test_C", start, 0,
		production.Products{{Name: "Ore", Rate: 5}},
		production.Products{{Name: "Ingot", Rate: 5}}, 0)

	// A real trade partner still gives Move() a gradient to climb -- this
	// guards against the fix for the tradeless case accidentally
	// disabling movement altogether.
	sellerLoc := point.Point{X: 600, Y: 500}
	f.RecordTrade(1, sellerLoc, 5)

	if err := f.Move(); err != nil {
		t.Fatalf("Move returned an error: %v", err)
	}
	if f.Loc == start {
		t.Error("expected the factory to move toward its trade partner, but it held still")
	}
	if f.Loc.X <= start.X {
		t.Errorf("expected the factory to move toward its trade partner at X=600, got %v", f.Loc)
	}
}

func Test_Factory_ProduceTick(t *testing.T) {
	f := New("Plates", "Recipe_Plates_C", point.Point{X: 0, Y: 0}, 0,
		production.Products{production.Production{Name: "IronIngot", Rate: 2}},
		production.Products{production.Production{Name: "IronPlate", Rate: 3}},
		100)
	// No input stock: nothing runs.
	if frac := f.ProduceTick(60); frac != 0 || f.ProducedLastTick {
		t.Fatalf("empty-stock ProduceTick = %v (produced=%v), want 0 (false)", frac, f.ProducedLastTick)
	}
	// Half the needed input: runs at half rate.
	f.InputStock.Add("IronIngot", 1)
	frac := f.ProduceTick(60)
	if frac < 0.499 || frac > 0.501 {
		t.Fatalf("half-stock ProduceTick = %v, want 0.5", frac)
	}
	if got := f.OutputStock.Get("IronPlate"); got < 1.499 || got > 1.501 {
		t.Fatalf("output stock = %v, want 1.5", got)
	}
	if got := f.InputStock.Get("IronIngot"); got > 1e-9 {
		t.Fatalf("input stock = %v, want 0", got)
	}
	if !f.ProducedLastTick {
		t.Fatal("ProducedLastTick should be true after a fractional run")
	}
	// Output cap limits the run: cap 1 tick's worth (3 units), stock
	// already 1.5, plenty of input -> only 0.5 ticks of room.
	f.InputStock.Add("IronIngot", 100)
	frac = f.ProduceTick(1)
	if frac < 0.499 || frac > 0.501 {
		t.Fatalf("cap-limited ProduceTick = %v, want 0.5", frac)
	}
	// Full stock: halts entirely.
	frac = f.ProduceTick(1)
	if frac != 0 || f.ProducedLastTick {
		t.Fatalf("full-stock ProduceTick = %v (produced=%v), want 0 (false)", frac, f.ProducedLastTick)
	}
}

func Test_Factory_Hunger(t *testing.T) {
	f := New("Plates", "Recipe_Plates_C", point.Point{X: 0, Y: 0}, 0,
		production.Products{production.Production{Name: "IronIngot", Rate: 2}},
		production.Products{production.Production{Name: "IronPlate", Rate: 3}},
		100)
	if got := f.Hunger("IronIngot", 10); got != 20 {
		t.Fatalf("empty Hunger = %v, want 20 (rate 2 x target 10)", got)
	}
	f.InputStock.Add("IronIngot", 15)
	if got := f.Hunger("IronIngot", 10); got != 5 {
		t.Fatalf("partial Hunger = %v, want 5", got)
	}
	f.InputStock.Add("IronIngot", 100)
	if got := f.Hunger("IronIngot", 10); got != 0 {
		t.Fatalf("overshoot Hunger = %v, want 0", got)
	}
	if got := f.Hunger("NotAnInput", 10); got != 0 {
		t.Fatalf("non-input Hunger = %v, want 0", got)
	}
}

func Test_Factory_TradeMemoryAndFlows(t *testing.T) {
	f := New("Plates", "Recipe_Plates_C", point.Point{X: 0, Y: 0}, 0,
		production.Products{production.Production{Name: "IronIngot", Rate: 2}},
		production.Products{production.Production{Name: "IronPlate", Rate: 3}},
		100)
	f.RecordTrade(10, point.Point{X: 5, Y: 5}, 4)
	f.RecordTrade(600, point.Point{X: 9, Y: 9}, 1)
	f.PruneTrades(610, 500) // keeps trades newer than tick 110
	if len(f.RecentTrades) != 1 || f.RecentTrades[0].Tick != 600 {
		t.Fatalf("PruneTrades kept %v, want only the tick-600 trade", f.RecentTrades)
	}
	f.TickInputSpend = 10
	f.TickRevenue = 30
	f.FoldTickFlows(0.5)
	if f.AvgInputSpend != 5 || f.AvgRevenue != 15 {
		t.Fatalf("EMAs = %v/%v, want 5/15", f.AvgInputSpend, f.AvgRevenue)
	}
	if f.TickInputSpend != 0 || f.TickRevenue != 0 {
		t.Fatal("FoldTickFlows must zero the per-tick accumulators")
	}
	// marginal cost: (5 + 0.5 upkeep) / 3 output rate
	got := f.StockMarginalUnitCost(0.5)
	if got < 1.8332 || got > 1.8334 {
		t.Fatalf("StockMarginalUnitCost = %v, want ~1.8333", got)
	}
}

func Test_Factory_Move_climbsTowardTradePartners(t *testing.T) {
	f := New("Plates", "Recipe_Plates_C", point.Point{X: 0, Y: 0}, 0,
		production.Products{production.Production{Name: "IronIngot", Rate: 1}},
		production.Products{production.Production{Name: "IronPlate", Rate: 2}},
		100)
	// No trades: holds still.
	if err := f.Move(); err != nil {
		t.Fatalf("Move error: %v", err)
	}
	if f.Loc.X != 0 || f.Loc.Y != 0 {
		t.Fatalf("tradeless factory moved to %v, want (0,0)", f.Loc)
	}
	// One partner far to the east: moves toward it (X increases).
	f.RecordTrade(1, point.Point{X: 100000, Y: 0}, 5)
	if err := f.Move(); err != nil {
		t.Fatalf("Move error: %v", err)
	}
	if f.Loc.X <= 0 {
		t.Fatalf("factory at %v, want X > 0 (moved toward partner)", f.Loc)
	}
}
