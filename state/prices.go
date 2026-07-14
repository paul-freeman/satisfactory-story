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
// tick. The escalating bid is the backward demand cascade. There is no
// affordability precondition anymore: the wallet clamp at trade time is
// the hard constraint, so a high bid with no money behind it buys
// nothing (this is what makes speculative price loops harmless).
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
			buyer.SetBidPrice(product, buyer.BidPriceFor(product)*(1+bidRaisePct))
		}
	}
}
