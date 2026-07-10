package state

import (
	"log/slog"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/production"
	"github.com/paul-freeman/satisfactory-story/recipes"
)

// renegotiateProbabilityPerTick is the chance, per factory per tick, that
// it bothers re-shopping its input contracts at all -- doing this for
// every factory every tick would be wasted search effort once the market
// has settled.
const renegotiateProbabilityPerTick = 0.02

// renegotiationMinMargin is how much cheaper (as a fraction of the current
// total price) a new offer must be before a factory switches suppliers.
// This avoids thrashing between two near-identical prices.
const renegotiationMinMargin = 0.05

// renegotiateContracts lets each factory re-shop each of its input
// contracts for a better offer, switching supplier if one is found that
// beats the current price by more than renegotiationMinMargin. This is
// what lets a newly spawned, better-positioned producer steal business
// from an incumbent instead of only ever competing for brand-new demand.
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
			candidate, found := recipes.FindBestSeller(s.producers, purchase.Order, f.Location())
			if !found || candidate.Seller == purchase.Seller {
				continue
			}

			currentTotal := purchase.ProductCost + purchase.TransportCost
			candidateTotal := candidate.Seller.SalesPriceFor(candidate.Order, candidate.TransportCost) + candidate.TransportCost
			if candidateTotal >= currentTotal*(1-renegotiationMinMargin) {
				continue
			}

			purchase.Cancel()
			if err := s.writeContract(l, candidate.Seller, f, candidate.Order, candidate.TransportCost); err != nil {
				l.Debug("failed to renegotiate contract", slog.String("error", err.Error()))
				continue
			}
			l.Debug("renegotiated contract",
				slog.String("factory", f.String()),
				slog.String("product", purchase.Order.Name))
		}
	}
}
