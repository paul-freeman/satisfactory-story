# Inventory Economy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the flow-contract economy with a discrete-unit stock-buffer economy (spot-market trades, hard wallet budgets, bounded stock with halt+salvage back-pressure) so a full-scale run delivers a SpaceElevatorPart within 100k ticks at seed 152.

**Architecture:** Producers hold real goods in stock; a production step runs each recipe against stock each tick; the existing order book crosses stock-backed asks against hunger-backed bids and executes one-shot trades (units + money move at trade time). Contracts and all their machinery are deleted. Spawning gains a crowding discount and transport-aware estimates; movement and map links derive from a rolling trade ledger.

**Tech Stack:** Go (backend), existing `market` order book, React/TS/D3 frontend (adapt-only — ideally zero frontend diffs).

**Spec:** `docs/superpowers/specs/2026-07-12-inventory-economy-design.md` — read it before starting any task.

## Global Constraints

- Work directly on `main` (established project preference — no worktree).
- `go build ./...` and `go test ./...` must pass at the end of EVERY task.
- Determinism: identical seeds must produce identical runs. Never range over a map where order can affect simulation state; iterate sorted keys or stable slices.
- `gofmt` all touched files before committing.
- Do not change HTTP endpoint paths or wire-type field names (`state/http/http.go` types keep their JSON shape).
- Agents may read public market state (prices, book, live producers, recent trades) but must never walk the recipe tree to derive demand.
- Constants that are milestone tuning knobs live in the `state` package unless stated otherwise and are passed into `factory`/`resources` methods as parameters — do not duplicate them in other packages.

---

### Task 1: `production.Inventory` and `Wallet.Adjust`

**Files:**
- Create: `production/inventory.go`, `production/inventory_test.go`
- Modify: `production/wallet.go`
- Test: `production/wallet_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces: `type Inventory map[string]float64` with `Get(name string) float64`, `Add(name string, qty float64)`, `Take(name string, qty float64) float64`; `(*Wallet).Adjust(delta float64)`.

- [ ] **Step 1: Write failing tests**

`production/inventory_test.go`:

```go
package production

import "testing"

func Test_Inventory(t *testing.T) {
	inv := make(Inventory)
	if got := inv.Get("IronOre"); got != 0 {
		t.Fatalf("empty Get = %v, want 0", got)
	}
	inv.Add("IronOre", 5)
	inv.Add("IronOre", 2.5)
	if got := inv.Get("IronOre"); got != 7.5 {
		t.Fatalf("Get after adds = %v, want 7.5", got)
	}
	took := inv.Take("IronOre", 3)
	if took != 3 || inv.Get("IronOre") != 4.5 {
		t.Fatalf("Take(3) = %v (stock %v), want 3 (stock 4.5)", took, inv.Get("IronOre"))
	}
	// Take clamps at what is available and never goes negative.
	took = inv.Take("IronOre", 100)
	if took != 4.5 || inv.Get("IronOre") != 0 {
		t.Fatalf("clamped Take = %v (stock %v), want 4.5 (stock 0)", took, inv.Get("IronOre"))
	}
	if took := inv.Take("Coal", 1); took != 0 {
		t.Fatalf("Take of absent product = %v, want 0", took)
	}
}
```

Append to `production/wallet_test.go`:

```go
func Test_Wallet_Adjust_doesNotTouchInsolvencyCounter(t *testing.T) {
	w := NewWallet(10)
	w.Adjust(-15) // balance -5, but Adjust must NOT count insolvency ticks
	if w.Cash() != -5 {
		t.Fatalf("Cash = %v, want -5", w.Cash())
	}
	if w.InsolventFor(1) {
		t.Fatal("Adjust must not advance the insolvency counter; only Apply does")
	}
	w.Apply(0) // the once-per-tick accounting call
	if !w.InsolventFor(1) {
		t.Fatal("Apply with negative balance should count one insolvent tick")
	}
}
```

- [ ] **Step 2: Run tests, verify they fail**

Run: `go test ./production -run 'Test_Inventory|Test_Wallet_Adjust' -v`
Expected: FAIL (undefined: Inventory, w.Adjust).

- [ ] **Step 3: Implement**

`production/inventory.go`:

```go
package production

// Inventory holds real goods: float quantities per product name. It is
// the physical state of the inventory economy -- everything a producer
// can sell is in an Inventory, and everything it consumes comes out of
// one.
type Inventory map[string]float64

// Get returns the quantity on hand (0 for absent products).
func (inv Inventory) Get(name string) float64 {
	return inv[name]
}

// Add puts qty units into stock.
func (inv Inventory) Add(name string, qty float64) {
	inv[name] += qty
}

// Take removes up to qty units and returns how much was actually taken,
// clamped at what is available. Stock never goes negative.
func (inv Inventory) Take(name string, qty float64) float64 {
	have := inv[name]
	if qty > have {
		qty = have
	}
	if qty < 0 {
		qty = 0
	}
	inv[name] = have - qty
	return qty
}
```

Append to `production/wallet.go`:

```go
// Adjust moves money without touching the consecutive-negative-ticks
// counter. Trades use Adjust (many per tick); the solvency step's single
// Apply per tick is what advances insolvency accounting.
func (w *Wallet) Adjust(delta float64) {
	w.Balance += delta
}
```

- [ ] **Step 4: Run tests, verify they pass**

Run: `go test ./production -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add production/inventory.go production/inventory_test.go production/wallet.go production/wallet_test.go
git commit -m "feat: production.Inventory and Wallet.Adjust for the stock economy"
```

---

### Task 2: per-unit transport cost

**Files:**
- Modify: `recipes/recipes.go` (add function; do NOT remove `TransportCost` yet — later tasks migrate callers, Task 12 deletes it)
- Test: `recipes/recipes_test.go` (create if absent)

**Interfaces:**
- Produces: `recipes.UnitTransportCost(origin, destination point.Point) float64` = `0.1 + d/10000`, with the same `d <= 1` collision guard (returns `1e12`) as the old function.

- [ ] **Step 1: Write failing test**

Append to (or create) `recipes/recipes_test.go`:

```go
package recipes

import (
	"testing"

	"github.com/paul-freeman/satisfactory-story/point"
)

func Test_UnitTransportCost(t *testing.T) {
	a := point.Point{X: 0, Y: 0}
	// Same-spot collision guard survives from TransportCost.
	if got := UnitTransportCost(a, point.Point{X: 1, Y: 0}); got != 1e12 {
		t.Fatalf("collision guard = %v, want 1e12", got)
	}
	// 10000 units of distance costs 0.1 fixed + 1.0 distance.
	got := UnitTransportCost(a, point.Point{X: 10000, Y: 0})
	if got < 1.0999 || got > 1.1001 {
		t.Fatalf("UnitTransportCost(10000) = %v, want ~1.1", got)
	}
}
```

- [ ] **Step 2: Run test, verify it fails**

Run: `go test ./recipes -run Test_UnitTransportCost -v`
Expected: FAIL with "undefined: UnitTransportCost".

- [ ] **Step 3: Implement**

Append to `recipes/recipes.go`:

```go
// Per-unit freight pricing for the inventory economy. Per-unit (not
// per-contract-per-tick) so low-rate products are not charged 30x the
// effective freight of high-rate ones.
const transportFixedPerUnit = 0.1
const transportPerDistance = 1.0 / 10000.0

// UnitTransportCost returns the cost of moving ONE unit of product from
// origin to destination. Distances <= 1 keep the prohibitive collision
// guard from TransportCost: producers must not stack on one location.
func UnitTransportCost(origin point.Point, destination point.Point) float64 {
	d := origin.Distance(destination)
	if d <= 1 {
		return 1e12
	}
	return transportFixedPerUnit + d*transportPerDistance
}
```

- [ ] **Step 4: Run tests, verify pass**

Run: `go test ./recipes -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add recipes/recipes.go recipes/recipes_test.go
git commit -m "feat: per-unit transport cost for the inventory economy"
```

---

### Task 3: factory stock machinery (additive)

**Files:**
- Modify: `factory/factory.go` (add fields + methods; touch NOTHING contract-related yet)
- Test: `factory/factory_test.go` (append)

**Interfaces:**
- Consumes: `production.Inventory` (Task 1).
- Produces (all on `*factory.Factory`):
  - fields `InputStock`, `OutputStock production.Inventory`; `ProducedLastTick bool`; `TickInputSpend`, `TickRevenue`, `AvgInputSpend`, `AvgRevenue float64`; `RecentTrades []TradeMemory`
  - `type TradeMemory struct { Tick int; Other point.Point; Qty float64 }`
  - `ProduceTick(outputCapTicks float64) float64` (fraction run, 0..1)
  - `Hunger(name string, targetTicks float64) float64`
  - `RecordTrade(tick int, other point.Point, qty float64)`
  - `PruneTrades(tick, memoryTicks int)`
  - `FoldTickFlows(smoothing float64)` (folds TickInputSpend/TickRevenue into the EMAs and zeroes them)
  - `StockMarginalUnitCost(upkeep float64) float64` = `(AvgInputSpend + upkeep) / totalOutputRate`

- [ ] **Step 1: Write failing tests**

Append to `factory/factory_test.go`:

```go
func Test_Factory_ProduceTick(t *testing.T) {
	f := New("Plates", "Recipe_Plates_C", point.Point{X: 0, Y: 0}, 0,
		production.Products{production.Production{Name: "IronIngot", Rate: 2}},
		production.Products{production.Production{Name: "IronPlate", Rate: 3}},
		100)
	// No input stock: nothing runs.
	if frac := f.ProduceTick(60); frac != 0 || f.ProducedLastTick {
		t.Fatalf("empty-stock ProduceTick = %v (produced=%v), want 0 (false)", frac, f.ProducedLastTick)
	}
	// Half the needed input: runs at half rate.
	f.InputStock.Add("IronIngot", 1)
	frac := f.ProduceTick(60)
	if frac < 0.499 || frac > 0.501 {
		t.Fatalf("half-stock ProduceTick = %v, want 0.5", frac)
	}
	if got := f.OutputStock.Get("IronPlate"); got < 1.499 || got > 1.501 {
		t.Fatalf("output stock = %v, want 1.5", got)
	}
	if got := f.InputStock.Get("IronIngot"); got > 1e-9 {
		t.Fatalf("input stock = %v, want 0", got)
	}
	if !f.ProducedLastTick {
		t.Fatal("ProducedLastTick should be true after a fractional run")
	}
	// Output cap limits the run: cap 1 tick's worth (3 units), stock
	// already 1.5, plenty of input -> only 0.5 ticks of room.
	f.InputStock.Add("IronIngot", 100)
	frac = f.ProduceTick(1)
	if frac < 0.499 || frac > 0.501 {
		t.Fatalf("cap-limited ProduceTick = %v, want 0.5", frac)
	}
	// Full stock: halts entirely.
	frac = f.ProduceTick(1)
	if frac != 0 || f.ProducedLastTick {
		t.Fatalf("full-stock ProduceTick = %v (produced=%v), want 0 (false)", frac, f.ProducedLastTick)
	}
}

