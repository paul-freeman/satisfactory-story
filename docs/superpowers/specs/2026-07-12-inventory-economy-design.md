# Inventory Economy Design

Date: 2026-07-12
Status: approved (brainstormed with user; all forks below chosen explicitly)
Supersedes the trade-clearing model of
`2026-07-11-order-book-market-design.md` (the order book itself survives;
what a *match* means changes).

## Goal

A full ~139-recipe economy self-assembles a production chain that delivers
a `SpaceElevatorPart_*` to its goal sink **within 100,000 ticks at seed
152** — the same bar Phase 3 missed. Secondary: preserve the "lived-in
world" qualities (geographic niches, churn, multiple producers per
product) and determinism per seed.

## Why (diagnosis summary)

The 2026-07-12 experimental investigation (see memory + commit `86fec24`
for the earlier layer) established five verified mechanisms; the deepest
one is architectural:

> The economy clears all trade as synchronized continuous flows with no
> buffering anywhere. A factory only offers goods while producing;
> producing without a buyer is fatally loss-making within a few hundred
> ticks; so supply windows and standing demand for the same product miss
> each other in time (verified: an IronPlate ask at 9.02 went live with
> zero bidders, right after 10,000 ticks during which three factories bid
> 18.37 and died waiting). A deep chain requires every tier to be alive,
> contracted, and producing simultaneously — probability effectively zero.

Inventory dissolves this by construction: goods produced at tick 10,000
sit in stock and are still buyable at tick 25,000. Two smaller verified
fixes ride along: the **crowding discount** on spawn weights (without it,
sink-adjacent recipes capture the entire spawn draw and base tiers never
spawn) and **transport-aware cost estimates** (without them, seed capital
and quotes ignore the transport term that dominates delivered cost at
low rates).

Also verified, and binding on this design: **uncapped bid escalation is
poison** (Package/Unpackage recipe 2-cycles amplify each other's bids to
10^51 and capture the spawn draw). The discipline that prevents it here
is hard budget: money changes hands at trade time, so speculative loops
with no money in them cannot trade.

## Decisions made with the user

1. **Full discrete-unit economy** — stock buffers replace flow contracts
   entirely (not a hybrid, not hardened pending-contracts).
2. **Spot market + derived links** — no standing agreements; map
   transport links are aggregated from a rolling window of recent trades.
3. **Halt + salvage overflow** — bounded output stock; when full,
   production pauses, input buying stops (back-pressure), and a slow
   trickle sells to the on-site AWESOME sink at the salvage floor.
4. **Purity line: public market state OK** — agents may read prices, the
   book, who exists and what they make, and recent trades. Still
   forbidden: deriving demand by walking the recipe tree top-down.
5. **Success bar unchanged** — first part within 100k ticks at seed 152.
6. **Frontend: adapt only** — keep the current UI working; no new
   visualizations. Prefer a wire-compatible shape so frontend changes
   are minimal or nil.

## Core model

### Stock

- Every producer holds real goods as float quantities per product.
- `factory.Factory`: input stock and output stock.
  - Output stock cap: `outputStockCapTicks` (default 60) × output rate,
    per output product.
  - Input stock target: `inputStockTargetTicks` (default 60) × input
    rate, per input product. The target drives buying; it is not a hard
    cap (a large fill can overshoot slightly — harmless).
- `resources.Resource`: output stock only, cap `outputStockCapTicks` ×
  purity rate. Produces every tick while below cap.
- `sink.Sink`: holds nothing; units bought are consumed immediately and
  counted (`Delivered` total). The milestone event is `Delivered > 0` on
  a space-elevator sink.

### Production step (each tick, before trading)

For a factory with recipe rates `r_i` (inputs) and `r_o` (outputs):

```
f = min(1,
        min_i(inputStock_i / r_i),
        min_o(outputRoom_o / r_o))
consume f * r_i from each input stock
add     f * r_o to each output stock
```

`f` is the fraction of one tick's production the factory can actually
run. A factory "is producing" this tick iff `f > RateEpsilon` — a
display/observability fact, not a contractual state.

### Trade (spot market)

Each tick, the book is rebuilt from live state exactly as today, but:

- **Asks** are backed by stock: quantity = units on hand (output stock
  for factories, stock for resources), at the producer's persistent ask
  price.
- **Bids** are hunger: quantity = `max(0, target_i - inputStock_i)`, at
  the factory's persistent bid price. Sinks post their standing bid
  (`goalBidUnitPrice = 1000`, quantity `sinkDemandRate`) every tick.
- **Matching** keeps the current algorithm shape (bids descending by
  price; each bid takes the lowest per-unit *delivered* cost ask it can
  cross), but a match now executes immediately as a one-shot trade:

```
unitDelivered = ask.UnitPrice + unitTransport(distance(seller, buyer))
crossable when bid.UnitPrice >= unitDelivered
qty = min(bid remaining, ask remaining, buyer wallet / unitDelivered)   // quantities are floats
buyer.wallet  -= qty * unitDelivered
seller.wallet += qty * ask.UnitPrice
seller stock  -= qty;  buyer input stock += qty
record trade in ledger
```

  (Sinks have an infinite wallet; the transport share of the buyer's
  payment leaves the economy — it is a cost, not anyone's income, same
  as today.) The budget clamp `qty <= wallet / unitDelivered` is the
  hard constraint that replaces Phase 3's affordability cap.

- **Per-unit transport**:

```
unitTransport(d) = transportFixedPerUnit + d * transportPerDistance
                 = 0.1 + d/10000        (defaults; both tunable)
unitTransport(d <= 1) = prohibitive (1e12)   // same-spot collision guard
```

  Per-unit (not per-contract-per-tick) pricing removes the old model's
  distortion where a rate-1 product paid 30× the effective freight of a
  rate-30 product. The `d <= 1` guard keeps its current role of stopping
  producers from stacking on one location; the spawn offset (5) already
  clears it.

### Money

- Money enters only at goal sinks (1000/unit, infinite wallet) and the
  salvage floor; it drains through per-tick upkeep (0.5, unchanged) and
  the transport share of purchases. Wallets never go negative from a
  purchase (budget clamp); only upkeep can push a wallet below zero,
  and the existing `insolvencyGrace` (300) removal applies unchanged.
- A removed factory's stock vanishes with it.

### Prices (local adjustment, stock signals)

Persistent per-product ask/bid prices survive on producers, adjusted
after trading each tick:

- **Ask**: if stock is empty after trading (sold out) → raise by
  `askRaisePct` (5%). If stock grew or sits at cap → decay by
  `askLowerPct` (2%) toward a floor:
  - factory floor = `(avgInputSpendPerTick + upkeepPerTick) / totalOutputRate`,
    where `avgInputSpendPerTick` is an exponential moving average
    (smoothing ~0.05) of actual input spend — the stock-world marginal
    cost;
  - resource floor = `production.MinUnitPrice`.
- **Bid**: if hunger remains unfilled after trading → raise by
  `bidRaisePct` (2%). No decay, no affordability precondition — the
  escalating bid is still the backward demand cascade, and the wallet
  clamp at trade time is what keeps it honest. (A high bid price with no
  money behind it buys nothing.)

### Salvage (AWESOME sink, overflow trickle)

In the solvency step, a factory whose output stock is at cap sells up to
`salvageTrickleFraction` (default 0.25) × one tick's output rate from
stock at `floorUnitPrice` (0.1/unit, unchanged). This frees a little
room (so a buyer-less pioneer keeps producing at ~25% rate, keeps a
trickle of upstream demand alive, and earns a little) without clearing
production at full rate — full-rate clearing would erase the
back-pressure signal entirely. Factories below cap earn no salvage;
real trade always dominates.

## Supporting mechanisms

### Spawning

Weighted expected-profit draw over active recipes, unchanged in shape,
with:

- **Crowding discount**: `weight = (baseline + max(0, expectedProfit)) /
  (1 + liveFactoriesRunningRecipe)` — reads the live producer population
  (legal under the purity line).
- **Transport-aware estimates**: `estimatedDeliveredCost(product) =
  estimatedUnitCost(product) + defaultTransportEstimate` (default 2.0
  per unit) used everywhere a cost is estimated (expected profit, seed
  capital, initial bids). A flat allowance, deliberately: expected
  profit is computed before a location exists, and precision here only
  needs to prevent the verified transport-blindness failure (estimates
  of ~0.01 against delivered costs of ~1-2), not price real freight.
- **Seed capital** = initial input-stock purchase at estimated delivered
  prices (`inputStockTargetTicks` worth) + `seedCapitalBufferTicks`
  (300) × upkeep runway.
- Spawn-near-sourceable-inputs (centroid of best-ask sellers + offset)
  unchanged.

### Movement

