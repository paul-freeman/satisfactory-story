# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

An agent-based economic simulation of a [Satisfactory](https://satisfactorygame.com) factory network. A Go backend simulates producers (resource nodes and factories) that source ingredients, sign contracts, and relocate to minimize transport cost. A React + TypeScript + D3 single-page app visualizes the world on a tiled map and drives the simulation over HTTP.

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

`State` owns the whole world: `producers`, `recipes`, a `market` (cheapest known price per product), `sinks`, RNG seed, tick counter, and world bounds. It is guarded by a `sync.Mutex`; `Tick`, `Run`, `Stop`, and `Reset` all lock.

Each `Tick` runs the full mechanism pipeline: `publishOrders` rebuilds the order book (`market.Book`) from live producer state (unsold capacity → asks, unmet inputs → bids, sinks → standing bids); `matchOrders` crosses it into `production.Contract`s; `moveProducers` hill-climbs on transport cost; `spawnNewProducer` (probability-gated) picks a recipe by expected profit against the book and spawns it *idle* — no sourcing at spawn; `renegotiateContracts` (probability-gated) re-shops existing contracts against residual asks; `applySolvency` charges upkeep, credits salvage (unsold producing capacity earns `floorUnitPrice` per unit — the AWESOME-sink buyer of last resort), cancels sales of non-producing factories, and removes the persistently insolvent; `adjustPrices` lets sellers/buyers react locally (unsold asks decay toward marginal cost, unfilled bids escalate within an affordability cap). Demand cascades backward through recipe tiers as escalating bids — see `docs/superpowers/specs/2026-07-11-order-book-market-design.md`.

`Run` launches a goroutine that ticks continuously until the cancellation context fires; `Stop` cancels it. The single cancel func is tracked via `setCancellationFunc`, which warns on double-set/double-clear.

### Producers and contracts

`production.Producer` (`production/products.go`) is the central interface — anything that has a location, products, profit/profitability, capacity, and can sign contracts. There are three concrete implementations:
- `resources.Resource` — a fixed ore/liquid node (from `Resource.json`), only sells.
- `factory.Factory` — a recipe instance; both buys inputs and sells outputs, and is the only `MoveableProducer` (implements `Move`/`Remove`).
- `sink.Sink` — a fixed demand point (e.g. `SpaceElevatorPart_1`), only buys.

Trade happens through `production.Contract`, a pointer shared by buyer and seller. Either side may `Cancel()`, so contracts must be checked for cancellation regularly (cancelled contracts are the primary signal for culling factories). `State.signContract` (`state/orders.go`) turns a matched order-book `market.Match` into a contract: it re-checks capacity, requires both `SignAsSeller` and `SignAsBuyer` to succeed, and records the traded unit price — the trade price itself is determined by matching (the ask's unit price), not computed at signing time.

### Recipes and data ingestion

Recipe and resource data are **embedded** via `//go:embed`:
- `recipes/Docs.json` — the game's exported recipe database (UTF-16LE; decoded with `golang.org/x/text`). Recipes prefixed `Alternate:` are parsed but left inactive.
- `resources/Resource.json` — resource node coordinates (see README for the SCIM console snippet that generates it). Purity suffix (`Impure`/`Normal`/`Pure`) maps to 30/60/120 rate; lat/lng are scaled ×1000 into integer `point.Point` coordinates.

The FactoryGame JSON encodes products as parenthesized strings, so `production.Products.UnmarshalJSON` and `recipes.Producer.UnmarshalJSON` do custom hand-parsing (`production/utils.go`). Note the documented hack: product durations are unknown at parse time, so amounts are stored with duration `1` and rates are corrected later against the recipe.

### HTTP boundary (`state/http/`)

`http.go` defines the `Server` interface the core satisfies, the wire types (`State`, `Resource`, `Factory`, `Sink`, `Transport`, `Recipe`), and the handlers. Endpoints: `/state`, `/tick`, `/run`, `/stop`, `/reset`, `/recipes`, `/recipe/{name}/{0|1}`. `/` serves the built frontend from `frontend/dist` (`http.FileServer(http.Dir("frontend/dist"))`).

`State.MarshalJSON` → `toHTTP` is the translation layer from the rich internal model to these flat wire types; it also prunes cancelled sales and derives per-contract `Transport` links. The wire types are intentionally lossy (e.g. only the first input/output is sent for a recipe).

### Frontend (`frontend/src/`)

`App.tsx` is the root component: it polls the backend, holds top-level state, and lays out the nav panels and map. `components/MapView.tsx` renders the pannable/zoomable D3 map over the PNG tiles (resources, sinks, factories, transport links). `hooks/usePolling.ts` drives the periodic `/state` fetch loop. `types.ts` holds TypeScript types matching the HTTP wire types, and `api.ts` wraps the `fetch` calls to `/state`, `/tick`, `/run`, `/stop`, `/reset`, `/recipes`, and `/recipe/{name}/{0|1}` (proxied to `http://localhost:28100` in dev via `frontend/vite.config.ts`, same-origin in the production build).

## Notes

- The default seed is hardcoded in `cmd/story/main.go` (`seed := 152`); the simulation is deterministic given the seed.
- Map tiles (`0_0.png` … `4_4.png`) are checked-in assets under `frontend/public/`, copied into `frontend/dist` by Vite's build; `frontend/dist` and `frontend/node_modules` are gitignored.
