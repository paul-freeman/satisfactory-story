# Order-Book Market Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace one-shot spawn-time sourcing with a standing bid/ask order book so that goal demand propagates backward through recipe tiers as money, per `docs/superpowers/specs/2026-07-11-order-book-market-design.md`.

**Architecture:** A new `market` package holds a per-product `Book` of standing asks (unsold capacity) and bids (unmet demand), rebuilt from live producer state each tick; only prices persist, carried on the producers. A per-tick matching pass crosses bids and asks into the existing `production.Contract` machinery. Factories may exist idle (unfilled input bids, paying upkeep), spawning becomes an expected-profit draw against the book, and local price adjustment (asks decay unsold, bids escalate unfilled) replaces the old cost-plus/ratchet pricing.

**Tech Stack:** Go 1.21 (backend, `go test`), React + TypeScript + Vite (frontend, one small task).

## Global Constraints

- Determinism given the seed: all randomness through `s.randSrc`; never depend on Go map iteration order (the book iterates products sorted, orders in posting order).
- Prices in the book are **unit prices** (money per unit of rate). A contract's `ProductCost = UnitPrice × Order.Rate` (money per tick for the flow).
- Transport cost stays flat per contract via `recipes.TransportCost`; **both** sides pay it (existing `production.Contract` semantics — do not change).
- `go build ./...` and `go test ./...` must pass at the end of every task.
- Simulation code lives in the existing packages; follow their comment style (constants get a doc comment explaining *why* the value is what it is).
- Commit at the end of every task (commands given per task).

## Interim-state notes (read before starting)

- Tasks 5–9 rewrite the economy piece by piece. The old mechanisms (spawn gate, `s.market` map, `SalesPriceFor`) are kept compiling until the task that replaces their last caller, then deleted. Each task says exactly what dies.
- Task 5 converts the long-run convergence test in `state/state_test.go` to `t.Skip` (the economy is intentionally half-rebuilt between tasks 5 and 9). Task 12 re-enables it with new assertions. All other tests stay green throughout.
- **Two deliberate refinements of the spec** (surface both in the final report):
  1. The spec says an idle factory "pays no input costs". Contracts are binding flows, so a partially-sourced factory *does* pay its signed purchase contracts (holding inventory costs money and motivates escalating the remaining bids); only a factory with *no* purchases pays upkeep alone. The spec's intent (idle factories bleed slowly and legibly) is preserved.
  2. The spec models the floor buyer as "a floor-price, unlimited-rate bid on every product — no special mechanism". That mechanism is broken by the transport cost floor (`recipes.TransportCost` is always ≥ 1.0 per contract): a 0.1/unit floor bid can never cross an ask at typical recipe rates (per-unit transport alone exceeds it), and where it *could* cross, the resulting contracts would permanently lock up capacity that real buyers want next tick. Instead, the floor is implemented as **salvage revenue**: `applySolvency` credits every producing factory `floorUnitPrice` per unit of unsold output capacity, contract-free and transport-free (thematically: every factory has its own AWESOME-sink hookup on site). Same economic function — speculative producers earn something while waiting, real trade always beats the floor — without the dead-channel and lockup bugs. There is no floor sink entity.

---

### Task 1: Market package — order book

**Files:**
- Create: `market/book.go`
- Test: `market/book_test.go`
- Modify: `production/production.go` (add shared constants)

**Interfaces:**
- Consumes: `production.Producer` (existing), `production.Production` (existing).
- Produces (later tasks rely on these exact names):
  - `production.DefaultUnitPrice = 1.0`, `production.MinUnitPrice = 0.01`, `production.RateEpsilon = 1e-9`
  - `market.Ask{Seller production.Producer; Product string; Remaining, UnitPrice float64}`
  - `market.Bid{Buyer production.Producer; Product string; Remaining, UnitPrice float64}`
  - `market.NewBook() *Book`, `(*Book) Clear()`,
    `(*Book) PostAsk(seller production.Producer, product string, rate, unitPrice float64)`,
    `(*Book) PostBid(buyer production.Producer, product string, rate, unitPrice float64)`,
    `(*Book) Asks(product string) []*Ask`, `(*Book) Bids(product string) []*Bid`,
    `(*Book) BestAsk(product string) (*Ask, bool)`, `(*Book) BestBid(product string) (*Bid, bool)`,
    `(*Book) Products() []string`

- [ ] **Step 1: Add shared constants to `production/production.go`**

Append to `production/production.go`:

```go
// DefaultUnitPrice seeds a producer's ask/bid price for a product the
// first time it is quoted, before market adjustment takes over.
const DefaultUnitPrice = 1.0

// MinUnitPrice is the lower bound for the ask price of a producer with no
// purchase costs (raw resource nodes) -- extraction is treated as nearly
// free, but a zero price would make price adjustment multiplicative
// against zero and stick there forever.
const MinUnitPrice = 0.01

// RateEpsilon is the smallest production rate treated as non-zero when
// publishing, matching, and inspecting orders.
const RateEpsilon = 1e-9
```

- [ ] **Step 2: Write the failing tests**

Create `market/book_test.go`:

```go
package market

import (
	"testing"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
)

func testProducer(x, y int) production.Producer {
	return factory.New("Test", "Recipe_Test_C", point.Point{X: x, Y: y},
		0, production.Products{}, production.Products{}, 0)
}

func Test_Book_BestAsk_returns_lowest_unfilled(t *testing.T) {
	b := NewBook()
	s1 := testProducer(0, 0)
	s2 := testProducer(1, 1)
	b.PostAsk(s1, "Ingot", 5, 2.0)
	b.PostAsk(s2, "Ingot", 5, 1.5)

	ask, ok := b.BestAsk("Ingot")
	if !ok {
		t.Fatal("expected a best ask")
	}
	if ask.UnitPrice != 1.5 || ask.Seller != s2 {
		t.Errorf("expected the 1.5 ask from s2, got %f from %v", ask.UnitPrice, ask.Seller)
	}

	// Fully consumed asks are ignored.
	ask.Remaining = 0
	ask, ok = b.BestAsk("Ingot")
	if !ok || ask.UnitPrice != 2.0 {
		t.Errorf("expected the 2.0 ask once the cheaper one is consumed, got %+v ok=%v", ask, ok)
	}
}

func Test_Book_BestBid_returns_highest_unfilled(t *testing.T) {
	b := NewBook()
	b1 := testProducer(0, 0)
	b2 := testProducer(1, 1)
	b.PostBid(b1, "Ingot", 5, 2.0)
	b.PostBid(b2, "Ingot", 5, 3.0)

	bid, ok := b.BestBid("Ingot")
	if !ok || bid.UnitPrice != 3.0 || bid.Buyer != b2 {
		t.Errorf("expected the 3.0 bid from b2, got %+v ok=%v", bid, ok)
	}

	if _, ok := b.BestBid("NoSuchProduct"); ok {
		t.Error("expected no bid for an unknown product")
	}
}

func Test_Book_ignores_zero_rate_orders(t *testing.T) {
	b := NewBook()
	p := testProducer(0, 0)
	b.PostAsk(p, "Ingot", 0, 1.0)
	b.PostBid(p, "Ingot", 0, 1.0)
	if len(b.Asks("Ingot")) != 0 || len(b.Bids("Ingot")) != 0 {
		t.Error("zero-rate orders should not be recorded")
	}
}

func Test_Book_Products_sorted_and_Clear(t *testing.T) {
	b := NewBook()
	p := testProducer(0, 0)
	b.PostAsk(p, "Zinc", 1, 1.0)
	b.PostBid(p, "Alumina", 1, 1.0)
	b.PostBid(p, "Zinc", 1, 1.0) // product on both sides appears once

	got := b.Products()
	if len(got) != 2 || got[0] != "Alumina" || got[1] != "Zinc" {
		t.Errorf("expected sorted unique products [Alumina Zinc], got %v", got)
	}

	b.Clear()
	if len(b.Products()) != 0 {
		t.Error("expected an empty book after Clear")
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./market -v`
Expected: FAIL — package does not exist / undefined `NewBook`.

- [ ] **Step 4: Write the implementation**

Create `market/book.go`:

```go
// Package market implements the order book through which producers
// discover each other: sellers post asks (unsold capacity at a price),
// buyers post bids (unmet demand at a price), and a per-tick matching
// pass crosses them into contracts. The book is rebuilt from live
// producer state every tick -- only the prices carried by the producers
// themselves persist between ticks.
package market

import (
	"sort"

	"github.com/paul-freeman/satisfactory-story/production"
)

// Ask is a standing offer to sell Remaining units/sec of Product at
// UnitPrice money per unit.
type Ask struct {
	Seller    production.Producer
	Product   string
	Remaining float64
	UnitPrice float64
}

// Bid is a standing offer to buy Remaining units/sec of Product at up to
// UnitPrice money per unit.
type Bid struct {
	Buyer     production.Producer
	Product   string
	Remaining float64
	UnitPrice float64
}

// Book holds the standing asks and bids for every product. Orders are
// kept in posting order; queries that need price order pick minima with
// strict comparison, so ties resolve by posting order and results are
// deterministic.
type Book struct {
	asks map[string][]*Ask
	bids map[string][]*Bid
}

func NewBook() *Book {
	return &Book{
		asks: make(map[string][]*Ask),
		bids: make(map[string][]*Bid),
	}
}

// Clear drops every order. Called at the top of each tick before
// producers republish from live state.
func (b *Book) Clear() {
	b.asks = make(map[string][]*Ask)
	b.bids = make(map[string][]*Bid)
}

func (b *Book) PostAsk(seller production.Producer, product string, rate, unitPrice float64) {
	if rate <= production.RateEpsilon {
		return
	}
	b.asks[product] = append(b.asks[product],
		&Ask{Seller: seller, Product: product, Remaining: rate, UnitPrice: unitPrice})
}

func (b *Book) PostBid(buyer production.Producer, product string, rate, unitPrice float64) {
	if rate <= production.RateEpsilon {
		return
	}
	b.bids[product] = append(b.bids[product],
		&Bid{Buyer: buyer, Product: product, Remaining: rate, UnitPrice: unitPrice})
}

// Asks returns the asks for product in posting order.
func (b *Book) Asks(product string) []*Ask { return b.asks[product] }

// Bids returns the bids for product in posting order.
func (b *Book) Bids(product string) []*Bid { return b.bids[product] }

// BestAsk returns the unfilled ask with the lowest unit price.
func (b *Book) BestAsk(product string) (*Ask, bool) {
	var best *Ask
	for _, ask := range b.asks[product] {
		if ask.Remaining <= production.RateEpsilon {
			continue
		}
		if best == nil || ask.UnitPrice < best.UnitPrice {
			best = ask
		}
	}
	return best, best != nil
}

// BestBid returns the unfilled bid with the highest unit price.
func (b *Book) BestBid(product string) (*Bid, bool) {
	var best *Bid
	for _, bid := range b.bids[product] {
		if bid.Remaining <= production.RateEpsilon {
			continue
		}
		if best == nil || bid.UnitPrice > best.UnitPrice {
			best = bid
		}
	}
	return best, best != nil
}

// Products returns every product with at least one order, sorted, so
// callers can iterate the book deterministically.
func (b *Book) Products() []string {
	seen := make(map[string]bool)
	products := make([]string, 0, len(b.asks)+len(b.bids))
	for product := range b.asks {
		if !seen[product] {
			seen[product] = true
			products = append(products, product)
		}
	}
	for product := range b.bids {
		if !seen[product] {
			seen[product] = true
			products = append(products, product)
		}
	}
	sort.Strings(products)
	return products
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./market -v && go build ./...`
Expected: PASS (4 tests).