func Test_Factory_Hunger(t *testing.T) {
	f := New("Plates", "Recipe_Plates_C", point.Point{X: 0, Y: 0}, 0,
		production.Products{production.Production{Name: "IronIngot", Rate: 2}},
		production.Products{production.Production{Name: "IronPlate", Rate: 3}},
		100)
	if got := f.Hunger("IronIngot", 10); got != 20 {
		t.Fatalf("empty Hunger = %v, want 20 (rate 2 x target 10)", got)
	}
	f.InputStock.Add("IronIngot", 15)
	if got := f.Hunger("IronIngot", 10); got != 5 {
		t.Fatalf("partial Hunger = %v, want 5", got)
	}
	f.InputStock.Add("IronIngot", 100)
	if got := f.Hunger("IronIngot", 10); got != 0 {
		t.Fatalf("overshoot Hunger = %v, want 0", got)
	}
	if got := f.Hunger("NotAnInput", 10); got != 0 {
		t.Fatalf("non-input Hunger = %v, want 0", got)
	}
}

func Test_Factory_TradeMemoryAndFlows(t *testing.T) {
	f := New("Plates", "Recipe_Plates_C", point.Point{X: 0, Y: 0}, 0,
		production.Products{production.Production{Name: "IronIngot", Rate: 2}},
		production.Products{production.Production{Name: "IronPlate", Rate: 3}},
		100)
	f.RecordTrade(10, point.Point{X: 5, Y: 5}, 4)
	f.RecordTrade(600, point.Point{X: 9, Y: 9}, 1)
	f.PruneTrades(610, 500) // keeps trades newer than tick 110
	if len(f.RecentTrades) != 1 || f.RecentTrades[0].Tick != 600 {
		t.Fatalf("PruneTrades kept %v, want only the tick-600 trade", f.RecentTrades)
	}
	f.TickInputSpend = 10
	f.TickRevenue = 30
	f.FoldTickFlows(0.5)
	if f.AvgInputSpend != 5 || f.AvgRevenue != 15 {
		t.Fatalf("EMAs = %v/%v, want 5/15", f.AvgInputSpend, f.AvgRevenue)
	}
	if f.TickInputSpend != 0 || f.TickRevenue != 0 {
		t.Fatal("FoldTickFlows must zero the per-tick accumulators")
	}
	// marginal cost: (5 + 0.5 upkeep) / 3 output rate
	got := f.StockMarginalUnitCost(0.5)
	if got < 1.8332 || got > 1.8334 {
		t.Fatalf("StockMarginalUnitCost = %v, want ~1.8333", got)
	}
}
```

- [ ] **Step 2: Run tests, verify they fail**

Run: `go test ./factory -run 'Test_Factory_ProduceTick|Test_Factory_Hunger|Test_Factory_TradeMemoryAndFlows' -v`
Expected: FAIL (undefined fields/methods).

- [ ] **Step 3: Implement**

In `factory/factory.go`, add to the `Factory` struct (after the `AskPrices`/`BidPrices` fields):

```go
	// InputStock and OutputStock hold real goods. Production consumes
	// from InputStock into OutputStock; trades move units between a
	// seller's OutputStock and a buyer's InputStock.
	InputStock  production.Inventory
	OutputStock production.Inventory
	// ProducedLastTick records whether the recipe ran at all last tick
	// (observability, not a contractual state).
	ProducedLastTick bool
	// TickInputSpend / TickRevenue accumulate this tick's trade flows;
	// the solvency step folds them into the EMAs and zeroes them.
	TickInputSpend float64
	TickRevenue    float64
	AvgInputSpend  float64
	AvgRevenue     float64
	// RecentTrades is this factory's own memory of who it traded with,
	// used for the movement gradient.
	RecentTrades []TradeMemory
```

In `New(...)`, add to the returned literal:

```go
		InputStock:  make(production.Inventory),
		OutputStock: make(production.Inventory),
```

Add the type and methods (near the bottom of the file):

```go
// TradeMemory is one remembered trade endpoint: where the counterparty
// was and how much moved. Movement hill-climbs on these.
type TradeMemory struct {
	Tick  int
	Other point.Point
	Qty   float64
}

// ProduceTick runs up to one tick of the recipe, limited by input stock
// and by room left under the output cap (outputCapTicks x output rate
// per product). Returns the fraction of a full tick actually run.
func (f *Factory) ProduceTick(outputCapTicks float64) float64 {
	frac := 1.0
	for _, in := range f.Input {
		if in.Rate <= production.RateEpsilon {
			continue
		}
		frac = math.Min(frac, f.InputStock.Get(in.Name)/in.Rate)
	}
	for _, out := range f.Output {
		if out.Rate <= production.RateEpsilon {
			continue
		}
		room := out.Rate*outputCapTicks - f.OutputStock.Get(out.Name)
		frac = math.Min(frac, room/out.Rate)
	}
	frac = math.Max(0, math.Min(1, frac))
	if frac <= production.RateEpsilon {
		f.ProducedLastTick = false
		return 0
	}
	for _, in := range f.Input {
		f.InputStock.Take(in.Name, in.Rate*frac)
	}
	for _, out := range f.Output {
		f.OutputStock.Add(out.Name, out.Rate*frac)
	}
	f.ProducedLastTick = true
	return frac
}

// Hunger is how many units of the named input the factory wants to buy
// right now: the gap between its input-stock target and what it holds.
func (f *Factory) Hunger(name string, targetTicks float64) float64 {
	for _, in := range f.Input {
		if in.Name != name {
			continue
		}
		h := in.Rate*targetTicks - f.InputStock.Get(name)
		if h < 0 {
			return 0
		}
		return h
	}
	return 0
}

// RecordTrade remembers a trade endpoint for the movement gradient.
func (f *Factory) RecordTrade(tick int, other point.Point, qty float64) {
	f.RecentTrades = append(f.RecentTrades, TradeMemory{Tick: tick, Other: other, Qty: qty})
}

// PruneTrades drops remembered trades older than memoryTicks.
func (f *Factory) PruneTrades(tick, memoryTicks int) {
	kept := f.RecentTrades[:0]
	for _, tr := range f.RecentTrades {
		if tick-tr.Tick <= memoryTicks {
			kept = append(kept, tr)
		}
	}
	f.RecentTrades = kept
}

// FoldTickFlows folds this tick's accumulated spend/revenue into the
// exponential moving averages and zeroes the accumulators. Called once
// per tick by the solvency step.
func (f *Factory) FoldTickFlows(smoothing float64) {
	f.AvgInputSpend = f.AvgInputSpend*(1-smoothing) + f.TickInputSpend*smoothing
	f.AvgRevenue = f.AvgRevenue*(1-smoothing) + f.TickRevenue*smoothing
	f.TickInputSpend = 0
	f.TickRevenue = 0
}

// StockMarginalUnitCost is the stock-world cost basis per output unit:
// smoothed input spend plus upkeep, spread over total output rate. The
// floor for ask-price decay.
func (f *Factory) StockMarginalUnitCost(upkeep float64) float64 {
	totalRate := 0.0
	for _, out := range f.Output {
		totalRate += out.Rate
	}
	if totalRate <= production.RateEpsilon {
		return upkeep
	}
	return (f.AvgInputSpend + upkeep) / totalRate
}
```

Add `"math"` to the imports if not already present.

- [ ] **Step 4: Run tests, verify pass**

Run: `go test ./factory -v` and `go build ./...`
Expected: PASS, build OK.

- [ ] **Step 5: Commit**

```bash
git add factory/factory.go factory/factory_test.go
git commit -m "feat: factory stock, production step, hunger, trade memory (additive)"
```

---

### Task 4: resource stock and sink delivery counter (additive)

**Files:**
- Modify: `resources/resources.go`, `sink/sink.go`
- Test: `resources/resources_test.go`, `sink/sink_test.go` (create if absent)

**Interfaces:**
- Produces: `(*resources.Resource)` field `Stock float64`, method `ProduceTick(outputCapTicks float64)`; `(*sink.Sink)` field `Delivered production.Inventory`, methods `RecordDelivery(name string, qty float64)`, `TotalDelivered() float64`.

- [ ] **Step 1: Write failing tests**

Append to `resources/resources_test.go` (create with package header if absent):

```go
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
```

Create `sink/sink_test.go`:

```go
package sink

import (
	"testing"

	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
)

func Test_Sink_RecordDelivery(t *testing.T) {
	sk := New("SpaceElevatorPart_1", point.Point{X: 0, Y: 0}, production.Products{
		production.Production{Name: "SpaceElevatorPart_1", Rate: 1},
	}, 1000)
	if sk.TotalDelivered() != 0 {
		t.Fatalf("fresh sink TotalDelivered = %v, want 0", sk.TotalDelivered())
	}
	sk.RecordDelivery("SpaceElevatorPart_1", 2.5)
	sk.RecordDelivery("SpaceElevatorPart_1", 0.5)
	if sk.TotalDelivered() != 3 {
		t.Fatalf("TotalDelivered = %v, want 3", sk.TotalDelivered())
	}
}
```

- [ ] **Step 2: Run tests, verify they fail**

Run: `go test ./resources ./sink -v`
Expected: FAIL (undefined Stock/ProduceTick/Delivered).

- [ ] **Step 3: Implement**

In `resources/resources.go`, add to the `Resource` struct:

```go
	// Stock is the units of extracted product on hand, bounded by the
	// production step's cap. Asks are backed by this.
	Stock float64
