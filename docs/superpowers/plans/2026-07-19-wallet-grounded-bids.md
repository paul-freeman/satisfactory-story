# Wallet-Grounded Bids + Water Nodes (Phase 5) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Cap every factory's standing bid price at what its wallet can actually pay (`Cash / Hunger`), add 8 synthetic Water resource nodes, and harden the long-run milestone test into a hard sustained-delivery assertion — per `docs/superpowers/specs/2026-07-16-wallet-grounded-bids-design.md`.

**Architecture:** One mechanism change in `state/prices.go` (`adjustPrices` becomes escalate-then-clamp for factory bids), one data change in `resources/Resource.json` + `resources/resources.go` (Water nodes), one test change in `state/state_test.go` (milestone becomes a hard assertion with a 20k-tick sustained-delivery window). `state/spawn.go` is untouched by design.

**Tech Stack:** Go 1.x, standard `testing` package (`state` tests also use `stretchr/testify/assert`).

## Global Constraints

- **`state/spawn.go` must not be modified.** The whole point of the cap is that every book price becomes money-backed, so the spawn estimator keeps trusting the book with zero special cases.
- **Sink bids stay fixed** at `goalBidUnitPrice` (1000). `adjustPrices` already skips non-`*factory.Factory` buyers — that skip must survive.
- **No frontend or wire-format changes.** Nothing under `frontend/` or `state/http/` changes.
- **Determinism:** the simulation must stay deterministic for a given seed. No new iteration over Go maps may influence simulation state. `Test_state_determinism` must pass.
- **Cap invariant (spec, verbatim):** `BidPrice(product) <= Cash() / Hunger(product, inputStockTargetTicks)`, enforced only in `adjustPrices`, applying **even when it lowers the price** below the current one.
- **Water nodes (spec, verbatim):** 8 entries with `"id": "waterPure"` (pure ⇒ 120/min ⇒ rate 2.0 units/tick) at exactly these (lat, lng) pairs: (−105.91, 52.68), (−105.91, 82.03), (−105.91, 111.39), (−80.13, 52.68), (−80.13, 111.39), (−54.36, 52.68), (−54.36, 82.03), (−54.36, 111.39).
- **Milestone (spec, verbatim):** first `SpaceElevatorPart_1` delivery within 100k ticks at seed 152, then ≥ 5 more units delivered within the following 20k ticks. Hard assertion — no `t.Skipf`. If the milestone run fails: **stop and report diagnostics to the user; do not tune constants.**
- Commit messages end with `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`.

---

### Task 1: Wallet-grounded bid cap in `adjustPrices`

**Files:**
- Modify: `state/prices.go:18-23` (the `bidRaisePct` comment) and `state/prices.go:50-59` (the bid loop)
- Modify: `CLAUDE.md` (one sentence describing `adjustPrices` in the tick pipeline)
- Test: `state/prices_test.go` (append three tests)

**Interfaces:**
- Consumes (all already exist, do not change them): `(*factory.Factory).Cash() float64` (via embedded `production.Wallet`), `(*factory.Factory).Hunger(name string, targetTicks float64) float64`, `(*factory.Factory).BidPriceFor(name string) float64`, `(*factory.Factory).SetBidPrice(name string, price float64)`, constants `inputStockTargetTicks = 60.0` (`state/produce.go`), `bidRaisePct = 0.02`, `production.RateEpsilon = 1e-9`.
- Produces: no new exported API. Later tasks rely only on the behavior change.

**Background for the implementer:** `adjustPrices` runs once per tick, last in the tick pipeline. Its bid loop escalates every unfilled factory bid by 2% forever, with no upper bound — that unbounded escalation is the Phase 4 bug this task fixes. The fix: after computing the escalated price, clamp it to `Cash / Hunger` — the most per unit the wallet could pay for the full quantity the factory wants. One subtlety: when `Hunger` is ~0 (a bid that got mostly filled this tick), the quotient is `+Inf` (or `NaN` if `Cash` is also 0 — and `math.Min(x, NaN)` returns `NaN` in Go, which would poison the price). So the clamp is guarded: only applied when hunger is above epsilon. Skipping the clamp at zero hunger is the spec's intended semantics (no cap), expressed without the NaN hazard.

- [ ] **Step 1: Write the three failing tests**

Append to `state/prices_test.go` (the file already imports `factory`, `point`, `production`; `newTestState()` and `testLogger()` are defined in `state/orders_test.go`):