- [ ] **Step 6: Commit**

```bash
git add production/production.go market/
git commit -m "feat: add market package with per-product order book"
```

---

### Task 2: Market package — matching

**Files:**
- Create: `market/match.go`
- Test: `market/match_test.go`

**Interfaces:**
- Consumes: `Book`, `Ask`, `Bid` from Task 1; `point.Point`.
- Produces:
  - `market.Match{Seller, Buyer production.Producer; Order production.Production; UnitPrice, TransportCost float64}`
  - `(*Book) MatchAll(transport func(origin, destination point.Point) float64, sign func(Match) error)`

**Matching semantics (implement exactly):** products in sorted order; per product, bids served in descending unit-price order (stable sort, ties by posting order). Each bid repeatedly takes the live ask with the lowest **per-unit delivered cost** = `ask.UnitPrice + transport/rate` where `rate = min(bid.Remaining, ask.Remaining)`, until no ask it can afford remains (`bid.UnitPrice >= per-unit delivered cost`). Trades execute at the ask's unit price. If `sign` fails, that ask is skipped for that bid and nothing is consumed. Self-trades (`ask.Seller == bid.Buyer`) are never matched.

- [ ] **Step 1: Write the failing tests**

Create `market/match_test.go`:

```go
package market

import (
	"errors"
	"testing"

	"github.com/paul-freeman/satisfactory-story/point"
)

// flatTransport makes delivered cost independent of geometry so price
// assertions are exact.
func flatTransport(_, _ point.Point) float64 { return 1.0 }

func collectMatches(matches *[]Match) func(Match) error {
	return func(m Match) error {
		*matches = append(*matches, m)
		return nil
	}
}

func Test_MatchAll_crosses_bid_and_ask_at_ask_price(t *testing.T) {
	b := NewBook()
	seller := testProducer(0, 0)
	buyer := testProducer(10, 10)
	b.PostAsk(seller, "Ingot", 5, 2.0)
	b.PostBid(buyer, "Ingot", 5, 3.0) // 3.0 >= 2.0 + 1.0/5

	var matches []Match
	b.MatchAll(flatTransport, collectMatches(&matches))

	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	m := matches[0]
	if m.Seller != seller || m.Buyer != buyer {
		t.Error("match connected the wrong parties")
	}
	if m.Order.Name != "Ingot" || m.Order.Rate != 5 {
		t.Errorf("expected order Ingot@5, got %s@%f", m.Order.Name, m.Order.Rate)
	}
	if m.UnitPrice != 2.0 {
		t.Errorf("trade should execute at the ask price 2.0, got %f", m.UnitPrice)
	}
	if m.TransportCost != 1.0 {
		t.Errorf("expected transport 1.0, got %f", m.TransportCost)
	}
}

func Test_MatchAll_does_not_cross_unaffordable(t *testing.T) {
	b := NewBook()
	b.PostAsk(testProducer(0, 0), "Ingot", 5, 2.0)
	b.PostBid(testProducer(1, 1), "Ingot", 5, 1.0) // 1.0 < 2.0 + transport

	var matches []Match
	b.MatchAll(flatTransport, collectMatches(&matches))
	if len(matches) != 0 {
		t.Fatalf("expected no matches, got %d", len(matches))
	}
}

func Test_MatchAll_partial_fill_spans_two_asks(t *testing.T) {
	b := NewBook()
	s1 := testProducer(0, 0)
	s2 := testProducer(1, 1)
	buyer := testProducer(2, 2)
	b.PostAsk(s1, "Ingot", 3, 1.0) // cheaper, taken first
	b.PostAsk(s2, "Ingot", 10, 2.0)
	b.PostBid(buyer, "Ingot", 5, 10.0)

	var matches []Match
	b.MatchAll(flatTransport, collectMatches(&matches))

	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
	if matches[0].Seller != s1 || matches[0].Order.Rate != 3 {
		t.Errorf("first match should take all of s1 (rate 3), got %+v", matches[0])
	}
	if matches[1].Seller != s2 || matches[1].Order.Rate != 2 {
		t.Errorf("second match should take rate 2 from s2, got %+v", matches[1])
	}
	if bid := b.Bids("Ingot")[0]; bid.Remaining > 1e-9 {
		t.Errorf("bid should be fully consumed, has %f remaining", bid.Remaining)
	}
}

func Test_MatchAll_higher_bid_served_first(t *testing.T) {
	b := NewBook()
	seller := testProducer(0, 0)
	rich := testProducer(1, 1)
	poor := testProducer(2, 2)
	b.PostAsk(seller, "Ingot", 5, 1.0)
	b.PostBid(poor, "Ingot", 5, 2.0)
	b.PostBid(rich, "Ingot", 5, 9.0)

	var matches []Match
	b.MatchAll(flatTransport, collectMatches(&matches))

	if len(matches) != 1 {
		t.Fatalf("expected exactly 1 match (capacity 5), got %d", len(matches))
	}
	if matches[0].Buyer != rich {
		t.Error("the higher bid should be served first")
	}
}

func Test_MatchAll_prefers_lower_delivered_cost(t *testing.T) {
	b := NewBook()
	near := testProducer(0, 0)
	far := testProducer(100, 100)
	buyer := testProducer(0, 1)
	// Same ask price; transport must decide.
	distanceTransport := func(o, d point.Point) float64 { return o.Distance(d) }
	b.PostAsk(far, "Ingot", 5, 1.0)
	b.PostAsk(near, "Ingot", 5, 1.0)
	b.PostBid(buyer, "Ingot", 5, 100.0)

	var matches []Match
	b.MatchAll(distanceTransport, collectMatches(&matches))

	if len(matches) != 1 || matches[0].Seller != near {
		t.Fatalf("expected the near seller to win, got %+v", matches)
	}
}

func Test_MatchAll_skips_ask_when_sign_fails(t *testing.T) {
	b := NewBook()
	bad := testProducer(0, 0)
	good := testProducer(1, 1)
	buyer := testProducer(2, 2)
	b.PostAsk(bad, "Ingot", 5, 1.0) // cheaper but sign will reject it
	b.PostAsk(good, "Ingot", 5, 2.0)
	b.PostBid(buyer, "Ingot", 5, 10.0)

	var matches []Match
	b.MatchAll(flatTransport, func(m Match) error {
		if m.Seller == bad {
			return errors.New("rejected")
		}
		matches = append(matches, m)
		return nil
	})

	if len(matches) != 1 || matches[0].Seller != good {
		t.Fatalf("expected fallback to the good seller, got %+v", matches)
	}
	if b.Asks("Ingot")[0].Remaining != 5 {
		t.Error("a failed sign must not consume the ask")
	}
}

func Test_MatchAll_never_self_trades(t *testing.T) {
	b := NewBook()
	p := testProducer(0, 0)
	b.PostAsk(p, "Ingot", 5, 1.0)
	b.PostBid(p, "Ingot", 5, 10.0)

	var matches []Match
	b.MatchAll(flatTransport, collectMatches(&matches))
	if len(matches) != 0 {
		t.Fatalf("expected no self-trade, got %d matches", len(matches))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./market -v`
Expected: FAIL — undefined `Match`, `MatchAll`.

- [ ] **Step 3: Write the implementation**

Create `market/match.go`:

```go
package market

import (
	"math"
	"sort"

	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
)

// Match is a crossed bid/ask pair ready to become a contract. The trade
// executes at the ask's unit price; the contract's ProductCost is
// UnitPrice * Order.Rate.
type Match struct {
	Seller        production.Producer
	Buyer         production.Producer
	Order         production.Production
	UnitPrice     float64
	TransportCost float64
}

// MatchAll crosses bids and asks product by product and calls sign for
// each match. Bids are served in descending price order (ties by posting
// order); each bid takes the ask with the lowest per-unit delivered cost
// (ask price plus transport amortized over the candidate rate) until no
// ask it can afford remains. If sign returns an error the ask is skipped
// for that bid and nothing is consumed.
//
// Transport is passed in (rather than importing recipes) so tests can
// use simple geometries.
func (b *Book) MatchAll(transport func(origin, destination point.Point) float64, sign func(Match) error) {
	for _, product := range b.Products() {
		bids := make([]*Bid, len(b.bids[product]))
		copy(bids, b.bids[product])
		sort.SliceStable(bids, func(i, j int) bool {
			return bids[i].UnitPrice > bids[j].UnitPrice
		})
		for _, bid := range bids {
			skipped := make(map[*Ask]bool)
			for bid.Remaining > production.RateEpsilon {
				ask, rate, unitCost := b.bestDeliveredAsk(product, bid, skipped, transport)
				if ask == nil || bid.UnitPrice < unitCost {
					break
				}
				m := Match{
					Seller:        ask.Seller,
					Buyer:         bid.Buyer,
					Order:         production.Production{Name: product, Rate: rate},
					UnitPrice:     ask.UnitPrice,
					TransportCost: transport(ask.Seller.Location(), bid.Buyer.Location()),
				}
				if err := sign(m); err != nil {
					skipped[ask] = true
					continue
				}
				ask.Remaining -= rate
				bid.Remaining -= rate
			}
		}
	}
}

// bestDeliveredAsk returns the live ask with the lowest per-unit
// delivered cost for this bid (nil if none remains), along with the
// candidate rate and that per-unit cost.
func (b *Book) bestDeliveredAsk(
	product string,
	bid *Bid,
	skipped map[*Ask]bool,
	transport func(point.Point, point.Point) float64,
) (*Ask, float64, float64) {
	var best *Ask
	var bestRate, bestCost float64
	for _, ask := range b.asks[product] {
		if skipped[ask] || ask.Remaining <= production.RateEpsilon || ask.Seller == bid.Buyer {
			continue
		}
		rate := math.Min(bid.Remaining, ask.Remaining)
		unitCost := ask.UnitPrice + transport(ask.Seller.Location(), bid.Buyer.Location())/rate
		if best == nil || unitCost < bestCost {
			best, bestRate, bestCost = ask, rate, unitCost
		}
	}
	return best, bestRate, bestCost
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./market -v && go build ./...`
Expected: PASS (all market tests).

- [ ] **Step 5: Commit**

```bash
git add market/
git commit -m "feat: add transport-aware bid/ask matching to the order book"
```

---

### Task 3: Factory market state (ask/bid prices, producing/idle, marginal cost)

**Files:**
- Modify: `factory/factory.go`
- Test: `factory/factory_test.go` (append)

**Interfaces:**
- Consumes: `production.DefaultUnitPrice`, `production.RateEpsilon` (Task 1).
- Produces (exact signatures, used by tasks 5–9):
  - Fields `AskPrices map[string]float64`, `BidPrices map[string]float64` on `Factory` (initialized by `New`)
  - `(f *Factory) AskPriceFor(name string) float64` / `SetAskPrice(name string, price float64)`
  - `(f *Factory) BidPriceFor(name string) float64` / `SetBidPrice(name string, price float64)`
  - `(f *Factory) UnmetInputRate(name string) float64`
  - `(f *Factory) Producing() bool`
  - `(f *Factory) MarginalUnitCost(upkeep float64) float64`

This task is purely additive — nothing existing changes, everything keeps compiling.

- [ ] **Step 1: Write the failing tests**

Append to `factory/factory_test.go`:

