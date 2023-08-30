package state

import (
	"fmt"

	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/products"
	"github.com/paul-freeman/satisfactory-story/recipes"
	"github.com/paul-freeman/satisfactory-story/resources"
)

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
	Profit() float64
}

// specifier is a type that can be used to specify a producer
type specifier interface {
	Name() string
	String() string
}

type state struct {
	producers []producer
	specs     []specifier
}

func New() (*state, error) {
	resources, err := resources.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create producers: %w", err)
	}
	recipes, err := recipes.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create recipes: %w", err)
	}
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
	}, nil
}

func (s *state) Tick() error {
	// Step 1: Remove unprofitable producers

	// Step 2: Spawn new producers

	// Step 3: Move producers

	return nil
}

type producerProfit struct {
	p      producer
	profit float64
}

func (s *state) removeUnprofitableProducers() (int, error) {
	// Group producers by product
	groupedProducers := make(map[string][](producerProfit))
	for _, p := range s.producers {
		pp := producerProfit{p, p.Profit()}
		productsStr := p.Products().String()
		producers, ok := groupedProducers[productsStr]
		if !ok {
			groupedProducers[productsStr] = []producerProfit{pp}
		} else {
			groupedProducers[productsStr] = append(producers, pp)
		}
	}

	// Sort producers groups by profit

	// Remove unprofitable producers

	// Return number of removed producers
	return 0, nil
}
