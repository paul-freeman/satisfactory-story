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
	rate := 0.0
	if p.duration != 0 {
		rate = p.amount / p.duration
	}
	return fmt.Sprintf("%s (%.2f)", p.name, rate)
}