```go
func Test_Factory_ask_and_bid_prices_default_and_set(t *testing.T) {
	f := New("Test", "Recipe_Test_C", point.Point{X: 0, Y: 0}, 0,
		production.Products{{Name: "Ore", Rate: 5}},
		production.Products{{Name: "Ingot", Rate: 5}}, 0)

	if got := f.AskPriceFor("Ingot"); got != production.DefaultUnitPrice {
		t.Errorf("unquoted ask should default to %f, got %f", production.DefaultUnitPrice, got)
	}
	f.SetAskPrice("Ingot", 2.5)
	if got := f.AskPriceFor("Ingot"); got != 2.5 {
		t.Errorf("got %f, want 2.5", got)
	}

	if got := f.BidPriceFor("Ore"); got != production.DefaultUnitPrice {
		t.Errorf("unquoted bid should default to %f, got %f", production.DefaultUnitPrice, got)
	}
	f.SetBidPrice("Ore", 0.7)
	if got := f.BidPriceFor("Ore"); got != 0.7 {
		t.Errorf("got %f, want 0.7", got)
	}
}

func Test_Factory_UnmetInputRate_and_Producing(t *testing.T) {
	f := New("Test", "Recipe_Test_C", point.Point{X: 0, Y: 0}, 0,
		production.Products{{Name: "Ore", Rate: 5}, {Name: "Coal", Rate: 2}},
		production.Products{{Name: "Ingot", Rate: 5}}, 0)

	if got := f.UnmetInputRate("Ore"); got != 5 {
		t.Errorf("got %f, want 5", got)
	}
	if f.Producing() {
		t.Error("a factory with no input contracts must not be producing")
	}

	// Two partial contracts covering Ore, one covering Coal.
	f.Purchases = append(f.Purchases,
		&production.Contract{Order: production.Production{Name: "Ore", Rate: 3}},
		&production.Contract{Order: production.Production{Name: "Ore", Rate: 2}},
	)
	if got := f.UnmetInputRate("Ore"); got != 0 {
		t.Errorf("got %f, want 0", got)
	}
	if f.Producing() {
		t.Error("Coal is still unsourced -- must not be producing")
	}

	coal := &production.Contract{Order: production.Production{Name: "Coal", Rate: 2}}
	f.Purchases = append(f.Purchases, coal)
	if !f.Producing() {
		t.Error("all inputs covered -- must be producing")
	}

	// Cancelled contracts stop counting.
	coal.Cancel()
	if f.Producing() {
		t.Error("a cancelled contract must not count as coverage")
	}
	if got := f.UnmetInputRate("NotAnInput"); got != 0 {
		t.Errorf("unknown input should report 0 unmet, got %f", got)
	}
}

func Test_Factory_MarginalUnitCost(t *testing.T) {
	f := New("Test", "Recipe_Test_C", point.Point{X: 0, Y: 0}, 0,
		production.Products{{Name: "Ore", Rate: 5}},
		production.Products{{Name: "Ingot", Rate: 4}}, 0)
	f.Purchases = append(f.Purchases, &production.Contract{
		Order:         production.Production{Name: "Ore", Rate: 5},
		ProductCost:   10,
		TransportCost: 2,
	})

	// (10 + 2 purchases + 0.5 upkeep) / 4 output rate = 3.125
	if got := f.MarginalUnitCost(0.5); got != 3.125 {
		t.Errorf("got %f, want 3.125", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./factory -v`
Expected: FAIL — undefined `AskPriceFor`, `UnmetInputRate`, etc.

- [ ] **Step 3: Write the implementation**

In `factory/factory.go`, add the two fields to the struct (after `Sales []*production.Contract`):

```go
	// AskPrices holds this factory's standing per-unit sale price for
	// each output; BidPrices the per-unit price it currently offers for
	// each input. Both are adjusted by the market loop (state/prices.go)
	// -- they are the only market state that persists between ticks.
	AskPrices map[string]float64
	BidPrices map[string]float64
```

Initialize them in `New` (alongside `Purchases`/`Sales`):

```go
		AskPrices: make(map[string]float64),
		BidPrices: make(map[string]float64),
```

Append the methods:

```go
// AskPriceFor returns the standing per-unit sale price for the named
// product, defaulting on first quote.
func (f *Factory) AskPriceFor(name string) float64 {
	if f.AskPrices == nil {
		f.AskPrices = make(map[string]float64)
	}
	price, ok := f.AskPrices[name]
	if !ok {
		price = production.DefaultUnitPrice
		f.AskPrices[name] = price
	}
	return price
}

// SetAskPrice records a new standing per-unit sale price.
func (f *Factory) SetAskPrice(name string, price float64) {
	if f.AskPrices == nil {
		f.AskPrices = make(map[string]float64)
	}
	f.AskPrices[name] = price
}

// BidPriceFor returns the standing per-unit purchase offer for the named
// input, defaulting on first quote.
func (f *Factory) BidPriceFor(name string) float64 {
	if f.BidPrices == nil {
		f.BidPrices = make(map[string]float64)
	}
	price, ok := f.BidPrices[name]
	if !ok {
		price = production.DefaultUnitPrice
		f.BidPrices[name] = price
	}
	return price
}

// SetBidPrice records a new standing per-unit purchase offer.
func (f *Factory) SetBidPrice(name string, price float64) {
	if f.BidPrices == nil {
		f.BidPrices = make(map[string]float64)
	}
	f.BidPrices[name] = price
}

// UnmetInputRate returns how much of the named input's required rate is
// not yet covered by active purchase contracts.
func (f *Factory) UnmetInputRate(name string) float64 {
	required := 0.0
	for _, input := range f.Input {
		if input.Name == name {
			required = input.Rate
			break
		}
	}
	for _, purchase := range f.Purchases {
		if !purchase.Cancelled && purchase.Order.Name == name {
			required -= purchase.Order.Rate
		}
	}
	if required < 0 {
		return 0
	}
	return required
}

// Producing reports whether every input is fully covered by active
// purchase contracts. A factory that is not producing publishes no asks
// and sells nothing -- it is idle, waiting for its input bids to fill.
func (f *Factory) Producing() bool {
	for _, input := range f.Input {
		if f.UnmetInputRate(input.Name) > production.RateEpsilon {
			return false
		}
	}
	return true
}

// MarginalUnitCost is the factory's current per-unit cost basis: what it
// pays per tick (active purchases including transport, plus upkeep)
// spread over its total output rate. Used as the floor when the market
// loop lowers this factory's ask prices -- it never knowingly sells
// below cost.
func (f *Factory) MarginalUnitCost(upkeep float64) float64 {
	cost := upkeep
	for _, purchase := range f.Purchases {
		if !purchase.Cancelled {
			cost += purchase.ProductCost + purchase.TransportCost
		}
	}
	totalRate := 0.0
	for _, output := range f.Output {
		totalRate += output.Rate
	}
	if totalRate <= production.RateEpsilon {
		return cost
	}
	return cost / totalRate
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./factory ./market -v && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add factory/
git commit -m "feat: add market price state and idle/producing lifecycle to Factory"
```

---

### Task 4: Resource ask price, sink bid price

**Files:**
- Modify: `resources/resources.go`, `sink/sink.go`, `state/sinks.go`
- Test: `resources/resources_test.go` (append), `state/sinks_test.go` (modify)

**Interfaces:**
- Consumes: `production.DefaultUnitPrice` (Task 1); `sink.Sink` (existing).
- Produces:
  - Field `AskPrice float64` on `resources.Resource`; `(r *Resource) AskPriceFor(name string) float64`, `(r *Resource) SetAskPrice(name string, price float64)`
  - Field `BidUnitPrice float64` on `sink.Sink`; `sink.New(name string, loc point.Point, input production.Products, bidUnitPrice float64) *Sink` (**signature change**)
  - `state` constants: `goalBidUnitPrice = 1000.0`, `sinkDemandRate = 100.0`
- Note: there is **no floor sink entity** — the floor is salvage revenue, added in Task 6 (see Interim-state notes).

- [ ] **Step 1: Write the failing tests**

Append to `resources/resources_test.go`:

```go
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
```

(If `resources/resources_test.go` does not already import `production`, add the import.)

In `state/sinks_test.go`, extend `Test_newSinks_finds_distinct_space_elevator_parts` with a price assertion — add at the end of the function:

```go
	for _, sk := range sinks {
		if sk.BidUnitPrice != goalBidUnitPrice {
			t.Errorf("goal sink %s should bid %f, got %f", sk.Name, goalBidUnitPrice, sk.BidUnitPrice)
		}
	}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./resources ./state -run 'Test_Resource_ask|Test_newSinks' -v`
Expected: FAIL — undefined `AskPriceFor`, `BidUnitPrice`, `goalBidUnitPrice`.

- [ ] **Step 3: Implement — resource ask price**

In `resources/resources.go`, add to the `Resource` struct:

```go
	// AskPrice is the persistent per-unit sale price for this node's
	// product, adjusted by the market loop. Zero means "not yet quoted";
	// it defaults on first use.
	AskPrice float64
```

Append the methods:

```go
// AskPriceFor returns the standing per-unit sale price for the named
// product (0 for a product this node does not produce), defaulting on
// first quote.
func (r *Resource) AskPriceFor(name string) float64 {
	if name != r.Production.Name {
		return 0
	}
	if r.AskPrice == 0 {
		r.AskPrice = production.DefaultUnitPrice
	}
	return r.AskPrice
}

// SetAskPrice records a new standing per-unit sale price.
func (r *Resource) SetAskPrice(name string, price float64) {
	if name != r.Production.Name {
		return
	}
	r.AskPrice = price
}
```

- [ ] **Step 4: Implement — sink bid price**

In `sink/sink.go`, add to the `Sink` struct:

```go
	// BidUnitPrice is the standing per-unit price this sink offers for
	// every product it wants. Goal sinks bid high -- their demand is the
	// engine of the whole economy.
	BidUnitPrice float64
```

Change `New` to accept it:

```go
func New(
	name string,
	loc point.Point,
	input production.Products,
	bidUnitPrice float64,
) *Sink {
	return &Sink{
		Name:         name,
		Loc:          loc,
		Input:        input,
		Purchases:    make([]*production.Contract, 0),
		BidUnitPrice: bidUnitPrice,
	}
}
```

- [ ] **Step 5: Implement — state constants**

In `state/sinks.go`, add constants (next to `spaceElevatorPartPrefix`; delete `sinkPerpetualShortage` **only in Task 5** — leave it for now):

```go
// goalBidUnitPrice is what a space-elevator sink offers per unit of its
// part. It is deliberately far above any plausible production cost so
// the recipe that makes the part is always the most profitable thing in
// the book -- this bid is the money that cascades backward through the
// supply tiers.
const goalBidUnitPrice = 1000.0

// sinkDemandRate is the standing bid rate for sinks. Effectively
// unlimited against realistic production rates (single recipes run at
// ~0.1-10 units/sec) while staying readable in the UI and safe in
// min() arithmetic.
const sinkDemandRate = 100.0
```

Update the `sink.New` call inside `newSinks` to pass the price:

```go
			sinks = append(sinks, sink.New(output.Name, center, production.Products{
				production.New(output.Name, 1, 1),
			}, goalBidUnitPrice))
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go build ./... && go test ./...`
Expected: PASS (everything, including untouched packages — `sourceSinks` still compiles because `newSinks` still returns `[]*sink.Sink`).

- [ ] **Step 7: Commit**

```bash
git add resources/ sink/ state/
git commit -m "feat: add persistent ask/bid prices to Resource and Sink, floor-buyer constructor"
```

---

### Task 5: Publish + match in the tick loop; retire `sourceSinks`

**Files:**
- Create: `state/orders.go`
- Test: `state/orders_test.go`
- Modify: `state/state.go` (State struct, `getInitialState`, `Tick`), `state/sinks.go` (delete `sourceSinks` + `sinkPerpetualShortage`), `state/sinks_test.go` (delete `Test_sourceSinks_buys_all_available_capacity`), `state/spawn_test.go` (extend `newTestState`), `state/state_test.go` (skip long-run test)

