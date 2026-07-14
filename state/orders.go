package state

import (
	"log/slog"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/market"
	"github.com/paul-freeman/satisfactory-story/production"
	"github.com/paul-freeman/satisfactory-story/recipes"
	"github.com/paul-freeman/satisfactory-story/resources"
	"github.com/paul-freeman/satisfactory-story/sink"
)

// publishOrders rebuilds the book from live physical state: every unit
// of stock on hand becomes an ask, every unit of input hunger becomes a
// bid. Zero-quantity orders are dropped by the book itself. Only prices
// persist between ticks (on the producers); quantities can never go
// stale because they are re-derived here every tick.
func (s *State) publishOrders(_ *slog.Logger) {
	s.book.Clear()
	for _, p := range s.producers {
		switch producer := p.(type) {
		case *resources.Resource:
			name := producer.Production.Name
			s.book.PostAsk(producer, name, producer.Stock, producer.AskPriceFor(name))
		case *factory.Factory:
			for _, output := range producer.Output {
				s.book.PostAsk(producer, output.Name,
					producer.OutputStock.Get(output.Name),
					producer.AskPriceFor(output.Name))
			}
			for _, input := range producer.Input {
				s.book.PostBid(producer, input.Name,
					producer.Hunger(input.Name, inputStockTargetTicks),
					producer.BidPriceFor(input.Name))
			}
		case *sink.Sink:
			for _, want := range producer.Input {
				s.book.PostBid(producer, want.Name, sinkDemandRate, producer.BidUnitPrice)
			}
		}
	}
}

// matchOrders crosses the book and executes a spot trade per match.
func (s *State) matchOrders(l *slog.Logger) {
	s.book.MatchAll(recipes.UnitTransportCost, func(m market.Match) (float64, error) {
		return s.executeTrade(l, m)
	})
}

// executeTrade is the only place trades become real: quantity is
// re-clamped against live seller stock and the buyer's wallet, then
// units and money move immediately. Returns the executed quantity.
// A factory buyer pays (unit price + unit transport) per unit and can
// never overdraw its wallet -- this hard budget is what keeps escalated
// bid prices honest. The transport share of the payment leaves the
// economy (it is a cost, not anyone's income).
func (s *State) executeTrade(l *slog.Logger, m market.Match) (float64, error) {
	qty := m.Order.Rate

	// Clamp by what the seller physically has.
	switch seller := m.Seller.(type) {
	case *resources.Resource:
		if seller.Stock < qty {
			qty = seller.Stock
		}
	case *factory.Factory:
		if have := seller.OutputStock.Get(m.Order.Name); have < qty {
			qty = have
		}
	default:
		return 0, nil // sinks never sell
	}

	// Clamp by what the buyer can pay (sinks have infinite money).
	unitDelivered := m.UnitPrice + m.UnitTransport
	if buyer, ok := m.Buyer.(*factory.Factory); ok && unitDelivered > 0 {
		if affordable := buyer.Wallet.Cash() / unitDelivered; affordable < qty {
			qty = affordable
		}
	}
	if qty <= production.RateEpsilon {
		return 0, nil
	}

	// Move the goods.
	switch seller := m.Seller.(type) {
	case *resources.Resource:
		seller.Stock -= qty
	case *factory.Factory:
		seller.OutputStock.Take(m.Order.Name, qty)
		seller.TickRevenue += qty * m.UnitPrice
		seller.Wallet.Adjust(qty * m.UnitPrice)
		seller.RecordTrade(s.tick, m.Buyer.Location(), qty)
	}
	switch buyer := m.Buyer.(type) {
	case *factory.Factory:
		buyer.InputStock.Add(m.Order.Name, qty)
		buyer.Wallet.Adjust(-qty * unitDelivered)
		buyer.TickInputSpend += qty * unitDelivered
		buyer.RecordTrade(s.tick, m.Seller.Location(), qty)
	case *sink.Sink:
		buyer.RecordDelivery(m.Order.Name, qty)
	}

	s.lastTrade[m.Order.Name] = m.UnitPrice
	s.ledger.record(s.tick, m.Seller, m.Buyer, m.Order.Name, qty, m.UnitPrice)
	l.Debug("executed trade",
		slog.String("product", m.Order.Name),
		slog.Float64("qty", qty),
		slog.Float64("unitPrice", m.UnitPrice),
		slog.Float64("unitTransport", m.UnitTransport),
	)
	return qty, nil
}
