package state

import (
	"math"
	"math/rand"
	"testing"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/market"
	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
	"github.com/paul-freeman/satisfactory-story/recipes"
	"github.com/paul-freeman/satisfactory-story/resources"
)

func newTestStateWithProducers(rs recipes.Recipes, producers []production.Producer) *State {
	return &State{
		recipes:   rs,
		producers: producers,
		book:      market.NewBook(),
		lastTrade: make(map[string]float64),
		ledger:    &tradeLedger{},
		randSrc:   rand.New(rand.NewSource(1)),
		xmin:      0, xmax: 1000, ymin: 0, ymax: 1000,
	}
}

// testRecipe creates a recipe fixture with exactly one input at Rate 1 and is Active.
func testRecipe(t *testing.T) *recipes.Recipe {
	return &recipes.Recipe{
		ClassName:      "Recipe_Test_C",
		DisplayName:    "Test Recipe",
		Active:         true,
		InputProducts:  production.Products{{Name: "Input", Rate: 1}},
		OutputProducts: production.Products{{Name: "Output", Rate: 1}},
	}
}

func Test_estimatedDeliveredCost_includesTransportAllowance(t *testing.T) {
	s := newTestState()
	// No ask, no trade history: pessimistic default + transport.
	if got := s.estimatedDeliveredCost("Unknown"); got != unknownInputUnitCost+defaultTransportEstimate {
		t.Fatalf("unknown = %v, want %v", got, unknownInputUnitCost+defaultTransportEstimate)
	}
	s.lastTrade["Traded"] = 3.0
	if got := s.estimatedDeliveredCost("Traded"); got != 3.0+defaultTransportEstimate {
		t.Fatalf("traded = %v, want %v", got, 3.0+defaultTransportEstimate)
	}
}

func Test_spawnWeights_crowdingDiscount(t *testing.T) {
	s := newTestState()
	s.randSrc = rand.New(rand.NewSource(1))
	// Two live factories already run Recipe_A_C; none run Recipe_B_C.
	for i := 0; i < 2; i++ {
		s.producers = append(s.producers, factory.New("A", "Recipe_A_C",
			point.Point{X: i * 10, Y: 0}, 0,
			production.Products{production.Production{Name: "In", Rate: 1}},
			production.Products{production.Production{Name: "Out", Rate: 1}},
			100))
	}
	crowd := s.recipeCrowding()
	if crowd["Recipe_A_C"] != 2 || crowd["Recipe_B_C"] != 0 {
		t.Fatalf("crowding = %v, want A:2 B:0", crowd)
	}
	// With identical expected profit, B's weight must be 3x A's:
	// (1+p)/(1+2) vs (1+p)/(1+0).
	wA := (baselineOpportunityWeight + 0) / float64(1+crowd["Recipe_A_C"])
	wB := (baselineOpportunityWeight + 0) / float64(1+crowd["Recipe_B_C"])
	if wB <= wA*2.9 {
		t.Fatalf("crowding discount too weak: wA=%v wB=%v", wA, wB)
	}
}

func Test_spawnNewProducer_seedCoversStockAndUpkeep(t *testing.T) {
	s := newTestState()
	s.randSrc = rand.New(rand.NewSource(1))
	s.xmin, s.xmax, s.ymin, s.ymax = 0, 1000, 0, 1000
	recipe := testRecipe(t) // helper below
	s.recipes = append(s.recipes, recipe)

	s.spawnNewProducer(testLogger())
	var spawned *factory.Factory
	for _, p := range s.producers {
		if f, ok := p.(*factory.Factory); ok {
			spawned = f
		}
	}
	if spawned == nil {
		t.Fatal("no factory spawned")
	}
	// One input at rate 1, no ask/history: delivered estimate = 10 + 2.
	want := (unknownInputUnitCost+defaultTransportEstimate)*1*inputStockTargetTicks +
		upkeepPerTick*seedCapitalBufferTicks
	if got := spawned.Wallet.Cash(); got < want-1e-6 || got > want+1e-6 {
		t.Fatalf("seed capital = %v, want %v", got, want)
	}
}

func Test_spawnNewProducer_spawns_idle_without_sourcing(t *testing.T) {
	// No Ore seller exists at all -- under the old economy this spawn
	// was impossible. Under the order book the factory spawns idle and
	// its bids will summon a supplier.
	rs := recipes.Recipes{
		{
			ClassName:      "Recipe_Smelt_C",
			DisplayName:    "Smelt Ore",
			Active:         true,
			InputProducts:  production.Products{{Name: "Ore", Rate: 5}},
			OutputProducts: production.Products{{Name: "Ingot", Rate: 5}},
		},
	}
	s := newTestStateWithProducers(rs, []production.Producer{})

	s.spawnNewProducer(testLogger())

	if len(s.producers) != 1 {
		t.Fatalf("expected an idle factory to spawn, got %d producers", len(s.producers))
	}
	f := s.producers[0].(*factory.Factory)
	// A freshly spawned factory has never ticked, so ProducedLastTick's
	// zero value (false) is exactly "idle" here -- this replaces the old
	// contract-era Producing() check, deleted in the Task 12 cutover.
	if f.ProducedLastTick {
		t.Error("the factory should be idle -- there is nothing to source")
	}
	// Seed capital: no ask, no trade history -> pessimistic estimate.
	// New formula: (estimatedDeliveredCost * rate) * inputStockTargetTicks + upkeep * seedCapitalBufferTicks
	want := (unknownInputUnitCost+defaultTransportEstimate)*5*inputStockTargetTicks +
		upkeepPerTick*seedCapitalBufferTicks
	if math.Abs(f.Wallet.Cash()-want) > 0.0001 {
		t.Errorf("expected seed capital %f, got %f", want, f.Wallet.Cash())
	}
}

