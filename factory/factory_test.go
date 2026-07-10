package factory

import (
	"testing"

	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
)

func Test_Factory_Cash(t *testing.T) {
	f := New("Test Factory", point.Point{X: 0, Y: 0}, 0, production.Products{}, production.Products{}, 250)
	if f.Cash() != 250 {
		t.Errorf("got %f, want 250", f.Cash())
	}
}

func Test_Factory_RemainingCapacityFor(t *testing.T) {
	output := production.Products{{Name: "Widget", Rate: 10}}
	f := New("Test Factory", point.Point{X: 0, Y: 0}, 0, production.Products{}, output, 0)

	if got := f.RemainingCapacityFor("Widget"); got != 10 {
		t.Errorf("got %f, want 10", got)
	}
	if got := f.RemainingCapacityFor("NotProduced"); got != 0 {
		t.Errorf("got %f, want 0", got)
	}

	f.Sales = append(f.Sales, &production.Contract{
		Order: production.Production{Name: "Widget", Rate: 6},
	})
	if got := f.RemainingCapacityFor("Widget"); got != 4 {
		t.Errorf("got %f, want 4", got)
	}

	if err := f.HasCapacityFor(production.Production{Name: "Widget", Rate: 4}); err != nil {
		t.Errorf("expected capacity for 4, got error: %v", err)
	}
	if err := f.HasCapacityFor(production.Production{Name: "Widget", Rate: 5}); err == nil {
		t.Errorf("expected an error oversubscribing capacity, got nil")
	}
}

func Test_Factory_SalesPriceFor(t *testing.T) {
	f := New("Test Factory", point.Point{X: 0, Y: 0}, 0, production.Products{}, production.Products{}, 0)
	f.Purchases = append(f.Purchases, &production.Contract{
		ProductCost:   10,
		TransportCost: 2,
	})
	// (10 + 2 input cost + 3 outbound transport) * 1.5 = 22.5
	got := f.SalesPriceFor(production.Production{Name: "Widget", Rate: 1}, 3)
	want := 22.5
	if got != want {
		t.Errorf("got %f, want %f", got, want)
	}
}

func Test_Factory_Profit(t *testing.T) {
	f := New("Test Factory", point.Point{X: 0, Y: 0}, 0, production.Products{}, production.Products{}, 0)
	f.Sales = append(f.Sales, &production.Contract{
		ProductCost:   30,
		TransportCost: 5,
	})
	f.Purchases = append(f.Purchases, &production.Contract{
		ProductCost:   10,
		TransportCost: 2,
	})
	// (30 - 5) - (10 + 2) = 13
	got := f.Profit()
	want := 13.0
	if got != want {
		t.Errorf("got %f, want %f", got, want)
	}
}

func Test_Factory_Profit_ignores_cancelled_contracts(t *testing.T) {
	f := New("Test Factory", point.Point{X: 0, Y: 0}, 0, production.Products{}, production.Products{}, 0)
	f.Sales = append(f.Sales, &production.Contract{
		ProductCost:   30,
		TransportCost: 5,
		Cancelled:     true,
	})
	f.Purchases = append(f.Purchases, &production.Contract{
		ProductCost:   10,
		TransportCost: 2,
	})
	// cancelled sale contributes nothing: 0 - (10 + 2) = -12
	got := f.Profit()
	want := -12.0
	if got != want {
		t.Errorf("got %f, want %f", got, want)
	}
	if len(f.Sales) != 0 {
		t.Errorf("cancelled sale should have been pruned, got %d sales left", len(f.Sales))
	}
}
