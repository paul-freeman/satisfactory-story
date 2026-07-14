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

	Purchases []*production.Contract
	Sales     []*production.Contract

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
		Purchases:   make([]*production.Contract, 0),
		Sales:       make([]*production.Contract, 0),
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

// Remove implements producer.
func (f *Factory) Remove() error {
	for _, sale := range f.Sales {
		sale.Cancel()
	}
	for _, purchase := range f.Purchases {
		purchase.Cancel()
	}
	return nil
}

// Location implements producer.
func (f *Factory) Location() point.Point {
	return f.Loc
}

// RemainingCapacityFor returns how much of the given product this factory
// could still sell, after subtracting rate already committed to active
// sales. Returns 0 if the factory doesn't produce that product.
func (f *Factory) RemainingCapacityFor(name string) float64 {
	var rate float64
	found := false
	for _, output := range f.Output {
		if output.Name == name {
			rate = output.Rate
			found = true
			break
		}
	}
	if !found {
		return 0
	}
	for _, sale := range f.Sales {
		if sale.Cancelled || sale.Order.Name != name {
			continue
		}
		rate -= sale.Order.Rate
	}
	if rate < 0 {
		return 0
	}
	return rate
}

// HasCapacityFor implements producer.
func (f *Factory) HasCapacityFor(order production.Production) error {
	if order.Rate <= 0 {
		return fmt.Errorf("production rate must be positive")
	}
	if order.Rate > f.RemainingCapacityFor(order.Name) {
		return fmt.Errorf("factory %s cannot produce %s at rate %f", f.String(), order.Key(), order.Rate)
	}
	return nil
}

// Products implements producer.
func (f *Factory) Products() production.Products {
	return f.Output
}

// Profit implements producer. Revenue from a sale is its ProductCost minus
// the TransportCost the factory pays to ship it; the cost of a purchase is
// its ProductCost plus the TransportCost paid to receive it. This mirrors
// resources.Resource.Profit's accounting.
func (f *Factory) Profit() float64 {
	profit := 0.0

	newSales := make([]*production.Contract, 0)
	for _, sale := range f.Sales {
		if !sale.Cancelled {
			profit += sale.ProductCost - sale.TransportCost
			newSales = append(newSales, sale)
		}
	}
	f.Sales = newSales

	newPurchases := make([]*production.Contract, 0)
	for _, purchase := range f.Purchases {
		if !purchase.Cancelled {
			profit -= purchase.ProductCost + purchase.TransportCost
			newPurchases = append(newPurchases, purchase)
		}
	}
	f.Purchases = newPurchases

	return profit
}

func (f *Factory) Profitability() float64 {
	income := 0.0
	expenses := 0.0
	for _, sale := range f.Sales {
		if !sale.Cancelled {
			income += sale.ProductCost
			expenses += sale.TransportCost
		}
	}
	for _, purchase := range f.Purchases {
		if !purchase.Cancelled {
			expenses += purchase.ProductCost
			expenses += purchase.TransportCost
		}
	}
	return income / expenses
}

// String implements producer.
func (f *Factory) String() string {
	return fmt.Sprintf("%s [%s]+>[%s]", f.Name, f.Input.Key(), f.Output.Key())
}

// SignAsBuyer implements production.Producer.
func (f *Factory) SignAsBuyer(contract *production.Contract) error {
	f.Purchases = append(f.Purchases, contract)
	return nil
}

// SignAsSeller implements production.Producer.
func (f *Factory) SignAsSeller(contract *production.Contract) error {
	f.Sales = append(f.Sales, contract)
	return nil
}

// ContractsIn implements production.Producer.
func (f *Factory) ContractsIn() []*production.Contract {
	return f.Purchases
}

func (f *Factory) Move() error {
	if len(f.Purchases) == 0 && len(f.Sales) == 0 {
		// No contracts means no transport-cost gradient to climb --
		// transportCostsAt is 0 in every direction, so the tie-break
		// below would otherwise always pick the same neighbor and the
		// factory would march off the map forever while it waits for
		// its first bid to fill.
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

func (f *Factory) transportCostsAt(p point.Point) float64 {
	c := 0.0
	for _, sale := range f.Sales {
		c += recipes.TransportCost(sale.Buyer.Location(), p)
	}
	for _, purchase := range f.Purchases {
		c += recipes.TransportCost(p, purchase.Seller.Location())
	}
	return c
}

func (f *Factory) moveTo(loc point.Point) {
	f.Loc = loc
	for _, sale := range f.Sales {
		sale.TransportCost = recipes.TransportCost(loc, sale.Buyer.Location())
	}
	for _, purchase := range f.Purchases {
		purchase.TransportCost = recipes.TransportCost(purchase.Seller.Location(), loc)
	}
	return
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

// UnmetInputRate returns how much of the named input's required rate is
// not yet covered by active purchase contracts.
func (f *Factory) UnmetInputRate(name string) float64 {
	required := 0.0
	for _, input := range f.Input {
		if input.Name == name {
			required = input.Rate
			break
		}
	}
	for _, purchase := range f.Purchases {
		if !purchase.Cancelled && purchase.Order.Name == name {
			required -= purchase.Order.Rate
		}
	}
	if required < 0 {
		return 0
	}
	return required
}

// Producing reports whether every input is fully covered by active
// purchase contracts. A factory that is not producing publishes no asks
// and sells nothing -- it is idle, waiting for its input bids to fill.
func (f *Factory) Producing() bool {
	for _, input := range f.Input {
		if f.UnmetInputRate(input.Name) > production.RateEpsilon {
			return false
		}
	}
	return true
}

// MarginalUnitCost is the factory's current per-unit cost basis: what it
// pays per tick (active purchases including transport, plus upkeep)
// spread over its total output rate. Used as the floor when the market
// loop lowers this factory's ask prices -- it never knowingly sells
// below cost.
func (f *Factory) MarginalUnitCost(upkeep float64) float64 {
	cost := upkeep
	for _, purchase := range f.Purchases {
		if !purchase.Cancelled {
			cost += purchase.ProductCost + purchase.TransportCost
		}
	}
	totalRate := 0.0
	for _, output := range f.Output {
		totalRate += output.Rate
	}
	if totalRate <= production.RateEpsilon {
		return cost
	}
	return cost / totalRate
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
