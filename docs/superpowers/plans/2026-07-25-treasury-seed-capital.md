# Treasury-Grounded Seed Capital (Phase 6) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fund new factories from a finite, rent-replenished treasury instead of minting seed capital from nothing, closing the Phase 5 seed-capital money pump.

**Architecture:** Add one scalar money pot (`treasury float64`) to `State`. `spawnNewProducer` withdraws each new factory's `seedCapital` from it and skips the spawn when the pot is short. `applySolvency` replenishes it by routing per-factory upkeep in as rent (the factory still loses the same 0.5/tick — only the money's destination moves) and recycling positive culled-factory residuals. No change to sinks, resources, trade, or salvage.

**Tech Stack:** Go (backend only). No frontend or wire-format change.

## Global Constraints

Copied verbatim from `docs/superpowers/specs/2026-07-22-treasury-seed-capital-design.md`:

- `const initialTreasuryFund = 10000.0` — the initial fund; `treasury` is never negative.
- **Spawn gates on the treasury:** compute `seedCapital` exactly as today (formula unchanged, may still read book asks), then `if s.treasury < seedCapital { return }` (skip the whole spawn this tick — no partial funding); otherwise `s.treasury -= seedCapital` before creating the factory.
- **Rent:** in `applySolvency`, every factory that pays upkeep also adds `upkeepPerTick` to `s.treasury`. The factory's own wallet change (`Wallet.Apply(salvage - upkeepPerTick)`) is **unchanged** — per-factory solvency dynamics must stay identical; only the money's destination moves from burned to collected.
- **Recycled corpses (dormant):** when a factory is culled, `if cash := f.Wallet.Cash(); cash > 0 { s.treasury += cash }`. This branch is **unreachable today** (cull only fires on `InsolventFor`, so residual cash is always negative) — it is deliberate defensive accounting and gets **no dedicated test**. Do not add a contrived test for it.
- **Determinism:** `treasury` is a single scalar updated in the existing deterministic producer-slice iteration order. Introduce **no** new map iteration. `Test_state_determinism` must stay green.
- Existing economic constants (do not change): `upkeepPerTick = 0.5`, `insolvencyGrace = 300`, `inputStockTargetTicks = 60.0`, `seedCapitalBufferTicks = 300.0`, `unknownInputUnitCost = 10.0`, `defaultTransportEstimate = 2.0`.
- Scope is Phase 5 root cause #1 only. **Cascade starvation (root cause #2) is explicitly out of scope** — do not touch bid escalation, the sink, or the cascade.

---

### Task 1: Treasury state, funded construction, and treasury-gated spawning

**Files:**
- Modify: `state/state.go` (add `initialTreasuryFund` const; add `treasury` field to `State`; initialize it in `getInitialState`)
- Modify: `state/spawn.go:82-91` (gate `spawnNewProducer` on the treasury)
- Modify: `state/orders_test.go:19-25` (`newTestState` funds the treasury)
- Modify: `state/spawn_test.go:16-27` (`newTestStateWithProducers` funds the treasury)
- Test: `state/spawn_test.go` (new tests for withdrawal and skip-when-short)

