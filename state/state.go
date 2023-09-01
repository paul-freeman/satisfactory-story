package state

import (
	"fmt"
	"log/slog"
	"math/rand"
	"sort"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/products"
	"github.com/paul-freeman/satisfactory-story/recipes"
	"github.com/paul-freeman/satisfactory-story/resources"
)

const borderMultiplier = 1.1

// producer is a type that can be used to produce a resource
type producer interface {
	Name() string
	String() string
	Location() point.Point
	// IsMovable returns true if the producer can be moved.
	IsMovable() bool
	// IsRemovable returns true if the producer can be removed.
	IsRemovable() bool
	// Products returns the products that the producer produces.
	Products() products.Products
	// Profit returns the profit of the producer.
	Profit() float64
	// HasProduct returns true if the producer produces the given product.
	HasProduct(products.Product) bool
}

// specifier is a type that can be used to specify a producer
type specifier interface {
	Name() string
	String() string
	// Inputs returns the products that the producer requires as inputs.
	Inputs() products.Products
	// Outputs returns the products that the producer produces as outputs.
	Outputs() products.Products
	// Duration returns the time it takes for the producer to produce its
	// outputs.
	Duration() float64
}

type state struct {
	producers []producer
	specs     []specifier

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

	// Create producers and specs
	producers := make([]producer, len(resources))
	for i, resource := range resources {
		producers[i] = resource
	}
	specs := make([]specifier, len(recipes))
	for i, recipe := range recipes {
		specs[i] = recipe
	}
	return &state{
		producers: producers,
		specs:     specs,

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
	p      producer
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
	finalProducers := make([]producer, 0, len(s.producers))
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
				l.Info("removed producer", slog.String("producer", pp.p.String()))
				removedCount++
			}
		}
	}

	// Save new producers
	s.producers = finalProducers
}

type producerCost struct {
	p    producer
	cost float64
}

func (s *state) spawnNewProducers(l *slog.Logger) {
	// Pick a location to spawn the new producer
	loc := point.Point{
		X: s.randSrc.Intn(s.xmax-s.xmin) + s.xmin,
		Y: s.randSrc.Intn(s.ymax-s.ymin) + s.ymin,
	}

	// Select a spec for the new producer
	spec := s.specs[s.randSrc.Intn(len(s.specs))]

	// Find the cheapest source of each input product
	sourcedProducts := make(map[string]producerCost)
	for _, input := range spec.Inputs() {
		// Find producers that produce the input product
		var bestProducer producer
		var bestCost float64
		for _, p := range s.producers {
			if p.HasProduct(input) {
				if bestProducer == nil {
					bestProducer = p
				} else {
					cost := costFunction(p, loc, input)
					if cost < bestCost {
						bestProducer = p
						bestCost = cost
					}
				}
			} else {
				l.Debug("producer does not produce input", slog.String("producer", p.String()), slog.String("input", input.Name()))
			}
		}
		if bestProducer == nil {
			l.Debug("failed to find producer for input", slog.String("input", input.Name()))
			return
		}
		sourcedProducts[input.Name()] = producerCost{
			p:    bestProducer,
			cost: bestCost,
		}
	}

	// Check that all products are available
	if len(sourcedProducts) != len(spec.Inputs()) {
		l.Error("failed to find all inputs", slog.String("spec", spec.String()))
		return
	}

	// Add the new producer
	factoryBuilding := factory.New(spec.Name(), loc, spec.Inputs(), spec.Outputs(), spec.Duration())
	for _, pc := range sourcedProducts {
		// TODO: Decide what to do here
		_ = pc
	}
	s.producers = append(s.producers, factoryBuilding)
	l.Info("spawned producer", slog.String("producer", factoryBuilding.String()), slog.Float64("profit", factoryBuilding.Profit()), slog.Float64("cost", factoryBuilding.Profit()-factoryBuilding.Profit()))
}

// costFunction returns the cost of transporting the given product from the
// given producer to the given location.
func costFunction(p producer, loc point.Point, product products.Product) float64 {
	return loc.Distance(p.Location())
}