```

Add method:

```go
// ProduceTick extracts one tick's worth of product into stock, clamped
// at outputCapTicks worth of production.
func (r *Resource) ProduceTick(outputCapTicks float64) {
	cap := r.Production.Rate * outputCapTicks
	r.Stock = math.Min(cap, r.Stock+r.Production.Rate)
}
```

(`math` is already imported in this file.)

In `sink/sink.go`, add to the `Sink` struct:

```go
	// Delivered counts units actually received, by product. The
	// space-elevator milestone is TotalDelivered() > 0 on a goal sink.
	Delivered production.Inventory
```

In `sink.New(...)`, add `Delivered: make(production.Inventory),` to the returned literal. Add methods:

```go
// RecordDelivery counts qty units of the named product as received.
func (f *Sink) RecordDelivery(name string, qty float64) {
	f.Delivered.Add(name, qty)
}

// TotalDelivered is the total units ever received across all products.
func (f *Sink) TotalDelivered() float64 {
	total := 0.0
	for _, qty := range f.Delivered {
		total += qty
	}
	return total
}
```

- [ ] **Step 4: Run tests, verify pass**

Run: `go test ./resources ./sink -v` and `go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add resources/resources.go resources/resources_test.go sink/sink.go sink/sink_test.go
git commit -m "feat: resource stock and sink delivery counter (additive)"
```

---

### Task 5: market executes trades with partial quantities

**Files:**
- Modify: `market/match.go`, `market/match_test.go`, `state/orders.go` (adapter only)

**Interfaces:**
- Consumes: existing `market.Book`.
- Produces:
  - `Match` struct: field `TransportCost` RENAMED to `UnitTransport` (per-UNIT freight, no rate division).
  - `MatchAll(unitTransport func(origin, destination point.Point) float64, execute func(Match) (float64, error))` — `execute` returns the quantity actually traded; the matcher decrements both sides by it. `executed == 0` (or error) skips that ask for that bid; a partial execution (`executed < candidate`) ends that bid's shopping this tick (its budget ran out).
  - Delivered cost is now `ask.UnitPrice + unitTransport(sellerLoc, buyerLoc)` — NO division by rate.

- [ ] **Step 1: Update the match tests**

Rewrite `market/match_test.go` expectations: everywhere a test passes a transport function, that function now returns a PER-UNIT cost, and crossing requires `bid.UnitPrice >= ask.UnitPrice + transport(...)`. Every `sign func(Match) error` fixture becomes `execute func(Match) (float64, error)` returning `(m.Order.Rate, nil)` for full execution. Add this new test:

```go
func Test_MatchAll_partialExecutionStopsBid(t *testing.T) {
	b := NewBook()
	seller1 := newTestProducer(point.Point{X: 0, Y: 100})
	seller2 := newTestProducer(point.Point{X: 0, Y: 200})
	buyer := newTestProducer(point.Point{X: 0, Y: 0})
	b.PostAsk(seller1, "IronPlate", 10, 1.0)
	b.PostAsk(seller2, "IronPlate", 10, 1.0)
	b.PostBid(buyer, "IronPlate", 20, 5.0)

	flat := func(_, _ point.Point) float64 { return 0.5 }
	var calls []float64
	b.MatchAll(flat, func(m Match) (float64, error) {
		calls = append(calls, m.Order.Rate)
		return 4, nil // budget only covers 4 of the 10 offered
	})
	// Partial execution (4 < 10) must stop this bid's shopping: the
	// second ask is never visited.
	if len(calls) != 1 {
		t.Fatalf("execute called %d times, want 1 (partial fill ends the bid)", len(calls))
	}
	if got := b.Bids("IronPlate")[0].Remaining; got != 16 {
		t.Fatalf("bid remaining = %v, want 16", got)
	}
	if got := b.Asks("IronPlate")[0].Remaining; got != 6 {
		t.Fatalf("ask remaining = %v, want 6", got)
	}
}
```

(Use the existing test-producer helper in `match_test.go`; if it has a different name, keep the file's own convention.)

- [ ] **Step 2: Run tests, verify they fail**

Run: `go test ./market -v`
Expected: FAIL (signature mismatches).

- [ ] **Step 3: Implement**

Replace `market/match.go`'s `Match`, `MatchAll`, and `bestDeliveredAsk` with:

```go
// Match is a crossed bid/ask pair ready to execute as a one-shot trade.
// The trade executes at the ask's unit price; the buyer additionally
// pays UnitTransport per unit of freight.
type Match struct {
	Seller        production.Producer
	Buyer         production.Producer
	Order         production.Production // Rate = candidate quantity (units)
	UnitPrice     float64
	UnitTransport float64
}

// MatchAll crosses bids and asks product by product and calls execute
// for each match. Bids are served in descending price order (ties by
// posting order); each bid takes the ask with the lowest per-unit
// delivered cost (ask price plus per-unit transport) it can cross.
// execute returns the quantity actually traded (the state layer clamps
// by seller stock and buyer budget): 0 or an error skips that ask for
// this bid; a partial execution ends this bid's shopping (its budget is
// exhausted).
func (b *Book) MatchAll(unitTransport func(origin, destination point.Point) float64, execute func(Match) (float64, error)) {
	for _, product := range b.Products() {
		bids := make([]*Bid, len(b.bids[product]))
		copy(bids, b.bids[product])
		sort.SliceStable(bids, func(i, j int) bool {
			return bids[i].UnitPrice > bids[j].UnitPrice
		})
		for _, bid := range bids {
			skipped := make(map[*Ask]bool)
			for bid.Remaining > production.RateEpsilon {
				ask, qty, unitCost := b.bestDeliveredAsk(product, bid, skipped, unitTransport)
				if ask == nil || bid.UnitPrice < unitCost {
					break
				}
				m := Match{
					Seller:        ask.Seller,
					Buyer:         bid.Buyer,
					Order:         production.Production{Name: product, Rate: qty},
					UnitPrice:     ask.UnitPrice,
					UnitTransport: unitTransport(ask.Seller.Location(), bid.Buyer.Location()),
				}
				executed, err := execute(m)
				if err != nil || executed <= production.RateEpsilon {
					skipped[ask] = true
					continue
				}
				ask.Remaining -= executed
				bid.Remaining -= executed
				if executed < qty-production.RateEpsilon {
					// Partial fill: the buyer ran out of money; further
					// asks are unaffordable too this tick.
					break
				}
			}
		}
	}
}

// bestDeliveredAsk returns the live ask with the lowest per-unit
// delivered cost for this bid (nil if none remains), along with the
// candidate quantity and that per-unit cost.
func (b *Book) bestDeliveredAsk(
	product string,
	bid *Bid,
	skipped map[*Ask]bool,
	unitTransport func(point.Point, point.Point) float64,
) (*Ask, float64, float64) {
	var best *Ask
	var bestQty, bestCost float64
	for _, ask := range b.asks[product] {
		if skipped[ask] || ask.Remaining <= production.RateEpsilon || ask.Seller == bid.Buyer {
			continue
		}
		qty := math.Min(bid.Remaining, ask.Remaining)
		unitCost := ask.UnitPrice + unitTransport(ask.Seller.Location(), bid.Buyer.Location())
		if best == nil || unitCost < bestCost {
			best, bestQty, bestCost = ask, qty, unitCost
		}
	}
	return best, bestQty, bestCost
}
```

Then make `state/orders.go` compile against the new signature WITHOUT changing behavior yet — in `matchOrders`, adapt the callback:

```go
func (s *State) matchOrders(l *slog.Logger) {
	s.book.MatchAll(recipes.UnitTransportCost, func(m market.Match) (float64, error) {
		if err := s.signContract(l, m); err != nil {
			return 0, err
		}
		return m.Order.Rate, nil
	})
}
```

And in `signContract`, replace the one use of the renamed field: `TransportCost: m.TransportCost,` becomes `TransportCost: m.UnitTransport * m.Order.Rate,` (temporary bridge — Task 7 replaces signContract entirely). `renegotiate.go` builds a `market.Match` literal with `TransportCost:`; rename that field use to `UnitTransport:` and set it to `transportCost / purchase.Order.Rate` (temporary bridge — Task 7 deletes this file).

- [ ] **Step 4: Run tests, verify pass**

Run: `go test ./... -short`
Expected: PASS (market tests green; state tests still green via the bridges).

- [ ] **Step 5: Commit**

```bash
git add market/match.go market/match_test.go state/orders.go state/renegotiate.go
git commit -m "feat: market matching executes partial-quantity trades with per-unit transport"
```

---

### Task 6: trade ledger (additive)

**Files:**
- Create: `state/trades.go`, `state/trades_test.go`

**Interfaces:**
- Produces (all in package `state`):
  - `const tradeMemoryTicks = 500`
  - `type trade struct { tick int; seller, buyer production.Producer; product string; qty, unitPrice float64 }`
  - `type tradeLedger struct { trades []trade }`
  - `(*tradeLedger).record(tick int, seller, buyer production.Producer, product string, qty, unitPrice float64)`
  - `(*tradeLedger).prune(tick, memoryTicks int)`
  - `(*tradeLedger).edges() []tradeEdge` where `type tradeEdge struct { seller, buyer production.Producer; qty float64 }` — aggregated by (seller, buyer) pair in FIRST-SEEN order (deterministic; no map iteration in the output path)
  - `(*tradeLedger).recentSellers() map[production.Producer]bool`

- [ ] **Step 1: Write failing tests**

`state/trades_test.go`:

```go
package state

import (
	"testing"

	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
	"github.com/paul-freeman/satisfactory-story/resources"
)

func testResourceAt(x, y int) *resources.Resource {
	return &resources.Resource{
		Production: production.Production{Name: "OreIron", Rate: 1},
		Loc:        point.Point{X: x, Y: y},
	}
}