```go
func Test_adjustPrices_bidCappedByWallet(t *testing.T) {
	s := newTestState()
	f := factory.New("Plates", "Recipe_Plates_C", point.Point{X: 0, Y: 0}, 0,
		production.Products{production.Production{Name: "IronIngot", Rate: 1}},
		production.Products{production.Production{Name: "IronPlate", Rate: 2}},
		100)
	s.producers = []production.Producer{f}
	// Hunger = rate 1 * inputStockTargetTicks 60 - stock 0 = 60.
	// Cap = Cash/Hunger = 100/60.
	cap := 100.0 / 60.0

	// Escalation would overshoot the cap: the bid lands exactly on it.
	f.SetBidPrice("IronIngot", 1.65) // 1.65 * 1.02 = 1.683 > cap
	s.book.Clear()
	s.book.PostBid(f, "IronIngot", 5, 1.65)
	s.adjustPrices(testLogger())
	if got := f.BidPriceFor("IronIngot"); got != cap {
		t.Fatalf("over-cap escalated bid = %v, want exactly cap %v", got, cap)
	}
}

func Test_adjustPrices_bidPulledDownToWalletCap(t *testing.T) {
	// A bid already far above the cap (e.g. the wallet drained since the
	// price was set) is pulled DOWN to the cap, not just stopped from
	// rising: dying demand fades honestly.
	s := newTestState()
	f := factory.New("Plates", "Recipe_Plates_C", point.Point{X: 0, Y: 0}, 0,
		production.Products{production.Production{Name: "IronIngot", Rate: 1}},
		production.Products{production.Production{Name: "IronPlate", Rate: 2}},
		100)
	s.producers = []production.Producer{f}
	cap := 100.0 / 60.0

	f.SetBidPrice("IronIngot", 5.0) // way above cap
	s.book.Clear()
	s.book.PostBid(f, "IronIngot", 5, 5.0)
	s.adjustPrices(testLogger())
	if got := f.BidPriceFor("IronIngot"); got != cap {
		t.Fatalf("above-cap bid = %v, want pulled down to cap %v", got, cap)
	}
}

func Test_adjustPrices_zeroHungerBidEscalatesUncapped(t *testing.T) {
	// A partially-filled bid can still be in the book when input stock is
	// already at target (hunger ~0). The cap quotient would be Cash/0:
	// the clamp must be skipped entirely (spec: no cap in this case), and
	// must not produce NaN even with an empty wallet.
	s := newTestState()
	f := factory.New("Plates", "Recipe_Plates_C", point.Point{X: 0, Y: 0}, 0,
		production.Products{production.Production{Name: "IronIngot", Rate: 1}},
		production.Products{production.Production{Name: "IronPlate", Rate: 2}},
		0) // empty wallet: Cash/Hunger would be 0/0 = NaN
	s.producers = []production.Producer{f}
	f.InputStock["IronIngot"] = 60 // rate 1 * target 60 => hunger 0
	f.SetBidPrice("IronIngot", 1.0)
	s.book.Clear()
	s.book.PostBid(f, "IronIngot", 5, 1.0)
	s.adjustPrices(testLogger())
	if got := f.BidPriceFor("IronIngot"); got != 1.0*(1+bidRaisePct) {
		t.Fatalf("zero-hunger bid = %v, want uncapped escalation %v", got, 1.0*(1+bidRaisePct))
	}
}
```

- [ ] **Step 2: Run the new tests to verify they fail**

Run: `go test ./state -run 'Test_adjustPrices_bidCappedByWallet|Test_adjustPrices_bidPulledDownToWalletCap|Test_adjustPrices_zeroHungerBidEscalatesUncapped' -v`

Expected: `Test_adjustPrices_bidCappedByWallet` and `Test_adjustPrices_bidPulledDownToWalletCap` FAIL (bid escalates past / stays above the cap); `Test_adjustPrices_zeroHungerBidEscalatesUncapped` PASSES (current code has no cap at all — that is fine, it is a regression guard for the clamp you are about to add).

- [ ] **Step 3: Implement the clamp**

In `state/prices.go`, replace the bid loop body (currently lines 50–59):

