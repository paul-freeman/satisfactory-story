package state

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"sort"
	"sync"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
	"github.com/paul-freeman/satisfactory-story/recipes"
	"github.com/paul-freeman/satisfactory-story/resources"
	statehttp "github.com/paul-freeman/satisfactory-story/state/http"
)

const borderMultiplier = 1.1

type State struct {
	m sync.Mutex

	producers []production.Producer
	recipes   recipes.Recipes
	market    map[string]float64

	seed int64
	tick int

	randSrc *rand.Rand

	xmin int
	xmax int
	ymin int
	ymax int
}

func New(l *slog.Logger, seed int64) (*State, error) {
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
	return &State{
		producers: producers,
		recipes:   recipes,
		market:    make(map[string]float64),

		tick: 0,

		randSrc: rand.New(rand.NewSource(seed)),

		xmin: int(float64(xmin) * borderMultiplier),
		xmax: int(float64(xmax) * borderMultiplier),
		ymin: int(float64(ymin) * borderMultiplier),
		ymax: int(float64(ymax) * borderMultiplier),
	}, nil
}

func (s *State) Tick(parentLogger *slog.Logger) error {
	// Block serving state while ticking
	s.m.Lock()
	defer s.m.Unlock()

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

func (s *State) removeUnprofitableProducers(l *slog.Logger) {
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
				pp.p.Remove()
				f, ok := pp.p.(*factory.Factory)
				if !ok {
					l.Error("removed non-factory producer")
				} else {
					l.Debug("removed producer",
						slog.String("factory", f.Name),
						slog.Float64("profit", pp.profit),
					)
				}
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

func (s *State) spawnNewProducers(l *slog.Logger) {
	// Pick a location to spawn the new producer
	loc := point.Point{
		X: s.randSrc.Intn(s.xmax-s.xmin) + s.xmin,
		Y: s.randSrc.Intn(s.ymax-s.ymin) + s.ymin,
	}

	// Select a recipe for the new producer
	recipe := s.recipes[s.randSrc.Intn(len(s.recipes))]

	// Find the cheapest source of each input product
	sources, err := recipe.SourceProducts(s.producers, loc)
	if err != nil {
		l.Debug("failed to source all recipe ingredients", slog.String("spec", recipe.String()))
		return
	}

	// Add the new producer
	newFactory := factory.New(recipe.Name(), loc, recipe.Inputs(), recipe.Outputs())
	for _, source := range sources {
		s.writeContract(l, source.Seller, newFactory, source.Order, source.TransportCost)
	}
	s.producers = append(s.producers, newFactory)
	l.Debug("spawned producer",
		slog.String("factory", newFactory.Name),
		slog.Float64("profit", newFactory.Profit()),
	)
}

func (s *State) writeContract(
	l *slog.Logger,
	seller production.Producer,
	buyer production.Producer,
	order production.Production,
	transportCost float64,
) error {
	if err := seller.HasCapacityFor(order); err != nil {
		return fmt.Errorf("cannot sign contract: %w", err)
	}

	// Calculate costs of existing contracts
	salesPrice := seller.SalesPriceFor(order, transportCost)

	// Check market price
	marketPrice, ok := s.market[order.Name]
	if !ok || salesPrice < marketPrice {
		s.market[order.Name] = salesPrice
	} else {
		salesPrice = marketPrice
	}

	// Create contract
	contract := &production.Contract{
		Seller:        seller,
		Buyer:         buyer,
		Order:         order,
		TransportCost: transportCost,
		ProductCost:   salesPrice,
	}

	// Sign contract
	if err := seller.SignAsSeller(contract); err != nil {
		contract.Cancel()
		return fmt.Errorf("seller rejected contract: %w", err)
	}
	if err := buyer.SignAsBuyer(contract); err != nil {
		contract.Cancel()
		return fmt.Errorf("buyer rejected contract: %w", err)
	}

	// Log contract
	l.Debug("signed contract",
		slog.String("order", order.String()),
		slog.Float64("transportCost", transportCost),
		slog.Float64("productCost", salesPrice),
	)

	return nil
}

func (s *State) ListFactories(l *slog.Logger) {
	for _, producer := range s.producers {
		f, ok := producer.(*factory.Factory)
		if ok {
			l.Info(f.String())
		}
	}
}

func (s *State) toHTTP() statehttp.State {
	factories := make([]statehttp.Factory, 0)
	transports := make([]statehttp.Transport, 0)
	for _, producer := range s.producers {
		// List producer products
		products := make([]string, 0)
		for _, product := range producer.Products() {
			products = append(products, product.Name)
		}

		// Append factory with list of products
		factory := statehttp.Factory{
			Location: statehttp.Location{
				X: producer.Location().X,
				Y: producer.Location().Y,
			},
			Products:      products,
			Profitability: producer.Profitability(),
		}
		factories = append(factories, factory)

		// List incoming transports
		for _, contract := range producer.ContractsIn() {
			transport := statehttp.Transport{
				Origin: statehttp.Location{
					X: contract.Seller.Location().X,
					Y: contract.Seller.Location().Y,
				},
				Destination: statehttp.Location{
					X: contract.Buyer.Location().X,
					Y: contract.Buyer.Location().Y,
				},
				Rate: contract.Order.Rate,
			}
			transports = append(transports, transport)
		}
	}
	return statehttp.State{
		Factories:  factories,
		Transports: transports,
		Tick:       s.tick,
		Xmin:       s.xmin,
		Xmax:       s.xmax,
		Ymin:       s.ymin,
		Ymax:       s.ymax,
	}
}

// ServeState serves the current state of the simulation.
func (s *State) Serve() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Block ticks while serving state
		s.m.Lock()
		defer s.m.Unlock()

		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(s.toHTTP()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}
