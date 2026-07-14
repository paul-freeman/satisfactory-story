package sink

import (
	"fmt"
	"math"

	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
	"github.com/paul-freeman/satisfactory-story/recipes"
)

type Sink struct {
	Name      string
	Loc       point.Point
	Input     production.Products
	Purchases []*production.Contract
	// BidUnitPrice is the standing per-unit price this sink offers for
	// every product it wants. Goal sinks bid high -- their demand is the
	// engine of the whole economy.
	BidUnitPrice float64
	// Delivered counts units actually received, by product. The
	// space-elevator milestone is TotalDelivered() > 0 on a goal sink.
	Delivered production.Inventory
}

func New(
	name string,
	loc point.Point,
	input production.Products,
	bidUnitPrice float64,
) *Sink {
	return &Sink{
		Name:         name,
		Loc:          loc,
		Input:        input,
		Purchases:    make([]*production.Contract, 0),
		BidUnitPrice: bidUnitPrice,
		Delivered:    make(production.Inventory),
	}
}

// IsMovable implements producer.
func (f *Sink) IsMovable() bool {
	return true
}

// IsRemovable implements producer.
func (f *Sink) IsRemovable() bool {
	return false
}

func (f *Sink) Remove() error {
	return fmt.Errorf("cannot remove sink")
}

// Location implements producer.
func (f *Sink) Location() point.Point {
	return f.Loc
}

// RemainingCapacityFor implements production.Producer. Sinks never sell
// anything, so they always report zero remaining capacity.
func (f *Sink) RemainingCapacityFor(_ string) float64 {
	return 0
}

// HasCapacityFor implements producer.
func (f *Sink) HasCapacityFor(order production.Production) error {
	return fmt.Errorf("sink %s cannot produce anything", f.String())
}

// Products implements producer.
func (f *Sink) Products() production.Products {
	return f.Input
}

// Profit implements producer.
func (f *Sink) Profit() float64 {
	profit := 0.0

	// Review purchases
	newPurchases := make([]*production.Contract, 0)
	for _, purchase := range f.Purchases {
		if !purchase.Cancelled {
			profit -= purchase.TransportCost
			newPurchases = append(newPurchases, purchase)
		}
	}
	f.Purchases = newPurchases

	return profit
}

func (f *Sink) Profitability() float64 {
	income := 1.0 // must have some income, or profitability is always 0
	expenses := 0.0

	for _, purchase := range f.Purchases {
		if !purchase.Cancelled {
			expenses += purchase.ProductCost
			expenses += purchase.TransportCost
		}
	}

	if expenses == 0 {
		return math.MaxFloat64
	}

	return income / expenses
}

// Cash reports an effectively infinite balance -- sinks are never removed
// for insolvency.
func (f *Sink) Cash() float64 {
	return math.MaxFloat64
}

// String implements producer.
func (f *Sink) String() string {
	return fmt.Sprintf("%s [%s]", f.Name, f.Input.Key())
}

// SignAsBuyer implements production.Producer.
func (f *Sink) SignAsBuyer(contract *production.Contract) error {
	f.Purchases = append(f.Purchases, contract)
	return nil
}

// SignAsSeller implements production.Producer.
func (f *Sink) SignAsSeller(contract *production.Contract) error {
	return fmt.Errorf("sink %s cannot sell anything", f.String())
}

// ContractsIn implements production.Producer.
func (f *Sink) ContractsIn() []*production.Contract {
	return f.Purchases
}

// RecordDelivery counts qty units of the named product as received.
func (f *Sink) RecordDelivery(name string, qty float64) {
	f.Delivered.Add(name, qty)
}

// TotalDelivered is the total units ever received across all products.
func (f *Sink) TotalDelivered() float64 {
	total := 0.0
	for _, qty := range f.Delivered {
		total += qty
	}
	return total
}

func (f *Sink) Move() error {
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
		f.moveTo(f.Loc.Up(1))
		return nil
	}
	if costsDown <= costsHere && costsDown <= costsUp && costsDown <= costsLeft && costsDown <= costsRight {
		f.moveTo(f.Loc.Down(1))
		return nil
	}
	if costsLeft <= costsHere && costsLeft <= costsUp && costsLeft <= costsDown && costsLeft <= costsRight {
		f.moveTo(f.Loc.Left(1))
		return nil
	}
	if costsRight <= costsHere && costsRight <= costsUp && costsRight <= costsDown && costsRight <= costsLeft {
		f.moveTo(f.Loc.Right(1))
		return nil
	}

	return nil
}

var _ production.Producer = (*Sink)(nil)

func (f *Sink) transportCostsAt(p point.Point) float64 {
	c := 0.0
	for _, purchase := range f.Purchases {
		c += recipes.TransportCost(p, purchase.Seller.Location())
	}
	return c
}

func (f *Sink) moveTo(loc point.Point) {
	f.Loc = loc
	for _, purchase := range f.Purchases {
		purchase.TransportCost = recipes.TransportCost(purchase.Seller.Location(), loc)
	}
	return
}