```go
		for _, bid := range s.book.Bids(product) {
			if bid.Remaining <= production.RateEpsilon {
				continue
			}
			buyer, ok := bid.Buyer.(*factory.Factory)
			if !ok {
				continue // sink bids are fixed
			}
			escalated := buyer.BidPriceFor(product) * (1 + bidRaisePct)
			// Wallet-grounded cap: a standing bid never promises more per
			// unit than the wallet could pay for the full hunger. It
			// applies even when it lowers the current price, so dying
			// demand fades honestly instead of screaming louder. Near-zero
			// hunger (a bid that just got mostly filled) means no cap this
			// tick -- guarded explicitly so Cash/0 can't inject Inf or NaN.
			if hunger := buyer.Hunger(product, inputStockTargetTicks); hunger > production.RateEpsilon {
				escalated = math.Min(escalated, buyer.Cash()/hunger)
			}
			buyer.SetBidPrice(product, escalated)
		}
```

Also replace the now-stale `bidRaisePct` comment (currently lines 19–23):

```go
// bidRaisePct is how much a buyer escalates an unfilled input bid per
// tick. The escalating bid is the backward demand cascade. Escalation is
// clamped in adjustPrices to Cash/Hunger -- the wallet-grounded cap --
// so every posted price is backed by money the buyer actually has and
// dead-end demand can never compound into absurd book prices.
const bidRaisePct = 0.02
```

- [ ] **Step 4: Run the new tests to verify they pass**

Run: `go test ./state -run 'Test_adjustPrices_bidCappedByWallet|Test_adjustPrices_bidPulledDownToWalletCap|Test_adjustPrices_zeroHungerBidEscalatesUncapped' -v`

Expected: all three PASS.

- [ ] **Step 5: Run the whole `state` package**

Run: `go test ./state`

Expected: PASS. In particular `Test_adjustPrices_bidRaisesWhileHungry` must pass unchanged (its bid escalates 1.0 → 1.02, well under its cap of 100/60 ≈ 1.667 — the cap must not alter already-affordable bids), along with the cascade tests in `state/cascade_test.go` and `Test_state_determinism`.

- [ ] **Step 6: Update the stale CLAUDE.md sentence**

In `CLAUDE.md`, the tick-pipeline item 7 currently reads:

> 7. `adjustPrices` lets sellers/buyers react locally to this tick's fill outcome: unsold asks decay toward marginal cost, sold-out asks rise; unfilled bids escalate (uncapped — the trade-time budget clamp, not a price ceiling, is what keeps speculative bidding harmless at the point of trade). Demand cascades backward through recipe tiers this way, one price signal at a time, with no global graph traversal.

Replace the parenthetical so the item reads:

> 7. `adjustPrices` lets sellers/buyers react locally to this tick's fill outcome: unsold asks decay toward marginal cost, sold-out asks rise; unfilled bids escalate, clamped to the wallet-grounded cap `Cash/Hunger` (see `docs/superpowers/specs/2026-07-16-wallet-grounded-bids-design.md`) so every posted price is backed by real money. Demand cascades backward through recipe tiers this way, one price signal at a time, with no global graph traversal.

- [ ] **Step 7: Commit**

