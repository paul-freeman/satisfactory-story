package state

import (
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

// goalBidUnitPrice is what a space-elevator sink offers per unit of its
// part. It is deliberately far above any plausible production cost so
// the recipe that makes the part is always the most profitable thing in
// the book -- this bid is the money that cascades backward through the
// supply tiers.
const goalBidUnitPrice = 1000.0

// sinkDemandRate is the standing bid rate for sinks. Effectively
// unlimited against realistic production rates (single recipes run at
// ~0.1-10 units/sec) while staying readable in the UI and safe in
// min() arithmetic.
const sinkDemandRate = 100.0

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
			}, goalBidUnitPrice))
		}
	}
	return sinks
}
