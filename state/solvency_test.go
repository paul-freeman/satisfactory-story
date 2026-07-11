package state

import (
	"log/slog"
	"math"
	"os"
	"testing"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func Test_applySolvency(t *testing.T) {
	t.Run("removes a factory that has been insolvent long enough", func(t *testing.T) {
		f := factory.New("Test", "Recipe_Test_C", point.Point{X: 0, Y: 0}, 0,
			production.Products{{Name: "Input", Rate: 1}},
			production.Products{{Name: "Output", Rate: 1}}, 0)
		f.Purchases = append(f.Purchases, &production.Contract{
			Seller:      &factory.Factory{},
			Order:       production.Production{Name: "Input", Rate: 1},
			ProductCost: 100, // expensive input, no sales -- guaranteed loss every tick
		})

		s := &State{producers: []production.Producer{f}}
		for i := 0; i < insolvencyGrace+1; i++ {
			s.applySolvency(testLogger())
		}

		if len(s.producers) != 0 {
			t.Errorf("expected the bankrupt factory to be removed, got %d producers left", len(s.producers))
		}
	})

	t.Run("keeps a factory that recovers before the grace window elapses", func(t *testing.T) {
		f := factory.New("Test", "Recipe_Test_C", point.Point{X: 0, Y: 0}, 0,
			production.Products{{Name: "Input", Rate: 1}},
			production.Products{{Name: "Output", Rate: 1}}, 100)
		f.Purchases = append(f.Purchases, &production.Contract{
			Seller:      &factory.Factory{},
			Order:       production.Production{Name: "Input", Rate: 1},
			ProductCost: 1, // cheap enough that seed capital covers it comfortably
		})

		s := &State{producers: []production.Producer{f}}
		s.applySolvency(testLogger())

		if len(s.producers) != 1 {
			t.Errorf("expected the solvent factory to survive, got %d producers left", len(s.producers))
		}
	})

	t.Run("keeps an idle factory but charges upkeep", func(t *testing.T) {
		f := factory.New("Test", "Recipe_Test_C", point.Point{X: 0, Y: 0}, 0,
			production.Products{{Name: "Input", Rate: 1}},
			production.Products{{Name: "Output", Rate: 1}}, 100)
		// no purchase signed for the required "Input" -- idle, NOT culled

		s := &State{producers: []production.Producer{f}}
		s.applySolvency(testLogger())

		if len(s.producers) != 1 {
			t.Fatalf("an idle factory must survive (it is waiting for supply), got %d producers", len(s.producers))
		}
		if got := f.Wallet.Cash(); got != 100-upkeepPerTick {
			t.Errorf("expected upkeep %f charged, cash %f, got %f", upkeepPerTick, 100-upkeepPerTick, got)
		}
	})

	t.Run("cancels the sales of a factory that stops producing", func(t *testing.T) {
		f := factory.New("Test", "Recipe_Test_C", point.Point{X: 0, Y: 0}, 0,
			production.Products{{Name: "Input", Rate: 1}},
			production.Products{{Name: "Output", Rate: 1}}, 100)
		sale := &production.Contract{
			Buyer:       &factory.Factory{},
			Order:       production.Production{Name: "Output", Rate: 1},
			ProductCost: 10,
		}
		f.Sales = append(f.Sales, sale)
		// Its input contract is gone (e.g. supplier bankrupt) -- it can no
		// longer honor the sale.

		s := &State{producers: []production.Producer{f}}
		s.applySolvency(testLogger())

		if !sale.Cancelled {
			t.Error("expected the idle factory's sale to be cancelled")
		}
		// Revenue from the cancelled sale must not have been credited, and
		// an idle factory produces nothing so there is nothing to salvage.
		if got := f.Wallet.Cash(); got != 100-upkeepPerTick {
			t.Errorf("expected cash %f (upkeep only), got %f", 100-upkeepPerTick, got)
		}
	})

	t.Run("producing factory salvages unsold capacity at the floor price", func(t *testing.T) {
		f := factory.New("Test", "Recipe_Test_C", point.Point{X: 0, Y: 0}, 0,
			production.Products{{Name: "Input", Rate: 1}},
			production.Products{{Name: "Output", Rate: 4}}, 100)
		f.Purchases = append(f.Purchases, &production.Contract{
			Seller:      &factory.Factory{},
			Order:       production.Production{Name: "Input", Rate: 1},
			ProductCost: 2,
		})
		f.Sales = append(f.Sales, &production.Contract{
			Buyer:       &factory.Factory{},
			Order:       production.Production{Name: "Output", Rate: 1},
			ProductCost: 10,
		})
		// Producing; 3 of 4 output units unsold -> salvaged at the floor.

		s := &State{producers: []production.Producer{f}}
		s.applySolvency(testLogger())

		// 100 + (10 sale - 2 purchase profit) + 3*floorUnitPrice salvage - upkeep
		want := 100 + 8 + 3*floorUnitPrice - upkeepPerTick
		if got := f.Wallet.Cash(); math.Abs(got-want) > 1e-9 {
			t.Errorf("expected cash %f, got %f", want, got)
		}
	})
}
