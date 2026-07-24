package state

import (
	"log/slog"
	"math"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/recipes"
)

// seedCapitalBufferTicks funds a new factory with this many ticks' worth
// of upkeep (only). Stock cost is budgeted separately via inputStockTargetTicks.
// It has to cover a realistic idle wait: the factory spends it while its
// input bids sit in the book waiting for supply to appear.
const seedCapitalBufferTicks = 300.0

// defaultTransportEstimate is the flat per-unit freight allowance used
// in every cost estimate. Precision only needs to prevent the verified
// transport-blindness failure (estimates of ~0.01 against delivered
// costs of ~1-2), not price real freight.
const defaultTransportEstimate = 2.0

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

	// Crowding discount: a recipe's opportunity is shared by every live
	// factory already running it, so saturated niches stop attracting
	// entrants and the draw naturally walks to the next unserved tier.
	crowd := s.recipeCrowding()
	weights := make([]float64, len(activeRecipes))
	total := 0.0
	for i, recipe := range activeRecipes {
		weights[i] = (baselineOpportunityWeight + math.Max(0, s.expectedProfit(recipe))) /
			float64(1+crowd[recipe.ID()])
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

	// Seed capital: enough to fill the input-stock target at estimated
	// delivered prices, plus an upkeep runway.
	stockCost := 0.0
	for _, input := range chosenRecipe.Inputs() {
		stockCost += s.estimatedDeliveredCost(input.Name) * input.Rate
	}
	seedCapital := stockCost*inputStockTargetTicks + upkeepPerTick*seedCapitalBufferTicks

	// Seed capital is withdrawn from the finite treasury, not minted. If
	// the treasury cannot cover this seed, skip the spawn entirely (no
	// partial funding): an inflated packaging-loop seed that outgrows the
	// pot is refused, starving the money pump, while cheap recipes stay
	// fundable from the same pot.
	if s.treasury < seedCapital {
		l.Debug("spawn skipped: treasury short",
			slog.Float64("treasury", s.treasury),
			slog.Float64("seedCapital", seedCapital))
		return
	}
	s.treasury -= seedCapital

	newFactory := factory.New(chosenRecipe.Name(), chosenRecipe.ID(), s.spawnLocation(chosenRecipe), s.tick,
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
		cost += s.estimatedDeliveredCost(input.Name) * input.Rate
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

// estimatedDeliveredCost is the best current estimate of what one unit
// of product costs to buy AND ship here.
func (s *State) estimatedDeliveredCost(product string) float64 {
	return s.estimatedUnitCost(product) + defaultTransportEstimate
}

// recipeCrowding counts live factories per recipe class. Reading the
// producer population is public market state (see the spec's purity
// line) -- it is not the recipe tree.
func (s *State) recipeCrowding() map[string]int {
	crowd := make(map[string]int)
	for _, p := range s.producers {
		if f, ok := p.(*factory.Factory); ok {
			crowd[f.RecipeClass]++
		}
	}
	return crowd
}

// spawnOffsetFromInput keeps a freshly-spawned factory from landing
// exactly on a seller's coordinates. recipes.UnitTransportCost treats
// any distance <= 1 as a same-location collision and charges 1e12 for
// it (see recipes.go) -- specifically to stop Move() from doing this --
// so spawning right on top of a seller would make that input
// permanently unaffordable instead of cheap. The offset only needs to
// clear that threshold, not model any real construction footprint.
const spawnOffsetFromInput = 5

// spawnLocation places a new factory near its currently sourceable
// inputs: the centroid of the best-ask sellers' locations for every
// input that has one right now, nudged away from exact collision (see
// spawnOffsetFromInput). This shrinks the transport-cost gap a fresh
// bid has to close to cross an ask, and stops freshly-spawned
// factories from starting nowhere near what they need. It reads only
// the live book (already-public ask locations), never the recipe tree,
// so it doesn't compromise the "prices only" demand-cascade design --
// a recipe with no currently sourceable input (the common case for a
// deep, not-yet-summoned tier) falls back to a random location, exactly
// as before.
func (s *State) spawnLocation(r *recipes.Recipe) point.Point {
	sumX, sumY, n := 0, 0, 0
	for _, input := range r.Inputs() {
		ask, ok := s.book.BestAsk(input.Name)
		if !ok {
			continue
		}
		loc := ask.Seller.Location()
		sumX += loc.X
		sumY += loc.Y
		n++
	}
	if n == 0 {
		return s.randomLocation()
	}
	return point.Point{
		X: sumX/n + spawnOffsetFromInput,
		Y: sumY/n + spawnOffsetFromInput,
	}
}

func (s *State) randomLocation() point.Point {
	return point.Point{
		X: s.randSrc.Intn(s.xmax-s.xmin) + s.xmin,
		Y: s.randSrc.Intn(s.ymax-s.ymin) + s.ymin,
	}
}