func Test_spawnNewProducer_initializes_bids_at_best_ask(t *testing.T) {
	ore := &resources.Resource{
		Production: production.Production{Name: "Ore", Rate: 100},
		Loc:        point.Point{X: 500, Y: 500},
	}
	ore.SetAskPrice("Ore", 0.4)
	ore.Stock = 10 // stock-backed ask
	rs := recipes.Recipes{
		{
			ClassName:      "Recipe_Smelt_C",
			DisplayName:    "Smelt Ore",
			Active:         true,
			InputProducts:  production.Products{{Name: "Ore", Rate: 5}},
			OutputProducts: production.Products{{Name: "Ingot", Rate: 5}},
		},
	}
	s := newTestStateWithProducers(rs, []production.Producer{ore})
	s.publishOrders(testLogger())

	s.spawnNewProducer(testLogger())

	var f *factory.Factory
	for _, p := range s.producers {
		if candidate, ok := p.(*factory.Factory); ok {
			f = candidate
		}
	}
	if f == nil {
		t.Fatal("expected a factory to spawn")
	}
	if got := f.BidPriceFor("Ore"); got != 0.4 {
		t.Errorf("bid should start at the best ask 0.4, got %f", got)
	}
	// Seed capital now uses delivered estimate = ask + transport.
	want := (0.4+defaultTransportEstimate)*5*inputStockTargetTicks +
		upkeepPerTick*seedCapitalBufferTicks
	if math.Abs(f.Wallet.Cash()-want) > 0.0001 {
		t.Errorf("expected seed capital %f, got %f", want, f.Wallet.Cash())
	}
}

func Test_spawnNewProducer_spawns_near_a_sourceable_input(t *testing.T) {
	ore := &resources.Resource{
		Production: production.Production{Name: "Ore", Rate: 100},
		Loc:        point.Point{X: 700, Y: 300},
	}
	ore.Stock = 10 // stock-backed ask
	rs := recipes.Recipes{
		{
			ClassName:      "Recipe_Smelt_C",
			DisplayName:    "Smelt Ore",
			Active:         true,
			InputProducts:  production.Products{{Name: "Ore", Rate: 5}},
			OutputProducts: production.Products{{Name: "Ingot", Rate: 5}},
		},
	}
	s := newTestStateWithProducers(rs, []production.Producer{ore})
	s.publishOrders(testLogger())

	s.spawnNewProducer(testLogger())

	var f *factory.Factory
	for _, p := range s.producers {
		if candidate, ok := p.(*factory.Factory); ok {
			f = candidate
		}
	}
	if f == nil {
		t.Fatal("expected a factory to spawn")
	}
	// Near, not AT -- recipes.UnitTransportCost treats distance <= 1 as a
	// same-location collision and charges 1e12 to avoid it (see
	// recipes.go), so a factory that spawns exactly on its seller's
	// coordinates would make that seller permanently unaffordable.
	if got := f.Loc.Distance(ore.Loc); got <= 1 {
		t.Errorf("expected the factory to spawn near, not on, its seller %v -- got %v (distance %f, must exceed the UnitTransportCost collision threshold)", ore.Loc, f.Loc, got)
	}
	if got := f.Loc.Distance(ore.Loc); got > 20 {
		t.Errorf("expected the factory to spawn close to its only sourceable input %v, got %v (distance %f)", ore.Loc, f.Loc, got)
	}
}

func Test_spawnNewProducer_spawns_at_centroid_of_multiple_sourceable_inputs(t *testing.T) {
	ore := &resources.Resource{
		Production: production.Production{Name: "Ore", Rate: 100},
		Loc:        point.Point{X: 400, Y: 400},
	}
	ore.Stock = 10 // stock-backed ask
	coal := &resources.Resource{
		Production: production.Production{Name: "Coal", Rate: 100},
		Loc:        point.Point{X: 600, Y: 600},
	}
	coal.Stock = 10 // stock-backed ask
	rs := recipes.Recipes{
		{
			ClassName:   "Recipe_Steel_C",
			DisplayName: "Steel Ingot",
			Active:      true,
			InputProducts: production.Products{
				{Name: "Ore", Rate: 5}, {Name: "Coal", Rate: 5},
			},
			OutputProducts: production.Products{{Name: "SteelIngot", Rate: 5}},
		},
	}
	s := newTestStateWithProducers(rs, []production.Producer{ore, coal})
	s.publishOrders(testLogger())

	s.spawnNewProducer(testLogger())

	var f *factory.Factory
	for _, p := range s.producers {
		if candidate, ok := p.(*factory.Factory); ok {
			f = candidate
		}
	}
	if f == nil {
		t.Fatal("expected a factory to spawn")
	}
	centroid := point.Point{X: 500, Y: 500} // midpoint of (400,400) and (600,600)
	if got := f.Loc.Distance(centroid); got > 20 {
		t.Errorf("expected the factory to spawn near the centroid %v of its sourceable inputs, got %v (distance %f)", centroid, f.Loc, got)
	}
}