**Interfaces:**
- Consumes: everything from Tasks 1–4.
- Produces:
  - `State` fields: `book *market.Book`, `lastTrade map[string]float64`
  - `(s *State) publishOrders(l *slog.Logger)`, `(s *State) matchOrders(l *slog.Logger)`
  - `(s *State) signContract(l *slog.Logger, m market.Match) error`
- Still alive after this task (deleted later): old `writeContract` + `s.market` (used by old spawn/renegotiate until Tasks 7–8), `shortage.go` (until Task 7).

- [ ] **Step 1: Write the failing tests**

Create `state/orders_test.go`:

```go
package state

import (
	"testing"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
	"github.com/paul-freeman/satisfactory-story/recipes"
	"github.com/paul-freeman/satisfactory-story/resources"
)

func Test_publishOrders_resource_ask_and_factory_orders(t *testing.T) {
	ore := &resources.Resource{
		Production: production.Production{Name: "Ore", Rate: 100},
		Loc:        point.Point{X: 500, Y: 500},
	}
	idle := factory.New("Smelt", "Recipe_Smelt_C", point.Point{X: 600, Y: 600}, 0,
		production.Products{{Name: "Ore", Rate: 5}},
		production.Products{{Name: "Ingot", Rate: 5}}, 1000)

	s := newTestState(recipes.Recipes{}, []production.Producer{ore, idle})
	s.publishOrders(testLogger())

	if asks := s.book.Asks("Ore"); len(asks) != 1 || asks[0].Remaining != 100 {
		t.Fatalf("expected one Ore ask at rate 100, got %+v", asks)
	}
	if bids := s.book.Bids("Ore"); len(bids) != 1 || bids[0].Remaining != 5 {
		t.Fatalf("expected one Ore bid at rate 5, got %+v", bids)
	}
	// The factory is idle (no input contracts) so it must not offer output.
	if asks := s.book.Asks("Ingot"); len(asks) != 0 {
		t.Fatalf("an idle factory must not publish asks, got %+v", asks)
	}
}

func Test_matchOrders_signs_contract_and_records_trade(t *testing.T) {
	ore := &resources.Resource{
		Production: production.Production{Name: "Ore", Rate: 100},
		Loc:        point.Point{X: 500, Y: 500},
	}
	buyer := factory.New("Smelt", "Recipe_Smelt_C", point.Point{X: 600, Y: 600}, 0,
		production.Products{{Name: "Ore", Rate: 5}},
		production.Products{{Name: "Ingot", Rate: 5}}, 1000)
	buyer.SetBidPrice("Ore", 50.0) // rich enough to cover ask + transport

	s := newTestState(recipes.Recipes{}, []production.Producer{ore, buyer})
	s.publishOrders(testLogger())
	s.matchOrders(testLogger())

	if !buyer.Producing() {
		t.Fatal("expected the buyer's Ore input to be fully contracted")
	}
	if got := ore.RemainingCapacityFor("Ore"); got != 95 {
		t.Errorf("expected 95 capacity left on the ore node, got %f", got)
	}
	if len(buyer.Purchases) != 1 {
		t.Fatalf("expected 1 purchase, got %d", len(buyer.Purchases))
	}
	p := buyer.Purchases[0]
	wantCost := production.DefaultUnitPrice * 5 // ask price * rate
	if p.ProductCost != wantCost {
		t.Errorf("expected ProductCost %f, got %f", wantCost, p.ProductCost)
	}
	wantTransport := recipes.TransportCost(ore.Loc, buyer.Loc)
	if p.TransportCost != wantTransport {
		t.Errorf("expected TransportCost %f, got %f", wantTransport, p.TransportCost)
	}
	if got := s.lastTrade["Ore"]; got != production.DefaultUnitPrice {
		t.Errorf("expected lastTrade recorded at %f, got %f", production.DefaultUnitPrice, got)
	}
}

func Test_matchOrders_goal_sink_buys_available_capacity(t *testing.T) {
	// A resource node standing in for a finished-part producer, matching
	// the fixture style of the old sourceSinks test.
	seller := &resources.Resource{
		Production: production.Production{Name: "SpaceElevatorPart_1", Rate: 5},
		Loc:        point.Point{X: 0, Y: 0},
	}
	rs := recipes.Recipes{
		{DisplayName: "A", OutputProducts: production.Products{{Name: "SpaceElevatorPart_1", Rate: 1}}},
	}
	sinks := newSinks(rs, 0, 1000, 0, 1000)

	producers := []production.Producer{seller}
	for _, sk := range sinks {
		producers = append(producers, sk)
	}
	s := newTestState(rs, producers)

	s.publishOrders(testLogger())
	s.matchOrders(testLogger())

	if got := seller.RemainingCapacityFor("SpaceElevatorPart_1"); got != 0 {
		t.Errorf("expected the goal sink to buy all capacity, got %f left", got)
	}
}
```

- [ ] **Step 2: Extend the test-state helper**

In `state/spawn_test.go`, update `newTestState` (add the two new fields; `market`/`unmet` stay for now):

```go
func newTestState(rs recipes.Recipes, producers []production.Producer) *State {
	return &State{
		recipes:   rs,
		producers: producers,
		market:    make(map[string]float64),
		unmet:     make(map[string]float64),
		book:      market.NewBook(),
		lastTrade: make(map[string]float64),
		randSrc:   rand.New(rand.NewSource(1)),
		xmin:      0, xmax: 1000, ymin: 0, ymax: 1000,
	}
}
```

Add `"github.com/paul-freeman/satisfactory-story/market"` to the file's imports.

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./state -run 'Test_publishOrders|Test_matchOrders' -v`
Expected: FAIL — `State` has no field `book`; undefined `publishOrders`.

- [ ] **Step 4: Implement — State fields and initialization**

In `state/state.go`, add to the `State` struct (after `unmet map[string]float64`):

```go
	// book is the per-tick order book: rebuilt from live producer state
	// by publishOrders, crossed by matchOrders. Persisted on State so
	// later phases of the same tick (spawning, renegotiation, price
	// adjustment, the wire format) can read post-matching residuals.
	book *market.Book
	// lastTrade remembers the most recent traded unit price per product,
	// used to estimate input costs for products with no current ask.
	lastTrade map[string]float64
```

Add the import `"github.com/paul-freeman/satisfactory-story/market"`.

In `getInitialState`, in the "Populate state" section:

```go
	s.book = market.NewBook()
	s.lastTrade = make(map[string]float64)
```

- [ ] **Step 5: Implement — orders**

Create `state/orders.go`:

```go
package state

import (
	"fmt"
	"log/slog"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/market"
	"github.com/paul-freeman/satisfactory-story/production"
	"github.com/paul-freeman/satisfactory-story/recipes"
	"github.com/paul-freeman/satisfactory-story/resources"
	"github.com/paul-freeman/satisfactory-story/sink"
)

// publishOrders rebuilds the book from live producer state: every unsold
// unit of capacity becomes an ask, every unmet unit of demand becomes a
// bid. Zero-rate orders are dropped by the book itself. Only prices
// persist between ticks (on the producers); quantities can never go
// stale because they are re-derived here every tick.
func (s *State) publishOrders(_ *slog.Logger) {
	s.book.Clear()
	for _, p := range s.producers {
		switch producer := p.(type) {
		case *resources.Resource:
			name := producer.Production.Name
			s.book.PostAsk(producer, name,
				producer.RemainingCapacityFor(name), producer.AskPriceFor(name))
		case *factory.Factory:
			// An idle factory sells nothing -- its capacity is
			// hypothetical until its inputs are contracted.
			if producer.Producing() {
				for _, output := range producer.Output {
					s.book.PostAsk(producer, output.Name,
						producer.RemainingCapacityFor(output.Name),
						producer.AskPriceFor(output.Name))
				}
			}
			for _, input := range producer.Input {
				s.book.PostBid(producer, input.Name,
					producer.UnmetInputRate(input.Name),
					producer.BidPriceFor(input.Name))
			}
		case *sink.Sink:
			for _, want := range producer.Input {
				s.book.PostBid(producer, want.Name, sinkDemandRate, producer.BidUnitPrice)
			}
		}
	}
}

// matchOrders crosses the book and signs a contract for every match.
func (s *State) matchOrders(l *slog.Logger) {
	s.book.MatchAll(recipes.TransportCost, func(m market.Match) error {
		return s.signContract(l, m)
	})
}

// signContract turns a match into a signed production.Contract. It is
// the only place matched trades become real: capacity is re-checked
// against live state (the book's Remaining is bookkeeping, not
// authority), both parties sign, and the traded unit price is recorded.
func (s *State) signContract(l *slog.Logger, m market.Match) error {
	if err := m.Seller.HasCapacityFor(m.Order); err != nil {
		return fmt.Errorf("cannot sign contract: %w", err)
	}
	contract := &production.Contract{
		Seller:        m.Seller,
		Buyer:         m.Buyer,
		Order:         m.Order,
		TransportCost: m.TransportCost,
		ProductCost:   m.UnitPrice * m.Order.Rate,
	}
	if err := m.Seller.SignAsSeller(contract); err != nil {
		contract.Cancel()
		return fmt.Errorf("seller rejected contract: %w", err)
	}
	if err := m.Buyer.SignAsBuyer(contract); err != nil {
		contract.Cancel()
		return fmt.Errorf("buyer rejected contract: %w", err)
	}
	s.lastTrade[m.Order.Name] = m.UnitPrice
	l.Debug("signed contract",
		slog.String("order", m.Order.Key()),
		slog.Float64("rate", m.Order.Rate),
		slog.Float64("unitPrice", m.UnitPrice),
		slog.Float64("transportCost", m.TransportCost),
	)
	return nil
}
```

- [ ] **Step 6: Wire into `Tick`, delete `sourceSinks`**

In `state/state.go`, replace the mechanism block of `Tick` with:

```go
	// Discovery first: rebuild the book from live state and cross it, so
	// every later mechanism this tick (moving, spawning, renegotiation,
	// solvency, price adjustment) sees post-matching reality.
	s.publishOrders(l)
	s.matchOrders(l)
	s.moveProducers(l)
	if s.randSrc.Float64() < spawnProbabilityPerTick {
		s.spawnNewProducer(l)
	}
	s.renegotiateContracts(l)
	s.applySolvency(l)
	s.decayShortages()
```

(The comment above the old block about spawn/move/cull phases can be dropped; `decayShortages` dies in Task 7.)

In `state/sinks.go`: delete the `sourceSinks` function and the `sinkPerpetualShortage` constant. In `state/sinks_test.go`: delete `Test_sourceSinks_buys_all_available_capacity` (replaced by `Test_matchOrders_goal_sink_buys_available_capacity`). Remove imports that become unused in both files.

- [ ] **Step 7: Skip the long-run convergence test**

In `state/state_test.go`, add as the first line of the `t.Run("converges on real production over a long run", ...)` body:

```go
		t.Skip("economy is mid-rework (order-book plan tasks 5-9); superseded by the milestone test re-enabled in the final task")
