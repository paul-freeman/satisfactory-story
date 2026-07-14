package state

import (
	"testing"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
)

func Test_adjustPrices_askStockSignals(t *testing.T) {
	s := newTestState()
	f := factory.New("Plates", "Recipe_Plates_C", point.Point{X: 0, Y: 0}, 0,
		production.Products{production.Production{Name: "IronIngot", Rate: 1}},
		production.Products{production.Production{Name: "IronPlate", Rate: 2}},
		100)
	s.producers = []production.Producer{f}
	f.SetAskPrice("IronPlate", 10)

	// Sold out (no stock -> ask fully consumed or never posted): the
	// remaining=0 ask means scarcity -> raise.
	s.book.Clear()
	s.book.PostAsk(f, "IronPlate", 5, 10)
	s.book.Asks("IronPlate")[0].Remaining = 0
	s.adjustPrices(testLogger())
	if got := f.AskPriceFor("IronPlate"); got != 10*(1+askRaisePct) {
		t.Fatalf("sold-out ask = %v, want %v", got, 10*(1+askRaisePct))
	}

	// Unsold stock: decay toward the stock marginal cost floor.
	f.SetAskPrice("IronPlate", 10)
	f.AvgInputSpend = 0 // floor = upkeep/2 = 0.25
	s.book.Clear()
	s.book.PostAsk(f, "IronPlate", 5, 10)
	s.adjustPrices(testLogger())
	if got := f.AskPriceFor("IronPlate"); got != 10*(1-askLowerPct) {
		t.Fatalf("unsold ask = %v, want %v", got, 10*(1-askLowerPct))
	}
}

func Test_adjustPrices_bidRaisesWhileHungry(t *testing.T) {
	s := newTestState()
	f := factory.New("Plates", "Recipe_Plates_C", point.Point{X: 0, Y: 0}, 0,
		production.Products{production.Production{Name: "IronIngot", Rate: 1}},
		production.Products{production.Production{Name: "IronPlate", Rate: 2}},
		100)
	s.producers = []production.Producer{f}
	f.SetBidPrice("IronIngot", 1.0)

	s.book.Clear()
	s.book.PostBid(f, "IronIngot", 5, 1.0) // unfilled hunger
	s.adjustPrices(testLogger())
	if got := f.BidPriceFor("IronIngot"); got != 1.0*(1+bidRaisePct) {
		t.Fatalf("hungry bid = %v, want %v (no affordability gate anymore)", got, 1.0*(1+bidRaisePct))
	}

	// Fully filled bid: no raise.
	f.SetBidPrice("IronIngot", 1.0)
	s.book.Clear()
	s.book.PostBid(f, "IronIngot", 5, 1.0)
	s.book.Bids("IronIngot")[0].Remaining = 0
	s.adjustPrices(testLogger())
	if got := f.BidPriceFor("IronIngot"); got != 1.0 {
		t.Fatalf("filled bid = %v, want 1.0 (unchanged)", got)
	}
}
