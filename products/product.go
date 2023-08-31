package products

import "fmt"

type Product struct {
	name     string
	amount   float64
	duration float64
}

func New(name string, amount float64, duration float64) Product {
	return Product{
		name:     name,
		amount:   amount,
		duration: duration,
	}
}

func (p Product) Name() string {
	return p.name
}

func (p Product) String() string {
	return fmt.Sprintf("%s (%.2f)", p.name, p.amount/p.duration)
}