```bash
git add state/prices.go state/prices_test.go CLAUDE.md
git commit -m "feat: clamp bid escalation to the wallet-grounded cap Cash/Hunger

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: Water resource nodes

**Files:**
- Modify: `resources/Resource.json` (append 8 entries at the end of the array)
- Modify: `resources/resources.go:128-153` (`toCanonicalName`)
- Test: `resources/resources_test.go` (append one test)

**Interfaces:**
- Consumes: `resources.New() ([]*Resource, error)`, `Resource.Production` (`production.Production{Name, Rate}`), `Resource.Purity`, `Resource.Loc` (`point.Point`), package-private constant `pure`.
- Produces: after this task, `resources.New()` returns 441 resources, 8 of them producing `"Water"` at rate 2.0/tick. Nothing else in the codebase needs changing — `state.getInitialState` picks up all loaded resources automatically, and the new nodes sit inside the existing bounding box so world bounds are unchanged.

**Background for the implementer:** `Resource.json` is a JSON array of `{"id", "lat", "lng"}` entries embedded at compile time. The `id` is a resource name with a purity suffix; `Pure` maps to 120 units/60s = rate 2.0. The parsed name (`water`) must map to the canonical in-game product name (`Water`) via `toCanonicalName`, or recipes demanding `Water` will never match. Locations are scaled `x = int(lng*1000)`, `y = int(lat*1000)`.

- [ ] **Step 1: Write the failing test**

Append to `resources/resources_test.go` (the file already imports `point` and `production`):

```go
func Test_Resource_water_nodes_load(t *testing.T) {
	rs, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	// The 8 synthetic water nodes from the Phase 5 spec
	// (docs/superpowers/specs/2026-07-16-wallet-grounded-bids-design.md):
	// a 3x3 interior grid of the resource bounding box minus its center.
	// Expected locations use the same lat/lng scaling as New().
	expected := make(map[point.Point]bool)
	for _, c := range []struct{ lat, lng float64 }{
		{-105.91, 52.68}, {-105.91, 82.03}, {-105.91, 111.39},
		{-80.13, 52.68}, {-80.13, 111.39},
		{-54.36, 52.68}, {-54.36, 82.03}, {-54.36, 111.39},
	} {
		expected[point.Point{X: int(c.lng * 1000), Y: int(c.lat * 1000)}] = true
	}

	found := 0
	for _, r := range rs {
		if r.Production.Name != "Water" {
			continue
		}
		found++
		if r.Purity != pure {
			t.Errorf("water node at %s: purity = %v, want %v", r.Loc.String(), r.Purity, pure)
		}
		if r.Production.Rate != 120.0/60.0 {
			t.Errorf("water node at %s: rate = %v, want %v", r.Loc.String(), r.Production.Rate, 120.0/60.0)
		}
		if !expected[r.Loc] {
			t.Errorf("unexpected water node location %s", r.Loc.String())
		}
		delete(expected, r.Loc)
	}
	if found != 8 {
		t.Errorf("found %d water nodes, want 8", found)
	}
	for loc := range expected {
		t.Errorf("missing water node at %s", loc.String())
	}
}
```

(The final loop over the `expected` map only emits error messages — it never influences simulation state, so map-iteration order is irrelevant.)

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./resources -run Test_Resource_water_nodes_load -v`

Expected: FAIL with `found 0 water nodes, want 8` plus 8 `missing water node` errors.

- [ ] **Step 3: Append the 8 water entries to `Resource.json`**

The file currently ends with:

```json
    {
        "id": "geyserPure",
        "lat": -117.31644533333338,
        "lng": 87.61684525592334
    }
]
```

Replace that ending with:

```json
    {
        "id": "geyserPure",
        "lat": -117.31644533333338,
        "lng": 87.61684525592334
    },
    {
        "id": "waterPure",
        "lat": -105.91,
        "lng": 52.68
    },
    {
        "id": "waterPure",
        "lat": -105.91,
        "lng": 82.03
    },
    {
        "id": "waterPure",
        "lat": -105.91,
        "lng": 111.39
    },
    {
        "id": "waterPure",
        "lat": -80.13,
        "lng": 52.68
    },
    {
        "id": "waterPure",
        "lat": -80.13,
        "lng": 111.39
    },
    {
        "id": "waterPure",
        "lat": -54.36,
        "lng": 52.68
    },
    {
        "id": "waterPure",
        "lat": -54.36,
        "lng": 82.03
    },
    {
        "id": "waterPure",
        "lat": -54.36,
        "lng": 111.39
    }
]
```

- [ ] **Step 4: Add the canonical-name mapping**

In `resources/resources.go`, `toCanonicalName`, add one case alongside the existing ones (e.g. directly after the `"uranium"` case):

```go
	case "water":
		return "Water"
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./resources -run Test_Resource_water_nodes_load -v`

Expected: PASS.

- [ ] **Step 6: Run the full suite**

Run: `go test ./...`

Expected: PASS everywhere. (`state` tests construct the world from the same embedded JSON; adding interior nodes changes no bounds and breaks no assertion — `Test_state_determinism` compares two fresh runs of the *same* code, so both see the water nodes.)

- [ ] **Step 7: Commit**

