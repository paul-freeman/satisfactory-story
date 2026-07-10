package state

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"sync"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/production"
	"github.com/paul-freeman/satisfactory-story/recipes"
	"github.com/paul-freeman/satisfactory-story/resources"
	storyresources "github.com/paul-freeman/satisfactory-story/resources"
	"github.com/paul-freeman/satisfactory-story/sink"
	statehttp "github.com/paul-freeman/satisfactory-story/state/http"
)

const (
	borderPaddingPct = 0.1
)

type State struct {
	m sync.Mutex

	producers []production.Producer
	recipes   recipes.Recipes
	market    map[string]float64
	unmet     map[string]float64

	seed   int64
	tick   int
	cancel context.CancelFunc

	randSrc *rand.Rand

	xmin int
	xmax int
	ymin int
	ymax int

	logLevel *slog.Level
}

func New(l *slog.Logger, logLevel *slog.Level, seed int64) (*State, error) {
	s := new(State)
	err := s.getInitialState(l, logLevel, seed)
	return s, err
}

func (s *State) getInitialState(l *slog.Logger, logLevel *slog.Level, seed int64) error {
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

	borderPaddingX := float64(xmax-xmin) * borderPaddingPct
	borderPaddingY := float64(ymax-ymin) * borderPaddingPct
	paddedXmin := int(float64(xmin) - borderPaddingX)
	paddedXmax := int(float64(xmax) + borderPaddingX)
	paddedYmin := int(float64(ymin) - borderPaddingY)
	paddedYmax := int(float64(ymax) + borderPaddingY)

	// Create producers
	producers := make([]production.Producer, 0)
	for _, resource := range resources {
		producers = append(producers, resource)
	}
	for _, sk := range newSinks(recipes, paddedXmin, paddedXmax, paddedYmin, paddedYmax) {
		producers = append(producers, sk)
	}

	// Populate state
	s.producers = producers
	s.recipes = recipes
	s.market = make(map[string]float64)
	s.unmet = make(map[string]float64)

	s.seed = seed
	s.tick = 0
	s.cancel = nil

	s.randSrc = rand.New(rand.NewSource(seed))

	s.xmin = paddedXmin
	s.xmax = paddedXmax
	s.ymin = paddedYmin
	s.ymax = paddedYmax

	s.logLevel = logLevel

	return nil
}

func (s *State) Tick(parentLogger *slog.Logger) error {
	// Lock state while ticking
	s.m.Lock()
	defer s.m.Unlock()

	s.tick++
	l := parentLogger.With(slog.Int("tick", s.tick))

	// Every mechanism runs every tick (spawning and renegotiation are each
	// individually probability-gated inside their own function) instead of
	// the old spawn/move/cull phases, which meant nothing moved or got
	// culled for the first third of any run.
	s.moveProducers(l)
	s.sourceSinks(l)
	if s.randSrc.Float64() < spawnProbabilityPerTick {
		s.spawnNewProducer(l)
	}
	s.renegotiateContracts(l)
	s.applySolvency(l)
	s.decayShortages()

	return nil
}

