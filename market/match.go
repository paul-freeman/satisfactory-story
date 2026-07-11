package market

import (
	"math"
	"sort"

	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
)

// Match is a crossed bid/ask pair ready to become a contract. The trade
// executes at the ask's unit price; the contract's ProductCost is
// UnitPrice * Order.Rate.
type Match struct {
	Seller        production.Producer
	Buyer         production.Producer
	Order         production.Production
	UnitPrice     float64
	TransportCost float64
}

// MatchAll crosses bids and asks product by product and calls sign for
// each match. Bids are served in descending price order (ties by posting
// order); each bid takes the ask with the lowest per-unit delivered cost
// (ask price plus transport amortized over the candidate rate) until no
// ask it can afford remains. If sign returns an error the ask is skipped
// for that bid and nothing is consumed.
//
// Transport is passed in (rather than importing recipes) so tests can
// use simple geometries.
func (b *Book) MatchAll(transport func(origin, destination point.Point) float64, sign func(Match) error) {
	for _, product := range b.Products() {
		bids := make([]*Bid, len(b.bids[product]))
		copy(bids, b.bids[product])
		sort.SliceStable(bids, func(i, j int) bool {
			return bids[i].UnitPrice > bids[j].UnitPrice
		})
		for _, bid := range bids {
			skipped := make(map[*Ask]bool)
			for bid.Remaining > production.RateEpsilon {
				ask, rate, unitCost := b.bestDeliveredAsk(product, bid, skipped, transport)
				if ask == nil || bid.UnitPrice < unitCost {
					break
				}
				m := Match{
					Seller:        ask.Seller,
					Buyer:         bid.Buyer,
					Order:         production.Production{Name: product, Rate: rate},
					UnitPrice:     ask.UnitPrice,
					TransportCost: transport(ask.Seller.Location(), bid.Buyer.Location()),
				}
				if err := sign(m); err != nil {
					skipped[ask] = true
					continue
				}
				ask.Remaining -= rate
				bid.Remaining -= rate
			}
		}
	}
}

// bestDeliveredAsk returns the live ask with the lowest per-unit
// delivered cost for this bid (nil if none remains), along with the
// candidate rate and that per-unit cost.
func (b *Book) bestDeliveredAsk(
	product string,
	bid *Bid,
	skipped map[*Ask]bool,
	transport func(point.Point, point.Point) float64,
) (*Ask, float64, float64) {
	var best *Ask
	var bestRate, bestCost float64
	for _, ask := range b.asks[product] {
		if skipped[ask] || ask.Remaining <= production.RateEpsilon || ask.Seller == bid.Buyer {
			continue
		}
		rate := math.Min(bid.Remaining, ask.Remaining)
		unitCost := ask.UnitPrice + transport(ask.Seller.Location(), bid.Buyer.Location())/rate
		if best == nil || unitCost < bestCost {
			best, bestRate, bestCost = ask, rate, unitCost
		}
	}
	return best, bestRate, bestCost
}