func Test_tradeLedger(t *testing.T) {
	a := testResourceAt(0, 0)
	b := testResourceAt(10, 0)
	c := testResourceAt(20, 0)

	tl := &tradeLedger{}
	tl.record(100, a, b, "OreIron", 2, 1.5)
	tl.record(101, a, b, "OreIron", 3, 1.5)
	tl.record(102, c, b, "OreIron", 1, 2.0)

	edges := tl.edges()
	if len(edges) != 2 {
		t.Fatalf("edges = %d, want 2 (a->b aggregated, c->b)", len(edges))
	}
	if edges[0].seller != production.Producer(a) || edges[0].qty != 5 {
		t.Fatalf("first edge = %+v, want a->b qty 5", edges[0])
	}
	if edges[1].seller != production.Producer(c) || edges[1].qty != 1 {
		t.Fatalf("second edge = %+v, want c->b qty 1", edges[1])
	}

	if !tl.recentSellers()[production.Producer(a)] || tl.recentSellers()[production.Producer(b)] {
		t.Fatal("recentSellers should contain a (sold) and not b (only bought)")
	}

	tl.prune(700, 500) // drops trades older than tick 200
	if len(tl.trades) != 3 {
		t.Fatalf("prune(700, 500) kept %d, want 3 (all within window)", len(tl.trades))
	}
	tl.prune(1000, 500) // window now starts at 500: everything dropped
	if len(tl.trades) != 0 {
		t.Fatalf("prune(1000, 500) kept %d, want 0", len(tl.trades))
	}
}
```

- [ ] **Step 2: Run tests, verify they fail**

Run: `go test ./state -run Test_tradeLedger -v`
Expected: FAIL (undefined tradeLedger).

- [ ] **Step 3: Implement**

`state/trades.go`:

```go
package state

import (
	"github.com/paul-freeman/satisfactory-story/production"
)

// tradeMemoryTicks is the rolling window for the trade ledger and the
// factories' own trade memories: it feeds the wire transport links,
// lastTrade prices, and movement gradients. A milestone tuning knob.
const tradeMemoryTicks = 500

// trade is one executed spot trade.
type trade struct {
	tick      int
	seller    production.Producer
	buyer     production.Producer
	product   string
	qty       float64
	unitPrice float64
}

// tradeLedger is the rolling record of recent trades.
type tradeLedger struct {
	trades []trade
}

func (tl *tradeLedger) record(tick int, seller, buyer production.Producer, product string, qty, unitPrice float64) {
	tl.trades = append(tl.trades, trade{
		tick: tick, seller: seller, buyer: buyer,
		product: product, qty: qty, unitPrice: unitPrice,
	})
}

// prune drops trades older than memoryTicks.
func (tl *tradeLedger) prune(tick, memoryTicks int) {
	kept := tl.trades[:0]
	for _, tr := range tl.trades {
		if tick-tr.tick <= memoryTicks {
			kept = append(kept, tr)
		}
	}
	tl.trades = kept
}

// tradeEdge is an aggregated seller->buyer flow over the window.
type tradeEdge struct {
	seller production.Producer
	buyer  production.Producer
	qty    float64
}

// edges aggregates the ledger by (seller, buyer) pair, in first-seen
// order so output is deterministic.
func (tl *tradeLedger) edges() []tradeEdge {
	type pair struct{ s, b production.Producer }
	index := make(map[pair]int)
	edges := make([]tradeEdge, 0)
	for _, tr := range tl.trades {
		key := pair{tr.seller, tr.buyer}
		if i, ok := index[key]; ok {
			edges[i].qty += tr.qty
			continue
		}
		index[key] = len(edges)
		edges = append(edges, tradeEdge{seller: tr.seller, buyer: tr.buyer, qty: tr.qty})
	}
	return edges
}

// recentSellers is the set of producers that sold anything within the
// window (used for the wire "active" flag on resources).
func (tl *tradeLedger) recentSellers() map[production.Producer]bool {
	sellers := make(map[production.Producer]bool)
	for _, tr := range tl.trades {
		sellers[tr.seller] = true
	}
	return sellers
}
```

- [ ] **Step 4: Run tests, verify pass**

Run: `go test ./state -run Test_tradeLedger -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add state/trades.go state/trades_test.go
git commit -m "feat: rolling trade ledger (additive)"
```

---

### Task 7: cutover A — production step, stock-backed orders, trade execution

**Files:**
- Create: `state/produce.go`
- Modify: `state/state.go` (Tick pipeline, State struct), `state/orders.go` (full rewrite of publishOrders + executeTrade replaces signContract)
- Delete: `state/renegotiate.go`, `state/renegotiate_test.go`
- Test: rewrite `state/orders_test.go`

**Interfaces:**
- Consumes: Tasks 1–6 (`Inventory`, `UnitTransportCost`, factory/resource/sink stock methods, new `MatchAll`, `tradeLedger`).
- Produces:
  - `const outputStockCapTicks = 60.0`, `const inputStockTargetTicks = 60.0` (in `state/produce.go`)
  - `(*State).produceGoods(l *slog.Logger)`
  - `(*State).executeTrade(l *slog.Logger, m market.Match) (float64, error)`
  - `State` struct gains `ledger *tradeLedger` (initialized in `getInitialState`).
  - New Tick pipeline: `produceGoods → publishOrders → matchOrders → moveProducers → spawnNewProducer(gated) → applySolvency → adjustPrices`, plus `s.ledger.prune(...)` and factory `PruneTrades(...)` each tick. `renegotiateContracts` call REMOVED.
- NOTE: `applySolvency`, `adjustPrices`, `spawnNewProducer` are NOT touched in this task (Tasks 8–9). Contract fields still exist but no contract is ever signed after this task. Existing `solvency_test.go` / `prices_test.go` / `spawn_test.go` keep passing because they unit-test those mechanisms directly.

- [ ] **Step 1: Write failing tests**

Replace `state/orders_test.go` entirely with:

```go
package state

