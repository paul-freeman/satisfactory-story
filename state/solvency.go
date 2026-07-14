package state

import (
	"log/slog"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/production"
)

// upkeepPerTick is the fixed cost every factory pays per tick just for
// existing. It is the clock on failure and the drain that balances the
// sinks' money faucet.
const upkeepPerTick = 0.5

// insolvencyGrace is how many consecutive ticks a factory's wallet may
// sit below zero before it is removed as bankrupt. Purchases can never
// overdraw a wallet (budget clamp at trade time); only upkeep drags a
// wallet negative, so this is a pure staying-power window.
const insolvencyGrace = 300

// floorUnitPrice is the salvage value of one unsold unit: every factory
// feeds overflow to its own on-site AWESOME sink. Well below any
// realistic traded price so real trade always dominates.
const floorUnitPrice = 0.1

// salvageTrickleFraction is how much of one tick's output rate a factory
// with a FULL output buffer may salvage per tick. Deliberately less than
// 1: a buyer-less factory keeps producing at only this fraction of its
// rate, so reduced input buying still propagates the no-demand signal
// upstream. A milestone tuning knob.
const salvageTrickleFraction = 0.25

// inputSpendSmoothing is the EMA weight for folding per-tick trade flows
// into AvgInputSpend/AvgRevenue.
const inputSpendSmoothing = 0.05

// applySolvency runs each factory's tick economics: salvage trickle on
// capped outputs, fold trade flows into the EMAs, apply upkeep, remove
// the persistently insolvent. Trade money itself already moved at trade
// time (executeTrade); this is the once-per-tick accounting call.
func (s *State) applySolvency(l *slog.Logger) {
	survivors := make([]production.Producer, 0, len(s.producers))
	for _, p := range s.producers {
		f, ok := p.(*factory.Factory)
		if !ok {
			survivors = append(survivors, p)
			continue
		}

		salvage := 0.0
		for _, output := range f.Output {
			cap := output.Rate * outputStockCapTicks
			if f.OutputStock.Get(output.Name) >= cap-production.RateEpsilon {
				qty := f.OutputStock.Take(output.Name, output.Rate*salvageTrickleFraction)
				salvage += qty * floorUnitPrice
			}
		}
		f.TickRevenue += salvage
		f.FoldTickFlows(inputSpendSmoothing)
		f.Wallet.Apply(salvage - upkeepPerTick)

		if f.Wallet.InsolventFor(insolvencyGrace) {
			l.Debug("removing bankrupt factory",
				slog.String("factory", f.String()),
				slog.Float64("cash", f.Wallet.Cash()))
			continue // not kept: the factory and its stock vanish
		}

		survivors = append(survivors, f)
	}
	s.producers = survivors
}
