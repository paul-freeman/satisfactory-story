package state

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"sort"
	"sync"
	"time"

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

	seed   int64
	tick   int
	cancel context.CancelFunc

	randSrc *rand.Rand

	xmin int
	xmax int
	ymin int
	ymax int
}

func New(l *slog.Logger, seed int64) (*State, error) {
	s := new(State)
	err := s.getInitialState(l, seed)
	return s, err
}

func (s *State) getInitialState(l *slog.Logger, seed int64) error {
	// Load resources and recipes
	resources, err := resources.New()
	if err != nil {
		return fmt.Errorf("failed to create producers: %w", err)
	}
	recipes, err := recipes.New()
	if err != nil {
		return fmt.Errorf("failed to create recipes: %w", err)
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

	// Populate state
	s.producers = producers
	s.recipes = recipes
	s.market = make(map[string]float64)

	s.seed = seed
	s.tick = 0
	s.cancel = nil

	s.randSrc = rand.New(rand.NewSource(seed))

	s.xmin = int(float64(xmin) - borderPaddingX)
	s.xmax = int(float64(xmax) + borderPaddingX)
	s.ymin = int(float64(ymin) - borderPaddingY)
	s.ymax = int(float64(ymax) + borderPaddingY)

	return nil
}

func (s *State) Tick(parentLogger *slog.Logger) error {
	// Lock state while ticking
	s.m.Lock()
	defer s.m.Unlock()

	s.tick++
	l := parentLogger.With(slog.Int("tick", s.tick))

	s.removeUnprofitableProducers(l)
	s.spawnNewProducers(l)
	s.moveProducer(l)

	if s.tick >= 50000 {
		// slow down simulation to save CPU
		time.Sleep(10 * time.Millisecond)
	}

	return nil
}

func (s *State) Run(parentLogger *slog.Logger) {
	// Set logger
	logger := parentLogger.With(slog.Bool("running", true))

	// Create cancellation context
	ctx, cancel := context.WithCancel(context.Background())
	s.setCancellationFunc(cancel, parentLogger)

	// Run simulation
	go func() {
		defer s.setCancellationFunc(nil, logger)
		for {
			select {
			case <-ctx.Done():
				return
			default:
				if err := s.Tick(logger); err != nil {
					logger.Error(fmt.Sprintf("failed to tick state: %v", err))
					return
				}
			}
		}
	}()
}

func (s *State) Stop(_ *slog.Logger) {
	s.m.Lock()
	defer s.m.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *State) Reset(l *slog.Logger) {
	s.m.Lock()
	defer s.m.Unlock()
	s.getInitialState(l, s.seed)
}

func (s *State) setCancellationFunc(cancel context.CancelFunc, logger *slog.Logger) {
	s.m.Lock()
	defer s.m.Unlock()
	if (s.cancel == nil && cancel == nil) || (s.cancel != nil && cancel != nil) {
		logger.Warn("setCancellationFunc error")
		return
	}
	s.cancel = cancel
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
			switch v := val.producer.(type) {
			case *factory.Factory:
				func() {
					for _, contract := range v.ContractsIn() {
						if contract.Cancelled {
							v.Remove()
							return
						}
					}
					if len(v.ContractsIn()) != len(v.Input) {
						v.Remove()
						return
					}
					if i < minProducersToKeep {
						finalProducers = append(finalProducers, val.producer)
						return
					}
					if len(v.ContractsIn()) == 0 {
						v.Remove()
						return
					}
					if val.profit <= 0 {
						v.Remove()
						return
					}
					// Keep producer
					finalProducers = append(finalProducers, val.producer)
				}()
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

func (s *State) MarshalJSON() ([]byte, error) {
	// Lock state while copying
	s.m.Lock()
	sJSON := s.toHTTP()
	s.m.Unlock()

	return json.Marshal(sJSON)
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
		recipe := "EMPTY"
		numContracts := len(producer.ContractsIn())
		switch val := producer.(type) {
		case *factory.Factory:
			recipe = val.Name
		case *resources.Resource:
			recipe = val.Production.Name
		}
		factory := statehttp.Factory{
			Location: statehttp.Location{
				X: producer.Location().X,
				Y: producer.Location().Y,
			},
			Recipe:        recipe + fmt.Sprintf(" (%d)", numContracts),
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
		Running:    s.cancel != nil,
		Xmin:       s.xmin,
		Xmax:       s.xmax,
		Ymin:       s.ymin,
		Ymax:       s.ymax,
	}
}
