# Economic Engine v2 — Design

## Goal

Rebuild the simulation so factories self-organize based on resource-node
location and transport cost, and so multiple independent producers of the
same product can coexist by finding their own geographic niche — a "free
market" that feels lived-in rather than one that converges on a single
optimal answer. Perfect optimization is explicitly not a goal.

## Why the current implementation doesn't get there

Investigation of the current code (see conversation history / git blame for
detail) turned up two bugs and three structural gaps that together explain
why the simulation "never really produced anything useful":

1. **No sinks exist at runtime.** `sink.New` is never called anywhere;
   `State.sinks` is just a `map[string]int` used as a cull-time protection
   count, not a real buyer. There is no genuine terminal demand anywhere in
   the economy.
2. **`Factory.Profit()` ignores `ProductCost` entirely** — it only nets
   `TransportCost` on both sides, so the exact function used to rank and cull
   factories never reflects real revenue or input cost.
3. **Spawning is undirected** — a uniformly random recipe is chosen every
   spawn tick regardless of whether its output has any buyer.
4. **Factories have no output-capacity limit**, unlike `Resource`, so the
   single cheapest producer of any input can be sold to indefinitely, which
   prevents any other producer of the same product from ever getting a
   contract — this alone rules out niches forming.
5. **Tick phases are coarse** (`(tick / 3000) % 3`): spawning, moving, and
   culling each run exclusively for 3000-tick blocks, so nothing moves or
   gets culled during the first 3000 ticks of any run.

## Guiding principle

Replace "instantaneous profit snapshot + omniscient cheapest-source search"
with **real capital + capacity-bounded local competition**. Two mechanisms
do most of the work:

- **Wallets** — factories hold real cash that rises and falls every tick.
  Death (bankruptcy) is a gradual process, not a snapshot judgment, so a
  struggling factory can persist for a while, undercut a rival, and then
  fail — this is what makes the world feel lived-in rather than computed.
- **Real capacity limits** — a producer can only sell up to its actual
  output rate. Once the cheapest producer of a product sells out, the next
  buyer is forced to the next-cheapest-with-capacity option, which is often
  a geographically distinct producer. Regional niches emerge from this
  directly, without modeling "regions" as an explicit concept.

Demand-driven spawning, contract renegotiation, and continuous ticking all
exist in service of letting that core loop actually play out.

## Scope and phasing

This is a full rebuild of the backend simulation core (`state`, `factory`,
`resources`, `sink`, `production`, `recipes`), followed by a full rewrite of
the frontend on a new stack. The backend must land first — the frontend
rewrite depends on the new wire format the backend model produces, and the
concrete visualization needs will be clearer once the new mechanics have
actually been run and observed.

- **Phase 1 (backend):** new economic engine, sections 1–5 below.
- **Phase 2 (frontend):** React + TypeScript + D3 rewrite, section 6 below.

## 1. Data model changes

