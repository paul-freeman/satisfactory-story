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

func Test_adjustPrices_bidCappedByWallet(t *testing.T) {
	s := newTestState()
	f := factory.New("Plates", "Recipe_Plates_C", point.Point{X: 0, Y: 0}, 0,
		production.Products{production.Production{Name: "IronIngot", Rate: 1}},
		production.Products{production.Production{Name: "IronPlate", Rate: 2}},
		100)
	s.producers = []production.Producer{f}
	// Hunger = rate 1 * inputStockTargetTicks 60 - stock 0 = 60.
	// Cap = Cash/Hunger = 100/60.
	cap := 100.0 / 60.0

	// Escalation would overshoot the cap: the bid lands exactly on it.
	f.SetBidPrice("IronIngot", 1.65) // 1.65 * 1.02 = 1.683 > cap
	s.book.Clear()
	s.book.PostBid(f, "IronIngot", 5, 1.65)
	s.adjustPrices(testLogger())
	if got := f.BidPriceFor("IronIngot"); got != cap {
		t.Fatalf("over-cap escalated bid = %v, want exactly cap %v", got, cap)
	}
}

func Test_adjustPrices_bidPulledDownToWalletCap(t *testing.T) {
	// A bid already far above the cap (e.g. the wallet drained since the
	// price was set) is pulled DOWN to the cap, not just stopped from
	// rising: dying demand fades honestly.
	s := newTestState()
	f := factory.New("Plates", "Recipe_Plates_C", point.Point{X: 0, Y: 0}, 0,
		production.Products{production.Production{Name: "IronIngot", Rate: 1}},
		production.Products{production.Production{Name: "IronPlate", Rate: 2}},
		100)
	s.producers = []production.Producer{f}
	cap := 100.0 / 60.0

	f.SetBidPrice("IronIngot", 5.0) // way above cap
	s.book.Clear()
	s.book.PostBid(f, "IronIngot", 5, 5.0)
	s.adjustPrices(testLogger())
	if got := f.BidPriceFor("IronIngot"); got != cap {
		t.Fatalf("above-cap bid = %v, want pulled down to cap %v", got, cap)
	}
}

func Test_adjustPrices_negativeCashBidFloorsAtMinAndRecovers(t *testing.T) {
	// A wallet driven negative by upkeep during the insolvency grace has a
	// negative Cash/Hunger cap. The floor must pull the bid to MinUnitPrice
	// -- NOT a negative price, which multiplicative escalation could never
	// climb back out of -- and once cash recovers the bid re-escalates.
	s := newTestState()
	f := factory.New("Plates", "Recipe_Plates_C", point.Point{X: 0, Y: 0}, 0,
		production.Products{production.Production{Name: "IronIngot", Rate: 1}},
		production.Products{production.Production{Name: "IronPlate", Rate: 2}},
		-50) // negative wallet: Cash/Hunger = -50/60 < 0
	s.producers = []production.Producer{f}

	f.SetBidPrice("IronIngot", 1.0)
	s.book.Clear()
	s.book.PostBid(f, "IronIngot", 5, 1.0)
	s.adjustPrices(testLogger())
	if got := f.BidPriceFor("IronIngot"); got != production.MinUnitPrice {
		t.Fatalf("negative-cash bid = %v, want floored at MinUnitPrice %v (not negative)",
			got, production.MinUnitPrice)
	}

	// Wallet recovers: the floored bid can now escalate back above the
	// floor -- the lock-out is gone.
	f.Wallet.Adjust(150) // balance now 100 -> cap = 100/60 ~ 1.667
	s.book.Clear()
	s.book.PostBid(f, "IronIngot", 5, production.MinUnitPrice)
	s.adjustPrices(testLogger())
	if got := f.BidPriceFor("IronIngot"); got != production.MinUnitPrice*(1+bidRaisePct) {
		t.Fatalf("recovered bid = %v, want re-escalated %v", got, production.MinUnitPrice*(1+bidRaisePct))
	}
}

func Test_adjustPrices_zeroHungerBidEscalatesUncapped(t *testing.T) {
	// A partially-filled bid can still be in the book when input stock is
	// already at target (hunger ~0). The cap quotient would be Cash/0:
	// the clamp must be skipped entirely (spec: no cap in this case), and
	// must not produce NaN even with an empty wallet.
	s := newTestState()
	f := factory.New("Plates", "Recipe_Plates_C", point.Point{X: 0, Y: 0}, 0,
		production.Products{production.Production{Name: "IronIngot", Rate: 1}},
		production.Products{production.Production{Name: "IronPlate", Rate: 2}},
		0) // empty wallet: Cash/Hunger would be 0/0 = NaN
	s.producers = []production.Producer{f}
	f.InputStock["IronIngot"] = 60 // rate 1 * target 60 => hunger 0
	f.SetBidPrice("IronIngot", 1.0)
	s.book.Clear()
	s.book.PostBid(f, "IronIngot", 5, 1.0)
	s.adjustPrices(testLogger())
	if got := f.BidPriceFor("IronIngot"); got != 1.0*(1+bidRaisePct) {
		t.Fatalf("zero-hunger bid = %v, want uncapped escalation %v", got, 1.0*(1+bidRaisePct))
	}
}
