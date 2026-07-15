package resources

import (
	_ "embed"
	"testing"

	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
)

func Test_Resource_ask_price_defaults_and_sets(t *testing.T) {
	r := &Resource{Production: production.Production{Name: "Ore", Rate: 100}}

	if got := r.AskPriceFor("Ore"); got != production.DefaultUnitPrice {
		t.Errorf("unquoted ask should default to %f, got %f", production.DefaultUnitPrice, got)
	}
	r.SetAskPrice("Ore", 0.5)
	if got := r.AskPriceFor("Ore"); got != 0.5 {
		t.Errorf("got %f, want 0.5", got)
	}
	if got := r.AskPriceFor("NotMyProduct"); got != 0 {
		t.Errorf("asking about a foreign product should return 0, got %f", got)
	}
}

func Test_Resource_ProduceTick(t *testing.T) {
	r := &Resource{
		Production: production.Production{Name: "OreIron", Rate: 2},
		Loc:        point.Point{X: 0, Y: 0},
	}
	r.ProduceTick(3) // cap = 6 units
	if r.Stock != 2 {
		t.Fatalf("stock after 1 tick = %v, want 2", r.Stock)
	}
	r.ProduceTick(3)
	r.ProduceTick(3)
	r.ProduceTick(3) // would be 8, clamps at cap 6
	if r.Stock != 6 {
		t.Fatalf("stock at cap = %v, want 6", r.Stock)
	}
}
