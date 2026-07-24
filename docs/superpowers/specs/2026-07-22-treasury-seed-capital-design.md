# Treasury-Grounded Seed Capital (Phase 6) — Design

**Goal:** close the seed-capital money pump identified in Phase 5 by
funding new factories from a finite, circulating treasury instead of
minting their seed capital from nothing — so the total money injected via
spawning is bounded and cannot compound with inflated book prices.

## Problem

Phase 5 (see `2026-07-16-wallet-grounded-bids-design.md`, "Status") wallet-
grounded every bid, but the milestone still failed. A deterministic probe
found two independent new root causes. The **first** is the target of this
phase:

**Seed-capital money pump.** `state/spawn.go` funds each new factory with

```
seedCapital = Σ estimatedDeliveredCost(input)·rate · inputStockTargetTicks
              + upkeepPerTick · seedCapitalBufferTicks
```

and `factory.New(..., seedCapital)` **mints that money ex nihilo** — no
account is debited. `estimatedDeliveredCost` reads the current book ask for
each input, so when a packaging loop's internal asks inflate, its new
entrants spawn with proportionally inflated wallets, whose wallet-backed
(cap-legal) bids then validate those very asks. It is an unbounded, self-
reinforcing money-injection loop: by tick 100k the alumina packaging loop
alone held ~121M combined cash across 96 factories (best Water bid
~25k/unit, fully wallet-backed) and absorbed nearly all spawn weight,
starving the iron/space-elevator chain.

### Money-flow audit (why this is the right lever)

The simulation is an open monetary system. Tracing `executeTrade`
(`state/orders.go`) and `applySolvency` (`state/solvency.go`):

| Flow | Effect | Bounded? |
|---|---|---|
| Sink purchases | **mint** (sinks have infinite money; seller is paid from nothing) | yes — per-tick fixed rate × 1000 |
| **Seed capital** | **mint** (`factory.New` funds a wallet ex nihilo) | **no — the pump** |
| Salvage | mint (tiny, `floorUnitPrice` = 0.1) | yes |
| Upkeep | **burn** (`Wallet.Apply(-0.5)` destroys it) | — |
| Transport share of a trade | burn (leaves the economy) | — |
| Buying from a resource node | burn (resources have no wallet; buyer pays, nobody receives) | — |
| Culled factory residual | vanishes (dead wallet dropped) | — |

Only the **seed-capital faucet** is the unbounded pump. The sink faucet is
bounded per tick; salvage is negligible. So the fix bounds the seed
channel specifically and leaves sinks, resources, and trade mechanics
untouched — critically, the sink stays an infinite demand faucet, which is
what pulls the milestone chain into existence.

## Decision 1: a finite seed-capital treasury

Add a single scalar money pot to `State`:

```go
treasury float64   // funds all seed capital; never negative
```

initialized to `initialTreasuryFund` at world start.

**Spawn (`spawnNewProducer`):** compute `seedCapital` exactly as today,
then gate on the treasury:

```go
seedCapital := stockCost*inputStockTargetTicks + upkeepPerTick*seedCapitalBufferTicks
if s.treasury < seedCapital {
    return // treasury short: skip this tick's spawn entirely
}
s.treasury -= seedCapital
newFactory := factory.New(...)
```

The seed formula is **unchanged** — it may still read inflated book asks —
because the treasury does the bounding: the moment a loop's inflated seed
request exceeds the treasury balance, that spawn is **skipped**, so the
loop stops recruiting, no new inflated wallets are minted, and the inflated
asks stop being validated. Cheap recipes (the iron chain, seeds in the
hundreds) keep spawning freely from the same pot. The pump dies
**emergently** from scarcity, with no per-unit price ceiling.

**Skip, not partial funding:** when the treasury cannot cover a seed, the
spawn is skipped rather than funded with whatever remains. Partial funding
would spawn short-runway, low-bid-cap factories that die fast and would let
a near-empty treasury emit a wave of doomed factories. Skipping preferentially
starves expensive-seed recipes exactly when money is scarce — which is the
pump-killing mechanism — while leaving cheap recipes fundable.

## Decision 2: replenishment (rent + recycled corpses)

The treasury is a **circulating** fund, not a one-shot allowance. It is
replenished in `applySolvency` from money that is destroyed today:

1. **Rent.** Route each factory's per-tick upkeep into the treasury:
   `s.treasury += upkeepPerTick` for every factory in the solvency loop.
   The factory's wallet still loses exactly `upkeepPerTick` (via the
   existing `Wallet.Apply(salvage - upkeepPerTick)`), so **per-factory
   solvency dynamics are completely unchanged** — only the destination of
   the money moves, from "burned" to "collected." Rent income is
   ≈ `upkeepPerTick × factoryCount` per tick, self-scaling with population.

2. **Recycled corpses (dormant).** When a factory is culled, return any
   positive residual cash to the treasury:
   `if cash := f.Wallet.Cash(); cash > 0 { s.treasury += cash }`.

   **Known limitation, kept deliberately:** the only cull path is
   `InsolventFor(insolvencyGrace)`, which requires the balance to have been
   negative for `insolvencyGrace` consecutive ticks — so a culled factory's
   cash is *always negative* and this branch **never fires today**. It is
   retained as correct, defensive accounting (2 lines) that would matter if
   a future phase culls profitable-but-idle factories; it is documented as
   dormant and gets no dedicated test (an unreachable branch cannot be
   exercised through the public path). Rent is the load-bearing income.

Negative culled residuals are left to vanish as today (a tiny money-
creation faucet); full monetary conservation is a non-goal — bounding the
seed channel is.

**No runaway:** rent income is bounded (`upkeepPerTick × bounded
population`). If the treasury accumulates idle rent over a long run, that
is harmless: a large treasury is only a pump if seeds *scale with inflating
asks*, and once the pump is starved early the asks stay near marginal cost,
so seeds stay small regardless of treasury size. Idle money is not a pump.

## Decision 3: initial fund

```go
const initialTreasuryFund = 10000.0
```

Typical honest seeds are a few hundred to low-thousand units; inflated pump
seeds run to millions. `10000` funds roughly 10–30 typical factories up
front, after which rent replenishment (≈ `0.5 × population` per tick)
carries it. It is far below any inflated seed, so the pump ceiling is
reached early. The tradeoff of a tight fund is a small risk of throttling
legitimate early spawning before the population — and thus rent income —
builds up; this is accepted and is a milestone tuning knob.

## Files

- `state/state.go` — add `treasury float64` to `State`; initialize to
  `initialTreasuryFund` in `New`. Define `initialTreasuryFund`.
- `state/spawn.go` — gate `spawnNewProducer` on the treasury; withdraw on
  spawn.
- `state/solvency.go` — collect rent into the treasury; recycle positive
  culled residuals (dormant).
- `state/orders_test.go` — `newTestState` initializes `treasury`.
- `state/spawn_test.go` — `newTestStateWithProducers` initializes
  `treasury` (existing spawn tests assert a factory appears and so require
  a funded treasury).

## Testing

- **Unit — spawn withdraws:** a funded treasury drops by exactly
  `seedCapital` when a factory spawns, and the factory is added.
- **Unit — spawn skipped when short:** with `treasury` set below the
  recipe's `seedCapital`, `spawnNewProducer` adds no producer and leaves
  the treasury unchanged.
- **Unit — rent collected:** one `applySolvency` tick over N factories
  increases the treasury by `upkeepPerTick × N`.
- **Regression:** `Test_state_determinism` (two same-seed runs stay byte-
  identical — the treasury is a deterministically-updated scalar, no new
  map iteration), the existing spawn/solvency/cascade suites, and the full
  `-short` suite stay green. Spawn tests require the treasury-initialized
  helpers.
- **Milestone:** re-run the hardened sustained-delivery gate in
  `state/state_test.go` at seed 152 and record the result in Status. This
  phase does **not** address the second Phase 5 root cause (cascade
  starvation), so the milestone may remain red; per the bounded-tuning
  protocol, measure the pump's death on a clean economy and report rather
  than tune. Do not stack a second mechanism change onto this one before
  measuring.

## Out of scope

- **Cascade starvation** (the sink-fed pioneer cannot bid high enough to
  summon its deep-tier inputs) — the second Phase 5 root cause. Deferred to
  a later phase so this lever can be measured in isolation.
- Bounding the seed *formula* / per-unit price ceiling (the Phase 6
  "minimal" alternative) — the treasury's skip-when-short rule bounds the
  channel emergently instead.
