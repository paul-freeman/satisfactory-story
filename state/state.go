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

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/market"
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
	// book is the per-tick order book: rebuilt from live producer state
	// by publishOrders, crossed by matchOrders. Persisted on State so
	// later phases of the same tick (spawning, renegotiation, price
	// adjustment, the wire format) can read post-matching residuals.
	book *market.Book
	// lastTrade remembers the most recent traded unit price per product,
	// used to estimate input costs for products with no current ask.
	lastTrade map[string]float64
	ledger    *tradeLedger

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
	s.book = market.NewBook()
	s.lastTrade = make(map[string]float64)
	s.ledger = &tradeLedger{}

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

	// Physical production first, then discovery: the book is rebuilt
	// from live stock and crossed, so every later mechanism this tick
	// (moving, spawning, solvency, price adjustment) sees post-trade
	// reality.
	s.produceGoods(l)
	s.publishOrders(l)
	s.matchOrders(l)
	s.moveProducers(l)
	if s.randSrc.Float64() < spawnProbabilityPerTick {
		s.spawnNewProducer(l)
	}
	s.applySolvency(l)
	s.adjustPrices(l)
	s.ledger.prune(s.tick, tradeMemoryTicks)
	for _, p := range s.producers {
		if f, ok := p.(*factory.Factory); ok {
			f.PruneTrades(s.tick, tradeMemoryTicks)
		}
	}

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
			ID:   recipe.ID(),
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

func (s *State) SetRecipe(l *slog.Logger, recipeID string, enabled bool) []statehttp.Recipe {
	// Find recipe
	for _, r := range s.recipes {
		if r.ID() == recipeID {
			r.Active = enabled
		}
	}

	if !enabled {
		// Remove all producers using this recipe
		for _, p := range s.producers {
			f, ok := p.(*factory.Factory)
			if ok && f.RecipeClass == recipeID {
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

// shortageWireLimit caps how many entries toHTTP reports, so the frontend
// panel doesn't get flooded once dozens of products have small residual
// shortages.
const shortageWireLimit = 20

// shortagesForWire reports unmet demand as it actually exists in the
// economy: the post-matching residual bid volume per product.
func (s *State) shortagesForWire() []statehttp.Shortage {
	totals := make(map[string]float64)
	prices := make(map[string]float64)
	for _, product := range s.book.Products() {
		for _, bid := range s.book.Bids(product) {
			if bid.Remaining <= production.RateEpsilon {
				continue
			}
			totals[product] += bid.Remaining
			if bid.UnitPrice > prices[product] {
				prices[product] = bid.UnitPrice
			}
		}
	}
	// Build from s.book.Products() (already sorted), not by ranging the
	// totals map directly, so the pre-sort order is deterministic and a
	// stable sort below can't leave amount-ties in map-iteration order.
	shortages := make([]statehttp.Shortage, 0, len(totals))
	for _, product := range s.book.Products() {
		amount, ok := totals[product]
		if !ok {
			continue
		}
		shortages = append(shortages, statehttp.Shortage{
			Product: product,
			Amount:  amount,
			Price:   prices[product],
		})
	}
	sort.SliceStable(shortages, func(i, j int) bool {
		return shortages[i].Amount > shortages[j].Amount
	})
	if len(shortages) > shortageWireLimit {
		shortages = shortages[:shortageWireLimit]
	}
	return shortages
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
			if math.IsNaN(profitability) || math.IsInf(profitability, 0) {
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
			if math.IsNaN(profitability) || math.IsInf(profitability, 0) {
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
				Cash:          producer.Cash(),
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
		Shortages:  s.shortagesForWire(),
		Tick:       s.tick,
		Running:    s.cancel != nil,
		Bounds:     bounds,
	}
}
