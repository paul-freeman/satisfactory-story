package state

import (
	"log/slog"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/production"
)

// upkeepPerTick is the fixed cost every factory pays per tick just for
// existing -- the clock on failure for each individual wallet. Since
// Phase 6 it is no longer a macro-level money drain: applySolvency
// collects it into the treasury as rent (funding future seed capital)
// rather than burning it. The remaining macro drains are the transport
// share of trades, purchases from wallet-less resource nodes, and
// negative culled residuals.
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
		// Rent: the upkeep the factory just paid is collected into the
		// treasury rather than burned. The factory's wallet change above
		// is identical either way, so solvency dynamics are unchanged --
		// only the money's destination moves, funding future seed capital.
		s.treasury += upkeepPerTick

		if f.Wallet.InsolventFor(insolvencyGrace) {
			l.Debug("removing bankrupt factory",
				slog.String("factory", f.String()),
				slog.Float64("cash", f.Wallet.Cash()))
			// Recycle any positive residual cash back into the treasury.
			// Dormant today: the only cull path is InsolventFor, so a
			// culled factory's cash is always negative and this never
			// fires. Kept as correct, defensive accounting for a future
			// phase that might cull profitable-but-idle factories.
			if cash := f.Wallet.Cash(); cash > 0 {
				s.treasury += cash
			}
			continue // not kept: the factory and its stock vanish
		}

		survivors = append(survivors, f)
	}
	s.producers = survivors
}
