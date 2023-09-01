package factory

import (
	"fmt"
	"slices"

	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/products"
)

type factory struct {
	name     string
	loc      point.Point
	input    products.Products
	output   products.Products
	duration float64
	profits  float64
}

func New(
	name string,
	loc point.Point,
	input products.Products,
	output products.Products,
	duration float64,
) *factory {
	return &factory{
		name:     name,
		loc:      loc,
		input:    input,
		output:   output,
		duration: duration,
	}
}

// IsMovable implements producer.
func (f *factory) IsMovable() bool {
	return true
}

// IsRemovable implements producer.
func (f *factory) IsRemovable() bool {
	return true
}

// Location implements producer.
func (f *factory) Location() point.Point {
	return f.loc
}

// Name implements producer.
func (f *factory) Name() string {
	return f.name
}

// HasProduct implements producer.
func (f *factory) HasProduct(p products.Product) bool {
	return slices.Contains(f.output, p)
}

// Products implements producer.
func (f *factory) Products() products.Products {
	return f.output
}

// Profit implements producer.
func (f *factory) Profit() float64 {
	return f.profits
}

// String implements producer.
func (f *factory) String() string {
	return fmt.Sprintf("%s @ %s", f.name, f.loc.String())
}