```bash
git add resources/Resource.json resources/resources.go resources/resources_test.go
git commit -m "feat: add 8 synthetic pure Water resource nodes

Water has no producible source in the SCIM export (extractors sit on
open water, not nodes). Grid placement per the Phase 5 spec.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: Harden the milestone test into a sustained-delivery assertion

**Files:**
- Modify: `state/state_test.go:18-21` (constants) and `state/state_test.go:105-244` (the long-run subtest)

**Interfaces:**
- Consumes: `sink.Sink.Name`, `(*sink.Sink).TotalDelivered() float64`, `spaceElevatorPartPrefix = "SpaceElevatorPart_"` (`state/sinks.go`), existing test constant `longRunTickCount = 100000`.
- Produces: test constants `sustainedWindowTicks = 20000` and `sustainedMinUnits = 5.0`, used only inside this test file.

**Background for the implementer:** the long-run subtest currently `t.Skipf`s when no part is delivered (a Phase 4 concession). Phase 5 makes it the acceptance gate: no delivery → `t.Fatalf` with the same diagnostics; and after the first delivery the sim must keep delivering (≥ 5 more units in the next 20k ticks). Delivery is polled every 100 ticks (a `TotalDelivered` scan is O(producers)); the sustained window starts at the poll that first sees a delivery, which is at most 99 ticks after the actual first delivery — an acceptable, deterministic approximation the spec's window absorbs.

- [ ] **Step 1: Add the constants**

In `state/state_test.go`, directly below `const longRunTickCount = 100000` (line 21), add:

```go
// sustainedWindowTicks and sustainedMinUnits define the Phase 5
// sustained-delivery bar (docs/superpowers/specs/
// 2026-07-16-wallet-grounded-bids-design.md): after the first
// space-elevator-part delivery, at least sustainedMinUnits more units
// must arrive within the next sustainedWindowTicks ticks.
const sustainedWindowTicks = 20000
const sustainedMinUnits = 5.0
```

- [ ] **Step 2: Rewrite the long-run subtest**

Replace the entire subtest body — from `t.Run("long run: real trade, niches, and a space-elevator delivery", func(t *testing.T) {` (line 105) through its closing `})` (line 244) — with:

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

		totalPartsDelivered := func() float64 {
			total := 0.0
			for _, p := range testState.producers {
				sk, ok := p.(*sink.Sink)
				if !ok || !strings.HasPrefix(sk.Name, spaceElevatorPartPrefix) {
					continue
				}
				total += sk.TotalDelivered()
			}
			return total
		}

		// everProduced and maxProducing are observability-only (they do
		// not affect simulation behavior): they let a milestone failure
		// report how far the economy actually got.
		everProduced := make(map[string]bool)
		maxProducing := 0

		delivered := false
		firstDeliveryTick := 0
		for i := 0; i < longRunTickCount && !delivered; i++ {
			err = testState.Tick(l)
			assert.NoError(t, err, "failed to tick state")
			producing := 0
			for _, p := range testState.producers {
				f, ok := p.(*factory.Factory)
				if !ok || !f.ProducedLastTick {
					continue
				}
				producing++
				for _, output := range f.Output {
					everProduced[output.Name] = true
				}
			}
			if producing > maxProducing {
				maxProducing = producing
			}
			if i%100 == 99 && totalPartsDelivered() > 0 {
				delivered = true
				firstDeliveryTick = i + 1
			}
		}

		// THE MILESTONE, part 1 (Phase 5 spec, hard assertion): a
		// space-elevator part must be delivered within longRunTickCount.
		if !delivered {
			t.Fatalf("milestone not reached: no SpaceElevatorPart_* delivery after %d ticks; "+
				"max simultaneously-producing factories=%d; distinct products ever produced=%d %v; "+
				"total recent trades=%d",
				longRunTickCount, maxProducing, len(everProduced), everProduced,
				len(testState.ledger.trades))
		}

		// THE MILESTONE, part 2: delivery must be sustained, not a fluke.
		baseline := totalPartsDelivered()
		for i := 0; i < sustainedWindowTicks; i++ {
			err = testState.Tick(l)
			assert.NoError(t, err, "failed to tick state")
		}
		sustainedDelta := totalPartsDelivered() - baseline
		if sustainedDelta < sustainedMinUnits {
			t.Fatalf("milestone not sustained: first delivery by tick %d, but only %.1f more "+
				"units arrived in the following %d ticks; want >= %.0f",
				firstDeliveryTick, sustainedDelta, sustainedWindowTicks, sustainedMinUnits)
		}

		// Conservation sanity check: stock physically cannot oversell
		// (Inventory.Take clamps at 0) or overfill (ProduceTick stops at
		// the cap), so this can only fail from a logic error upstream.
		for _, p := range testState.producers {
			switch producer := p.(type) {
			case *resources.Resource:
				assert.GreaterOrEqual(t, producer.Stock, 0.0,
					"resource %s has negative stock", producer.PrettyPrint())
			case *factory.Factory:
				for name, qty := range producer.OutputStock {
					assert.GreaterOrEqual(t, qty, 0.0,
						"factory %s has negative %s stock", producer.String(), name)
				}
				for _, output := range producer.Output {
					cap := output.Rate*outputStockCapTicks + 1e-6
					assert.LessOrEqual(t, producer.OutputStock.Get(output.Name), cap,
						"factory %s %s stock exceeds cap", producer.String(), output.Name)
				}
			}
		}

		// Real factory-to-factory trade must exist somewhere in the
		// recent ledger.
		factorySaleFound := false
		for _, tr := range testState.ledger.trades {
			if _, ok := tr.seller.(*factory.Factory); !ok {
				continue
			}
			if _, ok := tr.buyer.(*factory.Factory); ok {
				factorySaleFound = true
				break
			}
		}
		assert.True(t, factorySaleFound, "expected at least one factory-to-factory trade in the recent ledger")

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
	})
```

