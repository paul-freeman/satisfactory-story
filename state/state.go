package state

import (
	"fmt"
	"log/slog"
	"math/rand"
	"os"
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
	// AddProfits adds the given amount to the producer's profits.
	AddProfits(float64)
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

	tick int

	rSrc *rand.Rand

	xmin int
	xmax int
	ymin int
	ymax int

	logger *slog.Logger
}

func New(seed int64) (*state, error) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

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

		rSrc: rand.New(rand.NewSource(seed)),

		xmin: int(float64(xmin) * borderMultiplier),
		xmax: int(float64(xmax) * borderMultiplier),
		ymin: int(float64(ymin) * borderMultiplier),
		ymax: int(float64(ymax) * borderMultiplier),

		logger: logger,
	}, nil
}

func (s *state) Tick() error {
	// Step 1: Remove unprofitable producers
	_, err := s.removeUnprofitableProducers()
	if err != nil {
		return fmt.Errorf("failed to remove unprofitable producers: %w", err)
	}

	// Step 2: Spawn new producers
	err = s.spawnNewProducers()
	if err != nil {
		s.logger.Debug("failed to spawn new producers", err)
	}

	// Step 3: Move producers

	return nil
}

type producerProfit struct {
	p      producer
	profit float64
}

func (s *state) removeUnprofitableProducers() (int, error) {
	// Group producers by product
	groupedPPS := make(map[string][](producerProfit))
	for _, p := range s.producers {
		pp := producerProfit{p, p.Profit()}
		productsStr := p.Products().String()
		producers, ok := groupedPPS[productsStr]
		if !ok {
			groupedPPS[productsStr] = []producerProfit{pp}
		} else {
			groupedPPS[productsStr] = append(producers, pp)
		}
	}

	// Sort producers groups by profit - most profitable first
	for _, producers := range groupedPPS {
		sort.Slice(producers, func(i, j int) bool {
			return producers[i].profit > producers[j].profit
		})
	}

	// Remove unprofitable producers
	removedCount := 0
	s.producers = make([]producer, 0, len(s.producers))
	for _, pps := range groupedPPS {
		// Keep the most profitable producer
		s.producers = append(s.producers, pps[0].p)

		// Keep all producers that are profitable or not removable
		for _, pp := range pps[1:] {
			if pp.profit > 0 || !pp.p.IsRemovable() {
				// Keep producer
				s.producers = append(s.producers, pp.p)
			} else {
				// Remove producer (by not adding it to s.producers)
				s.logger.Info("removed producer", slog.Int("tick", s.tick), slog.String("producer", pp.p.String()))
				removedCount++
			}
		}
	}

	// Return number of removed producers
	return removedCount, nil
}

type producerCost struct {
	p    producer
	cost float64
}

func (s *state) spawnNewProducers() error {
	// Pick a location to spawn the new producer
	loc := point.Point{
		X: s.rSrc.Intn(s.xmax-s.xmin) + s.xmin,
		Y: s.rSrc.Intn(s.ymax-s.ymin) + s.ymin,
	}

	// Select a spec for the new producer
	spec := s.specs[s.rSrc.Intn(len(s.specs))]

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
			}
		}
		if bestProducer == nil {
			return fmt.Errorf("failed to find producer for input product %s", input.String())
		}
		sourcedProducts[input.Name()] = producerCost{
			p:    bestProducer,
			cost: bestCost,
		}
	}

	// Check that all products are available
	if len(sourcedProducts) != len(spec.Inputs()) {
		return fmt.Errorf("failed to find all input products")
	}

	// Add the new producer
	fact := factory.New(spec.Name(), loc, spec.Inputs(), spec.Outputs(), spec.Duration())
	for _, pc := range sourcedProducts {
		// TODO: This is not ideal, since the seller benefits from the distance
		// being greater.
		pc.p.AddProfits(pc.cost)
		fact.AddProfits(-pc.cost)
	}
	s.producers = append(s.producers, fact)
	s.logger.Info("spawned producer", slog.Int("tick", s.tick), slog.String("producer", fact.String()), slog.Float64("profit", fact.Profit()), slog.Float64("cost", fact.Profit()-fact.Profit()))

	return nil
}

// costFunction returns the cost of transporting the given product from the
// given producer to the given location.
func costFunction(p producer, loc point.Point, product products.Product) float64 {
	return loc.Distance(p.Location())
}
