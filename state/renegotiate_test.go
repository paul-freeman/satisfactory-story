package state

import (
	"math/rand"
	"testing"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
	"github.com/paul-freeman/satisfactory-story/recipes"
	"github.com/paul-freeman/satisfactory-story/resources"
)

func Test_renegotiateContracts_switches_to_a_much_cheaper_supplier(t *testing.T) {
	buyer := factory.New("Buyer", "Recipe_Test_C", point.Point{X: 1000, Y: 1000}, 0,
		production.Products{{Name: "Ore", Rate: 5}},
		production.Products{{Name: "Ingot", Rate: 5}}, 1000)

	farSeller := &resources.Resource{
		Production: production.Production{Name: "Ore", Rate: 10},
		Loc:        point.Point{X: 0, Y: 0}, // far from the buyer -- expensive transport
	}
	closeSeller := &resources.Resource{
		Production: production.Production{Name: "Ore", Rate: 10},
		Loc:        point.Point{X: 1001, Y: 1001}, // right next to the buyer -- cheap transport
	}

	oldContract := &production.Contract{
		Seller: farSeller, Buyer: buyer,
		Order:         production.Production{Name: "Ore", Rate: 5},
		TransportCost: recipes.TransportCost(farSeller.Location(), buyer.Location()),
	}
	oldContract.ProductCost = farSeller.SalesPriceFor(oldContract.Order, oldContract.TransportCost)
	buyer.Purchases = append(buyer.Purchases, oldContract)
	farSeller.Sales = append(farSeller.Sales, oldContract)

	s := &State{
		producers: []production.Producer{buyer, farSeller, closeSeller},
		market:    make(map[string]float64),
		randSrc:   rand.New(rand.NewSource(1)),
	}

	// renegotiateProbabilityPerTick only fires ~2% of the time per factory
	// per tick, so loop enough times that the fixed-seed RNG is virtually
	// certain to have triggered it at least once (chance of never firing
	// in 2000 tries is ~1 in 10^17).
	for i := 0; i < 2000 && !oldContract.Cancelled; i++ {
		s.renegotiateContracts(testLogger())
	}

	if !oldContract.Cancelled {
		t.Fatal("expected the far supplier's contract to be renegotiated away")
	}
	found := false
	for _, sale := range closeSeller.Sales {
		if !sale.Cancelled && sale.Buyer == production.Producer(buyer) {
			found = true
		}
	}
	if !found {
		t.Error("expected the buyer to have signed a new contract with the closer seller")
	}
}
