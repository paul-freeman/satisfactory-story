package state

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
// baseline so untested recipes are still occasionally tried, plus any
// currently recorded shortage.
func (s *State) weightForProduct(name string) float64 {
	return baselineOpportunityWeight + s.unmet[name]
}