```

- [ ] **Step 8: Run the full suite**

Run: `go build ./... && go test ./...`
Expected: PASS (long-run test SKIP; old spawn/renegotiate/solvency tests still green — their mechanisms are untouched so far).

- [ ] **Step 9: Commit**

```bash
git add state/
git commit -m "feat: publish and match orders each tick; sinks buy through the book"
```

---

### Task 6: Solvency rework — upkeep, salvage floor, short grace, idle factories live

**Files:**
- Modify: `state/solvency.go` (rewrite), `state/solvency_test.go` (rewrite)

**Interfaces:**
- Consumes: `(f *Factory) Producing()`, `(f *Factory) RemainingCapacityFor` (Task 3), `Wallet` (existing).
- Produces: constants `upkeepPerTick = 0.5`, `insolvencyGrace = 300`, `floorUnitPrice = 0.1` (Tasks 7 and 9 use `upkeepPerTick` and `floorUnitPrice`); salvage revenue (see Interim-state notes, refinement 2).
- Deletes: the "cull anything missing an input contract" rule and the 10,000-tick grace apologia.

- [ ] **Step 1: Rewrite the tests**

Replace the body of `state/solvency_test.go` (keep `testLogger` exactly as is) — the three subtests become:

```go
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
```

(Add `"math"` to the imports of `state/solvency_test.go`.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./state -run Test_applySolvency -v`
Expected: FAIL — undefined `upkeepPerTick`; "keeps an idle factory" fails against the old cull rule.

- [ ] **Step 3: Rewrite the implementation**

Replace `state/solvency.go`:

```go
package state

import (
	"log/slog"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/production"
)

// upkeepPerTick is the fixed cost every factory pays per tick just for
// existing. It is the clock on failure: an idle factory whose input bids
// never fill bleeds seed capital at this rate until bankruptcy. It also
// enters every factory's marginal cost, so nobody's ask price can settle
// below what existing actually costs.
const upkeepPerTick = 0.5

// insolvencyGrace is how many consecutive ticks a factory's wallet may
// sit below zero before it is removed as bankrupt. Unlike the old
// pre-order-book economy (which needed a 10,000-tick grace because a new
// factory could only be discovered by a lucky future spawn), discovery
// is now continuous through the book, so this only needs to cover a
// legible rough patch: a lost supplier, a renegotiation gap, a temporary
// glut.
const insolvencyGrace = 300

// floorUnitPrice is the salvage value of one unsold unit of output:
// thematically, every factory feeds its leftovers to its own on-site
// AWESOME sink. It must stay well below any realistic traded price so
// real trade always beats it; it exists so a producing factory that
// hasn't found buyers yet earns *something* while the market discovers
// it. It is also the guaranteed revenue floor used when estimating a
// prospective recipe's profit (spawn.go) and a factory's achievable
// revenue (prices.go).
const floorUnitPrice = 0.1

// applySolvency runs each factory's tick economics: an idle factory
// (missing input coverage) cannot honor sales, so its sale contracts are
// cancelled; then revenue and expenses (via Profit, which also prunes
// cancelled contracts), salvage on unsold capacity, and upkeep hit the
// wallet; factories insolvent beyond the grace window are removed. Being
// idle is a normal life stage -- a factory waiting for its input bids to
// fill is NOT culled.
func (s *State) applySolvency(l *slog.Logger) {
	survivors := make([]production.Producer, 0, len(s.producers))
	for _, p := range s.producers {
		f, ok := p.(*factory.Factory)
		if !ok {
			survivors = append(survivors, p)
			continue
		}

		salvage := 0.0
		if f.Producing() {
			for _, output := range f.Output {
				salvage += f.RemainingCapacityFor(output.Name) * floorUnitPrice
			}
		} else {
			for _, sale := range f.Sales {
				sale.Cancel()
			}
		}

		f.Wallet.Apply(f.Profit() + salvage - upkeepPerTick)

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

- [ ] **Step 4: Run the full suite**

Run: `go build ./... && go test ./...`
Expected: PASS. (The spawn tests still pass: old spawn creates fully-contracted factories, which the new rules treat the same.)

- [ ] **Step 5: Commit**

```bash
git add state/solvency.go state/solvency_test.go
git commit -m "feat: upkeep-based solvency; idle factories live, halted factories shed sales"
```

---

### Task 7: Spawn rework — expected-profit draw; delete the shortage system

**Files:**
- Modify: `state/spawn.go` (rewrite), `state/spawn_test.go` (rewrite tests, keep `newTestState`), `state/state.go` (`Tick`, `shortagesForWire`, State struct), `state/state_test.go` (`Test_toHTTP_wire_additions`)
- Delete: `state/shortage.go`, `state/shortage_test.go`

**Interfaces:**
- Consumes: `book`/`lastTrade` (Task 5), `upkeepPerTick` (Task 6), `floorUnitPrice` (Task 4), `factory.SetBidPrice` (Task 3).
- Produces:
  - `(s *State) expectedProfit(r *recipes.Recipe) float64`
  - `(s *State) estimatedUnitCost(product string) float64` (also used by nothing else — spawn-only)
  - constants `seedCapitalBufferTicks = 300.0`, `baselineOpportunityWeight = 1.0`, `unknownInputUnitCost = 10.0`; `spawnProbabilityPerTick = 0.05` (unchanged value, stays in spawn.go)
- Deletes: `recordShortage`, `decayShortages`, `weightForProduct`, `shortageDecay`, the `unmet` field, and spawn's sourcing gate/market veto. (`recipes.SourceProducts`/`FindBestSeller` lose their last caller after Task 8; they are deleted there.)

- [ ] **Step 1: Rewrite the spawn tests**

Replace both test functions in `state/spawn_test.go` (keep `newTestState`, now without `unmet`; see Step 4):

```go
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
	s := newTestState(rs, []production.Producer{})

	s.spawnNewProducer(testLogger())

	if len(s.producers) != 1 {
		t.Fatalf("expected an idle factory to spawn, got %d producers", len(s.producers))
	}
	f := s.producers[0].(*factory.Factory)
	if f.Producing() {
		t.Error("the factory should be idle -- there is nothing to source")
	}
	// Seed capital: no ask, no trade history -> pessimistic estimate.
	// (unknownInputUnitCost*5 + upkeepPerTick) * seedCapitalBufferTicks
	want := (unknownInputUnitCost*5 + upkeepPerTick) * seedCapitalBufferTicks
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
	rs := recipes.Recipes{
		{
			ClassName:      "Recipe_Smelt_C",
			DisplayName:    "Smelt Ore",
			Active:         true,
			InputProducts:  production.Products{{Name: "Ore", Rate: 5}},
			OutputProducts: production.Products{{Name: "Ingot", Rate: 5}},
		},
	}
	s := newTestState(rs, []production.Producer{ore})
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
	// Seed capital now uses the real ask price.
	want := (0.4*5 + upkeepPerTick) * seedCapitalBufferTicks
	if math.Abs(f.Wallet.Cash()-want) > 0.0001 {
		t.Errorf("expected seed capital %f, got %f", want, f.Wallet.Cash())
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
	s := newTestState(recipes.Recipes{recipe}, []production.Producer{})

	// No bids at all: revenue falls back to the floor price.
	wantFloor := floorUnitPrice*5 - (unknownInputUnitCost*5 + upkeepPerTick)
	if got := s.expectedProfit(recipe); math.Abs(got-wantFloor) > 0.0001 {
		t.Errorf("expected floor-based profit %f, got %f", wantFloor, got)
	}

	// A real standing bid for the output raises expected profit.
	s.book.PostBid(buyer, "Ingot", 5, 8.0)
	want := 8.0*5 - (unknownInputUnitCost*5 + upkeepPerTick)
	if got := s.expectedProfit(recipe); math.Abs(got-want) > 0.0001 {
		t.Errorf("expected bid-based profit %f, got %f", want, got)
	}

	// A cheap ask for the input raises it further.
	seller := &resources.Resource{Production: production.Production{Name: "Ore", Rate: 100}}
	s.book.PostAsk(seller, "Ore", 100, 0.5)
	want = 8.0*5 - (0.5*5 + upkeepPerTick)
	if got := s.expectedProfit(recipe); math.Abs(got-want) > 0.0001 {
		t.Errorf("expected ask-based profit %f, got %f", want, got)
	}
}
```

Update the file's imports: add `"math"`, keep the rest; remove any that become unused.

- [ ] **Step 2: Update the wire test**

In `state/state_test.go`, replace the shortage fixture in `Test_toHTTP_wire_additions`. Replace:

```go
	s.recordShortage("Widget", 30)
	s.recordShortage("Gadget", 10)
```

with:

```go
	hungry := factory.New("Hungry", "Recipe_Hungry_C", point.Point{X: 0, Y: 0}, 0,
		production.Products{}, production.Products{}, 0)
	s.book.PostBid(hungry, "Widget", 30, 5.0)
	s.book.PostBid(hungry, "Gadget", 10, 2.0)
```

and replace the trailing weight assertions:

```go
	widgetWeight := s.weightForProduct("Widget")
	gadgetWeight := s.weightForProduct("Gadget")
	assert.Greater(t, widgetWeight, gadgetWeight, "sanity check on the fixture itself")
```

with nothing (delete those three lines — the shortage ordering assertions above them stay).

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./state -run 'Test_spawnNewProducer|Test_expectedProfit|Test_toHTTP' -v`
Expected: FAIL — undefined `seedCapitalBufferTicks` semantics, `expectedProfit`, etc.

- [ ] **Step 4: Implement — rewrite `state/spawn.go`**

Replace the constants and `spawnNewProducer` (keep `randomLocation` as is):

```go
package state

import (
	"log/slog"
	"math"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/recipes"
)

// seedCapitalBufferTicks funds a new factory with this many ticks' worth
// of estimated input cost plus upkeep, representing the up-front cost of
// building it. It has to cover a realistic idle wait: the factory spends
// it while its input bids sit in the book waiting for supply to appear.
const seedCapitalBufferTicks = 300.0

// spawnProbabilityPerTick is the chance, per tick, that a new producer
// is attempted at all.
const spawnProbabilityPerTick = 0.05

// baselineOpportunityWeight keeps every active recipe in the spawn draw
// even when the book currently shows no profit in it, so novel recipes
// are still explored occasionally.
const baselineOpportunityWeight = 1.0

// unknownInputUnitCost is the pessimistic per-unit estimate for an input
// with no standing ask and no trade history. A penalty, not a veto:
// recipes with lucrative outputs but unsourceable inputs must still
// spawn now and then, because the bids they post are what summon the
// missing tier.
const unknownInputUnitCost = 10.0

// spawnNewProducer picks a recipe via a weighted random draw over every
// active recipe -- weighted by expected profit against the current order
// book -- and spawns it at a random location. No sourcing happens here:
// the factory starts idle with seed capital, and publishOrders will post
// its input bids next tick. This is how demand cascades backward with
// prices only: a lucrative standing bid for a product makes its recipe
// profitable to spawn, and that factory's own input bids make the next
// tier profitable in turn.
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

	weights := make([]float64, len(activeRecipes))
	total := 0.0
	for i, recipe := range activeRecipes {
		weights[i] = baselineOpportunityWeight + math.Max(0, s.expectedProfit(recipe))
		total += weights[i]
	}

	pick := s.randSrc.Float64() * total
	chosenRecipe := activeRecipes[len(activeRecipes)-1]
	cumulative := 0.0
	for i, weight := range weights {
		cumulative += weight
		if pick <= cumulative {
			chosenRecipe = activeRecipes[i]
			break
		}
	}

	inputCost := 0.0
	for _, input := range chosenRecipe.Inputs() {
		inputCost += s.estimatedUnitCost(input.Name) * input.Rate
	}
	seedCapital := (inputCost + upkeepPerTick) * seedCapitalBufferTicks

	newFactory := factory.New(chosenRecipe.Name(), chosenRecipe.ID(), s.randomLocation(), s.tick,
		chosenRecipe.Inputs(), chosenRecipe.Outputs(), seedCapital)
	// Start bidding at the going rate where one exists; the price loop
	// escalates from there if the bids go unfilled.
	for _, input := range chosenRecipe.Inputs() {
		if ask, ok := s.book.BestAsk(input.Name); ok {
			newFactory.SetBidPrice(input.Name, ask.UnitPrice)
		}
	}
	s.producers = append(s.producers, newFactory)
	l.Debug("spawned producer", slog.String("factory", newFactory.Name))
}

// expectedProfit estimates a recipe's per-tick profit against the
// current book: revenue at the best standing bids for its outputs
// (never below the salvage floor, which every producing factory earns
// on unsold capacity) minus estimated input costs and upkeep.
func (s *State) expectedProfit(r *recipes.Recipe) float64 {
	revenue := 0.0
	for _, output := range r.Outputs() {
		price := floorUnitPrice
		if bid, ok := s.book.BestBid(output.Name); ok && bid.UnitPrice > price {
			price = bid.UnitPrice
		}
		revenue += price * output.Rate
	}
	cost := upkeepPerTick
	for _, input := range r.Inputs() {
		cost += s.estimatedUnitCost(input.Name) * input.Rate
	}
	return revenue - cost
}

// estimatedUnitCost is the best current estimate of what one unit of
// product costs to buy: the best standing ask, else the last traded
// price, else a pessimistic default.
func (s *State) estimatedUnitCost(product string) float64 {
	if ask, ok := s.book.BestAsk(product); ok {
		return ask.UnitPrice
	}
	if price, ok := s.lastTrade[product]; ok {
		return price
	}
	return unknownInputUnitCost
}
```

