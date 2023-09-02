package factory

import (
	"fmt"
	"slices"

	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
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

// SalesPriceFor implements producer.
func (f *Factory) SalesPriceFor(order production.Production, transportCost float64) float64 {
	return 0.0
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

// String implements producer.
func (f *Factory) String() string {
	return fmt.Sprintf("%s @ %s", f.Name, f.loc.String())
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

var _ production.Producer = (*Factory)(nil)