**Interfaces:**
- Consumes: `State.book`/`recipes`/`randSrc` (existing), `s.estimatedDeliveredCost`, `chosenRecipe.Inputs()`, `factory.New(...)`, `production.Producer`.
- Produces: `State.treasury float64` (read/written by Task 2's `applySolvency`); `const initialTreasuryFund = 10000.0`.

- [ ] **Step 1: Add the constant and the struct field**

In `state/state.go`, extend the top const block (currently `const ( borderPaddingPct = 0.1 )`) to:

```go
const (
	borderPaddingPct = 0.1

	// initialTreasuryFund is the seed-capital pot's starting balance.
	// All new-factory seed capital is withdrawn from the treasury and it
	// is replenished by upkeep-as-rent (see applySolvency), so the total
	// money injected into the economy via spawning is bounded instead of
	// minted without limit. Deliberately far below any inflated
	// packaging-loop seed (millions): once a loop's seed request exceeds
	// the pot, that spawn is skipped and the money pump starves.
	initialTreasuryFund = 10000.0
)
```

Add the field to the `State` struct (after `ledger`):

```go
	ledger    *tradeLedger

	// treasury funds all new-factory seed capital; withdrawn on spawn,
	// replenished by upkeep-as-rent. Never negative. See the Phase 6 spec
	// (docs/superpowers/specs/2026-07-22-treasury-seed-capital-design.md).
	treasury float64
```

- [ ] **Step 2: Initialize the treasury in getInitialState**

In `state/state.go`, in `getInitialState`, alongside the other `s.` assignments (e.g. right after `s.ledger = &tradeLedger{}`):

```go
	s.treasury = initialTreasuryFund
```

- [ ] **Step 3: Fund the treasury in both test helpers**

In `state/orders_test.go`, `newTestState`:

```go
func newTestState() *State {
	return &State{
		book:      market.NewBook(),
		lastTrade: make(map[string]float64),
		ledger:    &tradeLedger{},
		treasury:  initialTreasuryFund,
	}
}
```

In `state/spawn_test.go`, `newTestStateWithProducers`:

```go
func newTestStateWithProducers(rs recipes.Recipes, producers []production.Producer) *State {
	return &State{
		recipes:   rs,
		producers: producers,
		book:      market.NewBook(),
		lastTrade: make(map[string]float64),
		ledger:    &tradeLedger{},
		randSrc:   rand.New(rand.NewSource(1)),
		xmin:      0, xmax: 1000, ymin: 0, ymax: 1000,
		treasury:  initialTreasuryFund,
	}
}
```

- [ ] **Step 4: Write the failing tests**

Add to `state/spawn_test.go`:

```go
func Test_spawnNewProducer_withdrawsSeedFromTreasury(t *testing.T) {
	s := newTestState()
	s.randSrc = rand.New(rand.NewSource(1))
	s.xmin, s.xmax, s.ymin, s.ymax = 0, 1000, 0, 1000
	s.recipes = append(s.recipes, testRecipe(t))

	before := s.treasury
	s.spawnNewProducer(testLogger())

	var spawned *factory.Factory
	for _, p := range s.producers {
		if f, ok := p.(*factory.Factory); ok {
			spawned = f
		}
	}
	if spawned == nil {
		t.Fatal("no factory spawned from a funded treasury")
	}
	// The withdrawal equals the seed capital, which equals the new
	// factory's starting cash.
	if got, want := s.treasury, before-spawned.Wallet.Cash(); got != want {
		t.Fatalf("treasury after spawn = %v, want %v (withdrew seedCapital %v)",
			got, want, spawned.Wallet.Cash())
	}
}

func Test_spawnNewProducer_skipsWhenTreasuryShort(t *testing.T) {
	s := newTestState()
	s.randSrc = rand.New(rand.NewSource(1))
	s.xmin, s.xmax, s.ymin, s.ymax = 0, 1000, 0, 1000
	s.recipes = append(s.recipes, testRecipe(t))
	// testRecipe seed = (10+2)*1*60 + 0.5*300 = 870. Fund below that.
	s.treasury = 1.0

	before := s.treasury
	nBefore := len(s.producers)
	s.spawnNewProducer(testLogger())

	if len(s.producers) != nBefore {
		t.Fatalf("producers = %d, want %d (spawn must be skipped when treasury is short)",
			len(s.producers), nBefore)
	}
	if s.treasury != before {
		t.Fatalf("treasury = %v, want %v unchanged (nothing withdrawn on a skip)", s.treasury, before)
	}
}
```

- [ ] **Step 5: Run the tests to verify they fail**

Run: `go test ./state -run 'Test_spawnNewProducer_withdrawsSeedFromTreasury|Test_spawnNewProducer_skipsWhenTreasuryShort' -v`
Expected: FAIL — `withdraws` sees the treasury unchanged (no withdrawal yet); `skipsWhenTreasuryShort` sees a factory spawned and the treasury driven negative, because the gate does not exist yet.

- [ ] **Step 6: Add the treasury gate to spawnNewProducer**

In `state/spawn.go`, replace the seed-capital block (currently):

```go
	seedCapital := stockCost*inputStockTargetTicks + upkeepPerTick*seedCapitalBufferTicks

	newFactory := factory.New(chosenRecipe.Name(), chosenRecipe.ID(), s.spawnLocation(chosenRecipe), s.tick,
		chosenRecipe.Inputs(), chosenRecipe.Outputs(), seedCapital)
```

with:

```go
	seedCapital := stockCost*inputStockTargetTicks + upkeepPerTick*seedCapitalBufferTicks

	// Seed capital is withdrawn from the finite treasury, not minted. If
	// the treasury cannot cover this seed, skip the spawn entirely (no
	// partial funding): an inflated packaging-loop seed that outgrows the
	// pot is refused, starving the money pump, while cheap recipes stay
	// fundable from the same pot.
	if s.treasury < seedCapital {
		l.Debug("spawn skipped: treasury short",
			slog.Float64("treasury", s.treasury),
			slog.Float64("seedCapital", seedCapital))
		return
	}
	s.treasury -= seedCapital

	newFactory := factory.New(chosenRecipe.Name(), chosenRecipe.ID(), s.spawnLocation(chosenRecipe), s.tick,
		chosenRecipe.Inputs(), chosenRecipe.Outputs(), seedCapital)
```

(`slog` is already imported in `spawn.go`.)

- [ ] **Step 7: Run the tests to verify they pass**

Run: `go test ./state -run 'Test_spawnNewProducer' -v`
Expected: PASS — the two new tests plus all existing `Test_spawnNewProducer_*` tests (they now run against a treasury funded to `initialTreasuryFund` = 10000, comfortably above the 870 testRecipe seed).

- [ ] **Step 8: Commit**

```bash
git add state/state.go state/spawn.go state/orders_test.go state/spawn_test.go
git commit -m "feat: fund seed capital from a finite treasury, skip spawn when short

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 2: Treasury replenishment — rent and recycled corpses

**Files:**
- Modify: `state/solvency.go` (`applySolvency`: collect rent; recycle positive culled residual)
- Test: `state/solvency_test.go` (new rent test)

**Interfaces:**
- Consumes: `State.treasury` (from Task 1), `upkeepPerTick`, `f.Wallet.Apply`, `f.Wallet.Cash`, `f.Wallet.InsolventFor`.
- Produces: nothing new for later tasks; this task closes the treasury's money-in side.

- [ ] **Step 1: Write the failing test**

Add to `state/solvency_test.go`:

```go
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./state -run 'Test_applySolvency_collectsUpkeepAsRent' -v`
Expected: FAIL — `treasury after rent` is unchanged from `before` because no rent is collected yet.

- [ ] **Step 3: Collect rent and recycle positive residuals in applySolvency**

In `state/solvency.go`, in the per-factory loop of `applySolvency`, after the existing line `f.Wallet.Apply(salvage - upkeepPerTick)`, add the rent collection:

```go
		f.Wallet.Apply(salvage - upkeepPerTick)
		// Rent: the upkeep the factory just paid is collected into the
		// treasury rather than burned. The factory's wallet change above
		// is identical either way, so solvency dynamics are unchanged --
		// only the money's destination moves, funding future seed capital.
		s.treasury += upkeepPerTick
```

Then change the cull branch. The existing branch is:

```go
		if f.Wallet.InsolventFor(insolvencyGrace) {
			l.Debug("removing bankrupt factory",
				slog.String("factory", f.String()),
				slog.Float64("cash", f.Wallet.Cash()))
			continue // not kept: the factory and its stock vanish
		}
```

Replace it with:

```go
		if f.Wallet.InsolventFor(insolvencyGrace) {
			l.Debug("removing bankrupt factory",
				slog.String("factory", f.String()),
				slog.Float64("cash", f.Wallet.Cash()))
			// Recycle any positive residual cash back into the treasury.
			// Dormant today: the only cull path is InsolventFor, so a
			// culled factory's cash is always negative and this never
			// fires. Kept as correct, defensive accounting for a future
			// phase that might cull profitable-but-idle factories.
			if cash := f.Wallet.Cash(); cash > 0 {
				s.treasury += cash
			}
			continue // not kept: the factory and its stock vanish
		}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./state -run 'Test_applySolvency' -v`
Expected: PASS — the new rent test plus the existing `Test_applySolvency_*` tests (which assert only per-factory wallet/stock outcomes, all unchanged).

- [ ] **Step 5: Commit**

```bash
git add state/solvency.go state/solvency_test.go
git commit -m "feat: replenish the treasury from upkeep rent and recycled residuals

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 3: Regression, determinism, and milestone acceptance run

**Files:**
- Modify: `docs/superpowers/specs/2026-07-22-treasury-seed-capital-design.md` (fill the Status section)

**Interfaces:**
- Consumes: the full assembled Phase 6 behavior from Tasks 1–2.
- Produces: a recorded milestone outcome (documentation only).

- [ ] **Step 1: Run the fast suite and determinism**

Run: `go test ./... -short`
Expected: PASS (all packages).

Run: `go test ./state -run 'Test_state_determinism' -v`
Expected: PASS — two same-seed runs stay byte-identical. The treasury is a deterministically-updated scalar; if this fails, a nondeterministic ordering was introduced (investigate before proceeding — do not paper over it).

- [ ] **Step 2: Run the milestone gate at seed 152**

Run: `go test ./state -run 'Test_state_Tick' -v` (this includes the ~60s hardened long-run sustained-delivery subtest at seed 152).
Record: whether `SpaceElevatorPart_*` is delivered within 100k ticks and, if so, whether ≥5 more units land in the following 20k ticks.

**Bounded-tuning protocol (same as Phase 4/5):** if the milestone is NOT reached, do **not** tune any constant. This phase deliberately addresses only root cause #1 (the money pump); root cause #2 (cascade starvation) is out of scope, so a still-red milestone is an anticipated outcome, not a failure of this task. Gather evidence instead (see Step 3).

- [ ] **Step 3: Gather diagnostic evidence (regardless of pass/fail)**

Whether or not the milestone passes, capture what the treasury did to the pump so the Status section is evidence-based. If a read-only probe is needed (as in Phase 5), write a temporary, uncommitted test that replays seed 152 and reports at checkpoints:
- treasury balance over time (did it stay bounded / oscillate / accumulate?);
- max combined cash held by the top packaging-loop recipe class (vs. the Phase 5 baseline of ~121M across 96 "Unpackage Alumina Solution" factories);
- whether the iron / space-elevator chain recipes now spawn at all (Phase 5: they never did);
- count of spawn-skips due to a short treasury.

Delete the probe before committing (do not leave it in the tree).

- [ ] **Step 4: Record the outcome in the spec Status section**

Replace the `_To be completed during implementation…_` placeholder in the spec's "Status (as implemented)" section with: the milestone result at seed 152, the observed treasury trajectory, and whether the packaging-loop pump is measurably starved vs. the Phase 5 baseline. If red, state precisely which root cause still blocks (expected: cascade starvation) so Phase 7 starts from evidence.

- [ ] **Step 5: Commit**

```bash
git add docs/superpowers/specs/2026-07-22-treasury-seed-capital-design.md
git commit -m "docs: record Phase 6 treasury milestone run results in the spec

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Self-Review

**1. Spec coverage:**
- Decision 1 (finite treasury; withdraw on spawn; skip when short) → Task 1. ✅
- Decision 2 (rent replenishment; dormant corpse recycle) → Task 2. ✅
- Decision 3 (`initialTreasuryFund = 10000`) → Task 1, Step 1. ✅
- Money-flow audit / "no runaway" reasoning → informs the design, no code beyond the above.
- Testing section (withdraw, skip-when-short, rent, regression, milestone) → Tasks 1–3. ✅
- Out-of-scope items (cascade, seed-formula ceiling, sink re-plumbing, wire changes) → enforced by Global Constraints and touched by no task. ✅

**2. Placeholder scan:** No TBD/TODO in the tasks. Every code step shows the actual code; the only intentionally-deferred content is the spec Status text, which is Task 3's deliverable. ✅

**3. Type consistency:** `treasury` (`float64`) is defined in Task 1 and read/written by Task 2 under the same name; `initialTreasuryFund` is defined once (Task 1) and reused in both test helpers and the constructor; `seedCapital` is the existing local in `spawnNewProducer`. `upkeepPerTick`, `insolvencyGrace`, `Wallet.Apply/Cash/InsolventFor` all match their existing definitions. ✅
