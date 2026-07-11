package state

import (
	"log/slog"
	"math"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/recipes"
)

// seedCapitalBufferTicks funds a new factory with this many ticks' worth
// of estimated input cost plus upkeep, representing the up-front cost of
// building it. It has to cover a realistic idle wait: the factory spends
// it while its input bids sit in the book waiting for supply to appear.
const seedCapitalBufferTicks = 300.0

// spawnProbabilityPerTick is the chance, per tick, that a new producer
// is attempted at all.
const spawnProbabilityPerTick = 0.05

// baselineOpportunityWeight keeps every active recipe in the spawn draw
// even when the book currently shows no profit in it, so novel recipes
// are still explored occasionally.
const baselineOpportunityWeight = 1.0

// unknownInputUnitCost is the pessimistic per-unit estimate for an input
// with no standing ask and no trade history. A penalty, not a veto:
// recipes with lucrative outputs but unsourceable inputs must still
// spawn now and then, because the bids they post are what summon the
// missing tier.
const unknownInputUnitCost = 10.0

// spawnNewProducer picks a recipe via a weighted random draw over every
// active recipe -- weighted by expected profit against the current order
// book -- and spawns it at a random location. No sourcing happens here:
// the factory starts idle with seed capital, and publishOrders will post
// its input bids next tick. This is how demand cascades backward with
// prices only: a lucrative standing bid for a product makes its recipe
// profitable to spawn, and that factory's own input bids make the next
// tier profitable in turn.
func (s *State) spawnNewProducer(l *slog.Logger) {
	activeRecipes := make([]*recipes.Recipe, 0, len(s.recipes))
	for _, recipe := range s.recipes {
		if recipe.Active {
			activeRecipes = append(activeRecipes, recipe)
		}
	}
	if len(activeRecipes) == 0 {
		return
	}

	weights := make([]float64, len(activeRecipes))
	total := 0.0
	for i, recipe := range activeRecipes {
		weights[i] = baselineOpportunityWeight + math.Max(0, s.expectedProfit(recipe))
		total += weights[i]
	}

	pick := s.randSrc.Float64() * total
	chosenRecipe := activeRecipes[len(activeRecipes)-1]
	cumulative := 0.0
	for i, weight := range weights {
		cumulative += weight
		if pick <= cumulative {
			chosenRecipe = activeRecipes[i]
			break
		}
	}

	inputCost := 0.0
	for _, input := range chosenRecipe.Inputs() {
		inputCost += s.estimatedUnitCost(input.Name) * input.Rate
	}
	seedCapital := (inputCost + upkeepPerTick) * seedCapitalBufferTicks

	newFactory := factory.New(chosenRecipe.Name(), chosenRecipe.ID(), s.randomLocation(), s.tick,
		chosenRecipe.Inputs(), chosenRecipe.Outputs(), seedCapital)
	// Start bidding at the going rate where one exists; the price loop
	// escalates from there if the bids go unfilled.
	for _, input := range chosenRecipe.Inputs() {
		if ask, ok := s.book.BestAsk(input.Name); ok {
			newFactory.SetBidPrice(input.Name, ask.UnitPrice)
		}
	}
	s.producers = append(s.producers, newFactory)
	l.Debug("spawned producer", slog.String("factory", newFactory.Name))
}

// expectedProfit estimates a recipe's per-tick profit against the
// current book: revenue at the best standing bids for its outputs
// (never below the salvage floor, which every producing factory earns
// on unsold capacity) minus estimated input costs and upkeep.
func (s *State) expectedProfit(r *recipes.Recipe) float64 {
	revenue := 0.0
	for _, output := range r.Outputs() {
		price := floorUnitPrice
		if bid, ok := s.book.BestBid(output.Name); ok && bid.UnitPrice > price {
			price = bid.UnitPrice
		}
		revenue += price * output.Rate
	}
	cost := upkeepPerTick
	for _, input := range r.Inputs() {
		cost += s.estimatedUnitCost(input.Name) * input.Rate
	}
	return revenue - cost
}

// estimatedUnitCost is the best current estimate of what one unit of
// product costs to buy: the best standing ask, else the last traded
// price, else a pessimistic default.
func (s *State) estimatedUnitCost(product string) float64 {
	if ask, ok := s.book.BestAsk(product); ok {
		return ask.UnitPrice
	}
	if price, ok := s.lastTrade[product]; ok {
		return price
	}
	return unknownInputUnitCost
}

func (s *State) randomLocation() point.Point {
	return point.Point{
		X: s.randSrc.Intn(s.xmax-s.xmin) + s.xmin,
		Y: s.randSrc.Intn(s.ymax-s.ymin) + s.ymin,
	}
}
