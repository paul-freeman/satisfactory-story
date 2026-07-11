package state

import (
	"log/slog"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/recipes"
)

// seedCapitalBufferTicks funds a new factory with this many ticks' worth
// of projected input cost as starting capital, representing the up-front
// cost of building the factory.
const seedCapitalBufferTicks = 5.0

// spawnProbabilityPerTick is the chance, per tick, that a new producer is
// attempted at all.
const spawnProbabilityPerTick = 0.05

// spawnNewProducer picks a recipe via a weighted random draw over every
// active recipe -- weighted by current shortage of its outputs (see
// shortage.go) -- and spawns it at a random location only if it can be
// sourced and would not be forced to sell below its own cost basis against
// the current market price.
//
// The draw is weighted across the entire recipe population, not a small
// uniformly-sampled handful, so a shortage in an intermediate product
// (e.g. because a downstream recipe failed to source it) actually raises
// the odds a producer of it gets attempted next -- otherwise demand can
// never propagate backward through a multi-tier supply chain.
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
		weight := 0.0
		for _, output := range recipe.Outputs() {
			weight += s.weightForProduct(output.Name)
		}
		weights[i] = weight
		total += weight
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
	loc := s.randomLocation()

	sources, unmet, err := chosenRecipe.SourceProducts(l, s.producers, loc)
	for _, order := range unmet {
		s.recordShortage(order.Name, order.Rate)
	}
	if err != nil {
		l.Debug("failed to source all recipe ingredients", slog.String("error", err.Error()))
		return
	}

	projectedCostBasis := 0.0
	for _, source := range sources {
		projectedCostBasis += source.Seller.SalesPriceFor(source.Order, source.TransportCost) + source.TransportCost
	}
	projectedSalePrice := projectedCostBasis * 1.50

	for _, output := range chosenRecipe.Outputs() {
		if marketPrice, ok := s.market[output.Name]; ok && projectedSalePrice > marketPrice {
			l.Debug("skipping spawn candidate that can't compete with the current market price",
				slog.String("recipe", chosenRecipe.Name()),
				slog.String("product", output.Name))
			s.recordShortage(output.Name, output.Rate)
			return
		}
	}

	newFactory := factory.New(chosenRecipe.Name(), chosenRecipe.ID(), loc, s.tick,
		chosenRecipe.Inputs(), chosenRecipe.Outputs(), projectedCostBasis*seedCapitalBufferTicks)
	for _, source := range sources {
		s.writeContract(l, source.Seller, newFactory, source.Order, source.TransportCost)
	}
	s.producers = append(s.producers, newFactory)
	l.Debug("spawned producer", slog.String("factory", newFactory.Name))
}

func (s *State) randomLocation() point.Point {
	return point.Point{
		X: s.randSrc.Intn(s.xmax-s.xmin) + s.xmin,
		Y: s.randSrc.Intn(s.ymax-s.ymin) + s.ymin,
	}
}
