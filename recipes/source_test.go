package recipes

import (
	"log/slog"
	"os"
	"testing"

	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
	"github.com/paul-freeman/satisfactory-story/resources"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func Test_FindBestSeller(t *testing.T) {
	near := &resources.Resource{
		Production: production.Production{Name: "Ore", Rate: 10},
		Loc:        point.Point{X: 0, Y: 0},
	}
	far := &resources.Resource{
		Production: production.Production{Name: "Ore", Rate: 10},
		Loc:        point.Point{X: 100000, Y: 100000},
	}
	sellers := []production.Producer{far, near}
	order := production.Production{Name: "Ore", Rate: 5}

	source, found := FindBestSeller(sellers, order, point.Point{X: 1, Y: 1})
	if !found {
		t.Fatal("expected to find a seller")
	}
	if source.Seller != production.Producer(near) {
		t.Errorf("expected the nearer seller to win, got %v", source.Seller)
	}

	_, found = FindBestSeller(sellers, production.Production{Name: "Nothing", Rate: 1}, point.Point{X: 1, Y: 1})
	if found {
		t.Errorf("expected no seller for an unproduced product")
	}
}

func Test_Recipe_SourceProducts_reports_unmet_inputs(t *testing.T) {
	r := Recipe{
		DisplayName: "Test Recipe",
		InputProducts: production.Products{
			{Name: "Ore", Rate: 5},
			{Name: "Missing", Rate: 5},
		},
	}
	seller := &resources.Resource{
		Production: production.Production{Name: "Ore", Rate: 10},
		Loc:        point.Point{X: 0, Y: 0},
	}

	_, unmet, err := r.SourceProducts(testLogger(), []production.Producer{seller}, point.Point{X: 1, Y: 1})
	if err == nil {
		t.Fatal("expected an error for the unmet input")
	}
	if len(unmet) != 1 || unmet[0].Name != "Missing" {
		t.Errorf("expected exactly the Missing input to be reported unmet, got %+v", unmet)
	}
}
