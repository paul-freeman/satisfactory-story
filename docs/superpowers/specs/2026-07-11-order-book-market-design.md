# Order-Book Market Design

**Date:** 2026-07-11
**Status:** Approved for planning

## Problem

Demand enters the simulation in only two places: sinks buy `SpaceElevatorPart_*`
products, and newly spawned factories buy their inputs once, at spawn time. A
factory producing an intermediate product has zero revenue until a future spawn
attempt happens to draw a recipe that consumes its output, source all of that
recipe's other inputs, and pass the market-price veto. The expected wait is
thousands of ticks (documented in `state/solvency.go`), which forced
`insolvencyGrace` to 10,000 ticks, and no run has ever assembled the 4–5 tier
chain needed to deliver an elevator part.

Root causes:

1. **Discovery is one-shot and one-directional.** Buyers shop only at birth;
   sellers never advertise; nothing connects an existing seller to a later buyer
   except `renegotiateContracts`, which requires the buyer to already exist.
2. **Prices carry no demand signal.** `SalesPriceFor` is cost-plus-50%, and the
   `s.market` map is a "cheapest price ever seen" ratchet that only moves down.
3. **Intermediate demand is a statistical echo, not money.** The `unmet`
   shortage map nudges spawn odds but pays nobody.
4. **Bootstrapping a multi-tier chain needs simultaneous lucky spawns** of every
   tier, because a factory cannot exist with unsourced inputs.

## Goals and constraints

- **Success bar:** a seeded run reliably self-assembles a full supply chain that
  delivers a `SpaceElevatorPart_*` product to its sink. Plausible geography and
  legible economic churn are also goals, but the chain is the milestone.
- **Prices only.** No agent or system component reads the recipe tree top-down
  to compute derived demand. Chain assembly must emerge from local price
  signals.
- **Universal floor buyer is in.** An AWESOME-sink-style buyer of last resort
  pays a low floor price for any product, low enough that real
  factory-to-factory trade always beats it.
- Determinism given the seed is preserved (single `randSrc`, single mutex).

## Design

### 1. Order book (new `market` package)

A `Book` holds, per product, standing **asks** and **bids**.

- **Ask:** `{seller, product, rate, unitPrice}`. Republished every tick from
  each producer's `RemainingCapacityFor`, so the book never goes stale against
  real capacity. Only the *ask price* is persistent state, carried per product
  on the seller.
- **Bid:** `{buyer, product, rate, unitPrice}`. Factories bid for input rate
  not yet under contract. Each goal sink posts an effectively-unlimited bid for
  its elevator part at a high price. The floor buyer is an unlimited-rate,
  floor-price bid on every product — no special mechanism.

**Local price adjustment** (no computed market price anywhere):

- A seller with unsold capacity at end of tick lowers its ask by a small
  percentage, bounded below by its marginal cost (it never knowingly sells at a
  loss; selling to the floor buyer below overall profitability is allowed and
  correct — bad businesses lose money).
- A seller that sold out raises its ask.
- A buyer whose bid went unfilled raises it each tick, capped by affordability
  (the revenue its own outputs can support). Filled bids go dormant.

**Matching**, once per tick per product: sort asks ascending and bids
descending, cross while `bid ≥ ask`, trade at the **ask price**. Each match
signs a `production.Contract` through the existing `writeContract` machinery
(minus its market-map logic). Partial fills are natural because orders are
rates. Contracts remain the persistent flow objects; the book is only the
discovery layer.

**The escalating bid is the demand cascade.** A factory that cannot source an
input keeps raising its standing bid for it; that increasingly lucrative bid is
what makes a producer of that input look profitable to spawn. Demand propagates
backward through the tiers as money, with no tree-reading.

### 2. Factory lifecycle

A factory's state is derived, not stored:

- **Producing** — all inputs under contract. Pays purchases, earns sales, as
  today.
- **Idle** — at least one input uncontracted. Produces nothing, publishes no
  asks, pays no input costs, pays a small per-tick **upkeep** from its wallet.
  Its input bids sit in the book, escalating.

Spawning no longer requires sourcing inputs (`SourceProducts` as a spawn gate
is removed). Upkeep is the failure clock: an idle factory whose bids never fill
bleeds out and goes bankrupt in a legible timeframe. Consequences:

