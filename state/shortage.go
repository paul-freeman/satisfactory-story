package state

import "math"

// shortageDecay is the fraction of a recorded shortage retained each tick.
// At 0.99 a shortage's contribution roughly halves every ~70 ticks, so
// stale demand signals fade but don't vanish instantly.
const shortageDecay = 0.99

// baselineOpportunityWeight is the minimum spawn-selection weight every
// product gets, even with zero recorded shortage, so novel/untested
// recipes still get tried occasionally rather than only ever reacting to
// existing demand.
const baselineOpportunityWeight = 1.0

// recordShortage notes that rate units/sec of product went unsourced or
// unmet this tick.
func (s *State) recordShortage(product string, rate float64) {
	if s.unmet == nil {
		s.unmet = make(map[string]float64)
	}
	s.unmet[product] += rate
}

// decayShortages ages out old shortage signals so the spawn picker
// reflects recent demand, not demand from thousands of ticks ago.
func (s *State) decayShortages() {
	for name, v := range s.unmet {
		v *= shortageDecay
		if v < 0.01 {
			delete(s.unmet, name)
			continue
		}
		s.unmet[name] = v
	}
}

// weightForProduct returns the spawn-selection weight for a product: a
// baseline so untested recipes are still occasionally tried, plus a
// log-compressed function of any currently recorded shortage. Log
// compression matters because a handful of products (e.g. space-elevator
// parts, which sinks want unconditionally and forever) can accumulate a
// shortage orders of magnitude larger than everything else -- used
// linearly, that one huge number would consume nearly all spawn-draw
// probability mass and starve every other recipe, including the tier-1
// ones a supply chain has to start from.
func (s *State) weightForProduct(name string) float64 {
	return baselineOpportunityWeight + math.Log1p(s.unmet[name])
}
