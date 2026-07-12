package state

import (
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

	order := production.Production{Name: "Ore", Rate: 5}
	oldTransport := recipes.TransportCost(farSeller.Location(), buyer.Location())
	oldContract := &production.Contract{
		Seller: farSeller, Buyer: buyer,
		Order:         order,
		TransportCost: oldTransport,
		// Signed back when Ore traded at twice today's price -- the
		// close seller's current default ask beats this by far more
		// than the 5% renegotiation margin.
		ProductCost: 2 * production.DefaultUnitPrice * order.Rate,
	}
	buyer.Purchases = append(buyer.Purchases, oldContract)
	farSeller.Sales = append(farSeller.Sales, oldContract)

	s := newTestState(recipes.Recipes{},
		[]production.Producer{buyer, farSeller, closeSeller})

	// Publish once so the book carries the close seller's ask, then loop
	// renegotiation. The 2%-per-tick gate means the fixed-seed RNG needs
	// a few hundred tries (chance of never firing in 2000 is ~1e-17).
	s.publishOrders(testLogger())
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
