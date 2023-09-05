package state

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
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

const (
	borderPaddingPct   = 0.1
	minProducersToKeep = 5
)

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
	xmin := resources[0].Location().X
	xmax := resources[0].Location().X
	ymin := resources[0].Location().Y
	ymax := resources[0].Location().Y
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

	borderPaddingX := float64(xmax-xmin) * borderPaddingPct
	borderPaddingY := float64(ymax-ymin) * borderPaddingPct

	// Return state
	return &State{
		producers: producers,
		recipes:   recipes,
		market:    make(map[string]float64),

		tick: 0,

		randSrc: rand.New(rand.NewSource(seed)),

		xmin: int(float64(xmin) - borderPaddingX),
		xmax: int(float64(xmax) + borderPaddingX),
		ymin: int(float64(ymin) - borderPaddingY),
		ymax: int(float64(ymax) + borderPaddingY),
	}, nil
}

func (s *State) Tick(parentLogger *slog.Logger) error {
	// Block serving state while ticking
	s.m.Lock()
	defer s.m.Unlock()

	s.tick++
	l := parentLogger.With(slog.Int("tick", s.tick))

	s.removeUnprofitableProducers(l)
	s.spawnNewProducers(l)
	s.moveProducer(l)

	return nil
}

type producerStats struct {
	producer production.Producer
	profit   float64
}

func (s *State) removeUnprofitableProducers(l *slog.Logger) {
	// Calculate profit for each producer
	stats := make([]producerStats, 0, len(s.producers))
	for _, producer := range s.producers {
		stats = append(stats, producerStats{producer, producer.Profit()})
	}

	// Group producers by product
	groupedStats := make(map[string][](producerStats))
	for _, stat := range stats {
		productsKey := stat.producer.Products().Key()
		currentStats, ok := groupedStats[productsKey]
		if !ok {
			groupedStats[productsKey] = []producerStats{stat}
		} else {
			groupedStats[productsKey] = append(currentStats, stat)
		}
	}

	// Sort producers groups by profit - most profitable first
	for _, statsGroup := range groupedStats {
		sort.Slice(statsGroup, func(i, j int) bool {
			return statsGroup[i].profit > statsGroup[j].profit
		})
	}

	// Remove unprofitable producers
	finalProducers := make([]production.Producer, 0, len(s.producers))
	for _, statsGroup := range groupedStats {
		for i, val := range statsGroup {
			// Keep the most profitable N producers
			if i < minProducersToKeep {
				finalProducers = append(finalProducers, val.producer)
				continue
			}

			switch v := val.producer.(type) {
			case production.MoveableProducer:
				if val.profit > 0 {
					// Keep producer
					finalProducers = append(finalProducers, val.producer)
				} else {
					v.Remove()
					f, ok := val.producer.(*factory.Factory)
					if !ok {
						l.Error("removed non-factory producer")
					} else {
						l.Debug("removed producer",
							slog.String("factory", f.Name),
							slog.Float64("profit", val.profit),
						)
					}
				}
			default:
				// Cannot remove immovable producer, so just keep it.
				finalProducers = append(finalProducers, statsGroup[i].producer)
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
	sources, err := recipe.SourceProducts(l, s.producers, loc)
	if err != nil {
		l.Debug("failed to source all recipe ingredients", slog.String("error", err.Error()))
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

func (s *State) moveProducer(l *slog.Logger) {
	for _, producer := range s.producers {
		switch producer := producer.(type) {
		case production.MoveableProducer:
			if err := producer.Move(); err != nil {
				l.Error("failed to move producer: " + err.Error())
			}
		default:
			// Do nothing
		}
	}
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
		slog.String("order", order.Key()),
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
		active := true
		resource, ok := producer.(*resources.Resource)
		if ok {
			newSales := make([]*production.Contract, 0)
			for _, sale := range resource.Sales {
				if !sale.Cancelled {
					newSales = append(newSales, sale)
				}
			}
			resource.Sales = newSales
			if len(resource.Sales) == 0 {
				active = false
			}
		}

		// List producer products
		products := make([]string, 0)
		for _, product := range producer.Products() {
			products = append(products, product.Name)
		}

		// Append factory with list of products
		profitability := producer.Profitability()
		if math.IsNaN(profitability) {
			profitability = 0
		}
		factory := statehttp.Factory{
			Location: statehttp.Location{
				X: producer.Location().X,
				Y: producer.Location().Y,
			},
			Products:      products,
			Profitability: profitability,
			Active:        active,
		}
		factories = append(factories, factory)

		// List incoming transports
		for _, contract := range producer.ContractsIn() {
			rate := contract.Order.Rate
			if math.IsNaN(rate) {
				rate = 0
			}
			transport := statehttp.Transport{
				Origin: statehttp.Location{
					X: contract.Seller.Location().X,
					Y: contract.Seller.Location().Y,
				},
				Destination: statehttp.Location{
					X: contract.Buyer.Location().X,
					Y: contract.Buyer.Location().Y,
				},
				Rate: rate,
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
func (s *State) Serve(l *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Block ticks while serving state
		s.m.Lock()
		defer s.m.Unlock()

		// Set CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "http://localhost:8000")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")

		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(s.toHTTP()); err != nil {
			l.Error("failed to encode state: " + err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}
