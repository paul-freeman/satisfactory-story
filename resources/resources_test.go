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

func Test_Resource_purity_rates(t *testing.T) {
	// Purity maps to 30/60/120 units per 60s, i.e. 0.5/1.0/2.0 units per
	// tick. The original code inverted these (amount and duration were
	// swapped into production.New), so impure nodes extracted 4x faster
	// than pure ones.
	rs, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	wantByPurity := map[purity]float64{
		impure: 30.0 / 60.0,
		normal: 60.0 / 60.0,
		pure:   120.0 / 60.0,
	}
	seen := make(map[purity]bool)
	for _, r := range rs {
		want, ok := wantByPurity[r.Purity]
		if !ok {
			t.Errorf("node at %s has unknown purity %q", r.Loc.String(), r.Purity)
			continue
		}
		seen[r.Purity] = true
		if r.Production.Rate != want {
			t.Errorf("%s node at %s: rate = %v, want %v",
				r.Purity, r.Loc.String(), r.Production.Rate, want)
		}
	}
	for p := range wantByPurity {
		if !seen[p] {
			t.Errorf("no %s node found in Resource.json", p)
		}
	}
}

func Test_Resource_water_nodes_load(t *testing.T) {
	rs, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	// The 8 synthetic water nodes from the Phase 5 spec
	// (docs/superpowers/specs/2026-07-16-wallet-grounded-bids-design.md):
	// a 3x3 interior grid of the resource bounding box minus its center.
	// Expected locations use the same lat/lng scaling as New().
	expected := make(map[point.Point]bool)
	for _, c := range []struct{ lat, lng float64 }{
		{-105.91, 52.68}, {-105.91, 82.03}, {-105.91, 111.39},
		{-80.13, 52.68}, {-80.13, 111.39},
		{-54.36, 52.68}, {-54.36, 82.03}, {-54.36, 111.39},
	} {
		expected[point.Point{X: int(c.lng * 1000), Y: int(c.lat * 1000)}] = true
	}

	found := 0
	for _, r := range rs {
		if r.Production.Name != "Water" {
			continue
		}
		found++
		if r.Purity != pure {
			t.Errorf("water node at %s: purity = %v, want %v", r.Loc.String(), r.Purity, pure)
		}
		if r.Production.Rate != 120.0/60.0 {
			t.Errorf("water node at %s: rate = %v, want %v", r.Loc.String(), r.Production.Rate, 120.0/60.0)
		}
		if !expected[r.Loc] {
			t.Errorf("unexpected water node location %s", r.Loc.String())
		}
		delete(expected, r.Loc)
	}
	if found != 8 {
		t.Errorf("found %d water nodes, want 8", found)
	}
	for loc := range expected {
		t.Errorf("missing water node at %s", loc.String())
	}
}