(`point` stays imported for `randomLocation`.)

- [ ] **Step 5: Implement — delete the shortage system, rewire the shortage panel**

- Delete `state/shortage.go` and `state/shortage_test.go`.
- In `state/state.go`: remove the `unmet map[string]float64` field, its initialization in `getInitialState`, and the `s.decayShortages()` call in `Tick`.
- In `state/spawn_test.go`'s `newTestState`: remove the `unmet: make(map[string]float64),` line.
- In `state/state.go`, replace `shortagesForWire` with the book-backed version (same `shortageWireLimit` cap):

```go
// shortagesForWire reports unmet demand as it actually exists in the
// economy: the post-matching residual bid volume per product.
func (s *State) shortagesForWire() []statehttp.Shortage {
	totals := make(map[string]float64)
	for _, product := range s.book.Products() {
		for _, bid := range s.book.Bids(product) {
			if bid.Remaining <= production.RateEpsilon {
				continue
			}
			totals[product] += bid.Remaining
		}
	}
	shortages := make([]statehttp.Shortage, 0, len(totals))
	for product, amount := range totals {
		shortages = append(shortages, statehttp.Shortage{Product: product, Amount: amount})
	}
	sort.Slice(shortages, func(i, j int) bool {
		return shortages[i].Amount > shortages[j].Amount
	})
	if len(shortages) > shortageWireLimit {
		shortages = shortages[:shortageWireLimit]
	}
	return shortages
}
```

Add `"github.com/paul-freeman/satisfactory-story/production"` to `state/state.go` imports if not already present.

- [ ] **Step 6: Run the full suite**

Run: `go build ./... && go test ./...`
Expected: PASS. Note: `Test_state_Tick/"can run multiple ticks"` now exercises the new spawn — factories spawn idle and post bids; that is correct behavior.

- [ ] **Step 7: Commit**

```bash
git add state/
git commit -m "feat: profit-seeking spawn draw against the book; delete the shortage echo"
```

---

### Task 8: Renegotiation via the book; delete the old pricing machinery

**Files:**
- Modify: `state/renegotiate.go` (rewrite), `state/renegotiate_test.go` (rewrite), `state/state.go` (delete `writeContract` + `market` field), `state/spawn_test.go` (`newTestState` drops `market`), `production/products.go` (drop `SalesPriceFor` from the interface), `factory/factory.go`, `factory/factory_test.go`, `resources/resources.go`, `sink/sink.go`
- Delete: `recipes/source.go`, `recipes/source_test.go`

**Interfaces:**
- Consumes: `signContract` (Task 5), `market.Match`, `book.Asks`.
- Produces: book-based `renegotiateContracts` (same constants `renegotiateProbabilityPerTick = 0.02`, `renegotiationMinMargin = 0.05`).
- Deletes: `State.writeContract`, `State.market`, `Producer.SalesPriceFor` (interface + all three implementations + `Test_Factory_SalesPriceFor`), `recipes.SourceProducts`, `recipes.FindBestSeller`, `recipes.Source`.

- [ ] **Step 1: Rewrite the renegotiation test**

Replace `state/renegotiate_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./state -run Test_renegotiateContracts -v`
Expected: FAIL — the old implementation shops via `FindBestSeller`/`SalesPriceFor`, whose pricing (transport × 1.5) differs from the fixture's book pricing; the assertion on the contract's replacement fails or the compile fails once Step 3 starts. (If it happens to pass against the old code, proceed — the implementation swap below is still verified by the full suite.)

- [ ] **Step 3: Rewrite the implementation**

Replace `state/renegotiate.go`:

```go
package state

import (
	"log/slog"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/market"
	"github.com/paul-freeman/satisfactory-story/production"
	"github.com/paul-freeman/satisfactory-story/recipes"
)

// renegotiateProbabilityPerTick is the chance, per factory per tick,
// that it bothers re-shopping its input contracts at all -- doing this
// for every factory every tick would be wasted search effort once the
// market has settled.
const renegotiateProbabilityPerTick = 0.02

// renegotiationMinMargin is how much cheaper (as a fraction of the
// current total price) a new offer must be before a factory switches
// suppliers. This avoids thrashing between two near-identical prices.
const renegotiationMinMargin = 0.05

// renegotiateContracts lets each factory re-shop each input contract
// against the book's post-matching residual asks, switching supplier if
// one beats the current deal by more than renegotiationMinMargin. This
// is what lets a newly spawned, better-positioned or cheaper producer
// steal business from an incumbent instead of only ever competing for
// brand-new demand.
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
			ask, transportCost, found := s.bestAlternativeAsk(f, purchase)
			if !found {
				continue
			}
			currentTotal := purchase.ProductCost + purchase.TransportCost
			candidateTotal := ask.UnitPrice*purchase.Order.Rate + transportCost
			if candidateTotal >= currentTotal*(1-renegotiationMinMargin) {
				continue
			}

			// Sign the replacement before cancelling the incumbent: if
			// signContract fails (e.g. capacity raced away), the factory
			// keeps its existing supplier rather than being left idle.
			m := market.Match{
				Seller:        ask.Seller,
				Buyer:         f,
				Order:         purchase.Order,
				UnitPrice:     ask.UnitPrice,
				TransportCost: transportCost,
			}
			if err := s.signContract(l, m); err != nil {
				l.Debug("failed to renegotiate contract", slog.String("error", err.Error()))
				continue
			}
			ask.Remaining -= purchase.Order.Rate
			purchase.Cancel()
			l.Debug("renegotiated contract",
				slog.String("factory", f.String()),
				slog.String("product", purchase.Order.Name))
		}
	}
}

// bestAlternativeAsk finds the residual ask with the lowest delivered
// total (product cost plus transport) that could fully replace the given
// purchase. The incumbent seller and the factory itself are excluded.
func (s *State) bestAlternativeAsk(f *factory.Factory, purchase *production.Contract) (*market.Ask, float64, bool) {
	var best *market.Ask
	var bestTransport, bestTotal float64
	for _, ask := range s.book.Asks(purchase.Order.Name) {
		if ask.Seller == purchase.Seller || ask.Seller == production.Producer(f) {
			continue
		}
		if ask.Remaining < purchase.Order.Rate {
			continue
		}
		transportCost := recipes.TransportCost(ask.Seller.Location(), f.Location())
		total := ask.UnitPrice*purchase.Order.Rate + transportCost
		if best == nil || total < bestTotal {
			best, bestTransport, bestTotal = ask, transportCost, total
		}
	}
	return best, bestTransport, best != nil
}
```

- [ ] **Step 4: Delete the old pricing machinery**

All of the following in one pass (they reference each other):

1. `state/state.go`: delete the `writeContract` method and the `market map[string]float64` field + its initialization in `getInitialState`.
2. `state/spawn_test.go`: remove `market: make(map[string]float64),` from `newTestState`.
3. `production/products.go`: remove `SalesPriceFor(Production, float64) float64` (and its comment) from the `Producer` interface.
4. `factory/factory.go`: delete the `SalesPriceFor` method. `factory/factory_test.go`: delete `Test_Factory_SalesPriceFor`.
5. `resources/resources.go`: delete the `SalesPriceFor` method.
6. `sink/sink.go`: delete the `SalesPriceFor` method.
7. Delete `recipes/source.go` and `recipes/source_test.go` (`SourceProducts`/`FindBestSeller` lost their last caller with the old spawn/renegotiate).
8. Remove any imports that become unused.

- [ ] **Step 5: Run the full suite**

Run: `go build ./... && go test ./...`
Expected: PASS everywhere. `grep -rn "SalesPriceFor\|writeContract\|FindBestSeller\|SourceProducts" --include="*.go" .` must return nothing.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat: renegotiate against the book; delete cost-plus pricing and the market ratchet"
```

---

### Task 9: Local price adjustment (`adjustPrices`)

**Files:**
- Create: `state/prices.go`
- Test: `state/prices_test.go`
- Modify: `state/state.go` (`Tick`)

**Interfaces:**
- Consumes: `book` residuals after matching (Tasks 1–2, 5), `factory.SetAskPrice`/`SetBidPrice`/`MarginalUnitCost`/`RemainingCapacityFor`/`UnmetInputRate` (Task 3), `resources.SetAskPrice` (Task 4), `upkeepPerTick` (Task 6).
- Produces: `(s *State) adjustPrices(l *slog.Logger)`; constants `askRaisePct = 0.05`, `askLowerPct = 0.02`, `bidRaisePct = 0.02`, `affordabilityMargin = 0.8`.

**Semantics (implement exactly):** runs at the very end of the tick, reading the post-matching book.
- Every **ask**: if fully consumed (`Remaining <= RateEpsilon`) the seller raises that product's ask price by `askRaisePct`; otherwise lowers it by `askLowerPct`, floored at the seller's marginal cost (`MarginalUnitCost(upkeepPerTick)` for factories, `production.MinUnitPrice` for resources).
- Every unfilled factory **bid**: the buyer raises its bid price by `bidRaisePct` **if it can afford to** — its committed spend (active purchases + planned spend on all its other unfilled bids) plus the proposed spend on this bid must stay within `affordabilityMargin` of its achievable revenue (active sales revenue + remaining capacity valued at the best standing bid for each output). This affordability cap is the backward demand cascade: a factory can only bid up its inputs as far as demand for its own outputs supports.
- Sink bids never adjust (fixed prices). Iteration is over `book.Products()` (sorted) then order slices (posting order) — deterministic.

- [ ] **Step 1: Write the failing tests**

Create `state/prices_test.go`:

```go
package state

import (
	"math"
	"testing"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
	"github.com/paul-freeman/satisfactory-story/recipes"
	"github.com/paul-freeman/satisfactory-story/resources"
)

