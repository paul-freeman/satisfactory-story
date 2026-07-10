package state

import (
	"log/slog"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/production"
)

// insolvencyGrace is how many consecutive ticks a factory's wallet may sit
// below zero before it is considered bankrupt and removed. A new factory
// signs no output/sale contract at spawn time -- it survives purely on the
// hope that some future spawn attempt discovers it and buys its output --
// so this needs to be long enough to plausibly outlast that wait, not just
// to ride out a rough patch (e.g. while renegotiating a cheaper supplier
// contract). With spawnProbabilityPerTick at 0.05 and roughly 139 competing
// active recipes sharing the draw, the expected wait for a matching buyer
// to even be attempted is on the order of thousands of ticks, so the grace
// window is set well above the ~200 ticks that only covers a short rough
// patch.
//
// Empirically (see task-11-fix3-report.md and the task-11 close-out
// report), raising this value from 200 steadily increases both a factory's
// median survival time and the number of factories alive concurrently at
// any given tick: ~2 concurrent at the old 200-tick grace, 7-30 concurrent
// at 10,000, 58-76 concurrent at 50,000. 8,000 was tried and failed to
// reliably pass the long-run test; 10,000 is the smallest value confirmed
// to reliably keep factories alive long enough to be discovered by a
// future spawn attempt and trade with each other (verified via the
// long-run test in state_test.go) -- it favors a more observable,
// "lived-in" cadence of bankruptcy-driven turnover in the live simulation
// over chasing the highest survival numbers. It does NOT
// get a producer all the way to a SpaceElevatorPart_* product at any value
// tried -- that requires a 4-5 tier recipe chain with several rare
// intermediate producers alive at once, and their rarity is governed by
// the spawn/shortage draw weighting (spawn.go/shortage.go), not by how
// long a spawned factory is allowed to wait. That gap is left for future
// design work; this constant is calibrated only against the achievable
// property of real factory-to-factory trade.
const insolvencyGrace = 10000

// applySolvency applies this tick's revenue and expenses to every
// factory's wallet (via Profit, which also prunes cancelled contracts) and
// removes any factory that is now missing a required input contract or has
// been insolvent for longer than insolvencyGrace.
func (s *State) applySolvency(l *slog.Logger) {
	survivors := make([]production.Producer, 0, len(s.producers))
	for _, p := range s.producers {
		f, ok := p.(*factory.Factory)
		if !ok {
			survivors = append(survivors, p)
			continue
		}

		f.Wallet.Apply(f.Profit())

		if len(f.Purchases) != len(f.Input) {
			l.Debug("removing factory with incomplete input contracts", slog.String("factory", f.String()))
			f.Remove()
			continue
		}
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
