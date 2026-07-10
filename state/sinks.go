package state

import (
	"log/slog"
	"strings"

	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
	"github.com/paul-freeman/satisfactory-story/recipes"
	"github.com/paul-freeman/satisfactory-story/sink"
)

// spaceElevatorPartPrefix identifies the final products the space
// elevator wants, discovered from the recipe data rather than hardcoded
// one at a time.
const spaceElevatorPartPrefix = "SpaceElevatorPart_"

// sinkPerpetualShortage is added to a sink's wanted product's shortage
// score every tick, before decay -- sinks want an unlimited amount of
// their product forever, so the demand signal must never fully vanish.
const sinkPerpetualShortage = 50.0

// newSinks creates one Sink per distinct space-elevator part product found
// among the recipe outputs, all located at the center of the world bounds
// (representing the player's base).
func newSinks(rs recipes.Recipes, xmin, xmax, ymin, ymax int) []*sink.Sink {
	center := point.Point{X: (xmin + xmax) / 2, Y: (ymin + ymax) / 2}

	seen := make(map[string]bool)
	sinks := make([]*sink.Sink, 0)
	for _, recipe := range rs {
		for _, output := range recipe.Outputs() {
			if !strings.HasPrefix(output.Name, spaceElevatorPartPrefix) || seen[output.Name] {
				continue
			}
			seen[output.Name] = true
			sinks = append(sinks, sink.New(output.Name, center, production.Products{
				production.New(output.Name, 1, 1),
			}))
		}
	}
	return sinks
}

// sourceSinks lets every sink buy up all currently unsold capacity of the
// product(s) it wants. Sinks are price-insensitive with unlimited demand,
// so -- unlike a factory sourcing its inputs -- they don't shop for the
// cheapest seller, they simply drain whatever capacity is available from
// everyone.
func (s *State) sourceSinks(l *slog.Logger) {
	for _, p := range s.producers {
		sk, ok := p.(*sink.Sink)
		if !ok {
			continue
		}
		for _, want := range sk.Input {
			s.recordShortage(want.Name, sinkPerpetualShortage)
			for _, seller := range s.producers {
				remaining := seller.RemainingCapacityFor(want.Name)
				if remaining <= 0 {
					continue
				}
				order := production.Production{Name: want.Name, Rate: remaining}
				transportCost := recipes.TransportCost(seller.Location(), sk.Location())
				if err := s.writeContract(l, seller, sk, order, transportCost); err != nil {
					l.Debug("sink failed to sign contract", slog.String("error", err.Error()))
				}
			}
		}
	}
}
