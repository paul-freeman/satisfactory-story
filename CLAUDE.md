# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

An agent-based economic simulation of a [Satisfactory](https://satisfactorygame.com) factory network. A Go backend simulates producers (resource nodes and factories) that hold real stock, trade it spot on an order book, and relocate to minimize transport cost. A React + TypeScript + D3 single-page app visualizes the world on a tiled map and drives the simulation over HTTP.

## Commands

Backend (Go):

```bash
go run ./cmd/story        # start the simulation + HTTP server on :28100
go build ./...            # build everything
go test ./...             # run all tests
go test ./state -run Test_state_Tick   # run a single test
```

Frontend (React + TypeScript + D3):

```bash
cd frontend && npm install   # first time only
cd frontend && npm run dev   # dev server (Vite) with API proxy to :28100
cd frontend && npm run build # production build, served by the Go backend from frontend/dist
```

Run the backend and `npm run dev` together. In dev, Vite proxies `/state`, `/tick`, `/run`, `/stop`, `/reset`, `/recipes`, and `/recipe` to `http://localhost:28100` (`frontend/vite.config.ts`), so requests never leave Vite's own origin and no CORS is involved. In production, `go run ./cmd/story` itself serves the built app from `frontend/dist` (`state/http/http.go`), again same-origin. The `Access-Control-Allow-Origin: http://localhost:8000` header still hardcoded in `state/http/http.go` is a vestige of the old Elm reactor setup and is unused by the current dev or build flow.

## Architecture

### Simulation core (`state/state.go`)

`State` owns the whole world: `producers`, `recipes`, a `market.Book` (the live order book), a trade `ledger`, `sinks`, RNG seed, tick counter, and world bounds. It is guarded by a `sync.Mutex`; `Tick`, `Run`, `Stop`, and `Reset` all lock.

This is a **discrete-unit stock economy**, not a flow-contract one — see `docs/superpowers/specs/2026-07-12-inventory-economy-design.md` for the full design and the diagnosis that motivated replacing contracts with stock. Every resource and factory holds real inventory (`production.Inventory`, a `map[string]float64`); goods produced now sit in stock and stay buyable far into the future, instead of only existing transiently while a contract is live. Each `Tick` runs the full mechanism pipeline, in this order:

1. `produceGoods` runs one tick of physical production for every resource and factory (`ProduceTick`), consuming `InputStock` into `OutputStock`, before the market runs so fresh goods are sellable this same tick.
2. `publishOrders` rebuilds the order book (`market.Book`) from live stock every tick — the book is never carried over: unsold `OutputStock` becomes asks, unmet input "hunger" (`Hunger`, the gap to `inputStockTargetTicks`) becomes bids, sinks post standing bids for their goal product.
3. `matchOrders` crosses the book (`market.Book.MatchAll`) and `executeTrade` (`state/orders.go`) makes each match a real **spot trade**: quantity is re-clamped against live seller stock and the buyer's wallet (a hard budget — a buyer can never overdraw), units and money move immediately, and `s.ledger.record` logs it. There are no contracts and nothing to renegotiate; every trade is one-shot.
4. `moveProducers` hill-climbs factories on transport cost using recent trade locations.
5. `spawnNewProducer` (probability-gated by `spawnProbabilityPerTick`) picks a recipe by expected profit against the book (discounted by how crowded that recipe already is) and spawns it *idle*, near its currently-sourceable inputs, with seed capital — no sourcing happens at spawn; `publishOrders` posts its bids starting next tick.
6. `applySolvency` charges upkeep, credits salvage on output buffers that are full and unsold (production continues at a trickle, feeding the on-site AWESOME sink at `floorUnitPrice` — back-pressure that still signals no-demand upstream), and removes factories insolvent for `insolvencyGrace` ticks running.
7. `adjustPrices` lets sellers/buyers react locally to this tick's fill outcome: unsold asks decay toward marginal cost, sold-out asks rise; unfilled bids escalate, clamped to the wallet-grounded cap `Cash/Hunger` (see `docs/superpowers/specs/2026-07-16-wallet-grounded-bids-design.md`) so every posted price is backed by real money. Demand cascades backward through recipe tiers this way, one price signal at a time, with no global graph traversal.

`Run` launches a goroutine that ticks continuously until the cancellation context fires; `Stop` cancels it. The single cancel func is tracked via `setCancellationFunc`, which warns on double-set/double-clear.

### Producers and stock

`production.Producer` (`production/products.go`) is the central interface — anything that has a location, products, profit/profitability, capacity, and can trade. There are three concrete implementations:
- `resources.Resource` — a fixed ore/liquid node (from `Resource.json`); extracts into its own `Stock` every tick, only sells.
- `factory.Factory` — a recipe instance; holds both `InputStock` and `OutputStock`, buys inputs and sells outputs, and is the only `MoveableProducer` (implements `Move`/`Remove`).
- `sink.Sink` — a fixed demand point (e.g. `SpaceElevatorPart_1`); only buys, and tracks `TotalDelivered()` per product.

There are **no contracts**: a trade is a single spot exchange, cleared and settled in the same tick it matches (`state/orders.go`'s `executeTrade`), with nothing left to cancel or renegotiate afterward. The only state that persists between ticks besides stock itself is each producer's standing ask/bid prices (`AskPrices`/`BidPrices` on `factory.Factory`) and the rolling trade `ledger` (`state/trades.go`), which the HTTP layer uses to derive `Transport` links and "active" flags for the frontend.

### Recipes and data ingestion

Recipe and resource data are **embedded** via `//go:embed`:
- `recipes/Docs.json` — the game's exported recipe database (UTF-16LE; decoded with `golang.org/x/text`). Recipes prefixed `Alternate:` are parsed but left inactive.
- `resources/Resource.json` — resource node coordinates (see README for the SCIM console snippet that generates it). Purity suffix (`Impure`/`Normal`/`Pure`) maps to 30/60/120 rate; lat/lng are scaled ×1000 into integer `point.Point` coordinates.

The FactoryGame JSON encodes products as parenthesized strings, so `production.Products.UnmarshalJSON` and `recipes.Producer.UnmarshalJSON` do custom hand-parsing (`production/utils.go`). Note the documented hack: product durations are unknown at parse time, so amounts are stored with duration `1` and rates are corrected later against the recipe.

### HTTP boundary (`state/http/`)

`http.go` defines the `Server` interface the core satisfies, the wire types (`State`, `Resource`, `Factory`, `Sink`, `Transport`, `Recipe`), and the handlers. Endpoints: `/state`, `/tick`, `/run`, `/stop`, `/reset`, `/recipes`, `/recipe/{name}/{0|1}`. `/` serves the built frontend from `frontend/dist` (`http.FileServer(http.Dir("frontend/dist"))`).

`State.MarshalJSON` → `toHTTP` is the translation layer from the rich internal model to these flat wire types; it derives `Transport` links by aggregating recent trades from the ledger (`s.ledger.edges()`), not from any persistent link object. The wire types are intentionally lossy (e.g. only the first input/output is sent for a recipe).

### Frontend (`frontend/src/`)

`App.tsx` is the root component: it polls the backend, holds top-level state, and lays out the nav panels and map. `components/MapView.tsx` renders the pannable/zoomable D3 map over the PNG tiles (resources, sinks, factories, transport links). `hooks/usePolling.ts` drives the periodic `/state` fetch loop. `types.ts` holds TypeScript types matching the HTTP wire types, and `api.ts` wraps the `fetch` calls to `/state`, `/tick`, `/run`, `/stop`, `/reset`, `/recipes`, and `/recipe/{name}/{0|1}` (proxied to `http://localhost:28100` in dev via `frontend/vite.config.ts`, same-origin in the production build).

## Notes

- The default seed is hardcoded in `cmd/story/main.go` (`seed := 152`); the simulation is deterministic given the seed.
- Map tiles (`0_0.png` … `4_4.png`) are checked-in assets under `frontend/public/`, copied into `frontend/dist` by Vite's build; `frontend/dist` and `frontend/node_modules` are gitignored.
