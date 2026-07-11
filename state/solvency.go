package state

import (
	"log/slog"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/production"
)

// upkeepPerTick is the fixed cost every factory pays per tick just for
// existing. It is the clock on failure: an idle factory whose input bids
// never fill bleeds seed capital at this rate until bankruptcy. It also
// enters every factory's marginal cost, so nobody's ask price can settle
// below what existing actually costs.
const upkeepPerTick = 0.5

// insolvencyGrace is how many consecutive ticks a factory's wallet may
// sit below zero before it is removed as bankrupt. Unlike the old
// pre-order-book economy (which needed a 10,000-tick grace because a new
// factory could only be discovered by a lucky future spawn), discovery
// is now continuous through the book, so this only needs to cover a
// legible rough patch: a lost supplier, a renegotiation gap, a temporary
// glut.
const insolvencyGrace = 300

// floorUnitPrice is the salvage value of one unsold unit of output:
// thematically, every factory feeds its leftovers to its own on-site
// AWESOME sink. It must stay well below any realistic traded price so
// real trade always beats it; it exists so a producing factory that
// hasn't found buyers yet earns *something* while the market discovers
// it. It is also the guaranteed revenue floor used when estimating a
// prospective recipe's profit (spawn.go) and a factory's achievable
// revenue (prices.go).
const floorUnitPrice = 0.1

// applySolvency runs each factory's tick economics: an idle factory
// (missing input coverage) cannot honor sales, so its sale contracts are
// cancelled; then revenue and expenses (via Profit, which also prunes
// cancelled contracts), salvage on unsold capacity, and upkeep hit the
// wallet; factories insolvent beyond the grace window are removed. Being
// idle is a normal life stage -- a factory waiting for its input bids to
// fill is NOT culled.
func (s *State) applySolvency(l *slog.Logger) {
	survivors := make([]production.Producer, 0, len(s.producers))
	for _, p := range s.producers {
		f, ok := p.(*factory.Factory)
		if !ok {
			survivors = append(survivors, p)
			continue
		}

		salvage := 0.0
		if f.Producing() {
			for _, output := range f.Output {
				salvage += f.RemainingCapacityFor(output.Name) * floorUnitPrice
			}
		} else {
			for _, sale := range f.Sales {
				sale.Cancel()
			}
		}

		f.Wallet.Apply(f.Profit() + salvage - upkeepPerTick)

		if f.Wallet.InsolventFor(insolvencyGrace) {
			l.Debug("removing bankrupt factory",
				slog.String("factory", f.String()),
				slog.Float64("cash", f.Wallet.Cash()))
			f.Remove()
			continue
		}

		survivors = append(survivors, f)
	}
	s.producers = survivors
}
