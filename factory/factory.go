package factory

import (
	"fmt"

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
