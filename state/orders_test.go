package state

import (
	"log/slog"
	"os"
	"testing"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/market"
	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
	"github.com/paul-freeman/satisfactory-story/resources"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func newTestState() *State {
	return &State{
		book:      market.NewBook(),
		lastTrade: make(map[string]float64),
		ledger:    &tradeLedger{},
		treasury:  initialTreasuryFund,
	}
}

func Test_publishOrders_stockBacked(t *testing.T) {
	s := newTestState()
	r := &resources.Resource{
		Production: production.Production{Name: "OreIron", Rate: 1},
		Loc:        point.Point{X: 0, Y: 0},
		Stock:      7,
	}
	f := factory.New("Smelter", "Recipe_IngotIron_C", point.Point{X: 100, Y: 0}, 0,
		production.Products{production.Production{Name: "OreIron", Rate: 1}},
		production.Products{production.Production{Name: "IronIngot", Rate: 1}},
		1000)
	f.OutputStock.Add("IronIngot", 3)
	f.InputStock.Add("OreIron", 10) // target is 60 -> hunger 50
	s.producers = []production.Producer{r, f}

	s.publishOrders(testLogger())

	ask, ok := s.book.BestAsk("OreIron")
	if !ok || ask.Remaining != 7 {
		t.Fatalf("resource ask = %+v (ok=%v), want remaining 7 (stock-backed)", ask, ok)
	}
	ingotAsk, ok := s.book.BestAsk("IronIngot")
	if !ok || ingotAsk.Remaining != 3 {
		t.Fatalf("factory ask = %+v (ok=%v), want remaining 3 (stock-backed)", ingotAsk, ok)
	}
	bid, ok := s.book.BestBid("OreIron")
	if !ok || bid.Remaining != 50 {
		t.Fatalf("factory bid = %+v (ok=%v), want remaining 50 (hunger)", bid, ok)
	}
}

func Test_executeTrade_movesGoodsAndMoney(t *testing.T) {
	s := newTestState()
	s.tick = 42
	r := &resources.Resource{
		Production: production.Production{Name: "OreIron", Rate: 1},
		Loc:        point.Point{X: 0, Y: 0},
		Stock:      10,
	}
	f := factory.New("Smelter", "Recipe_IngotIron_C", point.Point{X: 10000, Y: 0}, 0,
		production.Products{production.Production{Name: "OreIron", Rate: 1}},
		production.Products{production.Production{Name: "IronIngot", Rate: 1}},
		100)
	s.producers = []production.Producer{r, f}

	m := market.Match{
		Seller:        r,
		Buyer:         f,
		Order:         production.Production{Name: "OreIron", Rate: 4},
		UnitPrice:     2.0,
		UnitTransport: 1.1,
	}
	executed, err := s.executeTrade(testLogger(), m)
	if err != nil {
		t.Fatalf("executeTrade error: %v", err)
	}
	if executed != 4 {
		t.Fatalf("executed = %v, want 4", executed)
	}
	if r.Stock != 6 {
		t.Fatalf("seller stock = %v, want 6", r.Stock)
	}
	if got := f.InputStock.Get("OreIron"); got != 4 {
		t.Fatalf("buyer input stock = %v, want 4", got)
	}
	// Buyer paid (2.0 + 1.1) * 4 = 12.4
	if got := f.Wallet.Cash(); got < 87.59 || got > 87.61 {
		t.Fatalf("buyer cash = %v, want 87.6", got)
	}
	if got := f.TickInputSpend; got < 12.39 || got > 12.41 {
		t.Fatalf("TickInputSpend = %v, want 12.4", got)
	}
	if s.lastTrade["OreIron"] != 2.0 {
		t.Fatalf("lastTrade = %v, want 2.0", s.lastTrade["OreIron"])
	}
	if len(s.ledger.trades) != 1 || s.ledger.trades[0].qty != 4 {
		t.Fatalf("ledger = %+v, want one trade of qty 4", s.ledger.trades)
	}
	if len(f.RecentTrades) != 1 || f.RecentTrades[0].Other != r.Location() {
		t.Fatalf("buyer trade memory = %+v, want seller location recorded", f.RecentTrades)
	}
}

func Test_executeTrade_budgetClamp(t *testing.T) {
	s := newTestState()
	r := &resources.Resource{
		Production: production.Production{Name: "OreIron", Rate: 1},
		Loc:        point.Point{X: 0, Y: 0},
		Stock:      10,
	}
	f := factory.New("Smelter", "Recipe_IngotIron_C", point.Point{X: 10000, Y: 0}, 0,
		production.Products{production.Production{Name: "OreIron", Rate: 1}},
		production.Products{production.Production{Name: "IronIngot", Rate: 1}},
		6.2) // can only afford 2 units at 3.1 delivered
	s.producers = []production.Producer{r, f}

	m := market.Match{
		Seller:        r,
		Buyer:         f,
		Order:         production.Production{Name: "OreIron", Rate: 10},
		UnitPrice:     2.0,
		UnitTransport: 1.1,
	}
	executed, err := s.executeTrade(testLogger(), m)
	if err != nil {
		t.Fatalf("executeTrade error: %v", err)
	}
	if executed < 1.99 || executed > 2.01 {
		t.Fatalf("executed = %v, want 2 (wallet clamp)", executed)
	}
	if got := f.Wallet.Cash(); got < -0.01 || got > 0.01 {
		t.Fatalf("buyer cash = %v, want ~0 (never negative from a purchase)", got)
	}
}
