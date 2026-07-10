# Frontend v2 (Phase 2) — Design

## Goal

Replace the Elm frontend with a TypeScript + React + D3 app, per the
decision recorded in `docs/superpowers/specs/2026-07-10-economic-engine-v2-design.md`
(assistant fluency in this stack, and the app's needs — tiled map, live
overlay, polling, a few controls — are a well-trodden combination for it).
Alongside the port, add two visualizations Phase 1 makes possible: factory
financial health on the map, and a shortage/demand panel.

Scope: replicate every feature of the current Elm app (pan/zoom map, tile
rendering, resource/factory/sink markers, transport lines, tick counter,
run/stop/tick/reset controls, recipe toggle checkboxes, stats panel) plus
the two new visualizations below. Once verified working with feature
parity, delete the Elm app (`src/*.elm`, `elm.json`) — the user is not
attached to it and maintaining two parallel frontends has no value.

## 1. Project structure & tooling

New `frontend/` directory at the repo root, independent of the Go module:

```
frontend/
  package.json          — Vite + React + TypeScript + D3
  vite.config.ts        — dev server with a proxy for /state, /tick, etc. -> :28100
  src/
    main.tsx
    App.tsx
    api.ts               — typed fetch wrappers for every backend endpoint
    types.ts             — wire types, mirroring state/http/http.go exactly
    hooks/usePolling.ts  — the run/stop-aware polling loop
    components/
      MapView.tsx        — SVG map, D3 zoom/pan, tiles, markers, transport lines
      NavLeft.tsx         — tick counter, Run/Stop/Tick/Reset
      NavRight.tsx        — stats, recipe checkboxes
      ShortagePanel.tsx   — new: toggleable shortage list
  index.html
```

`npm run dev` runs Vite's dev server (proxying API calls to the Go backend
on `:28100`, replacing `elm reactor`). `npm run build` outputs to
`frontend/dist/`, which `state/http/http.go`'s existing
`http.FileServer(http.Dir("."))` will serve — same deployment story as
today's `index.html`, just pointed at the new build output (exact
mechanism — serving `frontend/dist` directly vs. a build step that copies
`dist/*` to the repo root — to be settled during implementation, whichever
reads cleaner against the existing `Serve` function).

No test framework is set up by default (Vitest could be added later, but
nothing here has complex enough logic to demand it up front).

## 2. Map rendering (D3 zoom/pan)

The current Elm version hand-rolls panning/zooming by recomputing an SVG
`viewBox` on every wheel/drag event. D3's standard pattern is cleaner: the
`<svg>` element stays a fixed size, and `d3-zoom` manages a single
transform (`translate(x,y) scale(k)`) applied to one wrapping `<g>` that
contains everything — tiles, markers, transport lines. Panning and zooming
become "update one transform," not "recompute a viewBox by hand," and D3
handles smooth wheel/drag/touch input for free.

Layering (back to front, matching today's z-order exactly):
1. Map tiles (`0_0.png` … `4_4.png`, same 5×5 grid, same world-space
   positioning: corner at `(0, -160300)`, each tile `32000` world units).
2. Inactive resource circles, then active resource circles.
3. Transport lines.
4. Labels (factory/sink text), then circles on top of labels — preserving
   the current app's "label under marker" order.

Coordinate system: the backend's `y` grows in the opposite direction from
SVG's, so every drawer today does `ymin + ymax - location.y`. This flip is
kept as a small `toSvgY(bounds, y)` helper rather than fought — a
one-line, well-understood convention already proven to work with the
existing tile images.

Marker color encoding (extends the existing
profitable=blue/unprofitable=purple/inactive=grey/sink=orange scheme):
factories get a red→green gradient by `Cash` sign and magnitude instead of
a flat two-color split — strongly negative (near bankruptcy) reads red,
healthy positive reads green. The exact scale is normalized against the
current tick's factory population (e.g. a D3 diverging color scale like
`d3.interpolateRdYlGn` domain-fit to the min/max cash observed that tick)
so it stays meaningful as the economy's scale changes over a run, rather
than a hardcoded threshold that could saturate to all-one-color. Exact
domain-fitting details to be tuned visually during implementation.

## 3. Backend wire format additions

Two small, additive changes to `state/http/http.go` and `state.toHTTP()` —
nothing else in Phase 1 reopens:

```go
type Factory struct {
	Location      Location `json:"location"`
	Recipe        string   `json:"recipe"`
	Products      []string `json:"products"`
	Profitability float64  `json:"profitability"`
	Cash          float64  `json:"cash"`          // new
}

type Shortage struct {
	Product string  `json:"product"`
	Amount  float64 `json:"amount"`
}

type State struct {
	// ...existing fields...
	Shortages []Shortage `json:"shortages"` // new
}
```

`toHTTP()` fills `Cash` from the existing `producer.Cash()` (already on
every `Factory` via its embedded `Wallet` — no new backend logic needed,
just wiring it through) and builds `Shortages` from `s.unmet`, sorted by
`Amount` descending, capped at a reasonable count (e.g. top 20) so the
panel doesn't get flooded once dozens of products have small residual
shortages.

This keeps the wire format backward-compatible (pure additions) and
doesn't touch anything from Phase 1's simulation logic — just exposes two
things that already exist internally.

## 4. Data flow & state management

Plain React state + one custom hook, no external state library (the app's
needs — one polled blob of state, a handful of controls — don't justify
Redux/Zustand):

- `usePolling()` owns the `State` object and mirrors the Elm app's exact
  behavior: fetch on mount, and after any state-returning response, if
  `running` is true, wait 50ms and fetch `/state` again; if not running,
  stop (matching today's `sleepAndPoll` exactly, including the interval).
- Button handlers (Run/Stop/Tick/Reset/recipe toggle) call the
  corresponding endpoint via `api.ts` and feed the result back into the
  same state update path — same one-way flow as the Elm `update` function,
  just as a React state setter instead of a message.
- `ShortagePanel` reads `state.shortages` directly off the polled state —
  no separate fetch or hook needed since it now rides along on `/state`.

## UI layout (matches current app)

- Left nav (200px): tick counter; Stop button when running, or
  Run/Tick/Reset buttons when stopped.
- Center: the map (SVG, D3 zoom/pan).
- Right nav (200px): stats (resource/factory/sink/transport counts,
  inactive count), recipe checkboxes (Alternate: recipes first, then
  normal, each alphabetically sorted, each toggling `/recipe/{name}/{0|1}`),
  and the new toggleable shortage panel.
