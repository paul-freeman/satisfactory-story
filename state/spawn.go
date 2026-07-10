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

// spawnCandidates is how many random (recipe, location) pairs are sampled
// and weighed against each other for each spawn attempt.
const spawnCandidates = 5

// spawnProbabilityPerTick is the chance, per tick, that a new producer is
// attempted at all.
const spawnProbabilityPerTick = 0.05

// spawnNewProducer samples a handful of candidate recipes, weighted toward
// products with recorded shortages (see shortage.go), and spawns the
// chosen one only if it can be sourced and would not be forced to sell
// below its own cost basis against the current market price.
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

	type candidate struct {
		recipe *recipes.Recipe
		loc    point.Point
		weight float64
	}
	candidates := make([]candidate, 0, spawnCandidates)
	for i := 0; i < spawnCandidates; i++ {
		recipe := activeRecipes[s.randSrc.Intn(len(activeRecipes))]
		loc := s.randomLocation()
		weight := 0.0
		for _, output := range recipe.Outputs() {
			weight += s.weightForProduct(output.Name)
		}
		candidates = append(candidates, candidate{recipe, loc, weight})
	}

	total := 0.0
	for _, c := range candidates {
		total += c.weight
	}
	pick := s.randSrc.Float64() * total
	chosen := candidates[len(candidates)-1]
	for _, c := range candidates {
		pick -= c.weight
		if pick <= 0 {
			chosen = c
			break
		}
	}

	sources, unmet, err := chosen.recipe.SourceProducts(l, s.producers, chosen.loc)
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

	for _, output := range chosen.recipe.Outputs() {
		if marketPrice, ok := s.market[output.Name]; ok && projectedSalePrice > marketPrice {
			l.Debug("skipping spawn candidate that can't compete with the current market price",
				slog.String("recipe", chosen.recipe.Name()),
				slog.String("product", output.Name))
			s.recordShortage(output.Name, output.Rate)
			return
		}
	}

	newFactory := factory.New(chosen.recipe.Name(), chosen.loc, s.tick,
		chosen.recipe.Inputs(), chosen.recipe.Outputs(), projectedCostBasis*seedCapitalBufferTicks)
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
