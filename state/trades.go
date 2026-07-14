package state

import (
	"github.com/paul-freeman/satisfactory-story/production"
)

// tradeMemoryTicks is the rolling window for the trade ledger and the
// factories' own trade memories: it feeds the wire transport links,
// lastTrade prices, and movement gradients. A milestone tuning knob.
const tradeMemoryTicks = 500

// trade is one executed spot trade.
type trade struct {
	tick      int
	seller    production.Producer
	buyer     production.Producer
	product   string
	qty       float64
	unitPrice float64
}

// tradeLedger is the rolling record of recent trades.
type tradeLedger struct {
	trades []trade
}

func (tl *tradeLedger) record(tick int, seller, buyer production.Producer, product string, qty, unitPrice float64) {
	tl.trades = append(tl.trades, trade{
		tick: tick, seller: seller, buyer: buyer,
		product: product, qty: qty, unitPrice: unitPrice,
	})
}

// prune drops trades older than memoryTicks.
func (tl *tradeLedger) prune(tick, memoryTicks int) {
	kept := tl.trades[:0]
	for _, tr := range tl.trades {
		if tick-tr.tick <= memoryTicks {
			kept = append(kept, tr)
		}
	}
	tl.trades = kept
}

// tradeEdge is an aggregated seller->buyer flow over the window.
type tradeEdge struct {
	seller production.Producer
	buyer  production.Producer
	qty    float64
}

// edges aggregates the ledger by (seller, buyer) pair, in first-seen
// order so output is deterministic.
func (tl *tradeLedger) edges() []tradeEdge {
	type pair struct{ s, b production.Producer }
	index := make(map[pair]int)
	edges := make([]tradeEdge, 0)
	for _, tr := range tl.trades {
		key := pair{tr.seller, tr.buyer}
		if i, ok := index[key]; ok {
			edges[i].qty += tr.qty
			continue
		}
		index[key] = len(edges)
		edges = append(edges, tradeEdge{seller: tr.seller, buyer: tr.buyer, qty: tr.qty})
	}
	return edges
}

// recentSellers is the set of producers that sold anything within the
// window (used for the wire "active" flag on resources).
func (tl *tradeLedger) recentSellers() map[production.Producer]bool {
	sellers := make(map[production.Producer]bool)
	for _, tr := range tl.trades {
		sellers[tr.seller] = true
	}
	return sellers
}