Hill-climb on transport cost unchanged in shape, but the gradient comes
from the factory's own recent trades (rolling window `tradeMemoryTicks`,
default 500): endpoints = locations of recent suppliers and customers,
weighted by traded quantity. No recent trades → hold still (preserves
commit `5e2fdeb`'s fix).

### Trade ledger

`State` keeps a rolling ledger of trades `{tick, seller, buyer, product,
qty, unitPrice}` pruned to `tradeMemoryTicks`. It feeds: wire transports
(aggregated seller→buyer→product edges with average rate over the
window), `lastTrade` prices, and movement gradients. Bounded memory.

### Deletions

`production.Contract` and everything that exists to manage it:
`SignAsBuyer/SignAsSeller/ContractsIn`, `Purchases/Sales` fields,
`Producing()` (contract sense), `RemainingCapacityFor`/`HasCapacityFor`
(oversell machinery), `signContract`, `renegotiate.go` and its tick
step, cancellation checks throughout, and the idle-factory sale-cancel
in solvency. The `production.Producer` interface shrinks to what is
actually used (location, products, string/pretty-print, wallet access
where applicable, plus new stock accessors).

### HTTP / frontend (adapt only)

Wire types keep their current shape and field names wherever possible so
the frontend needs few or no changes: `Transport{from, to, product,
rate}` now aggregated from the ledger; `Factory` keeps cash and gains
nothing new (a "producing" boolean maps to `f > 0` last tick); the
shortage panel keeps reading unfilled bids from the book. Endpoints
unchanged.

## Tunable constants (the milestone knobs)

| Constant | Default | Meaning |
|---|---|---|
| `outputStockCapTicks` | 60 | output buffer size, in ticks of production |
| `inputStockTargetTicks` | 60 | input buffer target, in ticks of consumption |
| `salvageTrickleFraction` | 0.25 | overflow sold to floor per tick, as fraction of output rate |
| `transportFixedPerUnit` / `transportPerDistance` | 0.1 / 1e-4 | per-unit freight |
| `seedCapitalBufferTicks` | 300 | upkeep runway in seed capital |
| `tradeMemoryTicks` | 500 | ledger/movement/transport-link window |

Existing constants unchanged: `upkeepPerTick` 0.5, `insolvencyGrace`
300, `floorUnitPrice` 0.1, `goalBidUnitPrice` 1000, `askRaisePct` 0.05,
`askLowerPct` 0.02, `bidRaisePct` 0.02, `spawnProbabilityPerTick` 0.05,
`baselineOpportunityWeight` 1.0, `unknownInputUnitCost` 10.0.

## Testing

- Unit tests per mechanism: production step (fractional runs, cap
  limits), spot matching (budget clamp, partial fills, delivered-cost
  ordering, determinism), overflow-halt back-pressure (full stock stops
  buying; trickle resumes it), crowding discount, ledger pruning and
  aggregation.
- Small-world cascade tests rebuilt on inventory semantics: a single
  goal sink plus hand-picked recipes/resources must deliver in a
  single-tier and a two-tier world (Phase 3's proof that the mechanism
  works before scale is attempted).
- The milestone test: full economy, seed 152, 100k ticks, delivery =
  `Delivered > 0` on a space-elevator sink. Bounded tuning protocol —
  the table's knobs, one at a time, instrument before turning — then
  STOP and report if not reached (no open-ended knob turning).
- Determinism: two runs at the same seed produce identical state.

## Out of scope

Stock-level UI, standing supply agreements / loyalty bias, spoilage,
storage costs, multi-good shipment batching, alternate recipes.

## Status (as implemented)

All 14 plan tasks landed; the design's core mechanism is proven at
small scale (Task 13's single- and two-tier cascade tests deliver
genuinely, independently reproduced deterministically). **The full
~139-recipe milestone (deliver a `SpaceElevatorPart_*` within 100k
ticks at seed 152) was NOT reached** after the bounded tuning protocol
(4 knobs, one at a time, none moved the outcome).

Root cause, verified against the code (not guessed): `state/prices.go`'s
bid escalation (`bidRaisePct`) is deliberately uncapped, relying on the
trade-time wallet clamp in `state/orders.go`'s `executeTrade` to keep
runaway prices harmless — and it does, for actual trades. But
`state/spawn.go`'s `expectedProfit`/`estimatedUnitCost` read the same
escalated book prices at face value with no plausibility bound, to
decide spawn weighting. In this data set, `Water` has no producible
source (absent from `resources/Resource.json`; every active recipe
that outputs it needs a Water-derived input to run, a deadlock, not a
simple cycle) — so bids for it escalate without limit (~10^63-10^69 by
tick ~7000), and recipes near that dead end capture nearly all
spawn-weight, starving viable recipes elsewhere in the book, including
the Water-free `SpaceElevatorPart_1` chain (`IronPlateReinforced` +
`Rotor`, fully producible from raw ore) that the milestone actually
needs. Water is the trigger this run happened to hit, not the target's
blocker — fixing the missing Water source alone would not be expected
to reach the milestone; bounding the spawn estimator against
implausible book prices is the load-bearing fix.

Not attempted in this phase (real design decisions, not constant
tweaks): capping bid escalation, or making
`expectedProfit`/`estimatedUnitCost` discount/ignore prices with no
recent trade behind them; separately, adding a Water resource/recipe
to fix the underlying data gap. Both are candidates for a future
phase if the user wants to pursue the milestone further.
