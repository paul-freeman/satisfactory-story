package state

import (
	"math"
	"testing"
)

func Test_shortage_tracking(t *testing.T) {
	s := &State{}

	if got := s.weightForProduct("Widget"); got != baselineOpportunityWeight {
		t.Errorf("got %f, want baseline %f", got, baselineOpportunityWeight)
	}

	s.recordShortage("Widget", 10)
	s.recordShortage("Widget", 5)
	want := baselineOpportunityWeight + math.Log1p(15)
	if got := s.weightForProduct("Widget"); got != want {
		t.Errorf("got %f, want %f", got, want)
	}

	for i := 0; i < 2000; i++ {
		s.decayShortages()
	}
	if got := s.weightForProduct("Widget"); got != baselineOpportunityWeight {
		t.Errorf("expected the shortage to have decayed away, got weight %f", got)
	}
}
