package factory

import (
	"fmt"
	"math"

	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
	"github.com/paul-freeman/satisfactory-story/recipes"
)

type Factory struct {
	Name        string
	RecipeClass string
	Loc         point.Point
	CreatedTick int

	Input    production.Products
	Output   production.Products
	Duration float64

	// AskPrices holds this factory's standing per-unit sale price for
	// each output; BidPrices the per-unit price it currently offers for
	// each input. Both are adjusted by the market loop (state/prices.go)
	// -- they are the only market state that persists between ticks.
	AskPrices map[string]float64
	BidPrices map[string]float64

	// InputStock and OutputStock hold real goods. Production consumes
	// from InputStock into OutputStock; trades move units between a
	// seller's OutputStock and a buyer's InputStock.
	InputStock  production.Inventory
	OutputStock production.Inventory
	// ProducedLastTick records whether the recipe ran at all last tick
	// (observability, not a contractual state).
	ProducedLastTick bool
	// TickInputSpend / TickRevenue accumulate this tick's trade flows;
	// the solvency step folds them into the EMAs and zeroes them.
	TickInputSpend float64
	TickRevenue    float64
	AvgInputSpend  float64
	AvgRevenue     float64
	// RecentTrades is this factory's own memory of who it traded with,
	// used for the movement gradient.
	RecentTrades []TradeMemory

	production.Wallet
}

func New(
	name string,
	recipeClass string,
	loc point.Point,
	tick int,
	input production.Products,
	output production.Products,
	seedCapital float64,
) *Factory {
	return &Factory{
		Name:        name,
		RecipeClass: recipeClass,
		Loc:         loc,
		CreatedTick: tick,
		Input:       input,
		Output:      output,
		AskPrices:   make(map[string]float64),
		BidPrices:   make(map[string]float64),
		InputStock:  make(production.Inventory),
		OutputStock: make(production.Inventory),
		Wallet:      production.NewWallet(seedCapital),
	}
}

// IsMovable implements producer.
func (f *Factory) IsMovable() bool {
	return true
}

// IsRemovable implements producer.
func (f *Factory) IsRemovable() bool {
	return true
}

// Location implements producer.
func (f *Factory) Location() point.Point {
	return f.Loc
}

// Products implements producer.
func (f *Factory) Products() production.Products {
	return f.Output
}

// String implements producer.
func (f *Factory) String() string {
	return fmt.Sprintf("%s [%s]+>[%s]", f.Name, f.Input.Key(), f.Output.Key())
}

func (f *Factory) Move() error {
	if len(f.RecentTrades) == 0 {
		// No trades means no transport-cost gradient to climb -- the
		// tie-break below would otherwise always pick the same neighbor
		// and the factory would march off the map forever while it
		// waits for its first trade.
		return nil
	}

	up := f.Loc.Up(1)
	down := f.Loc.Down(1)
	left := f.Loc.Left(1)
	right := f.Loc.Right(1)

	costsHere := f.transportCostsAt(f.Loc)
	costsUp := f.transportCostsAt(up)
	costsDown := f.transportCostsAt(down)
	costsLeft := f.transportCostsAt(left)
	costsRight := f.transportCostsAt(right)

	if costsUp <= costsHere && costsUp <= costsDown && costsUp <= costsLeft && costsUp <= costsRight {
		f.moveTo(f.Loc.Up(max(1, min(100, int(100000*(costsHere-costsUp)))))) // TODO: make this a function of the distance
		return nil
	}
	if costsDown <= costsHere && costsDown <= costsUp && costsDown <= costsLeft && costsDown <= costsRight {
		f.moveTo(f.Loc.Down(max(1, min(100, int(100000*(costsHere-costsDown)))))) // TODO: make this a function of the distance
		return nil
	}
	if costsLeft <= costsHere && costsLeft <= costsUp && costsLeft <= costsDown && costsLeft <= costsRight {
		f.moveTo(f.Loc.Left(max(1, min(100, int(100000*(costsHere-costsLeft)))))) // TODO: make this a function of the distance
		return nil
	}
	if costsRight <= costsHere && costsRight <= costsUp && costsRight <= costsDown && costsRight <= costsLeft {
		f.moveTo(f.Loc.Right(max(1, min(100, int(100000*(costsHere-costsRight)))))) // TODO: make this a function of the distance
		return nil
	}

	return nil
}

var _ production.Producer = (*Factory)(nil)

// transportCostsAt scores a location against the factory's remembered
// trade partners, weighted by traded quantity.
func (f *Factory) transportCostsAt(p point.Point) float64 {
	c := 0.0
	for _, tr := range f.RecentTrades {
		c += tr.Qty * recipes.UnitTransportCost(p, tr.Other)
	}
	return c
}

