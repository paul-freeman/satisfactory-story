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

_To be completed during implementation: milestone run result at seed 152,
observed treasury trajectory, and whether the packaging-loop pump is
starved (loop cash and spawn-weight share vs. the Phase 5 baseline of
~121M / 96 factories)._
