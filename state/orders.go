package state

import (
	"fmt"
	"log/slog"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/market"
	"github.com/paul-freeman/satisfactory-story/production"
	"github.com/paul-freeman/satisfactory-story/recipes"
	"github.com/paul-freeman/satisfactory-story/resources"
	"github.com/paul-freeman/satisfactory-story/sink"
)

// publishOrders rebuilds the book from live producer state: every unsold
// unit of capacity becomes an ask, every unmet unit of demand becomes a
// bid. Zero-rate orders are dropped by the book itself. Only prices
// persist between ticks (on the producers); quantities can never go
// stale because they are re-derived here every tick.
func (s *State) publishOrders(_ *slog.Logger) {
	s.book.Clear()
	for _, p := range s.producers {
		switch producer := p.(type) {
		case *resources.Resource:
			name := producer.Production.Name
			s.book.PostAsk(producer, name,
				producer.RemainingCapacityFor(name), producer.AskPriceFor(name))
		case *factory.Factory:
			// An idle factory sells nothing -- its capacity is
			// hypothetical until its inputs are contracted.
			if producer.Producing() {
				for _, output := range producer.Output {
					s.book.PostAsk(producer, output.Name,
						producer.RemainingCapacityFor(output.Name),
						producer.AskPriceFor(output.Name))
				}
			}
			for _, input := range producer.Input {
				s.book.PostBid(producer, input.Name,
					producer.UnmetInputRate(input.Name),
					producer.BidPriceFor(input.Name))
			}
		case *sink.Sink:
			for _, want := range producer.Input {
				s.book.PostBid(producer, want.Name, sinkDemandRate, producer.BidUnitPrice)
			}
		}
	}
}

// matchOrders crosses the book and signs a contract for every match.
func (s *State) matchOrders(l *slog.Logger) {
	s.book.MatchAll(recipes.UnitTransportCost, func(m market.Match) (float64, error) {
		if err := s.signContract(l, m); err != nil {
			return 0, err
		}
		return m.Order.Rate, nil
	})
}

// signContract turns a match into a signed production.Contract. It is
// the only place matched trades become real: capacity is re-checked
// against live state (the book's Remaining is bookkeeping, not
// authority), both parties sign, and the traded unit price is recorded.
func (s *State) signContract(l *slog.Logger, m market.Match) error {
	if err := m.Seller.HasCapacityFor(m.Order); err != nil {
		return fmt.Errorf("cannot sign contract: %w", err)
	}
	contract := &production.Contract{
		Seller:        m.Seller,
		Buyer:         m.Buyer,
		Order:         m.Order,
		TransportCost: m.UnitTransport * m.Order.Rate,
		ProductCost:   m.UnitPrice * m.Order.Rate,
	}
	if err := m.Seller.SignAsSeller(contract); err != nil {
		contract.Cancel()
		return fmt.Errorf("seller rejected contract: %w", err)
	}
	if err := m.Buyer.SignAsBuyer(contract); err != nil {
		contract.Cancel()
		return fmt.Errorf("buyer rejected contract: %w", err)
	}
	s.lastTrade[m.Order.Name] = m.UnitPrice
	l.Debug("signed contract",
		slog.String("order", m.Order.Key()),
		slog.Float64("rate", m.Order.Rate),
		slog.Float64("unitPrice", m.UnitPrice),
		slog.Float64("transportCost", m.UnitTransport*m.Order.Rate),
	)
	return nil
}