- Re-plumbing sinks, resource nodes, transport, or salvage into the
  treasury (full monetary conservation) — unnecessary for bounding the
  pump, and re-plumbing the sink would break the infinite-demand premise.
- Any frontend or wire-format change — the treasury is internal state.

## Status (as implemented)

Implemented 2026-07-25. `state/spawn.go` withdraws seed capital from the
treasury and skips the spawn when short (commit 7a3946f); `state/
solvency.go` routes upkeep rent into the treasury and recycles positive
culled residuals (commit 07fe2e4). Regression: `go test ./... -short`
passes for every package, and `Test_state_determinism` (seed 152, 2000
ticks) still produces byte-identical wire snapshots across two runs — the
treasury update is a deterministic scalar; no map-iteration nondeterminism
was introduced.

**Milestone run at seed 152: NOT REACHED (anticipated).** No
`SpaceElevatorPart_*` delivery in 100k ticks (`go test ./state -run
Test_state_Tick -v`; the long-run subtest took 15.5s wall clock, vs Phase
5's 61s for the same 100k ticks — one signal of a smaller, quieter
economy). Per the bounded-tuning protocol this phase addresses only root
cause #1 (the money pump), so a still-red milestone is the anticipated
outcome, not a regression.

A read-only deterministic probe (checkpoints at 5k/20k/50k/100k ticks,
extended to 120k to cover the sustained-delivery window; temporary test,
deleted before commit) confirms the pump is dead and pinpoints exactly
which remaining root cause blocks delivery:

1. **The seed-capital pump is measurably starved.** The max combined cash
   held by any single recipe class over the run was **~60.8k**
   (`Recipe_PackagedCrudeOil_C`) — roughly **2,000x smaller** than Phase
   5's ~121M. The specific Phase 5 offender, "Unpackage Alumina
   Solution," peaked at **~21k combined cash across at most 22
   factories**, vs Phase 5's 96 factories holding 121M combined — its
   combined cash collapsed by more than three orders of magnitude
   (~5,700x) and its factory headcount fell by roughly 4x. The treasury
   itself stayed bounded throughout, oscillating
   roughly 1.7k–70k across the run (drawn down hard by the first wave of
   spawns, then refilled by rent) — no runaway growth, matching the "no
   runaway" argument in Decision 2. The skip-when-short gate is actively
   firing: 378 spawns were skipped for a short treasury over the
   120k-tick run.

2. **Iron / space-elevator chain recipes now spawn — a first.** Of the
   12 space-elevator-part recipes known to the simulation, 9 spawned at
   least once during the run, including the pioneer "Smart Plating" (the
   `SpaceElevatorPart_1` recipe) itself. Of the 15 recipes producing the
   tier immediately below it, 5 spawned, including both of Smart
   Plating's direct inputs — "Reinforced Iron Plate" and "Rotor" — plus
   "Modular Frame," "Motor," and "Stator." In Phase 5 these recipes
   "never spawned" at all; this is a qualitative change directly
   attributable to the treasury no longer being monopolized by the
   packaging-loop pump.

3. **But root cause #2 (cascade starvation) still blocks delivery.**
   Spawning is no longer the bottleneck, yet none of these chain
   recipes ever completed a production cycle: the milestone gate's own
   `everProduced` accounting over the first 100k ticks lists 21 distinct
   products, none from the iron/space-elevator chain (no
   `IronPlateReinforced`, `Rotor`, or `SpaceElevatorPart_*`). Chain
   factories spawn, then evidently die from insolvency (300-tick grace)
   before their input bids can out-compete for Reinforced-Iron-Plate/
   Rotor supply and complete even one production run — exactly the
   mechanism Phase 5's probe diagnosed (bids escalate but the wallet cap
   tops out below the price needed to make a Rotor/Reinforced-Iron-Plate
   factory attractive). Phase 7 should start from this evidence: the
   pump is closed, so the next lever is getting a spawned pioneer's
   demand signal to survive long enough, or reach far enough, to summon
   its own deep-tier inputs — not the money supply.

The overall economy shape also shifted with the smaller, treasury-gated
population: max simultaneously-producing factories dropped from 21
(Phase 5) to 14, and recent trades from 8794 to 909 — but distinct
products ever produced rose slightly, 18 to 21. The smaller economy that
does form is more diverse per unit of activity, not merely shrunk
uniformly.
