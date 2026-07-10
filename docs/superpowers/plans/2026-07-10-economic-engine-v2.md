# Economic Engine v2 Implementation Plan

> **Status: executed, with Task 11 superseded during implementation.** Tasks
> 1-10 landed exactly as written below. Task 11's original assertions (reach
> an actual space-elevator-part delivery within 20,000 ticks) turned out to
> be combinatorially unreachable at any tuning, for reasons discovered only
> during execution. Task 11's section below is kept for historical record;
> the actual final implementation — the corrected `spawnNewProducer`
> weighting algorithm, `insolvencyGrace=10000`, and the relaxed long-run
> assertion — is documented in
> `.superpowers/sdd/progress.md` (gitignored scratch, may not exist in your
> checkout) and the final commit history from `87711bb` to `27e1e4a`. Read
> those before treating this section's Task 11 code as current.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebuild the backend simulation core (Phase 1 of the design in `docs/superpowers/specs/2026-07-10-economic-engine-v2-design.md`) so factories self-organize by resource location and transport cost, multiple producers of the same product can coexist in geographic niches, and the simulation converges on genuinely delivering finished goods instead of drifting forever.

**Architecture:** Replace instantaneous profit-snapshot culling and omniscient cheapest-source spawning with real per-factory cash wallets (bankruptcy over an insolvency grace window), capacity-bounded factory output, real `Sink` producers with genuine terminal demand, a shortage-weighted spawn picker, periodic contract renegotiation, and a continuous (non-phased) tick loop.

**Tech Stack:** Go 1.21, module `github.com/paul-freeman/satisfactory-story`, `github.com/stretchr/testify` for assertions (already a dependency). No new dependencies required.

## Global Constraints