import (
	"log/slog"
	"os"
	"testing"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/market"
	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
	"github.com/paul-freeman/satisfactory-story/resources"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func newTestState() *State {
	return &State{
		book:      market.NewBook(),
		lastTrade: make(map[string]float64),
		ledger:    &tradeLedger{},
	}
}

func Test_publishOrders_stockBacked(t *testing.T) {
	s := newTestState()
	r := &resources.Resource{
		Production: production.Production{Name: "OreIron", Rate: 1},
		Loc:        point.Point{X: 0, Y: 0},
		Stock:      7,
	}
	f := factory.New("Smelter", "Recipe_IngotIron_C", point.Point{X: 100, Y: 0}, 0,
		production.Products{production.Production{Name: "OreIron", Rate: 1}},
		production.Products{production.Production{Name: "IronIngot", Rate: 1}},
		1000)
	f.OutputStock.Add("IronIngot", 3)
	f.InputStock.Add("OreIron", 10) // target is 60 -> hunger 50
	s.producers = []production.Producer{r, f}

	s.publishOrders(testLogger())

	ask, ok := s.book.BestAsk("OreIron")
	if !ok || ask.Remaining != 7 {
		t.Fatalf("resource ask = %+v (ok=%v), want remaining 7 (stock-backed)", ask, ok)
	}
	ingotAsk, ok := s.book.BestAsk("IronIngot")
	if !ok || ingotAsk.Remaining != 3 {
		t.Fatalf("factory ask = %+v (ok=%v), want remaining 3 (stock-backed)", ingotAsk, ok)
	}
	bid, ok := s.book.BestBid("OreIron")
	if !ok || bid.Remaining != 50 {
		t.Fatalf("factory bid = %+v (ok=%v), want remaining 50 (hunger)", bid, ok)
	}
}

func Test_executeTrade_movesGoodsAndMoney(t *testing.T) {
	s := newTestState()
	s.tick = 42
	r := &resources.Resource{
		Production: production.Production{Name: "OreIron", Rate: 1},
		Loc:        point.Point{X: 0, Y: 0},
		Stock:      10,
	}
	f := factory.New("Smelter", "Recipe_IngotIron_C", point.Point{X: 10000, Y: 0}, 0,
		production.Products{production.Production{Name: "OreIron", Rate: 1}},
		production.Products{production.Production{Name: "IronIngot", Rate: 1}},
		100)
	s.producers = []production.Producer{r, f}

	m := market.Match{
		Seller:        r,
		Buyer:         f,
		Order:         production.Production{Name: "OreIron", Rate: 4},
		UnitPrice:     2.0,
		UnitTransport: 1.1,
	}
	executed, err := s.executeTrade(testLogger(), m)
	if err != nil {
		t.Fatalf("executeTrade error: %v", err)
	}
	if executed != 4 {
		t.Fatalf("executed = %v, want 4", executed)
	}
	if r.Stock != 6 {
		t.Fatalf("seller stock = %v, want 6", r.Stock)
	}
	if got := f.InputStock.Get("OreIron"); got != 4 {
		t.Fatalf("buyer input stock = %v, want 4", got)
	}
	// Buyer paid (2.0 + 1.1) * 4 = 12.4
	if got := f.Wallet.Cash(); got < 87.59 || got > 87.61 {
		t.Fatalf("buyer cash = %v, want 87.6", got)
	}
	if got := f.TickInputSpend; got < 12.39 || got > 12.41 {
		t.Fatalf("TickInputSpend = %v, want 12.4", got)
	}
	if s.lastTrade["OreIron"] != 2.0 {
		t.Fatalf("lastTrade = %v, want 2.0", s.lastTrade["OreIron"])
	}
	if len(s.ledger.trades) != 1 || s.ledger.trades[0].qty != 4 {
		t.Fatalf("ledger = %+v, want one trade of qty 4", s.ledger.trades)
	}
	if len(f.RecentTrades) != 1 || f.RecentTrades[0].Other != r.Location() {
		t.Fatalf("buyer trade memory = %+v, want seller location recorded", f.RecentTrades)
	}
}

func Test_executeTrade_budgetClamp(t *testing.T) {
	s := newTestState()
	r := &resources.Resource{
		Production: production.Production{Name: "OreIron", Rate: 1},
		Loc:        point.Point{X: 0, Y: 0},
		Stock:      10,
	}
	f := factory.New("Smelter", "Recipe_IngotIron_C", point.Point{X: 10000, Y: 0}, 0,
		production.Products{production.Production{Name: "OreIron", Rate: 1}},
		production.Products{production.Production{Name: "IronIngot", Rate: 1}},
		6.2) // can only afford 2 units at 3.1 delivered
	s.producers = []production.Producer{r, f}

	m := market.Match{
		Seller:        r,
		Buyer:         f,
		Order:         production.Production{Name: "OreIron", Rate: 10},
		UnitPrice:     2.0,
		UnitTransport: 1.1,
	}
	executed, err := s.executeTrade(testLogger(), m)
	if err != nil {
		t.Fatalf("executeTrade error: %v", err)
	}
	if executed < 1.99 || executed > 2.01 {
		t.Fatalf("executed = %v, want 2 (wallet clamp)", executed)
	}
	if got := f.Wallet.Cash(); got < -0.01 || got > 0.01 {
		t.Fatalf("buyer cash = %v, want ~0 (never negative from a purchase)", got)
	}
}
```

- [ ] **Step 2: Run tests, verify they fail**

Run: `go test ./state -run 'Test_publishOrders_stockBacked|Test_executeTrade' -v`
Expected: FAIL (State has no ledger field; publishOrders posts capacity not stock; executeTrade undefined).

- [ ] **Step 3: Implement**

3a. `state/state.go`: add `ledger *tradeLedger` to the `State` struct (next to `lastTrade`); in `getInitialState` add `s.ledger = &tradeLedger{}`.

3b. Create `state/produce.go`:

```go
package state

import (
	"log/slog"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/resources"
)

// outputStockCapTicks bounds every output buffer at this many ticks of
// production. When a buffer is full, production halts and input buying
// stops -- the back-pressure signal. A milestone tuning knob.
const outputStockCapTicks = 60.0

// inputStockTargetTicks is how many ticks of consumption a factory
// tries to keep on hand; the gap to it is the bid quantity (hunger).
// A milestone tuning knob.
const inputStockTargetTicks = 60.0

// produceGoods runs one tick of physical production for every producer:
// resources extract into stock, factories run their recipes against
// stock. Runs before the market so fresh goods are sellable this tick.
func (s *State) produceGoods(_ *slog.Logger) {
	for _, p := range s.producers {
		switch producer := p.(type) {
		case *resources.Resource:
			producer.ProduceTick(outputStockCapTicks)
		case *factory.Factory:
			producer.ProduceTick(outputStockCapTicks)
		}
	}
}
```

3c. Rewrite `state/orders.go` as:

```go
package state

import (
	"log/slog"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/market"
	"github.com/paul-freeman/satisfactory-story/recipes"
	"github.com/paul-freeman/satisfactory-story/resources"
	"github.com/paul-freeman/satisfactory-story/sink"
)

// publishOrders rebuilds the book from live physical state: every unit
// of stock on hand becomes an ask, every unit of input hunger becomes a
// bid. Zero-quantity orders are dropped by the book itself. Only prices
// persist between ticks (on the producers); quantities can never go
// stale because they are re-derived here every tick.
func (s *State) publishOrders(_ *slog.Logger) {
	s.book.Clear()
	for _, p := range s.producers {
		switch producer := p.(type) {
		case *resources.Resource:
			name := producer.Production.Name
			s.book.PostAsk(producer, name, producer.Stock, producer.AskPriceFor(name))
		case *factory.Factory:
			for _, output := range producer.Output {
				s.book.PostAsk(producer, output.Name,
					producer.OutputStock.Get(output.Name),
					producer.AskPriceFor(output.Name))
			}
			for _, input := range producer.Input {
				s.book.PostBid(producer, input.Name,
					producer.Hunger(input.Name, inputStockTargetTicks),
					producer.BidPriceFor(input.Name))
			}
		case *sink.Sink:
			for _, want := range producer.Input {
				s.book.PostBid(producer, want.Name, sinkDemandRate, producer.BidUnitPrice)
			}
		}
	}
}

// matchOrders crosses the book and executes a spot trade per match.
func (s *State) matchOrders(l *slog.Logger) {
	s.book.MatchAll(recipes.UnitTransportCost, func(m market.Match) (float64, error) {
		return s.executeTrade(l, m)
	})
}

// executeTrade is the only place trades become real: quantity is
// re-clamped against live seller stock and the buyer's wallet, then
// units and money move immediately. Returns the executed quantity.
// A factory buyer pays (unit price + unit transport) per unit and can
// never overdraw its wallet -- this hard budget is what keeps escalated
// bid prices honest. The transport share of the payment leaves the
// economy (it is a cost, not anyone's income).
func (s *State) executeTrade(l *slog.Logger, m market.Match) (float64, error) {
	qty := m.Order.Rate

	// Clamp by what the seller physically has.
	switch seller := m.Seller.(type) {
	case *resources.Resource:
		if seller.Stock < qty {
			qty = seller.Stock
		}
	case *factory.Factory:
		if have := seller.OutputStock.Get(m.Order.Name); have < qty {
			qty = have
		}
	default:
		return 0, nil // sinks never sell
	}

	// Clamp by what the buyer can pay (sinks have infinite money).
	unitDelivered := m.UnitPrice + m.UnitTransport
	if buyer, ok := m.Buyer.(*factory.Factory); ok && unitDelivered > 0 {
		if affordable := buyer.Wallet.Cash() / unitDelivered; affordable < qty {
			qty = affordable
		}
	}
	if qty <= production.RateEpsilon {
		return 0, nil
	}

	// Move the goods.
	switch seller := m.Seller.(type) {
	case *resources.Resource:
		seller.Stock -= qty
	case *factory.Factory:
		seller.OutputStock.Take(m.Order.Name, qty)
		seller.TickRevenue += qty * m.UnitPrice
		seller.Wallet.Adjust(qty * m.UnitPrice)
		seller.RecordTrade(s.tick, m.Buyer.Location(), qty)
	}
	switch buyer := m.Buyer.(type) {
	case *factory.Factory:
		buyer.InputStock.Add(m.Order.Name, qty)
		buyer.Wallet.Adjust(-qty * unitDelivered)
		buyer.TickInputSpend += qty * unitDelivered
		buyer.RecordTrade(s.tick, m.Seller.Location(), qty)
	case *sink.Sink:
		buyer.RecordDelivery(m.Order.Name, qty)
	}

	s.lastTrade[m.Order.Name] = m.UnitPrice
	s.ledger.record(s.tick, m.Seller, m.Buyer, m.Order.Name, qty, m.UnitPrice)
	l.Debug("executed trade",
		slog.String("product", m.Order.Name),
		slog.Float64("qty", qty),
		slog.Float64("unitPrice", m.UnitPrice),
		slog.Float64("unitTransport", m.UnitTransport),
	)
	return qty, nil
}
```

(Add `"github.com/paul-freeman/satisfactory-story/production"` to this file's imports for `production.RateEpsilon`.)

(Note: `signContract` is deleted by this rewrite. If anything else still references it the build will say so — the only other caller was `renegotiate.go`, deleted next.)

3d. Delete `state/renegotiate.go` and `state/renegotiate_test.go` (`git rm`).

3e. In `state/state.go` `Tick`, replace the pipeline block with:

```go
	// Physical production first, then discovery: the book is rebuilt
	// from live stock and crossed, so every later mechanism this tick
	// (moving, spawning, solvency, price adjustment) sees post-trade
	// reality.
	s.produceGoods(l)
	s.publishOrders(l)
	s.matchOrders(l)
	s.moveProducers(l)
	if s.randSrc.Float64() < spawnProbabilityPerTick {
		s.spawnNewProducer(l)
	}
	s.applySolvency(l)
	s.adjustPrices(l)
	s.ledger.prune(s.tick, tradeMemoryTicks)
	for _, p := range s.producers {
		if f, ok := p.(*factory.Factory); ok {
			f.PruneTrades(s.tick, tradeMemoryTicks)
		}
	}
```

- [ ] **Step 4: Run the full suite**

Run: `go test ./... -short`
Expected: PASS. If `state_test.go`'s basic tick tests fail on behavior (not compile), inspect: they only assert `Tick` returns no error, so they should pass. `solvency_test.go`/`prices_test.go`/`spawn_test.go` unit-test their mechanisms directly and must still pass unmodified in this task.

- [ ] **Step 5: Commit**

```bash
git add -A state/
git commit -m "feat: cutover to stock-backed orders and spot-trade execution"
```

---

### Task 8: cutover B — solvency (salvage trickle) and prices (stock signals)

**Files:**
- Modify: `state/solvency.go`, `state/prices.go`
- Test: rewrite `state/solvency_test.go`, `state/prices_test.go`

**Interfaces:**
- Consumes: factory EMA/flow methods (Task 3), stock fields.
- Produces:
  - `const salvageTrickleFraction = 0.25` and `const inputSpendSmoothing = 0.05` (in `state/solvency.go`)
  - `applySolvency`: per factory — salvage trickle on capped outputs, `FoldTickFlows`, single `Wallet.Apply(salvage - upkeepPerTick)`, insolvency removal (trade money already moved via `Adjust` at trade time)
  - `adjustPrices`: ask raise on sold-out stock / decay toward `StockMarginalUnitCost` (factory) or `MinUnitPrice` (resource); bid raise while hunger unfilled; NO affordability functions (deleted)

- [ ] **Step 1: Rewrite the tests**

Replace `state/solvency_test.go` content with tests that build factories via `factory.New` (as in Task 7's tests) and assert:

```go
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
```

Replace `state/prices_test.go` content with:

```go
func Test_adjustPrices_askStockSignals(t *testing.T) {
	s := newTestState()
	f := factory.New("Plates", "Recipe_Plates_C", point.Point{X: 0, Y: 0}, 0,
		production.Products{production.Production{Name: "IronIngot", Rate: 1}},
		production.Products{production.Production{Name: "IronPlate", Rate: 2}},
		100)
	s.producers = []production.Producer{f}
	f.SetAskPrice("IronPlate", 10)

	// Sold out (no stock -> ask fully consumed or never posted): the
	// remaining=0 ask means scarcity -> raise.
	s.book.Clear()
	s.book.PostAsk(f, "IronPlate", 5, 10)
	s.book.Asks("IronPlate")[0].Remaining = 0
	s.adjustPrices(testLogger())
	if got := f.AskPriceFor("IronPlate"); got != 10*(1+askRaisePct) {
		t.Fatalf("sold-out ask = %v, want %v", got, 10*(1+askRaisePct))
	}

	// Unsold stock: decay toward the stock marginal cost floor.
	f.SetAskPrice("IronPlate", 10)
	f.AvgInputSpend = 0 // floor = upkeep/2 = 0.25
	s.book.Clear()
	s.book.PostAsk(f, "IronPlate", 5, 10)
	s.adjustPrices(testLogger())
	if got := f.AskPriceFor("IronPlate"); got != 10*(1-askLowerPct) {
		t.Fatalf("unsold ask = %v, want %v", got, 10*(1-askLowerPct))
	}
}

func Test_adjustPrices_bidRaisesWhileHungry(t *testing.T) {
	s := newTestState()
	f := factory.New("Plates", "Recipe_Plates_C", point.Point{X: 0, Y: 0}, 0,
		production.Products{production.Production{Name: "IronIngot", Rate: 1}},
		production.Products{production.Production{Name: "IronPlate", Rate: 2}},
		100)
	s.producers = []production.Producer{f}
	f.SetBidPrice("IronIngot", 1.0)

	s.book.Clear()
	s.book.PostBid(f, "IronIngot", 5, 1.0) // unfilled hunger
	s.adjustPrices(testLogger())
	if got := f.BidPriceFor("IronIngot"); got != 1.0*(1+bidRaisePct) {
		t.Fatalf("hungry bid = %v, want %v (no affordability gate anymore)", got, 1.0*(1+bidRaisePct))
	}

	// Fully filled bid: no raise.
	f.SetBidPrice("IronIngot", 1.0)
	s.book.Clear()
	s.book.PostBid(f, "IronIngot", 5, 1.0)
	s.book.Bids("IronIngot")[0].Remaining = 0
	s.adjustPrices(testLogger())
	if got := f.BidPriceFor("IronIngot"); got != 1.0 {
		t.Fatalf("filled bid = %v, want 1.0 (unchanged)", got)
	}
}
```

(Reuse `newTestState` and `testLogger` from `state/orders_test.go` — same package.)

- [ ] **Step 2: Run tests, verify they fail**

Run: `go test ./state -run 'Test_applySolvency|Test_adjustPrices' -v`
Expected: FAIL (old implementations reference contracts / affordability).

- [ ] **Step 3: Implement**

Rewrite `state/solvency.go`:

```go
package state

import (
	"log/slog"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/production"
)

// upkeepPerTick is the fixed cost every factory pays per tick just for
// existing. It is the clock on failure and the drain that balances the
// sinks' money faucet.
const upkeepPerTick = 0.5

// insolvencyGrace is how many consecutive ticks a factory's wallet may
// sit below zero before it is removed as bankrupt. Purchases can never
// overdraw a wallet (budget clamp at trade time); only upkeep drags a
// wallet negative, so this is a pure staying-power window.
const insolvencyGrace = 300

// floorUnitPrice is the salvage value of one unsold unit: every factory
// feeds overflow to its own on-site AWESOME sink. Well below any
// realistic traded price so real trade always dominates.
const floorUnitPrice = 0.1

// salvageTrickleFraction is how much of one tick's output rate a factory
// with a FULL output buffer may salvage per tick. Deliberately less than
// 1: a buyer-less factory keeps producing at only this fraction of its
// rate, so reduced input buying still propagates the no-demand signal
// upstream. A milestone tuning knob.
const salvageTrickleFraction = 0.25

// inputSpendSmoothing is the EMA weight for folding per-tick trade flows
// into AvgInputSpend/AvgRevenue.
const inputSpendSmoothing = 0.05

// applySolvency runs each factory's tick economics: salvage trickle on
// capped outputs, fold trade flows into the EMAs, apply upkeep, remove
// the persistently insolvent. Trade money itself already moved at trade
// time (executeTrade); this is the once-per-tick accounting call.
func (s *State) applySolvency(l *slog.Logger) {
	survivors := make([]production.Producer, 0, len(s.producers))
	for _, p := range s.producers {
		f, ok := p.(*factory.Factory)
		if !ok {
			survivors = append(survivors, p)
			continue
		}

		salvage := 0.0
		for _, output := range f.Output {
			cap := output.Rate * outputStockCapTicks
			if f.OutputStock.Get(output.Name) >= cap-production.RateEpsilon {
				qty := f.OutputStock.Take(output.Name, output.Rate*salvageTrickleFraction)
				salvage += qty * floorUnitPrice
			}
		}
		f.TickRevenue += salvage
		f.FoldTickFlows(inputSpendSmoothing)
		f.Wallet.Apply(salvage - upkeepPerTick)

		if f.Wallet.InsolventFor(insolvencyGrace) {
			l.Debug("removing bankrupt factory",
				slog.String("factory", f.String()),
				slog.Float64("cash", f.Wallet.Cash()))
			continue // not kept: the factory and its stock vanish
		}

		survivors = append(survivors, f)
	}
	s.producers = survivors
}
```

(If `factory.Remove()` still exists at this point it is no longer called here — a removed factory has no contracts to cancel. Task 12 deletes it.)

Rewrite `state/prices.go`:

```go
package state

import (
	"log/slog"
	"math"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/production"
	"github.com/paul-freeman/satisfactory-story/resources"
)

// askRaisePct is how much a seller raises a product's ask price after
// selling out its stock, and askLowerPct how much it lowers while stock
// goes unsold. Raising faster than lowering damps oscillation.
const askRaisePct = 0.05
const askLowerPct = 0.02

// bidRaisePct is how much a buyer escalates an unfilled input bid per
// tick. The escalating bid is the backward demand cascade. There is no
// affordability precondition anymore: the wallet clamp at trade time is
// the hard constraint, so a high bid with no money behind it buys
// nothing (this is what makes speculative price loops harmless).
const bidRaisePct = 0.02

// adjustPrices lets every producer react locally to this tick's fill
// outcomes. No market-wide price is computed anywhere.
func (s *State) adjustPrices(_ *slog.Logger) {
	for _, product := range s.book.Products() {
		for _, ask := range s.book.Asks(product) {
			soldOut := ask.Remaining <= production.RateEpsilon
			switch seller := ask.Seller.(type) {
			case *factory.Factory:
				if soldOut {
					seller.SetAskPrice(product, seller.AskPriceFor(product)*(1+askRaisePct))
				} else {
					floor := seller.StockMarginalUnitCost(upkeepPerTick)
					seller.SetAskPrice(product,
						math.Max(floor, seller.AskPriceFor(product)*(1-askLowerPct)))
				}
			case *resources.Resource:
				if soldOut {
					seller.SetAskPrice(product, seller.AskPriceFor(product)*(1+askRaisePct))
				} else {
					seller.SetAskPrice(product,
						math.Max(production.MinUnitPrice, seller.AskPriceFor(product)*(1-askLowerPct)))
				}
			}
		}

		for _, bid := range s.book.Bids(product) {
			if bid.Remaining <= production.RateEpsilon {
				continue
			}
			buyer, ok := bid.Buyer.(*factory.Factory)
			if !ok {
				continue // sink bids are fixed
			}
			buyer.SetBidPrice(product, buyer.BidPriceFor(product)*(1+bidRaisePct))
		}
	}
}
```

(`achievableRevenue` and `plannedSpend` are deleted by this rewrite.)

- [ ] **Step 4: Run the full suite**

Run: `go test ./... -short`
Expected: PASS. `spawn_test.go` still passes untouched (spawn not modified yet). If `state_test.go`'s 1000-tick smoke test panics on anything contract-related, fix the reference the compiler/test names — but no contract is created anywhere now, so it should not.

- [ ] **Step 5: Commit**

```bash
git add state/solvency.go state/solvency_test.go state/prices.go state/prices_test.go
git commit -m "feat: solvency salvage trickle and stock-signal price adjustment"
```

---

### Task 9: cutover C — spawning (crowding discount, transport-aware estimates)

**Files:**
- Modify: `state/spawn.go`
- Test: rewrite `state/spawn_test.go`

**Interfaces:**
- Consumes: `estimatedUnitCost` (kept), factory `RecipeClass` field, `factory.New` (unchanged signature).
- Produces:
  - `const defaultTransportEstimate = 2.0`
  - `(*State).estimatedDeliveredCost(product string) float64` = `estimatedUnitCost(product) + defaultTransportEstimate`
  - `expectedProfit` uses `estimatedDeliveredCost` for inputs.
  - Spawn weight `= (baselineOpportunityWeight + max(0, expectedProfit)) / (1 + liveFactoriesRunningRecipe)`.
  - Seed capital `= sum_i(estimatedDeliveredCost(input_i) * rate_i) * inputStockTargetTicks + upkeepPerTick * seedCapitalBufferTicks`.

- [ ] **Step 1: Rewrite the spawn tests**

Replace `state/spawn_test.go` with:

```go
package state

import (
	"math/rand"
	"testing"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
)

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
```

For `testRecipe(t)`: the existing `spawn_test.go` almost certainly already constructs a `*recipes.Recipe` fixture — KEEP that existing helper/mechanism exactly as it is (rename to `testRecipe` if needed). If it builds recipes via JSON parsing, keep doing that; only the assertions above are new. The fixture must have exactly one input with Rate 1 and be Active.

- [ ] **Step 2: Run tests, verify they fail**

Run: `go test ./state -run 'Test_estimatedDeliveredCost|Test_spawnWeights|Test_spawnNewProducer' -v`
Expected: FAIL (undefined estimatedDeliveredCost/recipeCrowding; seed formula differs).

- [ ] **Step 3: Implement**

In `state/spawn.go`:

Add constant (near the others):

```go
// defaultTransportEstimate is the flat per-unit freight allowance used
// in every cost estimate. Precision only needs to prevent the verified
// transport-blindness failure (estimates of ~0.01 against delivered
// costs of ~1-2), not price real freight.
const defaultTransportEstimate = 2.0
```

Add functions:

```go
// estimatedDeliveredCost is the best current estimate of what one unit
// of product costs to buy AND ship here.
func (s *State) estimatedDeliveredCost(product string) float64 {
	return s.estimatedUnitCost(product) + defaultTransportEstimate
}

// recipeCrowding counts live factories per recipe class. Reading the
// producer population is public market state (see the spec's purity
// line) -- it is not the recipe tree.
func (s *State) recipeCrowding() map[string]int {
	crowd := make(map[string]int)
	for _, p := range s.producers {
		if f, ok := p.(*factory.Factory); ok {
			crowd[f.RecipeClass]++
		}
	}
	return crowd
}
```

In `spawnNewProducer`, replace the weight loop with:

```go
	// Crowding discount: a recipe's opportunity is shared by every live
	// factory already running it, so saturated niches stop attracting
	// entrants and the draw naturally walks to the next unserved tier.
	crowd := s.recipeCrowding()
	weights := make([]float64, len(activeRecipes))
	total := 0.0
	for i, recipe := range activeRecipes {
		weights[i] = (baselineOpportunityWeight + math.Max(0, s.expectedProfit(recipe))) /
			float64(1+crowd[recipe.ID()])
		total += weights[i]
	}
```

Replace the seed-capital block with:

```go
	// Seed capital: enough to fill the input-stock target at estimated
	// delivered prices, plus an upkeep runway.
	stockCost := 0.0
	for _, input := range chosenRecipe.Inputs() {
		stockCost += s.estimatedDeliveredCost(input.Name) * input.Rate
	}
	seedCapital := stockCost*inputStockTargetTicks + upkeepPerTick*seedCapitalBufferTicks
```

In `expectedProfit`, replace the input-cost loop:

```go
	cost := upkeepPerTick
	for _, input := range r.Inputs() {
		cost += s.estimatedDeliveredCost(input.Name) * input.Rate
	}
```

Update the doc comment on `seedCapitalBufferTicks` to say it is now only the upkeep-runway component (stock cost is budgeted separately).

- [ ] **Step 4: Run the full suite**

Run: `go test ./... -short`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add state/spawn.go state/spawn_test.go
git commit -m "feat: crowding-discounted, transport-aware spawning with stock-sized seed capital"
```

---

### Task 10: movement from trade memory; sinks hold still

**Files:**
- Modify: `factory/factory.go` (Move/transportCostsAt/moveTo), `sink/sink.go` (delete Move/transportCostsAt/moveTo)
- Test: `factory/factory_test.go` (append)

**Interfaces:**
- Consumes: `RecentTrades` (Task 3), `recipes.UnitTransportCost` (Task 2).
- Produces: `(*factory.Factory).Move()` gradient over `RecentTrades` (qty-weighted); `sink.Sink` no longer satisfies `production.MoveableProducer` (sinks are the player's base — fixed).

- [ ] **Step 1: Write failing test**

Append to `factory/factory_test.go`:

```go
func Test_Factory_Move_climbsTowardTradePartners(t *testing.T) {
	f := New("Plates", "Recipe_Plates_C", point.Point{X: 0, Y: 0}, 0,
		production.Products{production.Production{Name: "IronIngot", Rate: 1}},
		production.Products{production.Production{Name: "IronPlate", Rate: 2}},
		100)
	// No trades: holds still.
	if err := f.Move(); err != nil {
		t.Fatalf("Move error: %v", err)
	}
	if f.Loc.X != 0 || f.Loc.Y != 0 {
		t.Fatalf("tradeless factory moved to %v, want (0,0)", f.Loc)
	}
	// One partner far to the east: moves toward it (X increases).
	f.RecordTrade(1, point.Point{X: 100000, Y: 0}, 5)
	if err := f.Move(); err != nil {
		t.Fatalf("Move error: %v", err)
	}
	if f.Loc.X <= 0 {
		t.Fatalf("factory at %v, want X > 0 (moved toward partner)", f.Loc)
	}
}
```

- [ ] **Step 2: Run test, verify it fails**

Run: `go test ./factory -run Test_Factory_Move_climbsTowardTradePartners -v`
Expected: FAIL (Move still reads Purchases/Sales; with no contracts the factory holds still even with RecentTrades).

- [ ] **Step 3: Implement**

In `factory/factory.go`:

Replace the `Move` guard and `transportCostsAt`/`moveTo`:

```go
func (f *Factory) Move() error {
	if len(f.RecentTrades) == 0 {
		// No trades means no transport-cost gradient to climb -- the
		// tie-break below would otherwise always pick the same neighbor
		// and the factory would march off the map forever while it
		// waits for its first trade.
		return nil
	}
	// ... keep the existing four-direction hill-climb body unchanged ...
}

// transportCostsAt scores a location against the factory's remembered
// trade partners, weighted by traded quantity.
func (f *Factory) transportCostsAt(p point.Point) float64 {
	c := 0.0
	for _, tr := range f.RecentTrades {
		c += tr.Qty * recipes.UnitTransportCost(p, tr.Other)
	}
	return c
}

func (f *Factory) moveTo(loc point.Point) {
	f.Loc = loc
}
```

(The old `moveTo` recomputed contract transport costs — with spot trades there is nothing to update. The rest of `Move`'s body — the up/down/left/right comparisons — stays byte-for-byte.)

In `sink/sink.go`: delete `Move`, `transportCostsAt`, and `moveTo` entirely, and delete the `recipes` import if now unused. Change `IsMovable` to return `false` with the comment `// Sinks are the player's base: fixed at world center.` The `var _ production.Producer = (*Sink)(nil)` assertion still holds; sinks simply stop matching the `MoveableProducer` type switch in `state.moveProducers`.

- [ ] **Step 4: Run the full suite**

Run: `go test ./... -short`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add factory/factory.go factory/factory_test.go sink/sink.go
git commit -m "feat: movement follows recent trade partners; sinks hold still"
```

---

### Task 11: wire format from the ledger

**Files:**
- Modify: `state/state.go` (`toHTTP`, `shortagesForWire` unchanged), `state/http/http.go` (NO type changes — verify only)
- Test: `state/state_test.go` (append)

**Interfaces:**
- Consumes: `tradeLedger.edges()`, `recentSellers()` (Task 6), factory `ProducedLastTick`, `AvgRevenue`, `AvgInputSpend`, sink `TotalDelivered()`.
- Produces: `toHTTP` builds `Transports` from ledger edges (`Rate = qty / min(tick, tradeMemoryTicks)`), resource `Active` from `recentSellers()`, factory `Profitability = AvgRevenue / (AvgInputSpend + upkeepPerTick)` (0 if NaN/Inf), factory `Recipe` label = `Name + " (idle)"` when `!ProducedLastTick`, sink `Label = Name + fmt.Sprintf(" Sink (%.1f delivered)", TotalDelivered())`.

- [ ] **Step 1: Write failing test**

Append to `state/state_test.go`:

```go
func Test_toHTTP_ledgerTransports(t *testing.T) {
	s := newTestState()
	s.tick = 100
	r := &resources.Resource{
		Production: production.Production{Name: "OreIron", Rate: 1},
		Loc:        point.Point{X: 0, Y: 0},
		Stock:      5,
	}
	f := factory.New("Smelter", "Recipe_IngotIron_C", point.Point{X: 500, Y: 0}, 0,
		production.Products{production.Production{Name: "OreIron", Rate: 1}},
		production.Products{production.Production{Name: "IronIngot", Rate: 1}},
		100)
	s.producers = []production.Producer{r, f}
	s.ledger.record(90, r, f, "OreIron", 30, 1.0)
	s.ledger.record(95, r, f, "OreIron", 20, 1.0)

	wire := s.toHTTP()
	if len(wire.Transports) != 1 {
		t.Fatalf("transports = %d, want 1 aggregated edge", len(wire.Transports))
	}
	tr := wire.Transports[0]
	if tr.Origin.X != 0 || tr.Destination.X != 500 {
		t.Fatalf("transport endpoints = %+v, want origin X 0 -> dest X 500", tr)
	}
	wantRate := 50.0 / 100.0 // qty over window (tick < tradeMemoryTicks)
	if tr.Rate < wantRate-1e-9 || tr.Rate > wantRate+1e-9 {
		t.Fatalf("transport rate = %v, want %v", tr.Rate, wantRate)
	}
	if len(wire.Resources) != 1 || !wire.Resources[0].Active {
		t.Fatal("resource with a recent sale should be Active")
	}
}
```

- [ ] **Step 2: Run test, verify it fails**

Run: `go test ./state -run Test_toHTTP_ledgerTransports -v`
Expected: FAIL (toHTTP still walks ContractsIn; transports empty).

- [ ] **Step 3: Implement**

Rewrite `toHTTP` in `state/state.go`:

```go
func (s *State) toHTTP() statehttp.State {
	resources := make([]statehttp.Resource, 0)
	factories := make([]statehttp.Factory, 0)
	sinks := make([]statehttp.Sink, 0)

	recentSellers := s.ledger.recentSellers()

	for _, p := range s.producers {
		switch producer := p.(type) {
		case *storyresources.Resource:
			resources = append(resources, statehttp.Resource{
				Location: statehttp.Location{
					X: producer.Location().X,
					Y: producer.Location().Y,
				},
				Recipe:        producer.Production.Name,
				Product:       producer.Production.Name,
				Profitability: 0,
				Active:        recentSellers[p],
			})
		case *factory.Factory:
			products := make([]string, 0)
			for _, product := range producer.Products() {
				products = append(products, product.Name)
			}
			profitability := producer.AvgRevenue / (producer.AvgInputSpend + upkeepPerTick)
			if math.IsNaN(profitability) || math.IsInf(profitability, 0) {
				profitability = 0
			}
			label := producer.Name
			if !producer.ProducedLastTick {
				label += " (idle)"
			}
			factories = append(factories, statehttp.Factory{
				Location: statehttp.Location{
					X: producer.Location().X,
					Y: producer.Location().Y,
				},
				Recipe:        label,
				Products:      products,
				Profitability: profitability,
				Cash:          producer.Cash(),
			})
		case *sink.Sink:
			sinks = append(sinks, statehttp.Sink{
				Location: statehttp.Location{
					X: producer.Location().X,
					Y: producer.Location().Y,
				},
				Label: fmt.Sprintf("%s Sink (%.1f delivered)", producer.Name, producer.TotalDelivered()),
			})
		}
	}

	// Transport links: aggregated recent trades. Rate is volume over the
	// visible window so long-standing routes read stronger than blips.
	window := s.tick
	if window > tradeMemoryTicks {
		window = tradeMemoryTicks
	}
	if window < 1 {
		window = 1
	}
	transports := make([]statehttp.Transport, 0)
	for _, edge := range s.ledger.edges() {
		transports = append(transports, statehttp.Transport{
			Origin: statehttp.Location{
				X: edge.seller.Location().X,
				Y: edge.seller.Location().Y,
			},
			Destination: statehttp.Location{
				X: edge.buyer.Location().X,
				Y: edge.buyer.Location().Y,
			},
			Rate: edge.qty / float64(window),
		})
	}

	bounds := statehttp.Bounds{
		Xmin: s.xmin,
		Xmax: s.xmax,
		Ymin: s.ymin,
		Ymax: s.ymax,
	}

	return statehttp.State{
		Resources:  resources,
		Factories:  factories,
		Transports: transports,
		Sinks:      sinks,
		Shortages:  s.shortagesForWire(),
		Tick:       s.tick,
		Running:    s.cancel != nil,
		Bounds:     bounds,
	}
}
```

Verify `state/http/http.go` wire structs need NO changes (field names unchanged). Also update the 1000-tick smoke test in `state_test.go` if it logs `f.Profit()` — change that line to log `f.Cash()` instead.

- [ ] **Step 4: Run suite + frontend build**

Run: `go test ./... -short` — expected PASS.
Run: `cd frontend && npm run build` — expected: builds cleanly (no wire shape change).

- [ ] **Step 5: Commit**

```bash
git add state/state.go state/state_test.go
git commit -m "feat: wire transports and status from the trade ledger"
```

---

### Task 12: delete the contract machinery

**Files:**
- Delete: `production/contract.go` (and any contract-only test file)
- Modify: `production/products.go` (Producer interface), `factory/factory.go`, `resources/resources.go`, `sink/sink.go`, any test files still referencing contracts

**Interfaces:**
- Produces the FINAL `production.Producer` interface:

```go
type Producer interface {
	// Location returns the location of the producer.
	Location() point.Point
	// Products returns the products that the producer produces.
	Products() Products
}
```

`MoveableProducer` keeps `Producer + Move() error` (drop `Remove()` from it if nothing calls it — check first; `SetRecipe` in `state/state.go` calls `f.Remove()`: replace that call by filtering the producer out of `s.producers` directly, since cancelling contracts is no longer a thing).

- [ ] **Step 1: Delete and chase the compiler**

```bash
git rm production/contract.go
go build ./... 2>&1 | head -50
```

Remove, in each named file, everything the compiler flags:

- `factory/factory.go`: fields `Purchases`, `Sales`; methods `Remove` (replace body: see below), `RemainingCapacityFor`, `HasCapacityFor`, `Profit`, `Profitability`, `SignAsBuyer`, `SignAsSeller`, `ContractsIn`, `Producing`, `UnmetInputRate`, `MarginalUnitCost` (the old contract-based one — `StockMarginalUnitCost` stays). Keep `Remove` ONLY if `SetRecipe` still needs a hook — preferred: delete `Remove` and make `SetRecipe` filter `s.producers` directly:

```go
	if !enabled {
		// Remove all producers using this recipe.
		kept := make([]production.Producer, 0, len(s.producers))
		for _, p := range s.producers {
			if f, ok := p.(*factory.Factory); ok && f.RecipeClass == recipeID {
				continue
			}
			kept = append(kept, p)
		}
		s.producers = kept
	}
```

- `resources/resources.go`: field `Sales`; methods `Profit`, `Profitability`, `Cash`, `RemainingCapacityFor`, `HasCapacityFor`, `SignAsSeller`, `SignAsBuyer`, `ContractsIn` (keep `AskPriceFor`/`SetAskPrice`, `PrettyPrint`, `Location`, `Products`, `ProduceTick`).
- `sink/sink.go`: field `Purchases`; methods `Profit`, `Profitability`, `Cash`, `RemainingCapacityFor`, `HasCapacityFor`, `SignAsBuyer`, `SignAsSeller`, `ContractsIn`, `Remove`, `IsRemovable` (keep `Delivered`, `RecordDelivery`, `TotalDelivered`, `BidUnitPrice`, `Location`, `Products`, `String`, `IsMovable`).
- `production/products.go`: shrink `Producer` to the two-method interface above; adjust `MoveableProducer`.
- Any `_test.go` files still building contracts: delete those test functions (they test deleted behavior).
- `recipes/recipes.go`: delete the old `TransportCost` function if (and only if) `grep -rn "recipes.TransportCost" --include='*.go' .` returns nothing.

- [ ] **Step 2: Grep-verify the deletion is total**

```bash
grep -rn "Contract\b" --include='*.go' . | grep -v '_test.go' ; echo "---"
grep -rn "Producing()\|ContractsIn\|SignAsSeller\|SignAsBuyer\|HasCapacityFor\|RemainingCapacityFor" --include='*.go' .
```

Expected: no hits (or only hits inside comments you then also clean up).

- [ ] **Step 3: Run the full suite**

Run: `go build ./... && go test ./... -short && go vet ./...`
Expected: all green.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "refactor: delete contract machinery; Producer interface shrinks to Location+Products"
```

---

### Task 13: cascade tests on inventory semantics

**Files:**
- Modify: `state/cascade_test.go` (rewrite assertions and world setup where contract-based)

**Interfaces:**
- Consumes: everything above; sink `TotalDelivered()`.

- [ ] **Step 1: Read the existing file, keep its structure**

`state/cascade_test.go` builds small worlds (hand-picked resources, recipes, one goal sink) and asserts a delivery within a tick budget. Keep the world-building helpers; change:

1. Any construction of contracts, `Sales`/`Purchases`, or `Producing()` checks — delete.
2. The delivery check becomes:

```go
func delivered(s *State) bool {
	for _, p := range s.producers {
		if sk, ok := p.(*sink.Sink); ok && sk.TotalDelivered() > 0 {
			return true
		}
	}
	return false
}
```

3. Budgets: single-tier world must deliver within 2,000 ticks; two-tier within 5,000. (Stock targets are 60 ticks and bids escalate 2%/tick from the best ask, so first trades land within a few hundred ticks; these budgets are deliberately loose.)
4. The tests must still assert REAL chain assembly: after delivery, additionally assert that at least one factory has `ProducedLastTick == true` at some point (track a `sawProducing` bool across the loop) and that `s.ledger.trades` is non-empty at delivery time.

- [ ] **Step 2: Run the cascade tests**

Run: `go test ./state -run Test_cascade -v` (match the actual test names in the file)
Expected: PASS. If a cascade test fails, debug the mechanism (this is the small-scale proof the design depends on — do NOT loosen budgets beyond the stated numbers without flagging it in the task report).

- [ ] **Step 3: Run the full suite and commit**

```bash
go test ./... -short
git add state/cascade_test.go
git commit -m "test: cascade tests rebuilt on inventory semantics"
```

---

### Task 14: milestone test, tuning protocol, docs

**Files:**
- Modify: `state/state_test.go` (milestone subtest), `CLAUDE.md`

**Interfaces:**
- Consumes: everything.

- [ ] **Step 1: Rewrite the milestone subtest**

In `state/state_test.go`'s long-run subtest ("long run: real trade, niches, and a space-elevator delivery"):

1. `partDelivered` becomes: any sink with `strings.HasPrefix(sk.Name, spaceElevatorPartPrefix)` and `sk.TotalDelivered() > 0`.
2. The observability trackers become: `everProduced[output.Name] = true` for factories with `ProducedLastTick`; `producing` counts `ProducedLastTick` factories; also track `totalTrades := len(s.ledger.trades)` sampled at the end.
3. The oversell assertion block is DELETED (stock physically cannot oversell — `Take` clamps). Replace with a conservation sanity check: after the run, no factory stock is negative and no `OutputStock` exceeds `rate * outputStockCapTicks + 1e-6`.
4. The "real factory-to-factory trade" assertion becomes: the ledger (over its window) contains at least one trade whose seller AND buyer are both `*factory.Factory`.
5. Keep the `t.Skipf` protocol on milestone failure, reporting: ticks run, max simultaneously-producing factories, distinct products ever produced (count + list), and total recent trades.

- [ ] **Step 2: Run the milestone**

Run: `go test ./state -run 'Test_state_Tick/long' -v -timeout 1800s` (match the actual test path; the long-run test is skipped under `-short`).

If the milestone FAILS, follow the bounded tuning protocol — instrument first (a throwaway diagnostic that reports per-2000-tick: producing count, distinct products, ledger trade count, resource stock committed), then turn ONE knob at a time, fully reverting between attempts:

1. `outputStockCapTicks` / `inputStockTargetTicks` 60 → 120 (together).
2. `salvageTrickleFraction` 0.25 → 0.5.
3. `seedCapitalBufferTicks` 300 → 1000.
4. `transportFixedPerUnit`/`transportPerDistance` 0.1/1e-4 → 0.05/5e-5 (together).

After 4 attempts without success: STOP. Record observed status in the `t.Skipf` message and report to the user — do not keep tuning.

- [ ] **Step 3: Update CLAUDE.md**

Rewrite the "Simulation core" and "Producers and contracts" sections of `CLAUDE.md` to describe: the produce → publish → match(execute) → move → spawn → solvency → prices pipeline; stock buffers and back-pressure; spot trades and the ledger; the deletion of contracts. Point to `docs/superpowers/specs/2026-07-12-inventory-economy-design.md`.

- [ ] **Step 4: Full verification and commit**

```bash
go build ./... && go test ./... && go vet ./...
cd frontend && npm run build && cd ..
git add state/state_test.go CLAUDE.md
git commit -m "test: inventory-economy milestone -- first space elevator part within 100k ticks"
```

---

## Self-review notes

- Spec coverage: stock/production (T1,3,4,7), spot trades + budget clamp (T5,7), per-unit transport (T2), halt+salvage back-pressure (T3,8), prices on stock signals (T8), crowding discount + transport-aware estimates + seed capital (T9), movement/ledger/links (T6,10,11), deletions (T7,12), cascade + milestone + tuning protocol (T13,14), CLAUDE.md (T14). Frontend adapt-only: wire shape preserved (T11).
- Determinism: ledger and edges are slice-ordered; `recipeCrowding` map is only read by key (never ranged); `recentSellers` map is only read by key. No map iteration affects state.
- Type consistency: `UnitTransport` (Match field), `UnitTransportCost` (recipes func), `ProduceTick`/`Hunger`/`FoldTickFlows`/`StockMarginalUnitCost` (factory), `edges`/`recentSellers` (ledger) — names match across tasks.
