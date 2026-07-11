package state

import (
	"log/slog"
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
			ProductCost: 1, // cheap enough that seed capital covers it comfortably
		})

		s := &State{producers: []production.Producer{f}}
		s.applySolvency(testLogger())

		if len(s.producers) != 1 {
			t.Errorf("expected the solvent factory to survive, got %d producers left", len(s.producers))
		}
	})

	t.Run("removes a factory missing an input contract", func(t *testing.T) {
		f := factory.New("Test", "Recipe_Test_C", point.Point{X: 0, Y: 0}, 0,
			production.Products{{Name: "Input", Rate: 1}},
			production.Products{{Name: "Output", Rate: 1}}, 100)
		// no purchase signed for the required "Input" -- incomplete

		s := &State{producers: []production.Producer{f}}
		s.applySolvency(testLogger())

		if len(s.producers) != 0 {
			t.Errorf("expected the incomplete factory to be removed, got %d producers left", len(s.producers))
		}
	})
}
