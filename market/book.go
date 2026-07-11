// Package market implements the order book through which producers
// discover each other: sellers post asks (unsold capacity at a price),
// buyers post bids (unmet demand at a price), and a per-tick matching
// pass crosses them into contracts. The book is rebuilt from live
// producer state every tick -- only the prices carried by the producers
// themselves persist between ticks.
package market

import (
	"sort"

	"github.com/paul-freeman/satisfactory-story/production"
)

// Ask is a standing offer to sell Remaining units/sec of Product at
// UnitPrice money per unit.
type Ask struct {
	Seller    production.Producer
	Product   string
	Remaining float64
	UnitPrice float64
}

// Bid is a standing offer to buy Remaining units/sec of Product at up to
// UnitPrice money per unit.
type Bid struct {
	Buyer     production.Producer
	Product   string
	Remaining float64
	UnitPrice float64
}

// Book holds the standing asks and bids for every product. Orders are
// kept in posting order; queries that need price order pick minima with
// strict comparison, so ties resolve by posting order and results are
// deterministic.
type Book struct {
	asks map[string][]*Ask
	bids map[string][]*Bid
}

func NewBook() *Book {
	return &Book{
		asks: make(map[string][]*Ask),
		bids: make(map[string][]*Bid),
	}
}

// Clear drops every order. Called at the top of each tick before
// producers republish from live state.
func (b *Book) Clear() {
	b.asks = make(map[string][]*Ask)
	b.bids = make(map[string][]*Bid)
}

func (b *Book) PostAsk(seller production.Producer, product string, rate, unitPrice float64) {
	if rate <= production.RateEpsilon {
		return
	}
	b.asks[product] = append(b.asks[product],
		&Ask{Seller: seller, Product: product, Remaining: rate, UnitPrice: unitPrice})
}

func (b *Book) PostBid(buyer production.Producer, product string, rate, unitPrice float64) {
	if rate <= production.RateEpsilon {
		return
	}
	b.bids[product] = append(b.bids[product],
		&Bid{Buyer: buyer, Product: product, Remaining: rate, UnitPrice: unitPrice})
}

// Asks returns the asks for product in posting order.
func (b *Book) Asks(product string) []*Ask { return b.asks[product] }

// Bids returns the bids for product in posting order.
func (b *Book) Bids(product string) []*Bid { return b.bids[product] }

// BestAsk returns the unfilled ask with the lowest unit price.
func (b *Book) BestAsk(product string) (*Ask, bool) {
	var best *Ask
	for _, ask := range b.asks[product] {
		if ask.Remaining <= production.RateEpsilon {
			continue
		}
		if best == nil || ask.UnitPrice < best.UnitPrice {
			best = ask
		}
	}
	return best, best != nil
}

// BestBid returns the unfilled bid with the highest unit price.
func (b *Book) BestBid(product string) (*Bid, bool) {
	var best *Bid
	for _, bid := range b.bids[product] {
		if bid.Remaining <= production.RateEpsilon {
			continue
		}
		if best == nil || bid.UnitPrice > best.UnitPrice {
			best = bid
		}
	}
	return best, best != nil
}

// Products returns every product with at least one order, sorted, so
// callers can iterate the book deterministically.
func (b *Book) Products() []string {
	seen := make(map[string]bool)
	products := make([]string, 0, len(b.asks)+len(b.bids))
	for product := range b.asks {
		if !seen[product] {
			seen[product] = true
			products = append(products, product)
		}
	}
	for product := range b.bids {
		if !seen[product] {
			seen[product] = true
			products = append(products, product)
		}
	}
	sort.Strings(products)
	return products
}
