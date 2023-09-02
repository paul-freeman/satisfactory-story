package state

import (
	"fmt"
	"log/slog"
	"math/rand"
	"sort"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
	"github.com/paul-freeman/satisfactory-story/recipes"
	"github.com/paul-freeman/satisfactory-story/resources"
)

const borderMultiplier = 1.1

type state struct {
	producers []production.Producer
	recipes   recipes.Recipes

	seed int64
	tick int

	randSrc *rand.Rand

	xmin int
	xmax int
	ymin int
	ymax int
}

func New(l *slog.Logger, seed int64) (*state, error) {
	// Load resources and recipes
	resources, err := resources.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create producers: %w", err)
	}
	recipes, err := recipes.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create recipes: %w", err)
	}

	// Find resource location bounds
	xmin := 0
	xmax := 0
	ymin := 0
	ymax := 0
	for _, resource := range resources {
		loc := resource.Location()
		if loc.X < xmin {
			xmin = loc.X
		}
		if loc.X > xmax {
			xmax = loc.X
		}
		if loc.Y < ymin {
			ymin = loc.Y
		}
		if loc.Y > ymax {
			ymax = loc.Y
		}
	}

	// Create producers
	producers := make([]production.Producer, len(resources))
	for i, resource := range resources {
		producers[i] = resource
	}

	// Return state
	return &state{
		producers: producers,
		recipes:   recipes,

		tick: 0,

		randSrc: rand.New(rand.NewSource(seed)),

		xmin: int(float64(xmin) * borderMultiplier),
		xmax: int(float64(xmax) * borderMultiplier),
		ymin: int(float64(ymin) * borderMultiplier),
		ymax: int(float64(ymax) * borderMultiplier),
	}, nil
}

func (s *state) Tick(parentLogger *slog.Logger) error {
	s.tick++
	l := parentLogger.With(slog.Int("tick", s.tick))

	// Step 1: Remove unprofitable producers
	s.removeUnprofitableProducers(l)

	// Step 2: Spawn new producers
	s.spawnNewProducers(l)

	// Step 3: Move producers

	return nil
}

type producerStats struct {
	p      production.Producer
	profit float64
}

func (s *state) removeUnprofitableProducers(l *slog.Logger) {
	// Group producers by product
	groupedStats := make(map[string][](producerStats))
	for _, p := range s.producers {
		newStats := producerStats{p, p.Profit()}
		productsStr := p.Products().String()
		currentStats, ok := groupedStats[productsStr]
		if !ok {
			groupedStats[productsStr] = []producerStats{newStats}
		} else {
			groupedStats[productsStr] = append(currentStats, newStats)
		}
	}

	// Sort producers groups by profit - most profitable first
	for _, producers := range groupedStats {
		sort.Slice(producers, func(i, j int) bool {
			return producers[i].profit > producers[j].profit
		})
	}

	// Remove unprofitable producers
	removedCount := 0
	finalProducers := make([]production.Producer, 0, len(s.producers))
	for _, pps := range groupedStats {
		// Keep the most profitable producer
		finalProducers = append(finalProducers, pps[0].p)

		// Keep all producers that are profitable or not removable
		for _, pp := range pps[1:] {
			if pp.profit > 0 || !pp.p.IsRemovable() {
				// Keep producer
				finalProducers = append(finalProducers, pp.p)
			} else {
				// Remove producer (by not adding it to s.producers)
				removedCount++
			}
		}
	}

	// Save new producers
	s.producers = finalProducers
}

type producerCost struct {
	p    production.Producer
	cost float64
}

func (s *state) spawnNewProducers(l *slog.Logger) {
	// Pick a location to spawn the new producer
	loc := point.Point{
		X: s.randSrc.Intn(s.xmax-s.xmin) + s.xmin,
		Y: s.randSrc.Intn(s.ymax-s.ymin) + s.ymin,
	}

	// Select a recipe for the new producer
	spec := s.recipes[s.randSrc.Intn(len(s.recipes))]

	// Find the cheapest source of each input product
	sourcedProducts := make(map[string]producerCost)
	for _, product := range spec.Inputs() {
		// Find producers that produce the input product
		var bestProducer production.Producer
		var bestCost float64
		for _, p := range s.producers {
			err := p.HasCapacityFor(product)
			if err != nil {
				l.Debug("producer lacks capacity", slog.String("input", product.Name))
				continue
			}
			if bestProducer == nil {
				bestProducer = p
			} else {
				cost := costFunction(p, loc, product)
				if cost < bestCost {
					bestProducer = p
					bestCost = cost
				}
			}
		}
		if bestProducer == nil {
			l.Debug("failed to find producer for input", slog.String("input", product.Name))
			return
		}
		sourcedProducts[product.Name] = producerCost{
			p:    bestProducer,
			cost: bestCost,
		}
	}

	// Check that all products are available
	if len(sourcedProducts) != len(spec.Inputs()) {
		l.Debug("failed to find all inputs", slog.String("spec", spec.String()))
		return
	}

	// Add the new producer
	factoryBuilding := factory.New(spec.Name(), loc, spec.Inputs(), spec.Outputs(), spec.Duration())
	for _, pc := range sourcedProducts {
		// TODO: Decide what to do here
		_ = pc
	}
	s.producers = append(s.producers, factoryBuilding)
	l.Info("spawned producer",
		slog.String("producer", factoryBuilding.String()),
		slog.Float64("profit", factoryBuilding.Profit()),
		slog.Float64("cost", factoryBuilding.Profit()-factoryBuilding.Profit()),
	)
}

// costFunction returns the cost of transporting the given product from the
// given producer to the given location.
func costFunction(p production.Producer, loc point.Point, product production.Production) float64 {
	return loc.Distance(p.Location())
}