func (f *Factory) moveTo(loc point.Point) {
	f.Loc = loc
}

// AskPriceFor returns the standing per-unit sale price for the named
// product, defaulting on first quote.
func (f *Factory) AskPriceFor(name string) float64 {
	if f.AskPrices == nil {
		f.AskPrices = make(map[string]float64)
	}
	price, ok := f.AskPrices[name]
	if !ok {
		price = production.DefaultUnitPrice
		f.AskPrices[name] = price
	}
	return price
}

// SetAskPrice records a new standing per-unit sale price.
func (f *Factory) SetAskPrice(name string, price float64) {
	if f.AskPrices == nil {
		f.AskPrices = make(map[string]float64)
	}
	f.AskPrices[name] = price
}

// BidPriceFor returns the standing per-unit purchase offer for the named
// input, defaulting on first quote.
func (f *Factory) BidPriceFor(name string) float64 {
	if f.BidPrices == nil {
		f.BidPrices = make(map[string]float64)
	}
	price, ok := f.BidPrices[name]
	if !ok {
		price = production.DefaultUnitPrice
		f.BidPrices[name] = price
	}
	return price
}

// SetBidPrice records a new standing per-unit purchase offer.
func (f *Factory) SetBidPrice(name string, price float64) {
	if f.BidPrices == nil {
		f.BidPrices = make(map[string]float64)
	}
	f.BidPrices[name] = price
}

// TradeMemory is one remembered trade endpoint: where the counterparty
// was and how much moved. Movement hill-climbs on these.
type TradeMemory struct {
	Tick  int
	Other point.Point
	Qty   float64
}

// ProduceTick runs up to one tick of the recipe, limited by input stock
// and by room left under the output cap (outputCapTicks x output rate
// per product). Returns the fraction of a full tick actually run.
func (f *Factory) ProduceTick(outputCapTicks float64) float64 {
	frac := 1.0
	for _, in := range f.Input {
		if in.Rate <= production.RateEpsilon {
			continue
		}
		frac = math.Min(frac, f.InputStock.Get(in.Name)/in.Rate)
	}
	for _, out := range f.Output {
		if out.Rate <= production.RateEpsilon {
			continue
		}
		room := out.Rate*outputCapTicks - f.OutputStock.Get(out.Name)
		frac = math.Min(frac, room/out.Rate)
	}
	frac = math.Max(0, math.Min(1, frac))
	if frac <= production.RateEpsilon {
		f.ProducedLastTick = false
		return 0
	}
	for _, in := range f.Input {
		f.InputStock.Take(in.Name, in.Rate*frac)
	}
	for _, out := range f.Output {
		f.OutputStock.Add(out.Name, out.Rate*frac)
	}
	f.ProducedLastTick = true
	return frac
}

// Hunger is how many units of the named input the factory wants to buy
// right now: the gap between its input-stock target and what it holds.
func (f *Factory) Hunger(name string, targetTicks float64) float64 {
	for _, in := range f.Input {
		if in.Name != name {
			continue
		}
		h := in.Rate*targetTicks - f.InputStock.Get(name)
		if h < 0 {
			return 0
		}
		return h
	}
	return 0
}

// RecordTrade remembers a trade endpoint for the movement gradient.
func (f *Factory) RecordTrade(tick int, other point.Point, qty float64) {
	f.RecentTrades = append(f.RecentTrades, TradeMemory{Tick: tick, Other: other, Qty: qty})
}

// PruneTrades drops remembered trades older than memoryTicks.
func (f *Factory) PruneTrades(tick, memoryTicks int) {
	kept := f.RecentTrades[:0]
	for _, tr := range f.RecentTrades {
		if tick-tr.Tick <= memoryTicks {
			kept = append(kept, tr)
		}
	}
	f.RecentTrades = kept
}

// FoldTickFlows folds this tick's accumulated spend/revenue into the
// exponential moving averages and zeroes the accumulators. Called once
// per tick by the solvency step.
func (f *Factory) FoldTickFlows(smoothing float64) {
	f.AvgInputSpend = f.AvgInputSpend*(1-smoothing) + f.TickInputSpend*smoothing
	f.AvgRevenue = f.AvgRevenue*(1-smoothing) + f.TickRevenue*smoothing
	f.TickInputSpend = 0
	f.TickRevenue = 0
}

// StockMarginalUnitCost is the stock-world cost basis per output unit:
// smoothed input spend plus upkeep, spread over total output rate. The
// floor for ask-price decay.
func (f *Factory) StockMarginalUnitCost(upkeep float64) float64 {
	totalRate := 0.0
	for _, out := range f.Output {
		totalRate += out.Rate
	}
	if totalRate <= production.RateEpsilon {
		return upkeep
	}
	return (f.AvgInputSpend + upkeep) / totalRate
}
