package state

import (
	"log/slog"
	"math"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/production"
	"github.com/paul-freeman/satisfactory-story/resources"
)

// askRaisePct is how much a seller raises a product's ask price after
// selling out its stock, and askLowerPct how much it lowers while stock
// goes unsold. Raising faster than lowering damps oscillation.
const askRaisePct = 0.05
const askLowerPct = 0.02

// bidRaisePct is how much a buyer escalates an unfilled input bid per
// tick. The escalating bid is the backward demand cascade. Escalation is
// clamped in adjustPrices to Cash/Hunger -- the wallet-grounded cap --
// so every posted price is backed by money the buyer actually has and
// dead-end demand can never compound into absurd book prices.
const bidRaisePct = 0.02

// adjustPrices lets every producer react locally to this tick's fill
// outcomes. No market-wide price is computed anywhere.
func (s *State) adjustPrices(_ *slog.Logger) {
	for _, product := range s.book.Products() {
		for _, ask := range s.book.Asks(product) {
			soldOut := ask.Remaining <= production.RateEpsilon
			switch seller := ask.Seller.(type) {
			case *factory.Factory:
				if soldOut {
					seller.SetAskPrice(product, seller.AskPriceFor(product)*(1+askRaisePct))
				} else {
					floor := seller.StockMarginalUnitCost(upkeepPerTick)
					seller.SetAskPrice(product,
						math.Max(floor, seller.AskPriceFor(product)*(1-askLowerPct)))
				}
			case *resources.Resource:
				if soldOut {
					seller.SetAskPrice(product, seller.AskPriceFor(product)*(1+askRaisePct))
				} else {
					seller.SetAskPrice(product,
						math.Max(production.MinUnitPrice, seller.AskPriceFor(product)*(1-askLowerPct)))
				}
			}
		}

		for _, bid := range s.book.Bids(product) {
			if bid.Remaining <= production.RateEpsilon {
				continue
			}
			buyer, ok := bid.Buyer.(*factory.Factory)
			if !ok {
				continue // sink bids are fixed
			}
			escalated := buyer.BidPriceFor(product) * (1 + bidRaisePct)
			// Wallet-grounded cap: a standing bid never promises more per
			// unit than the wallet could pay for the full hunger. It
			// applies even when it lowers the current price, so dying
			// demand fades honestly instead of screaming louder. Near-zero
			// hunger (a bid that just got mostly filled) means no cap this
			// tick -- guarded explicitly so Cash/0 can't inject Inf or NaN.
			//
			// The cap is floored at MinUnitPrice so a transiently-insolvent
			// wallet (upkeep drove Cash negative during the insolvency
			// grace) fades its bid down to the floor instead of a NEGATIVE
			// price. A negative bid could never climb back through the
			// multiplicative escalation (neg*1.02 stays negative), which
			// would permanently lock out a factory that later regains cash;
			// flooring at MinUnitPrice keeps the bid non-matching but able
			// to re-escalate once the wallet recovers.
			if hunger := buyer.Hunger(product, inputStockTargetTicks); hunger > production.RateEpsilon {
				cap := math.Max(production.MinUnitPrice, buyer.Cash()/hunger)
				escalated = math.Min(escalated, cap)
			}
			buyer.SetBidPrice(product, escalated)
		}
	}
}
