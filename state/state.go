package state

import (
	"fmt"
	"math/rand"
	"sort"

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
	Location() (point.Point, error)
	// IsMovable returns true if the producer can be moved.
	IsMovable() bool
	// IsRemovable returns true if the producer can be removed.
	IsRemovable() bool
	// Products returns the products that the producer produces.
	Products() products.Products
	// Profit returns the profit of the producer.
	Profit() float32
	// Produces returns true if the producer produces the given product.
	Produces(products.Product) bool
}

// specifier is a type that can be used to specify a producer
type specifier interface {
	Name() string
	String() string
	// Inputs returns the products that the producer requires as inputs.
	Inputs() products.Products
	// Outputs returns the products that the producer produces as outputs.
	Outputs() products.Products
}

type state struct {
	producers []producer
	specs     []specifier

	rSrc *rand.Rand

	xmin int
	xmax int
	ymin int
	ymax int
}

func New(seed int64) (*state, error) {
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
		loc, err := resource.Location()
		if err != nil {
			return nil, fmt.Errorf("failed to get resource location: %w", err)
		}
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

		rSrc: rand.New(rand.NewSource(seed)),

		xmin: int(float64(xmin) * borderMultiplier),
		xmax: int(float64(xmax) * borderMultiplier),
		ymin: int(float64(ymin) * borderMultiplier),
		ymax: int(float64(ymax) * borderMultiplier),
	}, nil
}

func (s *state) Tick() error {
	// Step 1: Remove unprofitable producers
	_, err := s.removeUnprofitableProducers()
	if err != nil {
		return fmt.Errorf("failed to remove unprofitable producers: %w", err)
	}

	// Step 2: Spawn new producers

	// Step 3: Move producers

	return nil
}

type producerProfit struct {
	p      producer
	profit float32
}

func (s *state) removeUnprofitableProducers() (int, error) {
	// Group producers by product
	groupedProducers := make(map[string][](producerProfit))
	for _, p := range s.producers {
		pp := producerProfit{p, p.Profit()}
		productsStr := p.Products().String(1) // TODO: fix this 1 hack
		producers, ok := groupedProducers[productsStr]
		if !ok {
			groupedProducers[productsStr] = []producerProfit{pp}
		} else {
			groupedProducers[productsStr] = append(producers, pp)
		}
	}

	// Sort producers groups by profit - most profitable first
	for _, producers := range groupedProducers {
		sort.Slice(producers, func(i, j int) bool {
			return producers[i].profit > producers[j].profit
		})
	}

	// Remove unprofitable producers
	removedCount := 0
	s.producers = make([]producer, 0, len(s.producers))
	for _, producers := range groupedProducers {
		// Keep the most profitable producer
		s.producers = append(s.producers, producers[0].p)

		// Keep all producers that are profitable or not removable
		for _, p := range producers[1:] {
			if p.profit >= 0 || !p.p.IsRemovable() {
				// Keep producer
				s.producers = append(s.producers, p.p)
			} else {
				// Remove producer (by not adding it to s.producers)
				removedCount++
			}
		}
	}

	// Return number of removed producers
	return removedCount, nil
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
	sourcedProducts := make(map[string]producer)
	for _, input := range spec.Inputs() {
		// Find producers that produce the input product
		var bestProducer producer
		for _, p := range s.producers {
			if p.Produces(input) {
				// TODO: calculate cost of reaching producer
				if bestProducer == nil {
					bestProducer = p
				}
			}
		}
		if bestProducer == nil {
			return fmt.Errorf("failed to find producer for input product %s", input.String(1))
		}
		sourcedProducts[string(input.Name)] = bestProducer
	}

	// Check what products are available
	_ = loc
	return nil
}
