package factory

import (
	"fmt"
	"slices"

	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
	"github.com/paul-freeman/satisfactory-story/recipes"
)

type Factory struct {
	Name string
	loc  point.Point

	input    production.Products
	output   production.Products
	duration float64

	purchases []*production.Contract
	sales     []*production.Contract
}

func New(
	name string,
	loc point.Point,
	input production.Products,
	output production.Products,
) *Factory {
	return &Factory{
		Name:      name,
		loc:       loc,
		input:     input,
		output:    output,
		purchases: make([]*production.Contract, 0),
		sales:     make([]*production.Contract, 0),
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
	for _, sale := range f.sales {
		sale.Cancel()
	}
	for _, purchase := range f.purchases {
		purchase.Cancel()
	}
	return nil
}

// Location implements producer.
func (f *Factory) Location() point.Point {
	return f.loc
}

// SalesPriceFor is the price of a sale.
//
// For a factory, this is the sum of the purchase prices plus the transport
// cost. All of this is marked up by 50%
func (f *Factory) SalesPriceFor(order production.Production, transportCost float64) float64 {
	purchaseCosts := 0.0
	for _, purchase := range f.purchases {
		if !purchase.Cancelled {
			purchaseCosts += purchase.ProductCost
		}
	}
	return (purchaseCosts + transportCost) * 1.50 // 50% profit
}

// HasCapacityFor implements producer.
func (f *Factory) HasCapacityFor(order production.Production) error {
	if order.Rate <= 0 {
		return fmt.Errorf("production rate must be positive")
	}
	if !slices.Contains(f.output, order) {
		return fmt.Errorf("factory %s cannot produce %s", f.String(), order.String())
	}
	return nil
}

// Products implements producer.
func (f *Factory) Products() production.Products {
	return f.output
}

// Profit implements producer.
func (f *Factory) Profit() float64 {
	profit := 0.0

	// Review sales
	newSales := make([]*production.Contract, 0)
	for _, sale := range f.sales {
		if !sale.Cancelled {
			profit += sale.TransportCost
			newSales = append(newSales, sale)
		}
	}
	f.sales = newSales

	// Review purchases
	newPurchases := make([]*production.Contract, 0)
	for _, purchase := range f.purchases {
		if !purchase.Cancelled {
			profit -= purchase.TransportCost
			newPurchases = append(newPurchases, purchase)
		}
	}
	f.purchases = newPurchases

	return profit
}

func (f *Factory) Profitability() float64 {
	income := 0.0
	expenses := 0.0
	for _, sale := range f.sales {
		if !sale.Cancelled {
			income += sale.ProductCost
			expenses += sale.TransportCost
		}
	}
	for _, purchase := range f.purchases {
		if !purchase.Cancelled {
			expenses += purchase.ProductCost
			expenses += purchase.TransportCost
		}
	}
	return income / expenses
}

// String implements producer.
func (f *Factory) String() string {
	return fmt.Sprintf("%s [%s]+>[%s]", f.Name, f.input.String(), f.output.String())
}

// SignAsBuyer implements production.Producer.
func (f *Factory) SignAsBuyer(contract *production.Contract) error {
	f.purchases = append(f.purchases, contract)
	return nil
}

// SignAsSeller implements production.Producer.
func (f *Factory) SignAsSeller(contract *production.Contract) error {
	f.sales = append(f.sales, contract)
	return nil
}

// ContractsIn implements production.Producer.
func (f *Factory) ContractsIn() []*production.Contract {
	return f.purchases
}

// TryMove implements production.Producer.
func (f *Factory) TryMove() bool {
	up := f.loc.Up()
	down := f.loc.Down()
	left := f.loc.Left()
	right := f.loc.Right()

	costsHere := f.transportCostsAt(f.loc)
	costsUp := f.transportCostsAt(up)
	costsDown := f.transportCostsAt(down)
	costsLeft := f.transportCostsAt(left)
	costsRight := f.transportCostsAt(right)
	if costsUp < costsHere && costsUp <= costsDown && costsUp <= costsLeft && costsUp <= costsRight {
		f.MoveTo(up)
		return true
	}
	if costsUp < costsHere && costsDown <= costsUp && costsDown <= costsLeft && costsDown <= costsRight {
		f.MoveTo(down)
		return true
	}
	if costsUp < costsHere && costsLeft <= costsUp && costsLeft <= costsDown && costsLeft <= costsRight {
		f.MoveTo(left)
		return true
	}
	if costsUp < costsHere && costsRight <= costsUp && costsRight <= costsDown && costsRight <= costsLeft {
		f.MoveTo(right)
		return true
	}
	return false
}

var _ production.Producer = (*Factory)(nil)

func (f *Factory) transportCostsAt(p point.Point) float64 {
	c := 0.0
	for _, sale := range f.sales {
		c += recipes.TransportCost(sale.Buyer.Location(), p)
	}
	for _, purchase := range f.purchases {
		c += recipes.TransportCost(p, purchase.Seller.Location())
	}
	return c
}

func (f *Factory) MoveTo(loc point.Point) {
	f.loc = loc
	for _, sale := range f.sales {
		sale.TransportCost = recipes.TransportCost(loc, sale.Buyer.Location())
	}
	for _, purchase := range f.purchases {
		purchase.TransportCost = recipes.TransportCost(purchase.Seller.Location(), loc)
	}
	return
}
