package state

import (
	"log/slog"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/production"
)

// insolvencyGrace is how many consecutive ticks a factory's wallet may sit
// below zero before it is considered bankrupt and removed. This lets a
// factory ride out a rough patch (e.g. while renegotiating a cheaper
// supplier contract) instead of dying on the first bad tick.
const insolvencyGrace = 200

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