- Module path: `github.com/paul-freeman/satisfactory-story`. Go 1.21.0 (from `go.mod`) — do not use language features newer than this.
- Build/test commands (from `CLAUDE.md`): `go build ./...`, `go test ./...`, `go test ./state -run <TestName>` for a single test.
- This plan covers **Phase 1 (backend) only**. The Elm frontend is untouched by this plan — after every task, `go build ./...` and `go test ./...` must both pass so the existing frontend keeps working against the running server throughout.
- Every new exported symbol gets a doc comment only when it states a non-obvious constraint or rationale (matching this repo's existing style) — do not add comments that merely restate the name.
- All new tunable constants must be named (not inline magic numbers) with a one-line comment explaining the choice, per the design spec.
- Follow existing repo conventions: `production.Producer` is the interface every producer type implements; `var _ production.Producer = (*T)(nil)` compile-time assertions exist for `Resource`, `Factory`, `Sink` and must keep compiling after every task.

---

### Task 1: Wallets and `Cash()` on the `Producer` interface

**Files:**
- Create: `production/wallet.go`
- Create: `production/wallet_test.go`
- Modify: `production/products.go` (the `Producer` interface, ~line 12-35)
- Modify: `resources/resources.go` (add `Cash()`, ~near line 130)
- Modify: `sink/sink.go` (add `Cash()`, ~near line 100)
- Modify: `factory/factory.go` (embed `production.Wallet`, add seed-capital constructor param)
- Modify: `state/state.go` (the one call site of `factory.New`, ~line 362, to pass a placeholder `0` seed capital — real seed capital funding lands in Task 4)
- Test: `production/wallet_test.go`, `factory/factory_test.go` (new file)

**Interfaces:**
- Produces: `production.Wallet` struct with `NewWallet(seed float64) Wallet`, `(*Wallet) Apply(delta float64)`, `(*Wallet) Cash() float64`, `(*Wallet) InsolventFor(ticks int) bool`.
- Produces: `production.Producer` interface gains `Cash() float64`.
- Produces: `factory.New(name string, loc point.Point, tick int, input, output production.Products, seedCapital float64) *Factory` (new trailing param).
- Consumes: nothing new from other tasks.

- [ ] **Step 1: Write the failing wallet test**

Create `production/wallet_test.go`:

```go
package production

import "testing"

func Test_Wallet(t *testing.T) {
	t.Run("starts with the seed balance", func(t *testing.T) {
		w := NewWallet(100)
		if w.Cash() != 100 {
			t.Errorf("got %f, want 100", w.Cash())
		}
	})

	t.Run("Apply adds and subtracts from the balance", func(t *testing.T) {
		w := NewWallet(100)
		w.Apply(-30)
		w.Apply(10)
		if w.Cash() != 80 {
			t.Errorf("got %f, want 80", w.Cash())
		}
	})

	t.Run("InsolventFor is false until the balance has been negative long enough", func(t *testing.T) {
		w := NewWallet(10)
		w.Apply(-20) // balance now -10, 1 tick negative
		if w.InsolventFor(2) {
			t.Errorf("should not be insolvent after only 1 negative tick")
		}
		w.Apply(0) // still -10, 2 ticks negative
		if !w.InsolventFor(2) {
			t.Errorf("should be insolvent after 2 negative ticks")
		}
	})

	t.Run("a positive tick resets the negative streak", func(t *testing.T) {
		w := NewWallet(10)
		w.Apply(-20) // -10, 1 negative tick
		w.Apply(50)  // back to positive, streak resets
		w.Apply(-60) // -10, 1 negative tick again
		if w.InsolventFor(2) {
			t.Errorf("streak should have reset after the positive tick")
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./production -run Test_Wallet -v`
Expected: FAIL (build error — `NewWallet` undefined)

- [ ] **Step 3: Implement the wallet**

Create `production/wallet.go`:

```go
package production

// Wallet tracks a producer's cash balance and how many consecutive ticks
// it has been negative, so callers can detect sustained insolvency
// instead of culling on a single bad tick.
type Wallet struct {
	Balance       float64
	negativeTicks int
}

// NewWallet returns a Wallet funded with the given starting balance.
func NewWallet(seed float64) Wallet {
	return Wallet{Balance: seed}
}

// Apply adds delta (positive or negative) to the balance and updates the
// consecutive-negative-tick counter used by InsolventFor.
func (w *Wallet) Apply(delta float64) {
	w.Balance += delta
	if w.Balance < 0 {
		w.negativeTicks++
	} else {
		w.negativeTicks = 0
	}
}

// Cash returns the current balance.
func (w *Wallet) Cash() float64 {
	return w.Balance
}

// InsolventFor reports whether the balance has been continuously negative
// for at least the given number of ticks.
func (w *Wallet) InsolventFor(ticks int) bool {
	return w.negativeTicks >= ticks
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./production -run Test_Wallet -v`
Expected: PASS

- [ ] **Step 5: Add `Cash()` to the `Producer` interface**

In `production/products.go`, add to the `Producer` interface (after the `Profitability() float64` line):

```go
	// Cash returns the producer's current cash balance. Resources and
	// sinks report an effectively infinite balance since they never go
	// bankrupt; Factory reports its real Wallet balance.
	Cash() float64
```

- [ ] **Step 6: Run build to see the compile errors from the interface change**

Run: `go build ./...`
Expected: FAIL — `*resources.Resource`, `*sink.Sink`, `*factory.Factory` do not implement `production.Producer` (missing `Cash`)

- [ ] **Step 7: Implement `Cash()` on `Resource`**

In `resources/resources.go`, add `"math"` to the import block, and add this method near `Profitability`:

```go
// Cash reports an effectively infinite balance -- raw resource nodes are
// never removed for insolvency.
func (r *Resource) Cash() float64 {
	return math.MaxFloat64
}
```

- [ ] **Step 8: Implement `Cash()` on `Sink`**

In `sink/sink.go` (already imports `"math"`), add near `Profitability`:

```go
// Cash reports an effectively infinite balance -- sinks are never removed
// for insolvency.
func (f *Sink) Cash() float64 {
	return math.MaxFloat64
}
```

- [ ] **Step 9: Embed `Wallet` in `Factory` and add seed capital to the constructor**

In `factory/factory.go`, remove the unused `LastExpenses float64` field and add an embedded `production.Wallet`:

```go
type Factory struct {
	Name        string
	Loc         point.Point
	CreatedTick int

	Input    production.Products
	Output   production.Products
	Duration float64

	Purchases []*production.Contract
	Sales     []*production.Contract

	production.Wallet
}
```

Update `New` to take a seed capital argument and fund the wallet:

```go
func New(
	name string,
	loc point.Point,
	tick int,
	input production.Products,
	output production.Products,
	seedCapital float64,
) *Factory {
	return &Factory{
		Name:        name,
		Loc:         loc,
		CreatedTick: tick,
		Input:       input,
		Output:      output,
		Purchases:   make([]*production.Contract, 0),
		Sales:       make([]*production.Contract, 0),
		Wallet:      production.NewWallet(seedCapital),
	}
}
```

(`Cash()` is now satisfied automatically via the embedded `Wallet`'s promoted method.)

- [ ] **Step 10: Update the one existing call site**

In `state/state.go`, in `spawnNewProducers` (~line 362), update the call to pass a placeholder seed capital of `0` (real seed-capital funding is wired up in Task 4):

```go
	newFactory := factory.New(recipe.Name(), loc, s.tick, recipe.Inputs(), recipe.Outputs(), 0)
```

- [ ] **Step 11: Run full build and tests**

Run: `go build ./... && go test ./...`
Expected: PASS

- [ ] **Step 12: Write a Factory Cash test**

Create `factory/factory_test.go`:

```go
package factory

import (
	"testing"

	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
)

func Test_Factory_Cash(t *testing.T) {
	f := New("Test Factory", point.Point{X: 0, Y: 0}, 0, production.Products{}, production.Products{}, 250)
	if f.Cash() != 250 {
		t.Errorf("got %f, want 250", f.Cash())
	}
}
```

- [ ] **Step 13: Run test to verify it passes**

Run: `go test ./factory -run Test_Factory_Cash -v`
Expected: PASS

- [ ] **Step 14: Commit**

```bash
git add production/wallet.go production/wallet_test.go production/products.go \
  resources/resources.go sink/sink.go factory/factory.go factory/factory_test.go state/state.go
git commit -m "Add Wallet type and Cash() to the Producer interface"
```

---

### Task 2: Capacity-bounded output (`RemainingCapacityFor`)

Fixes the bug where a `Factory` can be sold to indefinitely regardless of its actual output rate — the mechanism that lets geographic niches emerge once the cheapest seller sells out.

**Files:**
- Modify: `production/products.go` (interface)
- Modify: `resources/resources.go` (`HasCapacityFor`, ~line 152-177)
- Modify: `factory/factory.go` (`HasCapacityFor`, ~line 85-93)
- Modify: `sink/sink.go` (add no-op `RemainingCapacityFor`)
- Test: `resources/resources_test.go`, `factory/factory_test.go`

**Interfaces:**
- Produces: `production.Producer.RemainingCapacityFor(name string) float64` — returns remaining unsold rate for the named product (0 if the producer doesn't make it or has none left).
- Consumes: `production.Wallet`/`Cash()` from Task 1 (no direct dependency, just builds on the same interface).

- [ ] **Step 1: Write the failing test for Resource capacity**

Add to `resources/resources_test.go`:

```go
func Test_resource_RemainingCapacityFor(t *testing.T) {
	r := &Resource{
		Production: production.Production{Name: "Stuff", Rate: 10},
		Purity:     "Normal",
		Loc:        point.Point{X: 0, Y: 0},
		Sales:      []*production.Contract{},
	}
	if got := r.RemainingCapacityFor("Stuff"); got != 10 {
		t.Errorf("got %f, want 10", got)
	}
	if got := r.RemainingCapacityFor("SomethingElse"); got != 0 {
		t.Errorf("got %f, want 0", got)
	}
	r.Sales = append(r.Sales, &production.Contract{
		Order: production.Production{Name: "Stuff", Rate: 4},
	})
	if got := r.RemainingCapacityFor("Stuff"); got != 6 {
		t.Errorf("got %f, want 6", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./resources -run Test_resource_RemainingCapacityFor -v`
Expected: FAIL (`RemainingCapacityFor` undefined)

- [ ] **Step 3: Implement `RemainingCapacityFor` and rewrite `HasCapacityFor` on `Resource`**

In `resources/resources.go`, replace the existing `HasCapacityFor` (~line 151-177) with:

```go
// RemainingCapacityFor returns how much of the given product this resource
// could still sell, after subtracting rate already committed to active
// sales. Returns 0 if the resource doesn't produce that product.
func (r *Resource) RemainingCapacityFor(name string) float64 {
	if name != r.Production.Name {
		return 0
	}
	rate := r.Production.Rate
	for _, sale := range r.Sales {
		if sale.Cancelled || sale.Order.Name != name {
			continue
		}
		rate -= sale.Order.Rate
	}
	if rate < 0 {
		return 0
	}
	return rate
}

// HasCapacityFor implements production.Producer.
func (r *Resource) HasCapacityFor(order production.Production) error {
	if order.Rate <= 0 {
		return fmt.Errorf("production rate must be positive")
	}
	if order.Rate > r.RemainingCapacityFor(order.Name) {
		return fmt.Errorf("resource %s cannot produce %s at rate %f", r.PrettyPrint(), order.Key(), order.Rate)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./resources -run Test_resource_RemainingCapacityFor -v`
Expected: PASS

- [ ] **Step 5: Run the full resources test suite (regression check)**

Run: `go test ./resources -v`
Expected: PASS (including the pre-existing `Test_resource_HasCapacityFor`)

- [ ] **Step 6: Add `RemainingCapacityFor` to the `Producer` interface**

In `production/products.go`, add after `Cash() float64`:

```go
	// RemainingCapacityFor returns how much of the given product this
	// producer could still sell (0 if it doesn't produce that product or
	// has none left).
	RemainingCapacityFor(name string) float64
```

- [ ] **Step 7: Run build to confirm `Factory` and `Sink` now fail to compile**

Run: `go build ./...`
Expected: FAIL — missing `RemainingCapacityFor` on `*factory.Factory` and `*sink.Sink`

- [ ] **Step 8: Write the failing test for Factory capacity**

Add to `factory/factory_test.go`:

```go
func Test_Factory_RemainingCapacityFor(t *testing.T) {
	output := production.Products{{Name: "Widget", Rate: 10}}
	f := New("Test Factory", point.Point{X: 0, Y: 0}, 0, production.Products{}, output, 0)

	if got := f.RemainingCapacityFor("Widget"); got != 10 {
		t.Errorf("got %f, want 10", got)
	}
	if got := f.RemainingCapacityFor("NotProduced"); got != 0 {
		t.Errorf("got %f, want 0", got)
	}

	f.Sales = append(f.Sales, &production.Contract{
		Order: production.Production{Name: "Widget", Rate: 6},
	})
	if got := f.RemainingCapacityFor("Widget"); got != 4 {
		t.Errorf("got %f, want 4", got)
	}

	if err := f.HasCapacityFor(production.Production{Name: "Widget", Rate: 4}); err != nil {
		t.Errorf("expected capacity for 4, got error: %v", err)
	}
	if err := f.HasCapacityFor(production.Production{Name: "Widget", Rate: 5}); err == nil {
		t.Errorf("expected an error oversubscribing capacity, got nil")
	}
}
```

- [ ] **Step 9: Run test to verify it fails**

Run: `go test ./factory -run Test_Factory_RemainingCapacityFor -v`
Expected: FAIL (`RemainingCapacityFor` undefined)

- [ ] **Step 10: Implement `RemainingCapacityFor` and rewrite `HasCapacityFor` on `Factory`**

In `factory/factory.go`, replace the existing `HasCapacityFor` (~line 84-93) with:

```go
// RemainingCapacityFor returns how much of the given product this factory
// could still sell, after subtracting rate already committed to active
// sales. Returns 0 if the factory doesn't produce that product.
func (f *Factory) RemainingCapacityFor(name string) float64 {
	var rate float64
	found := false
	for _, output := range f.Output {
		if output.Name == name {
			rate = output.Rate
			found = true
			break
		}
	}
	if !found {
		return 0
	}
	for _, sale := range f.Sales {
		if sale.Cancelled || sale.Order.Name != name {
			continue
		}
		rate -= sale.Order.Rate
	}
	if rate < 0 {
		return 0
	}
	return rate
}

// HasCapacityFor implements producer.
func (f *Factory) HasCapacityFor(order production.Production) error {
	if order.Rate <= 0 {
		return fmt.Errorf("production rate must be positive")
	}
	if order.Rate > f.RemainingCapacityFor(order.Name) {
		return fmt.Errorf("factory %s cannot produce %s at rate %f", f.String(), order.Key(), order.Rate)
	}
	return nil
}
```

- [ ] **Step 11: Run test to verify it passes**

Run: `go test ./factory -v`
Expected: PASS

- [ ] **Step 12: Add the no-op `RemainingCapacityFor` on `Sink`**

In `sink/sink.go`, add near `HasCapacityFor`:

```go
// RemainingCapacityFor implements production.Producer. Sinks never sell
// anything, so they always report zero remaining capacity.
func (f *Sink) RemainingCapacityFor(_ string) float64 {
	return 0
}
```

- [ ] **Step 13: Run full build and test suite**

Run: `go build ./... && go test ./...`
Expected: PASS

- [ ] **Step 14: Commit**

```bash
git add production/products.go resources/resources.go resources/resources_test.go \
  factory/factory.go factory/factory_test.go sink/sink.go
git commit -m "Add capacity-bounded output via RemainingCapacityFor"
```

---

### Task 3: Fix `Factory.Profit()` and `Factory.SalesPriceFor()` cost accounting

Fixes the bug where `Factory.Profit()` nets only `TransportCost` and ignores `ProductCost` entirely — the exact function that will drive wallet balances in Task 4. Also fixes `SalesPriceFor`, which currently omits the transport cost a factory paid on its own inbound purchases, meaning a factory far from its suppliers could never recoup what it actually spent no matter its markup.

**Files:**
- Modify: `factory/factory.go` (`Profit`, ~line 100-125; `SalesPriceFor`, ~line 70-82)
- Test: `factory/factory_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces: same signatures (`Profit() float64`, `SalesPriceFor(production.Production, float64) float64`), corrected behavior only.

- [ ] **Step 1: Write the failing test for `SalesPriceFor`**

Add to `factory/factory_test.go`:

```go
func Test_Factory_SalesPriceFor(t *testing.T) {
	f := New("Test Factory", point.Point{X: 0, Y: 0}, 0, production.Products{}, production.Products{}, 0)
	f.Purchases = append(f.Purchases, &production.Contract{
		ProductCost:   10,
		TransportCost: 2,
	})
	// (10 + 2 input cost + 3 outbound transport) * 1.5 = 22.5
	got := f.SalesPriceFor(production.Production{Name: "Widget", Rate: 1}, 3)
	want := 22.5
	if got != want {
		t.Errorf("got %f, want %f", got, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./factory -run Test_Factory_SalesPriceFor -v`
Expected: FAIL (got 19.5 — the current implementation drops the purchase's transport cost)

- [ ] **Step 3: Fix `SalesPriceFor`**

In `factory/factory.go`, replace `SalesPriceFor` (~line 70-82):

```go
// SalesPriceFor is the price of a sale.
//
// For a factory, this is the sum of everything it paid for its inputs
// (product cost *and* inbound transport) plus the transport cost of this
// sale, marked up 50%. Omitting inbound transport here would mean a
// factory could never recoup what it actually spent getting its
// ingredients, regardless of markup.
func (f *Factory) SalesPriceFor(order production.Production, transportCost float64) float64 {
	purchaseCosts := 0.0
	for _, purchase := range f.Purchases {
		if !purchase.Cancelled {
			purchaseCosts += purchase.ProductCost + purchase.TransportCost
		}
	}
	return (purchaseCosts + transportCost) * 1.50 // 50% profit
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./factory -run Test_Factory_SalesPriceFor -v`
Expected: PASS

- [ ] **Step 5: Write the failing test for `Profit`**

Add to `factory/factory_test.go`:

```go
func Test_Factory_Profit(t *testing.T) {
	f := New("Test Factory", point.Point{X: 0, Y: 0}, 0, production.Products{}, production.Products{}, 0)
	f.Sales = append(f.Sales, &production.Contract{
		ProductCost:   30,
		TransportCost: 5,
	})
	f.Purchases = append(f.Purchases, &production.Contract{
		ProductCost:   10,
		TransportCost: 2,
	})
	// (30 - 5) - (10 + 2) = 13
	got := f.Profit()
	want := 13.0
	if got != want {
		t.Errorf("got %f, want %f", got, want)
	}
}

func Test_Factory_Profit_ignores_cancelled_contracts(t *testing.T) {
	f := New("Test Factory", point.Point{X: 0, Y: 0}, 0, production.Products{}, production.Products{}, 0)
	f.Sales = append(f.Sales, &production.Contract{
		ProductCost:   30,
		TransportCost: 5,
		Cancelled:     true,
	})
	f.Purchases = append(f.Purchases, &production.Contract{
		ProductCost:   10,
		TransportCost: 2,
	})
	// cancelled sale contributes nothing: 0 - (10 + 2) = -12
	got := f.Profit()
	want := -12.0
	if got != want {
		t.Errorf("got %f, want %f", got, want)
	}
	if len(f.Sales) != 0 {
		t.Errorf("cancelled sale should have been pruned, got %d sales left", len(f.Sales))
	}
}
```

- [ ] **Step 6: Run test to verify it fails**

Run: `go test ./factory -run Test_Factory_Profit -v`
Expected: FAIL (`Test_Factory_Profit` reports 3, not 13 — current formula nets transport cost only)

- [ ] **Step 7: Fix `Profit`**

In `factory/factory.go`, replace `Profit` (~line 100-125):

```go
// Profit implements producer. Revenue from a sale is its ProductCost minus
// the TransportCost the factory pays to ship it; the cost of a purchase is
// its ProductCost plus the TransportCost paid to receive it. This mirrors
// resources.Resource.Profit's accounting.
func (f *Factory) Profit() float64 {
	profit := 0.0

	newSales := make([]*production.Contract, 0)
	for _, sale := range f.Sales {
		if !sale.Cancelled {
			profit += sale.ProductCost - sale.TransportCost
			newSales = append(newSales, sale)
		}
	}
	f.Sales = newSales

	newPurchases := make([]*production.Contract, 0)
	for _, purchase := range f.Purchases {
		if !purchase.Cancelled {
			profit -= purchase.ProductCost + purchase.TransportCost
			newPurchases = append(newPurchases, purchase)
		}
	}
	f.Purchases = newPurchases

	return profit
}
```

- [ ] **Step 8: Run tests to verify they pass**

Run: `go test ./factory -v`
Expected: PASS (all `Test_Factory_*` tests)

- [ ] **Step 9: Run full build and test suite (regression check)**

Run: `go build ./... && go test ./...`
Expected: PASS. Note: `state/state_test.go`'s "can run multiple ticks" subtest logs `f.Profit()` for informational purposes only and does not assert on its value, so this change will not break it.

- [ ] **Step 10: Commit**

```bash
git add factory/factory.go factory/factory_test.go
git commit -m "Fix Factory.Profit and SalesPriceFor to account for ProductCost"
```

---

### Task 4: Seed capital and insolvency-based culling

Wires the wallet into the simulation: new factories are funded with real starting capital, every tick applies real profit/loss to that balance, and sustained insolvency (not an instantaneous profit snapshot) becomes the only economic reason a factory is removed.

**Files:**
- Create: `state/solvency.go`
- Create: `state/solvency_test.go`

**Interfaces:**
- Consumes: `factory.Factory.Profit() float64` (Task 3), `production.Wallet.Apply`/`InsolventFor` (Task 1, promoted onto `*factory.Factory`).
- Produces: `(*State) applySolvency(l *slog.Logger)` — applies this tick's profit/loss to every factory's wallet and removes bankrupt or contract-incomplete factories. Called from `Tick` in Task 10; unit-tested directly here in the meantime.

- [ ] **Step 1: Write the failing test**

Create `state/solvency_test.go`:

```go
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
		f := factory.New("Test", point.Point{X: 0, Y: 0}, 0,
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
		f := factory.New("Test", point.Point{X: 0, Y: 0}, 0,
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
		f := factory.New("Test", point.Point{X: 0, Y: 0}, 0,
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./state -run Test_applySolvency -v`
Expected: FAIL (`applySolvency` and `insolvencyGrace` undefined)

- [ ] **Step 3: Implement `applySolvency`**

Create `state/solvency.go`:

```go
package state

import (
	"log/slog"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/production"
)

// insolvencyGrace is how many consecutive ticks a factory's wallet may sit
// below zero before it is considered bankrupt and removed. This lets a
// factory ride out a rough patch (e.g. while renegotiating a cheaper
// supplier contract) instead of dying on the first bad tick.
const insolvencyGrace = 200

// applySolvency applies this tick's revenue and expenses to every
// factory's wallet (via Profit, which also prunes cancelled contracts) and
// removes any factory that is now missing a required input contract or has
// been insolvent for longer than insolvencyGrace.
func (s *State) applySolvency(l *slog.Logger) {
	survivors := make([]production.Producer, 0, len(s.producers))
	for _, p := range s.producers {
		f, ok := p.(*factory.Factory)
		if !ok {
			survivors = append(survivors, p)
			continue
		}

		f.Wallet.Apply(f.Profit())

		if len(f.Purchases) != len(f.Input) {
			l.Debug("removing factory with incomplete input contracts", slog.String("factory", f.String()))
			f.Remove()
			continue
		}
		if f.Wallet.InsolventFor(insolvencyGrace) {
			l.Debug("removing bankrupt factory",
				slog.String("factory", f.String()),
				slog.Float64("cash", f.Wallet.Cash()))
			f.Remove()
			continue
		}

		survivors = append(survivors, f)
	}
	s.producers = survivors
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./state -run Test_applySolvency -v`
Expected: PASS

- [ ] **Step 5: Run full build and test suite**

Run: `go build ./... && go test ./...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add state/solvency.go state/solvency_test.go
git commit -m "Add wallet-driven insolvency culling"
```

---

### Task 5: Extract `recipes.FindBestSeller`; report unmet inputs

Pulls the "find the cheapest producer with capacity" search out of `Recipe.SourceProducts` into a standalone function so it can be reused by contract renegotiation (Task 9) and gives callers visibility into which specific inputs couldn't be sourced, which is what feeds the shortage signal (Task 6).

**Files:**
- Create: `recipes/source.go`
- Create: `recipes/source_test.go`
- Modify: `recipes/recipes.go` (remove `Source` type and `SourceProducts`/loop, ~line 84-128)
- Modify: `state/state.go` (`spawnNewProducers` call site of `SourceProducts`, ~line 355-359 — updated to the new three-value return; full replacement of this function happens in Task 8, this is a minimal compile fix)

**Interfaces:**
- Produces: `recipes.FindBestSeller(sellers []production.Producer, order production.Production, destination point.Point) (Source, bool)`.
- Produces: `Recipe.SourceProducts(l *slog.Logger, sellers []production.Producer, destination point.Point) (sources map[string]Source, unmet []production.Production, err error)` — now returns the list of inputs it failed to source (empty when err is nil) in addition to the existing map/error.
- Consumes: nothing new.

- [ ] **Step 1: Write the failing test**

Create `recipes/source_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./recipes -run 'Test_FindBestSeller|Test_Recipe_SourceProducts_reports_unmet_inputs' -v`
Expected: FAIL (`FindBestSeller` undefined; `SourceProducts` has the wrong return arity)

- [ ] **Step 3: Remove `Source`/`SourceProducts` from `recipes.go`**

In `recipes/recipes.go`, delete the `Source` type and the `SourceProducts` method (~line 84-128) — everything from `type Source struct {` through the closing brace of `SourceProducts`. This removal makes the `"log/slog"` import (used only by `SourceProducts`'s `l *slog.Logger` param and its `l.Debug` call) unused — remove that import line too, or `go build` will fail with "imported and not used".

- [ ] **Step 4: Create `recipes/source.go`**

```go
package recipes

import (
	"fmt"
	"log/slog"

	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
)

// Source describes where and at what transport cost a product can be
// bought.
type Source struct {
	Order         production.Production
	Seller        production.Producer
	TransportCost float64
}

// FindBestSeller returns the seller from sellers with spare capacity for
// order that is cheapest to transport to destination. Returns false if no
// seller currently has the capacity.
func FindBestSeller(sellers []production.Producer, order production.Production, destination point.Point) (Source, bool) {
	var bestProducer production.Producer
	var bestCost float64
	for _, seller := range sellers {
		if err := seller.HasCapacityFor(order); err != nil {
			continue
		}
		cost := TransportCost(seller.Location(), destination)
		if bestProducer == nil || cost < bestCost {
			bestProducer = seller
			bestCost = cost
		}
	}
	if bestProducer == nil {
		return Source{}, false
	}
	return Source{Order: order, Seller: bestProducer, TransportCost: bestCost}, true
}

// SourceProducts finds the cheapest available seller (with spare capacity)
// for each of the recipe's inputs. It always evaluates every input --
// unmet lists any inputs that could not be sourced, so callers can record
// them as shortages even when the recipe as a whole can't be built yet.
// err is non-nil exactly when unmet is non-empty.
func (r Recipe) SourceProducts(l *slog.Logger, sellers []production.Producer, destination point.Point) (map[string]Source, []production.Production, error) {
	sourced := make(map[string]Source)
	unmet := make([]production.Production, 0)
	for _, order := range r.Inputs() {
		source, found := FindBestSeller(sellers, order, destination)
		if !found {
			l.Debug("failed to find producer for input", slog.String("input", order.Name))
			unmet = append(unmet, order)
			continue
		}
		sourced[order.Name] = source
	}
	if len(unmet) > 0 {
		return sourced, unmet, fmt.Errorf("failed to find %d input(s) for %s", len(unmet), r.String())
	}
	return sourced, unmet, nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./recipes -v`
Expected: PASS

- [ ] **Step 6: Fix the compile break in `state/state.go`**

Run: `go build ./...`
Expected: FAIL — `spawnNewProducers` still calls `recipe.SourceProducts` expecting two return values.

In `state/state.go`, in `spawnNewProducers` (~line 354-359), update to the three-value return (the `unmet` slice is unused here — Task 8 replaces this whole function with the demand-weighted version that does use it):

```go
	// Find the cheapest source of each input product
	sources, _, err := recipe.SourceProducts(l, s.producers, loc)
	if err != nil {
		l.Debug("failed to source all recipe ingredients", slog.String("error", err.Error()))
		return
	}
```

- [ ] **Step 7: Run full build and test suite**

Run: `go build ./... && go test ./...`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add recipes/source.go recipes/source_test.go recipes/recipes.go state/state.go
git commit -m "Extract FindBestSeller and report unmet recipe inputs"
```

---

### Task 6: Shortage tracking

The demand signal that Tasks 7 (sinks) and 8 (spawning) both feed into and read from.

**Files:**
- Create: `state/shortage.go`
- Create: `state/shortage_test.go`
- Modify: `state/state.go` (add `unmet map[string]float64` field to `State`, initialize in `getInitialState`)

**Interfaces:**
- Produces: `(*State) recordShortage(product string, rate float64)`, `(*State) decayShortages()`, `(*State) weightForProduct(name string) float64`.
- Consumes: nothing new.

- [ ] **Step 1: Write the failing test**

Create `state/shortage_test.go`:

```go
package state

import "testing"

func Test_shortage_tracking(t *testing.T) {
	s := &State{}

	if got := s.weightForProduct("Widget"); got != baselineOpportunityWeight {
		t.Errorf("got %f, want baseline %f", got, baselineOpportunityWeight)
	}

	s.recordShortage("Widget", 10)
	s.recordShortage("Widget", 5)
	if got := s.weightForProduct("Widget"); got != baselineOpportunityWeight+15 {
		t.Errorf("got %f, want %f", got, baselineOpportunityWeight+15)
	}

	for i := 0; i < 2000; i++ {
		s.decayShortages()
	}
	if got := s.weightForProduct("Widget"); got != baselineOpportunityWeight {
		t.Errorf("expected the shortage to have decayed away, got weight %f", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./state -run Test_shortage_tracking -v`
Expected: FAIL (`recordShortage`/`decayShortages`/`weightForProduct`/`baselineOpportunityWeight` undefined)

- [ ] **Step 3: Implement shortage tracking**

Create `state/shortage.go`:

```go
package state

// shortageDecay is the fraction of a recorded shortage retained each tick.
// At 0.99 a shortage's contribution roughly halves every ~70 ticks, so
// stale demand signals fade but don't vanish instantly.
const shortageDecay = 0.99

// baselineOpportunityWeight is the minimum spawn-selection weight every
// product gets, even with zero recorded shortage, so novel/untested
// recipes still get tried occasionally rather than only ever reacting to
// existing demand.
const baselineOpportunityWeight = 1.0

// recordShortage notes that rate units/sec of product went unsourced or
// unmet this tick.
func (s *State) recordShortage(product string, rate float64) {
	if s.unmet == nil {
		s.unmet = make(map[string]float64)
	}
	s.unmet[product] += rate
}

// decayShortages ages out old shortage signals so the spawn picker
// reflects recent demand, not demand from thousands of ticks ago.
func (s *State) decayShortages() {
	for name, v := range s.unmet {
		v *= shortageDecay
		if v < 0.01 {
			delete(s.unmet, name)
			continue
		}
		s.unmet[name] = v
	}
}

// weightForProduct returns the spawn-selection weight for a product: a
// baseline so untested recipes are still occasionally tried, plus any
// currently recorded shortage.
func (s *State) weightForProduct(name string) float64 {
	return baselineOpportunityWeight + s.unmet[name]
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./state -run Test_shortage_tracking -v`
Expected: PASS

- [ ] **Step 5: Wire the field into `State` and its initializer**

In `state/state.go`, add `unmet map[string]float64` to the `State` struct (next to `market map[string]float64`):

```go
	producers []production.Producer
	recipes   recipes.Recipes
	market    map[string]float64
	unmet     map[string]float64
	sinks     map[string]int
```

In `getInitialState`, initialize it alongside `s.market` (~line 106):

```go
	s.market = make(map[string]float64)
	s.unmet = make(map[string]float64)
	s.sinks = sinks
```

- [ ] **Step 6: Run full build and test suite**

Run: `go build ./... && go test ./...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add state/shortage.go state/shortage_test.go state/state.go
git commit -m "Add shortage tracking for demand-weighted spawning"
```

---

### Task 7: Real sinks

Fixes the bug where no `Sink` producer is ever actually instantiated — `state.sinks` today is just a cull-time protection count, not a real buyer, so there is no genuine terminal demand anywhere in the economy.

**Files:**
- Create: `state/sinks.go`
- Create: `state/sinks_test.go`
- Modify: `state/state.go` (`getInitialState`, ~line 89-107 — replace the `sinks map[string]int` placeholder with real `sink.New` producers)

**Interfaces:**
- Consumes: `recipes.Recipes` (existing), `recipes.TransportCost` (existing), `production.Producer.RemainingCapacityFor` (Task 2), `(*State) recordShortage` (Task 6), `(*State) writeContract` (existing).
- Produces: `newSinks(rs recipes.Recipes, xmin, xmax, ymin, ymax int) []*sink.Sink`, `(*State) sourceSinks(l *slog.Logger)`.

- [ ] **Step 1: Write the failing test for sink discovery**

Create `state/sinks_test.go`:

```go
package state

import (
	"testing"

	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
	"github.com/paul-freeman/satisfactory-story/recipes"
	"github.com/paul-freeman/satisfactory-story/resources"
)

func Test_newSinks_finds_distinct_space_elevator_parts(t *testing.T) {
	rs := recipes.Recipes{
		{DisplayName: "A", OutputProducts: production.Products{{Name: "SpaceElevatorPart_1", Rate: 1}}},
		{DisplayName: "B", OutputProducts: production.Products{{Name: "SpaceElevatorPart_1", Rate: 1}}}, // duplicate product
		{DisplayName: "C", OutputProducts: production.Products{{Name: "SpaceElevatorPart_2", Rate: 1}}},
		{DisplayName: "D", OutputProducts: production.Products{{Name: "IronPlate", Rate: 1}}}, // not a sink product
	}

	sinks := newSinks(rs, 0, 1000, 0, 1000)
	if len(sinks) != 2 {
		t.Fatalf("expected 2 distinct sinks, got %d", len(sinks))
	}
	names := map[string]bool{}
	for _, sk := range sinks {
		names[sk.Name] = true
	}
	if !names["SpaceElevatorPart_1"] || !names["SpaceElevatorPart_2"] {
		t.Errorf("expected both space elevator parts represented, got %+v", names)
	}
}

func Test_sourceSinks_buys_all_available_capacity(t *testing.T) {
	seller := &resources.Resource{
		Production: production.Production{Name: "SpaceElevatorPart_1", Rate: 5},
		Loc:        point.Point{X: 0, Y: 0},
	}
	rs := recipes.Recipes{
		{DisplayName: "A", OutputProducts: production.Products{{Name: "SpaceElevatorPart_1", Rate: 1}}},
	}
	sinks := newSinks(rs, 0, 1000, 0, 1000)

	producers := make([]production.Producer, 0, len(sinks)+1)
	producers = append(producers, seller)
	for _, sk := range sinks {
		producers = append(producers, sk)
	}
	s := &State{producers: producers, market: make(map[string]float64)}

	s.sourceSinks(testLogger())

	if got := seller.RemainingCapacityFor("SpaceElevatorPart_1"); got != 0 {
		t.Errorf("expected the sink to buy all remaining capacity, got %f left", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./state -run 'Test_newSinks_finds_distinct_space_elevator_parts|Test_sourceSinks_buys_all_available_capacity' -v`
Expected: FAIL (`newSinks`/`sourceSinks` undefined)

- [ ] **Step 3: Implement `state/sinks.go`**

```go
package state

import (
	"log/slog"
	"strings"

	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
	"github.com/paul-freeman/satisfactory-story/recipes"
	"github.com/paul-freeman/satisfactory-story/sink"
)

// spaceElevatorPartPrefix identifies the final products the space
// elevator wants, discovered from the recipe data rather than hardcoded
// one at a time.
const spaceElevatorPartPrefix = "SpaceElevatorPart_"

// sinkPerpetualShortage is added to a sink's wanted product's shortage
// score every tick, before decay -- sinks want an unlimited amount of
// their product forever, so the demand signal must never fully vanish.
const sinkPerpetualShortage = 50.0

// newSinks creates one Sink per distinct space-elevator part product found
// among the recipe outputs, all located at the center of the world bounds
// (representing the player's base).
func newSinks(rs recipes.Recipes, xmin, xmax, ymin, ymax int) []*sink.Sink {
	center := point.Point{X: (xmin + xmax) / 2, Y: (ymin + ymax) / 2}

	seen := make(map[string]bool)
	sinks := make([]*sink.Sink, 0)
	for _, recipe := range rs {
		for _, output := range recipe.Outputs() {
			if !strings.HasPrefix(output.Name, spaceElevatorPartPrefix) || seen[output.Name] {
				continue
			}
			seen[output.Name] = true
			sinks = append(sinks, sink.New(output.Name, center, production.Products{
				production.New(output.Name, 1, 1),
			}))
		}
	}
	return sinks
}

// sourceSinks lets every sink buy up all currently unsold capacity of the
// product(s) it wants. Sinks are price-insensitive with unlimited demand,
// so -- unlike a factory sourcing its inputs -- they don't shop for the
// cheapest seller, they simply drain whatever capacity is available from
// everyone.
func (s *State) sourceSinks(l *slog.Logger) {
	for _, p := range s.producers {
		sk, ok := p.(*sink.Sink)
		if !ok {
			continue
		}
		for _, want := range sk.Input {
			s.recordShortage(want.Name, sinkPerpetualShortage)
			for _, seller := range s.producers {
				remaining := seller.RemainingCapacityFor(want.Name)
				if remaining <= 0 {
					continue
				}
				order := production.Production{Name: want.Name, Rate: remaining}
				transportCost := recipes.TransportCost(seller.Location(), sk.Location())
				if err := s.writeContract(l, seller, sk, order, transportCost); err != nil {
					l.Debug("sink failed to sign contract", slog.String("error", err.Error()))
				}
			}
		}
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./state -run 'Test_newSinks_finds_distinct_space_elevator_parts|Test_sourceSinks_buys_all_available_capacity' -v`
Expected: PASS

- [ ] **Step 5: Wire real sinks into `getInitialState`**

In `state/state.go`, replace the placeholder sinks block (~line 89-107):

```go
	// Create sinks
	sinks := map[string]int{
		"SpaceElevatorPart_1": 1,
	}

	// Create producers
	producers := make([]production.Producer, 0)
	for _, resource := range resources {
		producers = append(producers, resource)
	}

	borderPaddingX := float64(xmax-xmin) * borderPaddingPct
	borderPaddingY := float64(ymax-ymin) * borderPaddingPct
```

with:

```go
	borderPaddingX := float64(xmax-xmin) * borderPaddingPct
	borderPaddingY := float64(ymax-ymin) * borderPaddingPct
	paddedXmin := int(float64(xmin) - borderPaddingX)
	paddedXmax := int(float64(xmax) + borderPaddingX)
	paddedYmin := int(float64(ymin) - borderPaddingY)
	paddedYmax := int(float64(ymax) + borderPaddingY)

	// Create producers
	producers := make([]production.Producer, 0)
	for _, resource := range resources {
		producers = append(producers, resource)
	}
	for _, sk := range newSinks(recipes, paddedXmin, paddedXmax, paddedYmin, paddedYmax) {
		producers = append(producers, sk)
	}
```

Then remove the now-unused `sinks` field assignment (`s.sinks = sinks`, ~line 107) and the `sinks map[string]int` field from the `State` struct (~line 35) — the old cull-time protection-count concept is fully superseded by real sinks + wallet-based solvency (Task 4). Also replace the bounds assignment block (~line 115-118), which recomputed the same padded values, with the already-computed ones:

```go
	s.xmin = paddedXmin
	s.xmax = paddedXmax
	s.ymin = paddedYmin
	s.ymax = paddedYmax
```

- [ ] **Step 6: Fix the now-broken reference in `removeUnprofitableProducers`**

`removeUnprofitableProducers` (still present until Task 10) references `s.sinks[f.Products().Key()]` (~line 302). Since `s.sinks` no longer exists, change that line to `0`:

```go
				if i < 0 {
```

This is a throwaway fix to keep the build green — `removeUnprofitableProducers` is deleted entirely in Task 10.

- [ ] **Step 7: Run full build and test suite**

Run: `go build ./... && go test ./...`
Expected: PASS. Note `state_test.go`'s "all resources should be in a recipe" subtest only inspects `testState.producers` for products that are resources, so adding sink producers to the slice doesn't affect it (sinks aren't resources and aren't iterated the same way — verify by reading the subtest before running if unsure).

- [ ] **Step 8: Commit**

```bash
git add state/sinks.go state/sinks_test.go state/state.go
git commit -m "Instantiate real Sink producers with genuine terminal demand"
```

---

### Task 8: Demand-weighted spawning

Fixes the bug where a brand-new factory is spawned from a uniformly random recipe regardless of whether its output has any buyer or value. Replaces `spawnNewProducers` entirely.

**Files:**
- Create: `state/spawn.go`
- Create: `state/spawn_test.go`
- Modify: `state/state.go` (delete the old `spawnNewProducers` and the now-unused `producerCost` struct, ~line 332-371)

**Interfaces:**
- Consumes: `recipes.Recipe.SourceProducts` (Task 5, 3-value form), `(*State) recordShortage`/`weightForProduct` (Task 6), `factory.New(..., seedCapital float64)` (Task 1), `(*State) writeContract` (existing).
- Produces: `(*State) spawnNewProducer(l *slog.Logger)`.

- [ ] **Step 1: Write the failing test**

Create `state/spawn_test.go`:

```go
package state

import (
	"math/rand"
	"testing"

	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
	"github.com/paul-freeman/satisfactory-story/recipes"
	"github.com/paul-freeman/satisfactory-story/resources"
)

func newTestState(rs recipes.Recipes, producers []production.Producer) *State {
	return &State{
		recipes:   rs,
		producers: producers,
		market:    make(map[string]float64),
		unmet:     make(map[string]float64),
		randSrc:   rand.New(rand.NewSource(1)),
		xmin:      0, xmax: 1000, ymin: 0, ymax: 1000,
	}
}

func Test_spawnNewProducer_spawns_when_viable(t *testing.T) {
	ore := &resources.Resource{
		Production: production.Production{Name: "Ore", Rate: 100},
		Loc:        point.Point{X: 500, Y: 500},
	}
	rs := recipes.Recipes{
		{
			DisplayName:    "Smelt Ore",
			Active:         true,
			InputProducts:  production.Products{{Name: "Ore", Rate: 5}},
			OutputProducts: production.Products{{Name: "Ingot", Rate: 5}},
		},
	}
	s := newTestState(rs, []production.Producer{ore})

	s.spawnNewProducer(testLogger())

	if len(s.producers) != 2 {
		t.Fatalf("expected a new factory to spawn, got %d producers", len(s.producers))
	}
}

func Test_spawnNewProducer_skips_when_uneconomical(t *testing.T) {
	ore := &resources.Resource{
		Production: production.Production{Name: "Ore", Rate: 100},
		Loc:        point.Point{X: 500, Y: 500},
	}
	rs := recipes.Recipes{
		{
			DisplayName:    "Smelt Ore",
			Active:         true,
			InputProducts:  production.Products{{Name: "Ore", Rate: 5}},
			OutputProducts: production.Products{{Name: "Ingot", Rate: 5}},
		},
	}
	s := newTestState(rs, []production.Producer{ore})
	// Someone is already selling Ingot far below what this recipe could
	// ever recoup from its input costs -- spawning here would guarantee a
	// loss, so it should be skipped.
	s.market["Ingot"] = 0.0001

	s.spawnNewProducer(testLogger())

	if len(s.producers) != 1 {
		t.Fatalf("expected no new factory to spawn, got %d producers", len(s.producers))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./state -run Test_spawnNewProducer -v`
Expected: FAIL (`spawnNewProducer` undefined)

- [ ] **Step 3: Implement `state/spawn.go`**

```go
package state

import (
	"log/slog"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/recipes"
)

// seedCapitalBufferTicks funds a new factory with this many ticks' worth
// of projected input cost as starting capital, representing the up-front
// cost of building the factory.
const seedCapitalBufferTicks = 5.0

// spawnCandidates is how many random (recipe, location) pairs are sampled
// and weighed against each other for each spawn attempt.
const spawnCandidates = 5

// spawnNewProducer samples a handful of candidate recipes, weighted toward
// products with recorded shortages (see shortage.go), and spawns the
// chosen one only if it can be sourced and would not be forced to sell
// below its own cost basis against the current market price.
func (s *State) spawnNewProducer(l *slog.Logger) {
	activeRecipes := make([]*recipes.Recipe, 0, len(s.recipes))
	for _, recipe := range s.recipes {
		if recipe.Active {
			activeRecipes = append(activeRecipes, recipe)
		}
	}
	if len(activeRecipes) == 0 {
		return
	}

	type candidate struct {
		recipe *recipes.Recipe
		loc    point.Point
		weight float64
	}
	candidates := make([]candidate, 0, spawnCandidates)
	for i := 0; i < spawnCandidates; i++ {
		recipe := activeRecipes[s.randSrc.Intn(len(activeRecipes))]
		loc := s.randomLocation()
		weight := 0.0
		for _, output := range recipe.Outputs() {
			weight += s.weightForProduct(output.Name)
		}
		candidates = append(candidates, candidate{recipe, loc, weight})
	}

	total := 0.0
	for _, c := range candidates {
		total += c.weight
	}
	pick := s.randSrc.Float64() * total
	chosen := candidates[len(candidates)-1]
	for _, c := range candidates {
		pick -= c.weight
		if pick <= 0 {
			chosen = c
			break
		}
	}

	sources, unmet, err := chosen.recipe.SourceProducts(l, s.producers, chosen.loc)
	for _, order := range unmet {
		s.recordShortage(order.Name, order.Rate)
	}
	if err != nil {
		l.Debug("failed to source all recipe ingredients", slog.String("error", err.Error()))
		return
	}

	projectedCostBasis := 0.0
	for _, source := range sources {
		projectedCostBasis += source.Seller.SalesPriceFor(source.Order, source.TransportCost) + source.TransportCost
	}
	projectedSalePrice := projectedCostBasis * 1.50

	for _, output := range chosen.recipe.Outputs() {
		if marketPrice, ok := s.market[output.Name]; ok && projectedSalePrice > marketPrice {
			l.Debug("skipping spawn candidate that can't compete with the current market price",
				slog.String("recipe", chosen.recipe.Name()),
				slog.String("product", output.Name))
			s.recordShortage(output.Name, output.Rate)
			return
		}
	}

	newFactory := factory.New(chosen.recipe.Name(), chosen.loc, s.tick,
		chosen.recipe.Inputs(), chosen.recipe.Outputs(), projectedCostBasis*seedCapitalBufferTicks)
	for _, source := range sources {
		s.writeContract(l, source.Seller, newFactory, source.Order, source.TransportCost)
	}
	s.producers = append(s.producers, newFactory)
	l.Debug("spawned producer", slog.String("factory", newFactory.Name))
}

func (s *State) randomLocation() point.Point {
	return point.Point{
		X: s.randSrc.Intn(s.xmax-s.xmin) + s.xmin,
		Y: s.randSrc.Intn(s.ymax-s.ymin) + s.ymin,
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./state -run Test_spawnNewProducer -v`
Expected: PASS

- [ ] **Step 5: Delete the old `spawnNewProducers` and `producerCost`**

In `state/state.go`, delete the `producerCost` struct and the `spawnNewProducers` method entirely (~line 332-371, everything from `type producerCost struct {` through the end of the old `spawnNewProducers` function). `randomLocation` above replaces its inline location-picking logic. That deleted function (~line 339) was the only use of the `"github.com/paul-freeman/satisfactory-story/point"` import anywhere in this file — remove that import too, or `go build` will fail with "imported and not used".

- [ ] **Step 6: Run full build and test suite**

Run: `go build ./... && go test ./...`
Expected: FAIL — `Tick`'s phase-0 case still calls `s.spawnNewProducers(l)`. Update it to call `s.spawnNewProducer(l)` (singular) as a minimal fix; Task 10 replaces the whole `Tick` method.

In `state/state.go`, in `Tick` (~line 133-140):

```go
	switch (s.tick / simulatedAnnealingTicks) % 3 {
	case 0:
		s.spawnNewProducer(l)
	case 1:
		s.moveProducer(l)
	case 2:
		s.removeUnprofitableProducers(l)
	}
```

- [ ] **Step 7: Run full build and test suite again**

Run: `go build ./... && go test ./...`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add state/spawn.go state/spawn_test.go state/state.go
git commit -m "Replace random recipe spawning with demand-weighted spawning"
```

---

### Task 9: Contract renegotiation

Lets a new, better-positioned entrant steal business from an incumbent supplier, rather than only ever competing for brand-new demand.

**Files:**
- Create: `state/renegotiate.go`
- Create: `state/renegotiate_test.go`

**Interfaces:**
- Consumes: `recipes.FindBestSeller` (Task 5), `(*State) writeContract` (existing).
- Produces: `(*State) renegotiateContracts(l *slog.Logger)`.

- [ ] **Step 1: Write the failing test**

Create `state/renegotiate_test.go`:

```go
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
	buyer := factory.New("Buyer", point.Point{X: 1000, Y: 1000}, 0,
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./state -run Test_renegotiateContracts -v`
Expected: FAIL (`renegotiateContracts` undefined)

- [ ] **Step 3: Implement `state/renegotiate.go`**

```go
package state

import (
	"log/slog"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/production"
	"github.com/paul-freeman/satisfactory-story/recipes"
)

// renegotiateProbabilityPerTick is the chance, per factory per tick, that
// it bothers re-shopping its input contracts at all -- doing this for
// every factory every tick would be wasted search effort once the market
// has settled.
const renegotiateProbabilityPerTick = 0.02

// renegotiationMinMargin is how much cheaper (as a fraction of the current
// total price) a new offer must be before a factory switches suppliers.
// This avoids thrashing between two near-identical prices.
const renegotiationMinMargin = 0.05

// renegotiateContracts lets each factory re-shop each of its input
// contracts for a better offer, switching supplier if one is found that
// beats the current price by more than renegotiationMinMargin. This is
// what lets a newly spawned, better-positioned producer steal business
// from an incumbent instead of only ever competing for brand-new demand.
func (s *State) renegotiateContracts(l *slog.Logger) {
	for _, p := range s.producers {
		f, ok := p.(*factory.Factory)
		if !ok || s.randSrc.Float64() >= renegotiateProbabilityPerTick {
			continue
		}

		current := make([]*production.Contract, len(f.Purchases))
		copy(current, f.Purchases)
		for _, purchase := range current {
			if purchase.Cancelled {
				continue
			}
			candidate, found := recipes.FindBestSeller(s.producers, purchase.Order, f.Location())
			if !found || candidate.Seller == purchase.Seller {
				continue
			}

			currentTotal := purchase.ProductCost + purchase.TransportCost
			candidateTotal := candidate.Seller.SalesPriceFor(candidate.Order, candidate.TransportCost) + candidate.TransportCost
			if candidateTotal >= currentTotal*(1-renegotiationMinMargin) {
				continue
			}

			purchase.Cancel()
			if err := s.writeContract(l, candidate.Seller, f, candidate.Order, candidate.TransportCost); err != nil {
				l.Debug("failed to renegotiate contract", slog.String("error", err.Error()))
				continue
			}
			l.Debug("renegotiated contract",
				slog.String("factory", f.String()),
				slog.String("product", purchase.Order.Name))
		}
	}
}
```

(The just-cancelled contract is left in `f.Purchases` marked `Cancelled` — the next `applySolvency` call prunes it via `Factory.Profit()`, same as any other cancellation.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./state -run Test_renegotiateContracts -v`
Expected: PASS

- [ ] **Step 5: Run full build and test suite**

Run: `go build ./... && go test ./...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add state/renegotiate.go state/renegotiate_test.go
git commit -m "Add periodic contract renegotiation"
```

---

### Task 10: Continuous tick loop

Replaces the `(tick / 3000) % 3` phase switch — which means nothing moves or gets culled for the first 3000 ticks of any run — with a continuous loop where every mechanism runs (or is rolled) every tick.

**Files:**
- Modify: `state/state.go` (`Tick`, ~line 125-143; delete `removeUnprofitableProducers` and `producerStats`, ~line 239-330; rename/simplify `moveProducer`, ~line 373-384; delete now-unused constants `lifetime`, `simulatedAnnealingTicks`, ~line 23-27)

**Interfaces:**
- Consumes: `(*State) moveProducer` (existing, renamed `moveProducers`), `(*State) sourceSinks` (Task 7), `(*State) spawnNewProducer` (Task 8), `(*State) renegotiateContracts` (Task 9), `(*State) applySolvency` (Task 4), `(*State) decayShortages` (Task 6).
- Produces: same `Tick(parentLogger *slog.Logger) error` signature, new body.

- [ ] **Step 1: Delete `removeUnprofitableProducers` and `producerStats`**

In `state/state.go`, delete the `producerStats` struct and `removeUnprofitableProducers` method entirely (~line 239-330, everything from `type producerStats struct {` through the closing brace of `removeUnprofitableProducers`). `sort.Slice` (used at ~line 265) was the only use of the `"sort"` package anywhere in this file — remove the now-unused `"sort"` import too, or `go build` will fail with "imported and not used".

- [ ] **Step 2: Simplify `moveProducer` and rename it `moveProducers`**

Replace (~line 373-384):

```go
func (s *State) moveProducer(l *slog.Logger) {
	for _, producer := range s.producers {
		switch producer := producer.(type) {
		case production.MoveableProducer:
			if err := producer.Move(); err != nil {
				l.Error("failed to move producer: " + err.Error())
			}
		default:
			// Do nothing
		}
	}
}
```

with (only the name changes — the hill-climb logic itself is unchanged and carries over as-is):

```go
func (s *State) moveProducers(l *slog.Logger) {
	for _, producer := range s.producers {
		switch producer := producer.(type) {
		case production.MoveableProducer:
			if err := producer.Move(); err != nil {
				l.Error("failed to move producer: " + err.Error())
			}
		default:
			// Do nothing
		}
	}
}
```

- [ ] **Step 3: Rewrite `Tick`**

Replace (~line 125-143):

```go
func (s *State) Tick(parentLogger *slog.Logger) error {
	// Lock state while ticking
	s.m.Lock()
	defer s.m.Unlock()

	s.tick++
	l := parentLogger.With(slog.Int("tick", s.tick))

	switch (s.tick / simulatedAnnealingTicks) % 3 {
	case 0:
		s.spawnNewProducer(l)
	case 1:
		s.moveProducer(l)
	case 2:
		s.removeUnprofitableProducers(l)
	}

	return nil
}
```

with:

```go
func (s *State) Tick(parentLogger *slog.Logger) error {
	// Lock state while ticking
	s.m.Lock()
	defer s.m.Unlock()

	s.tick++
	l := parentLogger.With(slog.Int("tick", s.tick))

	// Every mechanism runs every tick (spawning and renegotiation are each
	// individually probability-gated inside their own function) instead of
	// the old spawn/move/cull phases, which meant nothing moved or got
	// culled for the first third of any run.
	s.moveProducers(l)
	s.sourceSinks(l)
	if s.randSrc.Float64() < spawnProbabilityPerTick {
		s.spawnNewProducer(l)
	}
	s.renegotiateContracts(l)
	s.applySolvency(l)
	s.decayShortages()

	return nil
}
```

- [ ] **Step 4: Add the spawn-probability constant**

In `state/spawn.go` (from Task 8), add alongside the other spawn constants:

```go
// spawnProbabilityPerTick is the chance, per tick, that a new producer is
// attempted at all.
const spawnProbabilityPerTick = 0.05
```

- [ ] **Step 5: Delete the now-unused phase constants**

In `state/state.go`, remove `lifetime` and `simulatedAnnealingTicks` from the top-level `const` block (~line 23-27), leaving only `borderPaddingPct`:

```go
const (
	borderPaddingPct = 0.1
)
```

- [ ] **Step 6: Run full build and test suite**

Run: `go build ./... && go test ./...`
Expected: PASS

- [ ] **Step 7: Manually sanity-check a short run**

Run: `go test ./state -run Test_state_Tick/can_run_multiple_ticks -v 2>&1 | tail -40`
Expected: PASS, and (unlike the pre-rewrite baseline) the logged `Smart Plating`/`Automated Wiring`/`Versatile Framework` factories (space-elevator-part producers) should show profit figures reflecting real revenue now that sinks exist, not just small transport-cost deltas. This is a manual spot-check, not an assertion — Task 11 adds the real automated check.

- [ ] **Step 8: Commit**

```bash
git add state/state.go state/spawn.go
git commit -m "Replace phased tick with a continuous per-tick loop"
```

---

### Task 11: Long-run integration test

Asserts the properties that used to be silently false, per the design spec's testing section.

**Files:**
- Modify: `state/state_test.go` (add a new subtest to `Test_state_Tick`)

**Interfaces:**
- Consumes: everything above.
- Produces: no new production code, test-only.

- [ ] **Step 1: Write the failing test**

Add a new subtest inside `Test_state_Tick` in `state/state_test.go` (alongside the existing "can run multiple ticks" subtest):

```go
	t.Run("converges on real production over a long run", func(t *testing.T) {
		l := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level:       slog.LevelError,
			ReplaceAttr: removeTimeAndLevel,
		}))
		seed := int64(152)

		logLevel := new(slog.Level)
		testState, err := New(l, logLevel, seed)
		assert.NoError(t, err, "failed to create state")
		for i := 0; i < 20000; i++ {
			err = testState.Tick(l)
			assert.NoError(t, err, "failed to tick state")
		}

		// No producer should ever be sold beyond its actual output rate.
		// (Checked directly against Sales/output rate, not via
		// RemainingCapacityFor -- that method clamps at 0, which would make
		// this assertion pass trivially even if oversell had occurred.)
		for _, p := range testState.producers {
			switch producer := p.(type) {
			case *resources.Resource:
				committed := 0.0
				for _, sale := range producer.Sales {
					if !sale.Cancelled && sale.Order.Name == producer.Production.Name {
						committed += sale.Order.Rate
					}
				}
				assert.LessOrEqual(t, committed, producer.Production.Rate,
					"resource %s oversold", producer.PrettyPrint())
			case *factory.Factory:
				for _, output := range producer.Output {
					committed := 0.0
					for _, sale := range producer.Sales {
						if !sale.Cancelled && sale.Order.Name == output.Name {
							committed += sale.Order.Rate
						}
					}
					assert.LessOrEqual(t, committed, output.Rate,
						"factory %s oversold %s", producer.String(), output.Name)
				}
			}
		}

		// At least one sink should have received a real, priced delivery --
		// i.e. genuine terminal demand actually got fulfilled.
		sinkReceivedSale := false
		for _, p := range testState.producers {
			if sk, ok := p.(*sink.Sink); ok {
				for _, purchase := range sk.ContractsIn() {
					if !purchase.Cancelled && purchase.Order.Rate > 0 {
						sinkReceivedSale = true
					}
				}
			}
		}
		assert.True(t, sinkReceivedSale, "expected at least one sink to have an active purchase after a long run")

		// At least one product should have more than one active,
		// independent producer -- evidence of a niche, not a monopoly.
		producersByProduct := make(map[string]int)
		for _, p := range testState.producers {
			if _, ok := p.(*sink.Sink); ok {
				continue
			}
			for _, product := range p.Products() {
				producersByProduct[product.Name]++
			}
		}
		foundNiche := false
		for _, count := range producersByProduct {
			if count > 1 {
				foundNiche = true
				break
			}
		}
		assert.True(t, foundNiche, "expected at least one product to have multiple coexisting producers")
	})
```

Add `"github.com/paul-freeman/satisfactory-story/resources"` and `"github.com/paul-freeman/satisfactory-story/sink"` to the import block of `state/state_test.go` (`factory` is already imported there).

- [ ] **Step 2: Run test to verify it fails or passes**

Run: `go test ./state -run Test_state_Tick/converges_on_real_production_over_a_long_run -v`
Expected: This exercises the whole rewritten system end-to-end for the first time at scale. If any assertion fails, treat it as a real bug in the rewrite (most likely candidates: a spawn/market-price interaction that's too conservative to ever let a first entrant appear for some product, or a niche never forming within 20000 ticks) and debug it using `superpowers:systematic-debugging` rather than loosening the assertion. Do not weaken these assertions to make the test pass -- they encode the actual goal of this rewrite.

- [ ] **Step 3: Run the full test suite once the long-run test passes**

Run: `go build ./... && go test ./... -v`
Expected: PASS across every package.

- [ ] **Step 4: Commit**

```bash
git add state/state_test.go
git commit -m "Add long-run integration test for the economic engine rewrite"
```

---

## After this plan

Phase 1 (this plan) makes the backend simulation actually converge on delivering finished goods with real competition. Phase 2 — rewriting the Elm frontend in TypeScript + React + D3 — is deliberately not covered here (per the design spec, its concrete wire-format needs should be shaped by watching Phase 1 run, not designed speculatively). Once this plan is fully executed and you've run the server for a while and looked at the results, come back for a follow-up brainstorming/planning pass on the frontend.