func (s *State) Run(parentLogger *slog.Logger) {
	// Set logger
	l := parentLogger.With(slog.Bool("running", true))

	// Create cancellation context
	ctx, cancel := context.WithCancel(context.Background())
	s.setCancellationFunc(cancel, parentLogger)

	// Run simulation
	go func() {
		defer s.setCancellationFunc(nil, l)
		for {
			select {
			case <-ctx.Done():
				return
			default:
				if err := s.Tick(l); err != nil {
					l.Error(fmt.Sprintf("failed to tick state: %v", err))
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

func (s *State) Reset(l *slog.Logger, logLevel *slog.Level) {
	s.m.Lock()
	defer s.m.Unlock()
	s.getInitialState(l, s.logLevel, s.seed)
}

func (s *State) Recipes(_ *slog.Logger) []statehttp.Recipe {
	recipes := make([]statehttp.Recipe, 0, len(s.recipes))
	for _, recipe := range s.recipes {
		recipes = append(recipes, statehttp.Recipe{
			Name: recipe.Name(),
			Inputs: []statehttp.Product{
				{
					Name: recipe.Inputs()[0].Name,
					Rate: recipe.Inputs()[0].Rate,
				},
			},
			Outputs: []statehttp.Product{
				{
					Name: recipe.Outputs()[0].Name,
					Rate: recipe.Outputs()[0].Rate,
				},
			},
			Active: recipe.Active,
		})
	}

	return recipes
}

func (s *State) SetRecipe(l *slog.Logger, recipeName string, enabled bool) []statehttp.Recipe {
	// Find recipe
	for _, r := range s.recipes {
		if r.DisplayName == recipeName {
			r.Active = enabled
		}
	}

	if !enabled {
		// Remove all producers using this recipe
		for _, p := range s.producers {
			f, ok := p.(*factory.Factory)
			if ok && f.Name == recipeName {
				f.Remove()
			}
		}
	}

	return s.Recipes(l)
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

func (s *State) moveProducers(l *slog.Logger) {
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
	resources := make([]statehttp.Resource, 0)
	factories := make([]statehttp.Factory, 0)
	transports := make([]statehttp.Transport, 0)
	sinks := make([]statehttp.Sink, 0)
	for _, p := range s.producers {
		switch producer := p.(type) {
		case *storyresources.Resource:
			active := true

			newSales := make([]*production.Contract, 0)
			for _, sale := range producer.Sales {
				if !sale.Cancelled {
					newSales = append(newSales, sale)
				}
			}
			producer.Sales = newSales
			if len(producer.Sales) == 0 {
				active = false
			}

			// List resource products
			products := make([]string, 0)
			for _, product := range producer.Products() {
				products = append(products, product.Name)
			}
			if len(products) != 1 {
				panic(fmt.Sprintf("resource %s has %d products", producer.PrettyPrint(), len(products)))
			}

			// Append factory with list of products
			profitability := producer.Profitability()
			if math.IsNaN(profitability) {
				profitability = 0
			}

			numContracts := len(producer.ContractsIn())

			// Create the resource object for sending.
			resource := statehttp.Resource{
				Location: statehttp.Location{
					X: producer.Location().X,
					Y: producer.Location().Y,
				},
				Recipe:        producer.Production.Name + fmt.Sprintf(" (%d)", numContracts),
				Product:       products[0],
				Profitability: profitability,
				Active:        active,
			}

			resources = append(resources, resource)
		case *factory.Factory:
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

			numContracts := len(producer.ContractsIn())

			// Create the factory object for sending.
			factory := statehttp.Factory{
				Location: statehttp.Location{
					X: producer.Location().X,
					Y: producer.Location().Y,
				},
				Recipe:        producer.Name + fmt.Sprintf(" (%d)", numContracts),
				Products:      products,
				Profitability: profitability,
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
		case *sink.Sink:
			profitability := producer.Profitability()

			label := producer.Name + fmt.Sprintf(" Sink (%.2f)", profitability)
			if profitability == math.MaxFloat64 {
				label = producer.Name + " Sink (max)"
			}

			sink := statehttp.Sink{
				Location: statehttp.Location{
					X: producer.Location().X,
					Y: producer.Location().Y,
				},
				Label: label,
			}
			sinks = append(sinks, sink)

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
	}

	bounds := statehttp.Bounds{
		Xmin: s.xmin,
		Xmax: s.xmax,
		Ymin: s.ymin,
		Ymax: s.ymax,
	}

	return statehttp.State{
		Resources:  resources,
		Factories:  factories,
		Transports: transports,
		Sinks:      sinks,
		Tick:       s.tick,
		Running:    s.cancel != nil,
		Bounds:     bounds,
	}
}