func Test_spawnNewProducer_never_collides_with_a_sourceable_input(t *testing.T) {
	// Regression test: recipes.UnitTransportCost charges an astronomical
	// 1e12 for any contract between two points <= 1 apart (see
	// recipes.go), specifically to stop factories from sitting exactly
	// on top of another producer. spawnLocation must always clear that
	// threshold, or the input it just spawned next to becomes
	// permanently unaffordable -- exactly the bug this guards against.
	ore := &resources.Resource{
		Production: production.Production{Name: "Ore", Rate: 100},
		Loc:        point.Point{X: 400, Y: 400},
	}
	rs := recipes.Recipes{
		{
			ClassName:      "Recipe_Smelt_C",
			DisplayName:    "Smelt Ore",
			Active:         true,
			InputProducts:  production.Products{{Name: "Ore", Rate: 5}},
			OutputProducts: production.Products{{Name: "Ingot", Rate: 5}},
		},
	}
	s := newTestStateWithProducers(rs, []production.Producer{ore})
	s.publishOrders(testLogger())

	s.spawnNewProducer(testLogger())

	f := s.producers[len(s.producers)-1].(*factory.Factory)
	if got := f.Loc.Distance(ore.Loc); got <= 1 {
		t.Fatalf("factory spawned at distance %f from its seller -- recipes.UnitTransportCost would charge 1e12", got)
	}
}

func Test_spawnNewProducer_falls_back_to_random_location_when_unsourceable(t *testing.T) {
	// No Ore seller exists at all -- spawnLocation must fall back to a
	// random in-bounds location rather than defaulting to the origin or
	// erroring.
	rs := recipes.Recipes{
		{
			ClassName:      "Recipe_Smelt_C",
			DisplayName:    "Smelt Ore",
			Active:         true,
			InputProducts:  production.Products{{Name: "Ore", Rate: 5}},
			OutputProducts: production.Products{{Name: "Ingot", Rate: 5}},
		},
	}
	s := newTestStateWithProducers(rs, []production.Producer{})

	s.spawnNewProducer(testLogger())

	f := s.producers[0].(*factory.Factory)
	if f.Loc.X < s.xmin || f.Loc.X > s.xmax || f.Loc.Y < s.ymin || f.Loc.Y > s.ymax {
		t.Errorf("expected the fallback location to be within map bounds, got %v", f.Loc)
	}
}

func Test_expectedProfit_reads_the_book(t *testing.T) {
	buyer := factory.New("Downstream", "Recipe_Down_C", point.Point{X: 0, Y: 0}, 0,
		production.Products{{Name: "Ingot", Rate: 5}},
		production.Products{{Name: "Plate", Rate: 5}}, 0)
	recipe := &recipes.Recipe{
		ClassName:      "Recipe_Smelt_C",
		DisplayName:    "Smelt Ore",
		Active:         true,
		InputProducts:  production.Products{{Name: "Ore", Rate: 5}},
		OutputProducts: production.Products{{Name: "Ingot", Rate: 5}},
	}
	s := newTestStateWithProducers(recipes.Recipes{recipe}, []production.Producer{})

	// No bids at all: revenue falls back to the floor price.
	// expectedProfit now uses estimatedDeliveredCost (add transport allowance).
	wantFloor := floorUnitPrice*5 - ((unknownInputUnitCost+defaultTransportEstimate)*5 + upkeepPerTick)
	if got := s.expectedProfit(recipe); math.Abs(got-wantFloor) > 0.0001 {
		t.Errorf("expected floor-based profit %f, got %f", wantFloor, got)
	}

	// A real standing bid for the output raises expected profit.
	s.book.PostBid(buyer, "Ingot", 5, 8.0)
	want := 8.0*5 - ((unknownInputUnitCost+defaultTransportEstimate)*5 + upkeepPerTick)
	if got := s.expectedProfit(recipe); math.Abs(got-want) > 0.0001 {
		t.Errorf("expected bid-based profit %f, got %f", want, got)
	}

	// A cheap ask for the input raises it further.
	seller := &resources.Resource{Production: production.Production{Name: "Ore", Rate: 100}}
	s.book.PostAsk(seller, "Ore", 100, 0.5)
	want = 8.0*5 - ((0.5+defaultTransportEstimate)*5 + upkeepPerTick)
	if got := s.expectedProfit(recipe); math.Abs(got-want) > 0.0001 {
		t.Errorf("expected ask-based profit %f, got %f", want, got)
	}
}
