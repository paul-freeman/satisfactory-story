# Wallet-Grounded Bids + Water Nodes (Phase 5) — Design

**Goal:** reach the first *stable* space-elevator-part production line: a
`SpaceElevatorPart_1` delivery within 100k ticks at seed 152, followed by
sustained deliveries afterward.

## Problem

Phase 4 (see `2026-07-12-inventory-economy-design.md`, "Status (as
implemented)") proved the stock/spot-trade mechanism at small scale but
failed the full-scale milestone. The verified root cause: bid escalation
(`state/prices.go`, `bidRaisePct`) is uncapped, relying on the trade-time
wallet clamp to make runaway prices harmless — but `state/spawn.go`'s
`expectedProfit`/`estimatedUnitCost` read those escalated book prices at
face value to weight the spawn draw. Any permanently-unfillable bid
escalates without bound (~10^63 observed by tick ~7000) and recipes near
that dead end capture nearly all spawn weight, starving viable recipes —
including the Water-free `SpaceElevatorPart_1` chain the milestone needs.

A reachability probe over the shipped data quantifies the dead-end
surface (fixed point over `resources/Resource.json` node products and
active-recipe outputs):

- **59 products** demanded by active recipes are transitively
  unsourceable; **78 of 139 active recipes can never run**.
- Granting Water fixes only 15 of them: **44 dead ends remain**, and most
  are *permanently* unfixable because they come from game mechanics the
  sim does not model — creature drops (`HogParts`, `AlienProtein`),
  foraging (`Wood`, `Leaves`, `Mycelia`), FICSMAS event items
  (`XmasBall*`, `Snow`, `Gift`), nitrogen gas wells.

Conclusion: the market mechanism itself must tolerate permanent dead
ends. A data fix alone (Water) just moves the poison to the next dead
end. Water is still worth adding — it unlocks 22 recipes (61 → 83
runnable), including the oil/aluminum branch — but the load-bearing fix
is bounding bids by real money.

## Decision 1: wallet-grounded bid cap

**Invariant:** a factory's standing bid price for an input never exceeds
what its wallet could pay for the quantity it is asking for:

```
BidPrice(product) <= Cash() / Hunger(product, inputStockTargetTicks)
```

**Enforcement point:** `adjustPrices` (`state/prices.go`), the only place
bids escalate. The escalation becomes escalate-then-clamp:

```go
cap := buyer.Cash() / buyer.Hunger(product, inputStockTargetTicks)
buyer.SetBidPrice(product, math.Min(buyer.BidPriceFor(product)*(1+bidRaisePct), cap))
```

The clamp applies even when it *lowers* the price below the current one:
a factory whose wallet has drained bids less, so dying demand fades
honestly instead of screaming louder.

**Formula choice (decided):** `Cash / Hunger`, not
`Cash / (nInputs × Hunger)`. Each bid is individually payable in full;
combined bids across inputs may overpromise, but the trade-time wallet
clamp in `executeTrade` already resolves that. One concept, no wallet
partitioning.

**Edge cases:**
- Hunger ≤ epsilon at publish time: no bid is posted, so there is
  nothing to escalate. A *partially filled* bid can, however, reach the
  clamp with near-zero hunger (it just bought most of what it wanted):
  the cap then approaches `+Inf` (Go float division, no crash) and the
  clamp is a no-op. That is correct — a stocked, actively-trading buyer
  paying a high marginal price is honest, transient, and self-limiting.
- `Cash() ≤ 0`: cap ≤ 0, so the bid price is forced to ≤ 0 and can never
  match; insolvency removes the factory within `insolvencyGrace` anyway.
- Sink bids stay fixed at `goalBidUnitPrice` (1000), untouched — sinks
  are not factories and `adjustPrices` already skips them.
- Staleness: the cap reads the wallet as of end-of-tick (`adjustPrices`
  runs last in `Tick`); at most one tick of upkeep/purchases can make a
  posted bid briefly exceed the live cap. Harmless — the poisoning
  required thousands of ticks of compounding.

**Why this cures the poisoning with no estimator changes:** every price
in the book is now backed by money the buyer actually has, so
`expectedProfit` can keep trusting the book with zero special cases —
`state/spawn.go` is untouched. Dead-end bids (all 59 unsourceable
products) still escalate, but only to ~wallet/hunger — the same order of
magnitude as seed-capital pricing (~10–20/unit), not 10^63. Match
ordering (`MatchAll` sorts by bid price, so absurd bids no longer get
first claim and buy dust), the shortages panel, and spawn weighting all
become sane as side effects.

**The backward cascade survives:** a fresh factory's cap works out to
roughly the delivered cost it was seeded for (seed capital =
stock-target cost + upkeep runway, so `Cash/Hunger` ≈ estimated
delivered unit cost at spawn), and it rises as the factory earns.
Profitable demand bids more; failing demand bids less. That is the
price signal the design wants.

## Decision 2: Water resource nodes

`Water` has no producible source in the shipped data (water extractors
are buildings on open water, not resource nodes, so the SCIM export
cannot emit them). Fix the gap directly in `resources/Resource.json`:

- **8 synthetic entries with `"id": "waterPure"`** (120/min, like any
  pure node), placed on a 3×3 interior grid of the existing resource
  bounding box (lat −131.69..−28.58, lng 23.33..140.74) minus the center
  point, so no region is starved of water and transport distances stay
  comparable to ore:

| lat | lng |
|---|---|
| −105.91 | 52.68 |
| −105.91 | 82.03 |
| −105.91 | 111.39 |
| −80.13 | 52.68 |
| −80.13 | 111.39 |
| −54.36 | 52.68 |
| −54.36 | 82.03 |
| −54.36 | 111.39 |

- Add `"water" → "Water"` to `toCanonicalName`
  (`resources/resources.go`) so the id parses.

The coordinates are synthetic by necessity; economic behavior depends
only on spread and rate, not on matching real in-game lakes.

## Decision 3: milestone = sustained delivery

The long-run milestone subtest in `state/state_test.go` becomes this
phase's acceptance criterion, as a **hard assertion** (no more
`t.Skipf`):

1. First `SpaceElevatorPart_1` delivery within 100k ticks at seed 152.
2. At least **5 more units** delivered to the sink within the **20k
   ticks following** the tick of first delivery (via
   `sink.TotalDelivered()` deltas).

If the implementation cannot pass this, stop and report with diagnostics
(same bounded-tuning protocol as Phase 4) rather than tune blindly.

## Testing

- **Unit — cap clamps escalation:** an unfilled bid whose escalated
  price would exceed `Cash/Hunger` lands exactly on the cap.
- **Unit — cap pulls down:** an unfilled bid priced above a
  newly-shrunken wallet's cap is lowered to the cap on the next
  `adjustPrices`, even without escalation room.
- **Unit — water nodes load:** `resources.New()` yields 8 `Water`
  producers at rate 120/60 with the expected locations.
- **Regression:** existing cascade tests (`state/cascade_test.go`),
  `Test_state_determinism`, and the full suite stay green. The cap must
  not change any test where bids were already affordable.
- **Milestone:** the sustained-delivery assertion above.

## Out of scope

- NitrogenGas (or any other) additional resource nodes.
- Trade-grounded estimator (Approach B) — held in reserve if the cap
  alone does not reach the milestone.
- Reachability filtering of never-runnable recipes — rejected: it is a
  global graph traversal the design forbids the market to use, and
  exploration spawns on dead-end recipes are bounded-cost once bids are
  wallet-grounded.
- Any frontend or wire-format change — capped prices flow through the
  existing fields.
