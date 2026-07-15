package sink

import (
	"fmt"

	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
)

type Sink struct {
	Name  string
	Loc   point.Point
	Input production.Products
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
		BidUnitPrice: bidUnitPrice,
		Delivered:    make(production.Inventory),
	}
}

// Location implements producer.
func (f *Sink) Location() point.Point {
	return f.Loc
}

// Products implements producer.
func (f *Sink) Products() production.Products {
	return f.Input
}

// String implements producer.
func (f *Sink) String() string {
	return fmt.Sprintf("%s [%s]", f.Name, f.Input.Key())
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

var _ production.Producer = (*Sink)(nil)