func Test_adjustPrices_ask_dynamics(t *testing.T) {
	t.Run("unsold ask is lowered toward the resource floor", func(t *testing.T) {
		ore := &resources.Resource{
			Production: production.Production{Name: "Ore", Rate: 100},
			Loc:        point.Point{X: 500, Y: 500},
		}
		s := newTestState(recipes.Recipes{}, []production.Producer{ore})
		s.publishOrders(testLogger()) // ask goes unmatched -- nobody bids

		s.adjustPrices(testLogger())

		want := production.DefaultUnitPrice * (1 - askLowerPct)
		if got := ore.AskPriceFor("Ore"); math.Abs(got-want) > 1e-9 {
			t.Errorf("expected lowered ask %f, got %f", want, got)
		}

		// Lowering never crosses the floor.
		ore.SetAskPrice("Ore", production.MinUnitPrice)
		s.publishOrders(testLogger())
		s.adjustPrices(testLogger())
		if got := ore.AskPriceFor("Ore"); got < production.MinUnitPrice {
			t.Errorf("ask fell below the floor: %f", got)
		}
	})

	t.Run("sold-out ask is raised", func(t *testing.T) {
		ore := &resources.Resource{
			Production: production.Production{Name: "Ore", Rate: 10},
			Loc:        point.Point{X: 500, Y: 500},
		}
		buyer := factory.New("Smelt", "Recipe_Smelt_C", point.Point{X: 501, Y: 501}, 0,
			production.Products{{Name: "Ore", Rate: 10}},
			production.Products{{Name: "Ingot", Rate: 5}}, 1000)
		buyer.SetBidPrice("Ore", 100.0)

		s := newTestState(recipes.Recipes{}, []production.Producer{ore, buyer})
		s.publishOrders(testLogger())
		s.matchOrders(testLogger()) // consumes the whole ask

		s.adjustPrices(testLogger())

		want := production.DefaultUnitPrice * (1 + askRaisePct)
		if got := ore.AskPriceFor("Ore"); math.Abs(got-want) > 1e-9 {
			t.Errorf("expected raised ask %f, got %f", want, got)
		}
	})

	t.Run("factory ask is floored at marginal cost", func(t *testing.T) {
		f := factory.New("Smelt", "Recipe_Smelt_C", point.Point{X: 0, Y: 0}, 0,
			production.Products{{Name: "Ore", Rate: 5}},
			production.Products{{Name: "Ingot", Rate: 4}}, 1000)
		f.Purchases = append(f.Purchases, &production.Contract{
			Order:         production.Production{Name: "Ore", Rate: 5},
			ProductCost:   10,
			TransportCost: 2,
		})
		// marginal cost = (12 + upkeep) / 4 = 3.125 > current default ask 1.0
		f.SetAskPrice("Ingot", 1.0)

		s := newTestState(recipes.Recipes{}, []production.Producer{f})
		s.publishOrders(testLogger()) // producing, unsold ask

		s.adjustPrices(testLogger())

		want := f.MarginalUnitCost(upkeepPerTick)
		if got := f.AskPriceFor("Ingot"); math.Abs(got-want) > 1e-9 {
			t.Errorf("expected ask floored at marginal cost %f, got %f", want, got)
		}
	})
}

func Test_adjustPrices_bid_dynamics(t *testing.T) {
	t.Run("unfilled bid escalates when downstream demand affords it", func(t *testing.T) {
		f := factory.New("Smelt", "Recipe_Smelt_C", point.Point{X: 0, Y: 0}, 0,
			production.Products{{Name: "Ore", Rate: 5}},
			production.Products{{Name: "Ingot", Rate: 5}}, 1000)
		rich := factory.New("Down", "Recipe_Down_C", point.Point{X: 1, Y: 1}, 0,
			production.Products{{Name: "Ingot", Rate: 5}},
			production.Products{{Name: "Plate", Rate: 5}}, 1000)

		s := newTestState(recipes.Recipes{}, []production.Producer{f, rich})
		rich.SetBidPrice("Ingot", 40.0) // huge downstream willingness
		s.publishOrders(testLogger())
		// No Ore seller exists: f's bid stays unfilled.

		s.adjustPrices(testLogger())

		want := production.DefaultUnitPrice * (1 + bidRaisePct)
		if got := f.BidPriceFor("Ore"); math.Abs(got-want) > 1e-9 {
			t.Errorf("expected escalated bid %f, got %f", want, got)
		}
	})

	t.Run("bid freezes at the affordability cap", func(t *testing.T) {
		f := factory.New("Smelt", "Recipe_Smelt_C", point.Point{X: 0, Y: 0}, 0,
			production.Products{{Name: "Ore", Rate: 5}},
			production.Products{{Name: "Ingot", Rate: 5}}, 1000)

		s := newTestState(recipes.Recipes{}, []production.Producer{f})
		// Nobody bids on Ingot: achievable revenue is only the salvage
		// floor (5 * 0.1 = 0.5), nowhere near the planned spend of an
		// escalated 3.0-and-up bid on 5 units of Ore.
		f.SetBidPrice("Ore", 3.0)
		s.publishOrders(testLogger())

		s.adjustPrices(testLogger())

		if got := f.BidPriceFor("Ore"); got != 3.0 {
			t.Errorf("bid should freeze without downstream revenue to support it, got %f", got)
		}
	})

	t.Run("sink bids never adjust", func(t *testing.T) {
		rs := recipes.Recipes{
			{DisplayName: "A", OutputProducts: production.Products{{Name: "SpaceElevatorPart_1", Rate: 1}}},
		}
		sinks := newSinks(rs, 0, 1000, 0, 1000)
		producers := make([]production.Producer, 0, len(sinks))
		for _, sk := range sinks {
			producers = append(producers, sk)
		}
		s := newTestState(rs, producers)
		s.publishOrders(testLogger())

		s.adjustPrices(testLogger())

		if sinks[0].BidUnitPrice != goalBidUnitPrice {
			t.Errorf("sink bid must stay fixed at %f, got %f", goalBidUnitPrice, sinks[0].BidUnitPrice)
		}
	})
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./state -run Test_adjustPrices -v`
Expected: FAIL — undefined `adjustPrices`, `askLowerPct`.

- [ ] **Step 3: Write the implementation**

Create `state/prices.go`:

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
// selling out, and askLowerPct how much it lowers after failing to sell.
// Raising faster than lowering lets prices spike quickly under scarcity
// and relax slowly, which damps oscillation between the two states.
const askRaisePct = 0.05
const askLowerPct = 0.02

// bidRaisePct is how much a buyer escalates an unfilled input bid per
// tick. The escalating bid is the backward demand cascade -- it is
// deliberately gentle so a factory's seed capital comfortably outlasts
// the climb to a market-clearing price.
const bidRaisePct = 0.02

// affordabilityMargin caps a factory's total planned input spend at this
// fraction of its achievable revenue, so escalation stops where the
// business stops making sense rather than at bankruptcy.
const affordabilityMargin = 0.8

// adjustPrices runs after matching and lets every producer react locally
// to this tick's fill outcomes. No market-wide price is computed
// anywhere: sellers with unsold stock undercut, sold-out sellers raise,
// hungry buyers bid up as far as their own downstream demand supports.
func (s *State) adjustPrices(_ *slog.Logger) {
	for _, product := range s.book.Products() {
		for _, ask := range s.book.Asks(product) {
			soldOut := ask.Remaining <= production.RateEpsilon
			switch seller := ask.Seller.(type) {
			case *factory.Factory:
				if soldOut {
					seller.SetAskPrice(product, seller.AskPriceFor(product)*(1+askRaisePct))
				} else {
					floor := seller.MarginalUnitCost(upkeepPerTick)
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
			proposed := buyer.BidPriceFor(product) * (1 + bidRaisePct)
			if s.plannedSpend(buyer, product, proposed) <= s.achievableRevenue(buyer)*affordabilityMargin {
				buyer.SetBidPrice(product, proposed)
			}
		}
	}
}

// achievableRevenue is what the factory's output is worth per tick right
// now: revenue from active sales plus unsold capacity valued at the best
// standing bid for each output (never below the salvage floor).
// Downstream bids raising this number is exactly how demand cascades
// backward through the tiers.
func (s *State) achievableRevenue(f *factory.Factory) float64 {
	revenue := 0.0
	for _, sale := range f.Sales {
		if !sale.Cancelled {
			revenue += sale.ProductCost - sale.TransportCost
		}
	}
	for _, output := range f.Output {
		remaining := f.RemainingCapacityFor(output.Name)
		if remaining <= production.RateEpsilon {
			continue
		}
		price := floorUnitPrice
		if bid, ok := s.book.BestBid(output.Name); ok && bid.UnitPrice > price {
			price = bid.UnitPrice
		}
		revenue += price * remaining
	}
	return revenue
}

// plannedSpend is the factory's per-tick input commitment if the bid for
// product moved to proposedPrice: actual active purchases plus every
// unmet input valued at its standing (or proposed) bid price.
func (s *State) plannedSpend(f *factory.Factory, product string, proposedPrice float64) float64 {
	spend := 0.0
	for _, purchase := range f.Purchases {
		if !purchase.Cancelled {
			spend += purchase.ProductCost + purchase.TransportCost
		}
	}
	for _, input := range f.Input {
		unmet := f.UnmetInputRate(input.Name)
		if unmet <= production.RateEpsilon {
			continue
		}
		price := f.BidPriceFor(input.Name)
		if input.Name == product {
			price = proposedPrice
		}
		spend += price * unmet
	}
	return spend
}
```

- [ ] **Step 4: Wire into `Tick`**

In `state/state.go`, append to the mechanism block of `Tick` (after `s.applySolvency(l)`):

```go
	s.adjustPrices(l)
```

- [ ] **Step 5: Run the full suite**

Run: `go build ./... && go test ./...`
Expected: PASS. The economy is now feature-complete; only wire format, integration tests, and the milestone remain.

- [ ] **Step 6: Commit**

```bash
git add state/
git commit -m "feat: local price adjustment -- asks decay unsold, bids escalate within affordability"
```

---

### Task 10: Shortage prices on the wire and in the UI

**Files:**
- Modify: `state/http/http.go` (`Shortage` type), `state/state.go` (`shortagesForWire`), `state/state_test.go` (`Test_toHTTP_wire_additions`), `frontend/src/types.ts`, `frontend/src/components/ShortagePanel.tsx`

**Interfaces:**
- Consumes: `shortagesForWire` (Task 7).
- Produces: `statehttp.Shortage{Product string; Amount, Price float64}` with JSON tag `price`; frontend `Shortage` gains `price: number`.

- [ ] **Step 1: Extend the wire test**

In `state/state_test.go` `Test_toHTTP_wire_additions`, after the existing shortage assertions, add:

```go
	assert.Equal(t, 5.0, wire.Shortages[0].Price, "shortage should carry the best bid price")
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./state -run Test_toHTTP_wire_additions -v`
Expected: FAIL — `wire.Shortages[0].Price` undefined.

- [ ] **Step 3: Implement backend**

In `state/http/http.go`:

```go
type Shortage struct {
	Product string  `json:"product"`
	Amount  float64 `json:"amount"`
	Price   float64 `json:"price"`
}
```

In `state/state.go` `shortagesForWire`, track the best price alongside totals — replace the aggregation loop body and slice build:

```go
	totals := make(map[string]float64)
	prices := make(map[string]float64)
	for _, product := range s.book.Products() {
		for _, bid := range s.book.Bids(product) {
			if bid.Remaining <= production.RateEpsilon {
				continue
			}
			totals[product] += bid.Remaining
			if bid.UnitPrice > prices[product] {
				prices[product] = bid.UnitPrice
			}
		}
	}
	shortages := make([]statehttp.Shortage, 0, len(totals))
	for product, amount := range totals {
		shortages = append(shortages, statehttp.Shortage{
			Product: product,
			Amount:  amount,
			Price:   prices[product],
		})
	}
```

(The sort + cap below stays unchanged.)

- [ ] **Step 4: Run backend tests**

Run: `go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 5: Implement frontend**

`frontend/src/types.ts`:

```ts
export interface Shortage {
  product: string;
  amount: number;
  price: number;
}
```

`frontend/src/components/ShortagePanel.tsx` — show the bid price next to the amount; replace the mapped row:

```tsx
          {shortages.map((s) => (
            <div key={s.product} style={{ ...itemStyle, fontSize: 11, display: 'flex', justifyContent: 'space-between', gap: 8 }}>
              <span>{s.product}</span>
              <span>
                {s.amount.toFixed(1)} @ {s.price.toFixed(2)}
              </span>
            </div>
          ))}
```

- [ ] **Step 6: Verify the frontend builds**

Run: `cd frontend && npm run build`
Expected: `tsc` and Vite both succeed.

- [ ] **Step 7: Commit**

```bash
git add state/ frontend/src/
git commit -m "feat: expose best bid price per shortage on the wire and in the panel"
```

---

### Task 11: Cascade integration tests

**Files:**
- Create: `state/cascade_test.go`

**Interfaces:**
- Consumes: the whole tick loop (Tasks 5–9), `goalBidUnitPrice`, `newTestState`.
- Produces: the key emergent property as regression tests — *a standing bid for X causes a producer chain for X to self-assemble*.

These tests run the real `Tick` (all mechanisms) on tiny synthetic worlds. They are the stability canary for the price dynamics: if bid escalation vs. ask decay is mistuned, these fail before the milestone does.

- [ ] **Step 1: Write the tests**

Create `state/cascade_test.go`:

```go
package state

import (
	"testing"

	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
	"github.com/paul-freeman/satisfactory-story/recipes"
	"github.com/paul-freeman/satisfactory-story/resources"
	"github.com/paul-freeman/satisfactory-story/sink"
)

// deliveredTo reports whether the sink holds an active purchase.
func deliveredTo(sk *sink.Sink) bool {
	for _, purchase := range sk.Purchases {
		if !purchase.Cancelled && purchase.Order.Rate > 0 {
			return true
		}
	}
	return false
}

// Test_cascade_single_tier: a goal sink bids for Ingot; only an Ore node
// exists. A smelter must spawn, source Ore through the book, produce,
// and deliver to the sink -- demand becomes supply with no tree-reading.
func Test_cascade_single_tier(t *testing.T) {
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
	goal := sink.New("Ingot", point.Point{X: 600, Y: 600},
		production.Products{{Name: "Ingot", Rate: 1}}, goalBidUnitPrice)

	s := newTestState(rs, []production.Producer{ore, goal})

	const budget = 20000
	for i := 0; i < budget && !deliveredTo(goal); i++ {
		if err := s.Tick(testLogger()); err != nil {
			t.Fatalf("tick %d failed: %v", i, err)
		}
	}

	if !deliveredTo(goal) {
		t.Fatalf("no Ingot delivered to the goal sink within %d ticks", budget)
	}
}

// Test_cascade_two_tier: the goal sink bids for Plate, which needs
// Ingot, which needs Ore. The Plate factory must spawn idle, its
// escalating Ingot bid must make smelting look profitable, a smelter
// must spawn and connect to Ore, and the full chain must flow.
func Test_cascade_two_tier(t *testing.T) {
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
		{
			ClassName:      "Recipe_Plate_C",
			DisplayName:    "Roll Plate",
			Active:         true,
			InputProducts:  production.Products{{Name: "Ingot", Rate: 3}},
			OutputProducts: production.Products{{Name: "Plate", Rate: 2}},
		},
	}
	goal := sink.New("Plate", point.Point{X: 600, Y: 600},
		production.Products{{Name: "Plate", Rate: 1}}, goalBidUnitPrice)

	s := newTestState(rs, []production.Producer{ore, goal})

	const budget = 50000
	for i := 0; i < budget && !deliveredTo(goal); i++ {
		if err := s.Tick(testLogger()); err != nil {
			t.Fatalf("tick %d failed: %v", i, err)
		}
	}

	if !deliveredTo(goal) {
		t.Fatalf("no Plate delivered to the goal sink within %d ticks", budget)
	}
}
```

- [ ] **Step 2: Run the cascade tests**

Run: `go test ./state -run Test_cascade -v -timeout 300s`
Expected: PASS. **If either fails, this is the tuning surface the spec warned about — do not brute-force.** Diagnose which link broke by logging every 1000 ticks: how many factories exist, whether they are producing, the Ingot/Plate bid prices, the ask prices. The likely knobs, in order of suspicion: `bidRaisePct` too low to cross ask+transport within the seed-capital window (try `0.05`); `seedCapitalBufferTicks` too small for the two-tier wait (try `1000`); `askLowerPct` too slow for asks initialized above the crossing point. Change one constant at a time, note the change and the observed effect in the commit message, and keep the cascade tests as the arbiter.

- [ ] **Step 3: Run the full suite**

Run: `go build ./... && go test ./... -timeout 600s`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add state/cascade_test.go
git commit -m "test: cascade tests -- a standing bid self-assembles its supply chain"
```