- `insolvencyGrace` drops from 10,000 to a few hundred ticks.
- The "cull anything missing an input contract" rule in `applySolvency` is
  deleted — missing contracts are a normal life stage.

### 3. Spawning as entrepreneurship

Keep the per-tick spawn probability. Replace the shortage-weighted recipe draw
with an **expected-profit draw** against the book:

- Expected revenue = Σ over outputs of (best standing bid price × rate). The
  floor buyer guarantees a bid always exists.
- Expected cost = Σ over inputs of (best standing ask price × rate); if a
  product has no ask, use its last trade price if seen, else an "unknown"
  estimate with a pessimism penalty — a penalty, not a veto, because recipes
  with rich output bids and missing inputs must still spawn occasionally; their
  posted bids are what summon the missing tier.
- Weight = `baseline + max(0, expectedProfit)`. The baseline keeps exploration
  alive (same spirit as today's `baselineOpportunityWeight`).

Seed capital stays "N ticks of projected costs," now sized to cover expected
idle wait. Location stays random; the existing `Move` hill-climb produces the
geography once contracts exist on both sides.

Money is non-conserved: sinks have unlimited budgets, seed capital is minted at
spawn. A conserved-money variant (per-tick sink budget) is a possible future
experiment; nothing in this design precludes it.

### 4. Tick order

All inside the existing mutex:

1. `publishOrders` — republish asks from live capacity; publish bids for unmet
   inputs (factories, sinks, floor buyer).
2. `matchOrders` — cross the book, sign contracts.
3. `moveProducers` — unchanged.
4. `spawnNewProducer` — probability-gated, profit-weighted draw.
5. `renegotiateContracts` — probability-gated; now "is there an ask meaningfully
   below my current contract price."
6. `applySolvency` — apply wallet deltas including upkeep; cull on sustained
   insolvency only.
7. `adjustPrices` — sellers/buyers nudge ask/bid prices from this tick's fill
   outcomes.

### 5. Removed / kept

**Removed:** `s.market` map; `shortage.go` (`recordShortage`, `decayShortages`,
`weightForProduct`); `sourceSinks`; the spawn-time `SourceProducts` gate and
market-price veto; the incomplete-input cull rule; `insolvencyGrace = 10000`.

**Kept:** `production.Contract` and both signing paths; `writeContract`
capacity enforcement; `Move` and transport-cost accounting; wallets; the sink
type (now expressed as standing bids); recipes/resources ingestion untouched.

### 6. HTTP / frontend

Wire shape stays compatible. `Shortages` is computed from unfilled bid volume
per product (the old signal, made honest). Add a per-product **price** (best
bid) to each shortage entry so the demand cascade climbing the tiers is visible
in the UI. Everything else renders as before.

### 7. Error handling

- Matching failures (capacity raced away, seller rejected) log-and-skip, as
  `writeContract` errors do today; the unfilled bid simply stays in the book.
- Removed/bankrupt producers shed their orders automatically at the next
  `publishOrders`, because the book is rebuilt from live producers each tick —
  no dangling-order state.

## Testing

- **Unit:** matching (crossing, partial fills, ask-price trades, deterministic
  ordering), ask decay with marginal-cost floor, bid escalation with
  affordability cap, expected-profit spawn weights (including the no-ask
  penalty path), upkeep and bankruptcy timing.
- **Cascade integration (the key property):** seeded world containing a bid for
  product X whose recipe needs input Y with no existing Y producer — assert a Y
  producer spawns and an X-to-sink contract flows within N ticks. Then a
  3-tier variant.
- **Milestone:** seeded long run asserting delivery of a `SpaceElevatorPart_*`
  product to its sink. Allowed to be slow; this is the test the tuning loop
  (upkeep, escalation rate, seed capital, floor price) iterates against.
- Tests for removed mechanisms (`shortage_test.go`, `sinks_test.go`, affected
  parts of `spawn_test.go` / `solvency_test.go`) are rewritten against the new
  mechanisms or dropped with their subjects.

## Known tuning surface

Bid-escalation rate vs. ask-decay rate is a genuine stability surface: set
badly, prices oscillate or the cascade stalls mid-chain. The cascade tests
exist to catch this. Tuning forks encountered during implementation are
surfaced to the user with a recommendation rather than decided silently.
