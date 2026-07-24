package state

import (
	"testing"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
)

func Test_applySolvency_salvageTrickleOnlyWhenCapped(t *testing.T) {
	s := newTestState()
	f := factory.New("Plates", "Recipe_Plates_C", point.Point{X: 0, Y: 0}, 0,
		production.Products{production.Production{Name: "IronIngot", Rate: 1}},
		production.Products{production.Production{Name: "IronPlate", Rate: 2}},
		100)
	s.producers = []production.Producer{f}

	// Below cap: no salvage, just upkeep.
	f.OutputStock.Add("IronPlate", 10)
	s.applySolvency(testLogger())
	if got := f.Wallet.Cash(); got != 100-upkeepPerTick {
		t.Fatalf("below-cap cash = %v, want %v", got, 100-upkeepPerTick)
	}
	if got := f.OutputStock.Get("IronPlate"); got != 10 {
		t.Fatalf("below-cap stock = %v, want 10 (untouched)", got)
	}

	// At cap (rate 2 x outputStockCapTicks): trickle 25% of one tick's
	// rate (0.5 units) at floorUnitPrice.
	cap := 2 * outputStockCapTicks
	f.OutputStock.Add("IronPlate", cap-10)
	before := f.Wallet.Cash()
	s.applySolvency(testLogger())
	wantSalvage := 0.5 * floorUnitPrice
	if got := f.Wallet.Cash(); got < before+wantSalvage-upkeepPerTick-1e-9 || got > before+wantSalvage-upkeepPerTick+1e-9 {
		t.Fatalf("capped cash delta = %v, want %v", got-before, wantSalvage-upkeepPerTick)
	}
	if got := f.OutputStock.Get("IronPlate"); got != cap-0.5 {
		t.Fatalf("capped stock = %v, want %v", got, cap-0.5)
	}
}

func Test_applySolvency_collectsUpkeepAsRent(t *testing.T) {
	s := newTestState()
	f1 := factory.New("A", "Recipe_A_C", point.Point{X: 0, Y: 0}, 0,
		production.Products{production.Production{Name: "In", Rate: 1}},
		production.Products{production.Production{Name: "Out", Rate: 1}},
		100)
	f2 := factory.New("B", "Recipe_B_C", point.Point{X: 10, Y: 0}, 0,
		production.Products{production.Production{Name: "In", Rate: 1}},
		production.Products{production.Production{Name: "Out", Rate: 1}},
		100)
	s.producers = []production.Producer{f1, f2}

	before := s.treasury
	s.applySolvency(testLogger())

	// Two solvent factories each pay one tick of upkeep into the treasury.
	if got, want := s.treasury, before+2*upkeepPerTick; got != want {
		t.Fatalf("treasury after rent = %v, want %v", got, want)
	}
	// Per-factory solvency is unchanged: each wallet still drops by exactly
	// upkeepPerTick (money's destination moved, not its amount).
	if got := f1.Wallet.Cash(); got != 100-upkeepPerTick {
		t.Fatalf("f1 cash = %v, want %v (upkeep still charged to the wallet)", got, 100-upkeepPerTick)
	}
	if got := f2.Wallet.Cash(); got != 100-upkeepPerTick {
		t.Fatalf("f2 cash = %v, want %v", got, 100-upkeepPerTick)
	}
}

func Test_applySolvency_removesInsolvent(t *testing.T) {
	s := newTestState()
	f := factory.New("Plates", "Recipe_Plates_C", point.Point{X: 0, Y: 0}, 0,
		production.Products{production.Production{Name: "IronIngot", Rate: 1}},
		production.Products{production.Production{Name: "IronPlate", Rate: 2}},
		-1) // already broke
	s.producers = []production.Producer{f}
	// Wallet.Apply counts one insolvent tick per applySolvency call;
	// removal happens on the call where the count reaches the grace.
	for i := 0; i < insolvencyGrace-1; i++ {
		s.applySolvency(testLogger())
		if len(s.producers) == 0 {
			t.Fatalf("removed after %d ticks, before grace %d expired", i+1, insolvencyGrace)
		}
	}
	s.applySolvency(testLogger())
	if len(s.producers) != 0 {
		t.Fatal("factory should be removed once insolvent for the full grace window")
	}
}