Notes on what changed vs. the old body: the `t.Skipf` bounded-protocol block is gone (replaced by the part-1 `t.Fatalf`); the `partDelivered` bool helper became `totalPartsDelivered` (a sum, so part 2 can measure deltas); part 2 (sustained window) is new; the conservation/factory-trade/niche assertions are unchanged and now run after the sustained window (120k ticks total); the old trailing `assert.True(t, delivered, ...)` is gone because part 1 already `Fatalf`s.

- [ ] **Step 3: Verify it compiles and short mode still skips it**

Run: `go test ./state -short`

Expected: PASS (the long-run subtest reports SKIP under `-short`; everything else passes). This confirms compilation and that no fast test broke. Do **not** run the full 120k-tick test in this task — that is Task 4's acceptance run.

- [ ] **Step 4: Commit**

```bash
git add state/state_test.go
git commit -m "test: milestone becomes a hard sustained-delivery assertion

First SpaceElevatorPart_* delivery within 100k ticks, then >=5 more
units within the following 20k ticks. The Phase 4 t.Skipf concession
is gone.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 4: Regression suite + milestone acceptance run

**Files:**
- Modify: `docs/superpowers/specs/2026-07-16-wallet-grounded-bids-design.md` (append a Status section with the results)
- No source files change in this task. **If the milestone fails, do not change any source file — report instead.**

**Interfaces:**
- Consumes: the hardened test from Task 3 (`go test ./state -run 'Test_state_Tick/long_run'`), plus the whole suite.

- [ ] **Step 1: Full fast suite**

Run: `go test ./... -short`

Expected: PASS (all packages).

- [ ] **Step 2: Determinism guard**

Run: `go test ./state -run Test_state_determinism -v`

Expected: PASS (two 2000-tick runs, byte-identical wire snapshots).

- [ ] **Step 3: The milestone acceptance run**

Run: `go test ./state -run 'Test_state_Tick/long_run' -v -timeout 120m`

(Go replaces spaces in subtest names with underscores, so `long_run` matches "long run: real trade, niches, and a space-elevator delivery". This is 120k ticks; expect it to take a while.)

Expected: PASS — first delivery within 100k ticks, ≥ 5 more units within the following 20k.

**If it FAILS:** stop. Do not tune constants, do not modify source. Capture the full failure message (it carries maxProducing / everProduced / trade-count diagnostics, or the sustained-delta shortfall) and report it to the user — the spec's bounded protocol says the next move (e.g. Approach B, the trade-grounded estimator) is a human decision.

- [ ] **Step 4: Record the outcome in the spec**

Append to `docs/superpowers/specs/2026-07-16-wallet-grounded-bids-design.md`:

```markdown
## Status (as implemented)

Implemented 2026-07-19. The wallet-grounded cap (state/prices.go), 8
Water nodes (resources/Resource.json), and the hardened
sustained-delivery milestone test (state/state_test.go) all landed.

Milestone run at seed 152: <fill in the actual observed result here —
first-delivery tick, units delivered in the sustained window, and
wall-clock duration of the test run. If the run failed, record the full
diagnostic output instead.>
```

Replace the `<fill in …>` placeholder with the real numbers from Step 3's output before committing.

- [ ] **Step 5: Commit**

```bash
git add docs/superpowers/specs/2026-07-16-wallet-grounded-bids-design.md
git commit -m "docs: record Phase 5 milestone run results in the spec

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```