**Wallets.** The `Producer` interface gains `Cash() float64`. `Resource` and
`Sink` report effectively infinite cash and never go bankrupt (matching
today's `IsRemovable() == false` for those types). `Factory` gets a real
`Cash float64` field:

- **Seed capital** at spawn time is funded from an estimate of one
  production cycle's input cost at the chosen location, times a small
  buffer constant — representing the up-front capital needed to start the
  business.
- **Every tick**, `Cash += revenue_this_tick - expenses_this_tick`, computed
  from each active contract's `ProductCost` (not just `TransportCost` — this
  is the direct fix for bug #2 above).
- **Bankruptcy**: if `Cash` stays negative continuously for an
  `insolvencyGrace` window of ticks, the factory is removed (all contracts
  cancelled, as today). A brief dip below zero is tolerated.

**Real sinks.** Instantiate actual `sink.New(...)` producers at startup for
the space-elevator part products, at real locations, with unlimited buying
appetite and deep pockets — genuine terminal demand that pays real money
back through the whole chain (fixes bug #1).

**Capacity-bounded factories.** `Factory.HasCapacityFor` tracks committed
sale rate against `Output.Rate`, mirroring `Resource.HasCapacityFor` today.
A factory cannot be oversold beyond what it actually produces (fixes bug
#4) — this is the mechanism that forces buyers to spread across multiple
sellers once the cheapest one is full, which is how niches form.

**Shortage signal.** A new registry on `State`: `unmet map[string]float64`,
product name → unmet demand rate. Updated whenever `Recipe.SourceProducts`
fails to fully source an input, or a sink wants more than it's currently
getting. Decays slowly each tick so stale shortages fade. This is the
"opportunity" signal that drives demand-weighted spawning (section 3) and is
also useful to expose to the frontend later (section 6).

## 2. Continuous tick loop

Drop the `(tick / 3000) % 3` phase switch. Every tick does a little of
everything, each gated by its own small independent probability (named
constants, tuned during implementation, not magic numbers scattered in
code):

- **Spawn check** (small chance/tick): attempt one new factory via the
  demand-weighted picker (section 3).
- **Move check** (small chance per movable factory): today's hill-climb
  toward lower total transport cost is sound and carries over unchanged,
  just decoupled from the phase gate.
- **Renegotiate check** (small chance per factory): re-evaluate each input
  contract; if a cheaper-with-capacity seller exists that beats the current
  price by more than a minimum margin (to avoid thrashing on noise), cancel
  the old contract and sign the new one.
- **Solvency check** (every tick, all factories): apply revenue/expense to
  `Cash`; cull anyone past their insolvency grace window, or with a
  cancelled/incomplete input contract (contract-integrity checks, as today).

This produces a constant low-level hum of small events — new factories
starting, some failing, some relocating, some switching suppliers — visible
on every tick rather than only once every few thousand.

## 3. Demand-weighted spawning

Replace `recipes[uniform_random]` with:

1. Sample a candidate product weighted toward larger entries in
   `state.unmet` (plus sinks' perpetual demand), with a baseline weight for
   everything else so novel/random ventures still happen sometimes — the
   goal is *opportunity-biased*, not purely reactive or optimal.
2. Pick recipe(s) that produce the chosen product; pick a candidate location
   (near the source of the unmet demand when there is shortage data, falling
   back to today's random-point choice early in a run or when there's no
   shortage signal yet).
3. Source inputs as today (cheapest-with-capacity, now capacity-bounded).
   Fund the new factory with seed capital and only actually spawn it if
   projected margin (output sale price vs. sourced input cost) is positive
   — a doomed business simply doesn't start, rather than starting and being
   culled later.

## 4. Contracts and culling

Contract formation keeps today's transport-cost-based pricing and markup
logic (`writeContract`), now capacity-checked on both sides. Culling
collapses to two paths only: bankruptcy (section 1) and contract-integrity
failures (cancelled/incomplete inputs). The old rank-by-profit /
keep-top-N-per-sink-count logic is removed entirely — real capital and real
capacity already produce the right survivor set without an artificial rule.

## 5. Testing strategy

**Unit tests** for each new piece in isolation:
- Wallet debit/credit arithmetic across a tick.
- Bankruptcy triggers after the grace window elapses, and does *not*
  trigger on a brief dip below zero.
- Capacity-bounded `HasCapacityFor` rejects an order that would oversell a
  factory's output rate.
- Renegotiation only switches suppliers when the new offer beats the old by
  more than the minimum margin (no thrashing on noise).
- Shortage-weighted recipe/product selection favors products with larger
  `unmet` entries without making low-shortage products impossible to pick.

**One integration-style test** extending `Test_state_Tick`: run a long tick
range and assert the properties that used to be silently false:
- Sinks receive nonzero deliveries of a final product over the run.
- No producer's committed sale rate ever exceeds its output rate.
- Multiple independent producers of at least one product coexist
  simultaneously at the end of the run (evidence of niches, not monopoly).

## 6. Frontend rewrite (Phase 2)

Replace the Elm frontend (`src/Main.elm`, `src/CustomSvg.elm`,
`src/Types.elm`) with **TypeScript + React + D3**, decided because the app's
needs — a pannable/zoomable SVG map over the existing PNG tiles, a
live-updating overlay of a few hundred markers/lines, HTTP polling, a
handful of buttons/toggles — are a well-trodden combination for that stack,
and `d3-zoom`/`d3-scale` reproduce what `CustomSvg.elm` hand-rolls for
pan/zoom today, so the visualization concept carries over directly rather
than being redesigned.

Concrete wire-format additions (e.g. `Factory.Cash`, an `unmet`/shortage
overlay) are deferred until Phase 1 is running and observed — speculating on
exact JSON shapes before the mechanics exist and have been tuned isn't
useful. This section will be revisited with its own design pass once Phase
1 is complete.
