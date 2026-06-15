# Radix Metrics Web UI — Design Spec

**Date:** 2026-06-15
**Status:** Approved

---

## Problem

Radix exposes usage metrics as JSON at `http://127.0.0.1:9090/_metrics`, readable only via `curl` or a Prometheus scraper. There is no human-readable view. Developers running `radix mock` or `radix proxy` have no quick way to see request counts, error rates, or latency while iterating locally.

## Goal

Add a self-contained metrics dashboard, served directly from the existing admin server at `http://127.0.0.1:9090`, that auto-refreshes and requires zero extra tooling to open.

---

## Tech Stack

| Layer | Choice |
|---|---|
| Bundler | Vite 6 + React 19 + TypeScript |
| UI base | shadcn/ui + Tailwind CSS v4 |
| Charts | Recharts (shadcn's own chart examples use it; `ResponsiveContainer` handles grid layout cleanly) |
| Theme | Catppuccin Frappe (dark) / Catppuccin Latte (light), toggled via CSS custom properties |

---

## Architecture

### Delivery

The React SPA is compiled to static files in `ui/dist/`, which are embedded into the Go binary via `//go:embed`. The admin server on `:9090` gains two new behaviours:

1. `/` → serves `index.html` (SPA shell)
2. All non-API paths → SPA fallback (returns `index.html` for any unknown path so deep links work)
3. `/_metrics` → existing JSON handler (unchanged), with one new `Access-Control-Allow-Origin: *` header so the Vite dev server on `:5173` can reach it during local UI development
4. `/healthz` → unchanged

A `ui/dist/.gitkeep` placeholder is committed so `go build` compiles without requiring Node. Attempting to load the UI from a placeholder build shows a minimal "run `make ui` first" message.

### Data refresh

`/_metrics` returns a point-in-time snapshot — it has no built-in history. The client accumulates polling results into a **rolling 2-minute ring buffer** (60 samples at 2-second intervals) held entirely in React state. The request-rate area chart renders from this buffer. No Go changes needed for time-series.

Polling lives in a single `useMetrics` hook. The interval is a named constant (`POLL_INTERVAL_MS = 2000`) at the top of the file.

---

## Page Layout

Single scrollable page, no routing needed.

```
┌─────────────────────────────────────────────────────┐
│  ⬡ Radix Metrics  [mock]  ● live   ↻ 2s  [☀/☾]   │  Header
├──────────┬──────────┬──────────┬──────────┬─────────┤
│ Requests │  Errors  │ p50 lat. │  Rate/s  │ Uptime  │  KPI row
├──────────┴──────────┼──────────┴──────────┴─────────┤
│  Request rate       │  Status codes                  │  Charts row
│  (area chart)       │  (horiz. bar)                  │
├─────────────────────┼────────────────────────────────┤
│  Latency p50/95/99  │  Bandwidth in / out            │  Stats row
├─────────────────────┴────────────────────────────────┤
│  Command-specific (mock / proxy / echo)              │  Conditional
└──────────────────────────────────────────────────────┘
```

**KPI cards:** Total requests · Error count + rate · p50 latency · Req/s · Uptime  
**Request rate chart:** Recharts `AreaChart` over the client-side ring buffer  
**Status codes:** Recharts horizontal `BarChart` — one bar per status text (OK, Not Found, …)  
**Latency panel:** Three stat boxes (p50 / p95 / p99) + a colour-coded range bar (green→yellow→red)  
**Bandwidth panel:** Bytes sent / received, formatted (KB/MB/GB)  
**Command stats:** Rendered only when `snapshot.command` is non-null; shows command-specific counters as a simple stat grid

---

## Theme

Two Catppuccin palettes wired as Tailwind CSS custom properties:

**Dark (Frappe):**  
`--ctp-base: #303446` · `--ctp-surface: #414559` · `--ctp-text: #c6d0f5` · `--ctp-subtext: #a5adce` · `--ctp-overlay: #737994` · `--ctp-blue: #8caaee` · `--ctp-lavender: #babbf1` · `--ctp-green: #a6d189` · `--ctp-yellow: #e5c890` · `--ctp-red: #e78284` · `--ctp-border: #51576d`

**Light (Latte):**  
`--ctp-base: #eff1f5` · `--ctp-surface: #e6e9ef` · `--ctp-text: #4c4f69` · `--ctp-subtext: #6c6f85` · `--ctp-overlay: #9ca0b0` · `--ctp-blue: #1e66f5` · `--ctp-lavender: #7287fd` · `--ctp-green: #40a02b` · `--ctp-yellow: #df8e1d` · `--ctp-red: #d20f39` · `--ctp-border: #ccd0da`

Theme is stored in `localStorage` and applied by toggling a `data-theme` attribute on `<html>`. `useTheme` hook reads system preference (`prefers-color-scheme`) as the default.

---

## File Tree

### New (frontend)

```
ui/
├── package.json
├── vite.config.ts          # proxy /_metrics → http://localhost:9090 in dev
├── tsconfig.json
├── tailwind.config.ts      # Catppuccin vars mapped to Tailwind color tokens
├── index.html
├── src/
│   ├── main.tsx
│   ├── App.tsx             # root layout, ThemeProvider wrapper
│   ├── index.css           # @layer base: Catppuccin CSS custom properties
│   ├── types/
│   │   └── metrics.ts      # TypeScript types mirroring Go Snapshot struct
│   ├── hooks/
│   │   ├── useMetrics.ts   # fetch loop, ring buffer, error state
│   │   └── useTheme.ts     # dark/light toggle + localStorage + system default
│   └── components/
│       ├── Header.tsx              # logo, command badge, live dot, theme toggle
│       ├── KpiCard.tsx             # reusable stat card
│       ├── RequestRateChart.tsx    # Recharts AreaChart over ring buffer
│       ├── StatusCodesChart.tsx    # Recharts horizontal BarChart
│       ├── LatencyPanel.tsx        # p50/p95/p99 + range bar
│       ├── BandwidthPanel.tsx      # bytes in/out with human-readable formatting
│       └── CommandStats.tsx        # conditional mock/proxy/echo counters
└── dist/
    └── .gitkeep            # allows go build without node
```

### Modified (Go)

```
internal/server/
└── ui.go          # NEW: //go:embed ui/dist; http.FileServer with SPA fallback; CORS on /_metrics

Makefile           # add `ui` target; add as dep of `build`
```

---

## Build Integration

```makefile
.PHONY: ui
ui:
	cd ui && npm ci && npm run build

build: ui
	go build -o bin/radix ./cmd/radix
```

CI runs `make build` which includes `make ui`. `ui/dist/.gitkeep` ensures `go build` alone (without node) still compiles — the embedded filesystem just serves the placeholder page.

For UI-only development: `cd ui && npm run dev` starts Vite on `:5173` with `/_metrics` proxied to `:9090` (requires radix to be running separately).

---

## Go Changes (minimal)

`internal/server/ui.go` (new file):
- `//go:embed all:ui/dist` directive
- `ServeUI(mux *http.ServeMux)` function that registers:
  - A file server for all static assets
  - SPA fallback: any 404 on a non-API path returns `index.html`
  - CORS header (`Access-Control-Allow-Origin: *`) on `/_metrics` responses

`internal/server/admin.go` (tiny change):
- Call `ServeUI(mux)` after registering `/_metrics` and `/healthz`

---

## TypeScript Types

```typescript
// mirrors internal/metrics/collector.go Snapshot()
interface MetricsSnapshot {
  server: { command: string; uptime_seconds: number; start_time: string; version: string }
  requests: { total: number; success: number; errors: number; rate_per_second: number }
  status_codes: Record<string, number>
  methods: Record<string, number>
  response_times: { min_ms: number; max_ms: number; avg_ms: number; p50_ms: number; p95_ms: number; p99_ms: number; count: number }
  bandwidth: { bytes_sent: number; bytes_received: number; avg_request_size_bytes: number; avg_response_size_bytes: number }
  command: {
    echo?: { delays_applied: number; custom_body_responses: number; path_status_hits: number }
    mock?: { route_matches_builtin: number; route_matches_custom: number; template_renders: number; template_errors: number; reloads: number; fail_injections: number; fallback_not_found: number; fallback_proxy: number }
    proxy?: { auth_injections: number; stream_connections: number }
  } | null
}
```

---

## Verification

1. `make ui` — builds `ui/dist/` without errors
2. `make build` — Go binary compiles with embedded UI
3. `./bin/radix mock examples/mock-routes.yml` — start radix
4. Open `http://127.0.0.1:9090` in browser — dashboard renders, dark/light toggle works
5. Send a few requests (`curl http://localhost:8080/anything`) — KPIs update within 2s, request rate chart accumulates points
6. `cd ui && npm run dev` with radix running — Vite dev server on `:5173` proxies metrics correctly
7. `make test` — existing Go tests still pass (no regressions in admin server)
