package market

import (
	"math"
	"sort"

	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
)

// Match is a crossed bid/ask pair ready to execute as a one-shot trade.
// The trade executes at the ask's unit price; the buyer additionally
// pays UnitTransport per unit of freight.
type Match struct {
	Seller        production.Producer
	Buyer         production.Producer
	Order         production.Production // Rate = candidate quantity (units)
	UnitPrice     float64
	UnitTransport float64
}

// MatchAll crosses bids and asks product by product and calls execute
// for each match. Bids are served in descending price order (ties by
// posting order); each bid takes the ask with the lowest per-unit
// delivered cost (ask price plus per-unit transport) it can cross.
// execute returns the quantity actually traded (the state layer clamps
// by seller stock and buyer budget): 0 or an error skips that ask for
// this bid; a partial execution ends this bid's shopping (its budget is
// exhausted).
func (b *Book) MatchAll(unitTransport func(origin, destination point.Point) float64, execute func(Match) (float64, error)) {
	for _, product := range b.Products() {
		bids := make([]*Bid, len(b.bids[product]))
		copy(bids, b.bids[product])
		sort.SliceStable(bids, func(i, j int) bool {
			return bids[i].UnitPrice > bids[j].UnitPrice
		})
		for _, bid := range bids {
			skipped := make(map[*Ask]bool)
			for bid.Remaining > production.RateEpsilon {
				ask, qty, unitCost := b.bestDeliveredAsk(product, bid, skipped, unitTransport)
				if ask == nil || bid.UnitPrice < unitCost {
					break
				}
				m := Match{
					Seller:        ask.Seller,
					Buyer:         bid.Buyer,
					Order:         production.Production{Name: product, Rate: qty},
					UnitPrice:     ask.UnitPrice,
					UnitTransport: unitTransport(ask.Seller.Location(), bid.Buyer.Location()),
				}
				executed, err := execute(m)
				if err != nil || executed <= production.RateEpsilon {
					skipped[ask] = true
					continue
				}
				ask.Remaining -= executed
				bid.Remaining -= executed
				if executed < qty-production.RateEpsilon {
					// Partial fill: the buyer ran out of money; further
					// asks are unaffordable too this tick.
					break
				}
			}
		}
	}
}

// bestDeliveredAsk returns the live ask with the lowest per-unit
// delivered cost for this bid (nil if none remains), along with the
// candidate quantity and that per-unit cost.
func (b *Book) bestDeliveredAsk(
	product string,
	bid *Bid,
	skipped map[*Ask]bool,
	unitTransport func(point.Point, point.Point) float64,
) (*Ask, float64, float64) {
	var best *Ask
	var bestQty, bestCost float64
	for _, ask := range b.asks[product] {
		if skipped[ask] || ask.Remaining <= production.RateEpsilon || ask.Seller == bid.Buyer {
			continue
		}
		qty := math.Min(bid.Remaining, ask.Remaining)
		unitCost := ask.UnitPrice + unitTransport(ask.Seller.Location(), bid.Buyer.Location())
		if best == nil || unitCost < bestCost {
			best, bestQty, bestCost = ask, qty, unitCost
		}
	}
	return best, bestQty, bestCost
}
