package state

import (
	"log/slog"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/market"
	"github.com/paul-freeman/satisfactory-story/production"
	"github.com/paul-freeman/satisfactory-story/recipes"
)

// renegotiateProbabilityPerTick is the chance, per factory per tick,
// that it bothers re-shopping its input contracts at all -- doing this
// for every factory every tick would be wasted search effort once the
// market has settled.
const renegotiateProbabilityPerTick = 0.02

// renegotiationMinMargin is how much cheaper (as a fraction of the
// current total price) a new offer must be before a factory switches
// suppliers. This avoids thrashing between two near-identical prices.
const renegotiationMinMargin = 0.05

// renegotiateContracts lets each factory re-shop each input contract
// against the book's post-matching residual asks, switching supplier if
// one beats the current deal by more than renegotiationMinMargin. This
// is what lets a newly spawned, better-positioned or cheaper producer
// steal business from an incumbent instead of only ever competing for
// brand-new demand.
func (s *State) renegotiateContracts(l *slog.Logger) {
	for _, p := range s.producers {
		f, ok := p.(*factory.Factory)
		if !ok || s.randSrc.Float64() >= renegotiateProbabilityPerTick {
			continue
		}

		current := make([]*production.Contract, len(f.Purchases))
		copy(current, f.Purchases)
		for _, purchase := range current {
			if purchase.Cancelled {
				continue
			}
			ask, transportCost, found := s.bestAlternativeAsk(f, purchase)
			if !found {
				continue
			}
			currentTotal := purchase.ProductCost + purchase.TransportCost
			candidateTotal := ask.UnitPrice*purchase.Order.Rate + transportCost
			if candidateTotal >= currentTotal*(1-renegotiationMinMargin) {
				continue
			}

			// Sign the replacement before cancelling the incumbent: if
			// signContract fails (e.g. capacity raced away), the factory
			// keeps its existing supplier rather than being left idle.
			m := market.Match{
				Seller:        ask.Seller,
				Buyer:         f,
				Order:         purchase.Order,
				UnitPrice:     ask.UnitPrice,
				TransportCost: transportCost,
			}
			if err := s.signContract(l, m); err != nil {
				l.Debug("failed to renegotiate contract", slog.String("error", err.Error()))
				continue
			}
			ask.Remaining -= purchase.Order.Rate
			purchase.Cancel()
			l.Debug("renegotiated contract",
				slog.String("factory", f.String()),
				slog.String("product", purchase.Order.Name))
		}
	}
}

// bestAlternativeAsk finds the residual ask with the lowest delivered
// total (product cost plus transport) that could fully replace the given
// purchase. The incumbent seller and the factory itself are excluded.
func (s *State) bestAlternativeAsk(f *factory.Factory, purchase *production.Contract) (*market.Ask, float64, bool) {
	var best *market.Ask
	var bestTransport, bestTotal float64
	for _, ask := range s.book.Asks(purchase.Order.Name) {
		if ask.Seller == purchase.Seller || ask.Seller == production.Producer(f) {
			continue
		}
		if ask.Remaining < purchase.Order.Rate {
			continue
		}
		transportCost := recipes.TransportCost(ask.Seller.Location(), f.Location())
		total := ask.UnitPrice*purchase.Order.Rate + transportCost
		if best == nil || total < bestTotal {
			best, bestTransport, bestTotal = ask, transportCost, total
		}
	}
	return best, bestTransport, best != nil
}