---

### Task 12: Milestone test, long-run re-enable, docs

**Files:**
- Modify: `state/state_test.go` (rewrite the long-run test), `CLAUDE.md` (architecture section)

**Interfaces:**
- Consumes: everything.
- Produces: the spec's success bar as a test — a full-recipe-set run delivering a `SpaceElevatorPart_*` to a goal sink.

- [ ] **Step 1: Rewrite the long-run test**

In `state/state_test.go`, replace the entire `t.Run("converges on real production over a long run", ...)` subtest (including its `t.Skip` from Task 5) with:

```go
	t.Run("long run: real trade, niches, and a space-elevator delivery", func(t *testing.T) {
		if testing.Short() {
			t.Skip("long-run milestone test")
		}
		l := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level:       slog.LevelError,
			ReplaceAttr: removeTimeAndLevel,
		}))
		seed := int64(152)

		logLevel := new(slog.Level)
		testState, err := New(l, logLevel, seed)
		assert.NoError(t, err, "failed to create state")

		partDelivered := func() bool {
			for _, p := range testState.producers {
				sk, ok := p.(*sink.Sink)
				if !ok || !strings.HasPrefix(sk.Name, spaceElevatorPartPrefix) {
					continue
				}
				for _, purchase := range sk.Purchases {
					if !purchase.Cancelled && purchase.Order.Rate > 0 {
						return true
					}
				}
			}
			return false
		}

		delivered := false
		for i := 0; i < longRunTickCount && !delivered; i++ {
			err = testState.Tick(l)
			assert.NoError(t, err, "failed to tick state")
			if i%100 == 99 {
				delivered = partDelivered()
			}
		}

		// No producer may ever be oversold. (Checked directly against
		// Sales/output rate, not via RemainingCapacityFor -- that method
		// clamps at 0, which would make this assertion pass trivially
		// even if oversell had occurred.)
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

		// Real factory-to-factory or factory-to-sink trade must exist.
		factorySaleFound := false
		for _, p := range testState.producers {
			f, ok := p.(*factory.Factory)
			if !ok {
				continue
			}
			for _, sale := range f.Sales {
				if !sale.Cancelled && sale.Order.Rate > 0 {
					factorySaleFound = true
				}
			}
		}
		assert.True(t, factorySaleFound, "expected at least one factory with an active sale")

		// At least one product should have multiple coexisting producers
		// -- a niche, not a monopoly.
		producersByProduct := make(map[string]int)
		for _, p := range testState.producers {
			f, ok := p.(*factory.Factory)
			if !ok {
				continue
			}
			for _, product := range f.Products() {
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
		assert.True(t, foundNiche, "expected at least one product with multiple coexisting producers")

		// THE MILESTONE (spec success bar): the economy self-assembles a
		// full multi-tier chain and delivers a space-elevator part.
		assert.True(t, delivered,
			"expected a SpaceElevatorPart_* delivery to a goal sink within %d ticks", longRunTickCount)
	})
```

Add `"github.com/paul-freeman/satisfactory-story/sink"` to the imports of `state/state_test.go`.

- [ ] **Step 2: Run the milestone**

Run: `go test ./state -run Test_state_Tick -v -timeout 1800s`

**Tuning protocol if the milestone assertion fails** (the cascade tests passing means the mechanism works; this is about scale — ~139 recipes competing, 4–5 tiers needed):

1. Instrument first, don't guess: log every 5000 ticks — count of factories by producing/idle, the top-5 shortage products with bid prices, and the deepest tier reached (does any factory make `SpaceElevatorPart_*` inputs?).
2. Then adjust **one constant per attempt**, in this order, re-running each time:
   - `spawnProbabilityPerTick` `0.05 → 0.2` (more draws; the profit weighting concentrates them where money is)
   - `bidRaisePct` `0.02 → 0.05` (faster cascade climb)
   - `seedCapitalBufferTicks` `300 → 1000` **and** `insolvencyGrace` `300 → 1000` (rare high-tier factories wait longer)
   - `goalBidUnitPrice` `1000 → 10000` (steeper profit gradient toward the goal)
3. Document every attempt and its observed effect in the commit message.
4. **If it still fails after these four attempts, STOP.** Change the milestone assertion to record the deepest tier reached instead of skipping silently, mark it with `t.Skip("milestone not yet reached: <observed status>")`, commit, and report the findings to the user for a decision — per the user's standing preference, tuning forks get surfaced with a recommendation, not guessed at.

- [ ] **Step 3: Full suite, race check**

Run: `go build ./... && go test ./... -timeout 1800s && go test ./state -run 'Test_state_Tick/can_run' -race`
Expected: PASS.

- [ ] **Step 4: Update CLAUDE.md**

In `CLAUDE.md`, replace the "Simulation core (`state/state.go`)" section's phase description (the paragraph starting "The simulation advances in **phases**...") with:

```markdown
Each `Tick` runs the full mechanism pipeline: `publishOrders` rebuilds the order book (`market.Book`) from live producer state (unsold capacity → asks, unmet inputs → bids, sinks → standing bids); `matchOrders` crosses it into `production.Contract`s; `moveProducers` hill-climbs on transport cost; `spawnNewProducer` (probability-gated) picks a recipe by expected profit against the book and spawns it *idle* — no sourcing at spawn; `renegotiateContracts` (probability-gated) re-shops existing contracts against residual asks; `applySolvency` charges upkeep, credits salvage (unsold producing capacity earns `floorUnitPrice` per unit — the AWESOME-sink buyer of last resort), cancels sales of non-producing factories, and removes the persistently insolvent; `adjustPrices` lets sellers/buyers react locally (unsold asks decay toward marginal cost, unfilled bids escalate within an affordability cap). Demand cascades backward through recipe tiers as escalating bids — see `docs/superpowers/specs/2026-07-11-order-book-market-design.md`.
```

Also in CLAUDE.md: in the "Producers and contracts" section, update the `State.writeContract` sentence to reference `signContract` (matching determines the price; capacity is re-checked at signing).

- [ ] **Step 5: Commit**

```bash
git add state/ CLAUDE.md
git commit -m "test: long-run milestone -- self-assembled chain delivers a space-elevator part"
```

---

## Execution notes

- Tasks 1–4 are independent of the tick loop and safe in any order (1 before 2; 3 and 4 any time after 1).
- Tasks 5–9 must run in order; each leaves `go test ./...` green.
- Task 11's cascade tests are the fast feedback loop for price-dynamic tuning; run them before and after touching any constant in Task 12.
- Final report to the user must include: the two spec refinements (idle factories pay for signed contracts; floor buyer implemented as salvage revenue — see Interim-state notes), any constants changed during tuning and why, and the milestone outcome.





