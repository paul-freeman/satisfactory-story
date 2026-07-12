package state

import (
	"log/slog"
	"math"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/production"
	"github.com/paul-freeman/satisfactory-story/resources"
)

// askRaisePct is how much a seller raises a product's ask price after
// selling out, and askLowerPct how much it lowers after failing to sell.
// Raising faster than lowering lets prices spike quickly under scarcity
// and relax slowly, which damps oscillation between the two states.
const askRaisePct = 0.05
const askLowerPct = 0.02

// bidRaisePct is how much a buyer escalates an unfilled input bid per
// tick. The escalating bid is the backward demand cascade -- it is
// deliberately gentle so a factory's seed capital comfortably outlasts
// the climb to a market-clearing price.
const bidRaisePct = 0.02

// affordabilityMargin caps a factory's total planned input spend at this
// fraction of its achievable revenue, so escalation stops where the
// business stops making sense rather than at bankruptcy.
const affordabilityMargin = 0.8

// adjustPrices runs after matching and lets every producer react locally
// to this tick's fill outcomes. No market-wide price is computed
// anywhere: sellers with unsold stock undercut, sold-out sellers raise,
// hungry buyers bid up as far as their own downstream demand supports.
func (s *State) adjustPrices(_ *slog.Logger) {
	for _, product := range s.book.Products() {
		for _, ask := range s.book.Asks(product) {
			soldOut := ask.Remaining <= production.RateEpsilon
			switch seller := ask.Seller.(type) {
			case *factory.Factory:
				if soldOut {
					seller.SetAskPrice(product, seller.AskPriceFor(product)*(1+askRaisePct))
				} else {
					floor := seller.MarginalUnitCost(upkeepPerTick)
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
			proposed := buyer.BidPriceFor(product) * (1 + bidRaisePct)
			if s.plannedSpend(buyer, product, proposed) <= s.achievableRevenue(buyer)*affordabilityMargin {
				buyer.SetBidPrice(product, proposed)
			}
		}
	}
}

// achievableRevenue is what the factory's output is worth per tick right
// now: revenue from active sales plus unsold capacity valued at the best
// standing bid for each output (never below the salvage floor).
// Downstream bids raising this number is exactly how demand cascades
// backward through the tiers.
func (s *State) achievableRevenue(f *factory.Factory) float64 {
	revenue := 0.0
	for _, sale := range f.Sales {
		if !sale.Cancelled {
			revenue += sale.ProductCost - sale.TransportCost
		}
	}
	for _, output := range f.Output {
		remaining := f.RemainingCapacityFor(output.Name)
		if remaining <= production.RateEpsilon {
			continue
		}
		price := floorUnitPrice
		if bid, ok := s.book.BestBid(output.Name); ok && bid.UnitPrice > price {
			price = bid.UnitPrice
		}
		revenue += price * remaining
	}
	return revenue
}

// plannedSpend is the factory's per-tick input commitment if the bid for
// product moved to proposedPrice: actual active purchases plus every
// unmet input valued at its standing (or proposed) bid price.
func (s *State) plannedSpend(f *factory.Factory, product string, proposedPrice float64) float64 {
	spend := 0.0
	for _, purchase := range f.Purchases {
		if !purchase.Cancelled {
			spend += purchase.ProductCost + purchase.TransportCost
		}
	}
	for _, input := range f.Input {
		unmet := f.UnmetInputRate(input.Name)
		if unmet <= production.RateEpsilon {
			continue
		}
		price := f.BidPriceFor(input.Name)
		if input.Name == product {
			price = proposedPrice
		}
		spend += price * unmet
	}
	return spend
}
