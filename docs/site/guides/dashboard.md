# Metrics dashboard

Every server command (`serve`, `proxy`, `echo`, `mock`) ships with a live
metrics dashboard — a self-contained web UI built into the radix binary. When
metrics are enabled (the default), just open the admin port in a browser and
watch requests, latency, status codes, and bandwidth update in real time. No
extra tooling, no flags, no internet.

![The Radix metrics dashboard: a KPI row, a request-rate chart, a status-code bar chart, latency percentiles, bandwidth, and per-command counters](/images/dashboard-light.png)

```bash
radix serve                          # app on :8080, admin on 127.0.0.1:9090
# then open http://127.0.0.1:9090 in your browser
```

The dashboard is served from the same **admin port** as `/_metrics` and
`/healthz` — `127.0.0.1:9090` by default. It behaves identically under `serve`,
`proxy`, `echo`, and `mock`. See [Observability](/guides/observability) for the
admin port, the raw endpoints, and the metrics formats behind it.

::: tip Fully offline
The dashboard is a React app compiled into the binary — there's no separate
process to start and no CDN or network dependency. It works on a machine with no
internet access.
:::

## What you see

The dashboard polls `/_metrics` every **2 seconds** and keeps a rolling
**~2-minute** history in your browser, so the charts animate as traffic flows.

| Panel | What it shows |
|-------|---------------|
| **KPI row** | Total requests, errors + error rate, p50 latency, requests/s, and uptime |
| **Request rate** | An area chart of requests/s, with errors plotted as a separate per-second rate |
| **Status codes** | A bar chart with one bar per response code, colored by HTTP class (2xx, 3xx, 4xx, 5xx) |
| **Latency** | p50, p95, and p99, with a bar placing p95 on a 0–1s scale |
| **Bandwidth** | Bytes in and out, in human-readable units (KB / MB / GB) |
| **Command counters** | Per-command activity for `mock`, `proxy`, and `echo` — the same data as the [per-command counters](/guides/observability#per-command-counters) table |

The command-counters panel appears only for `mock`, `proxy`, and `echo`; `serve`
has no per-command counters, so it isn't shown there.

If the admin server is briefly unreachable, the dashboard shows a banner and
keeps retrying — it reconnects on its own once metrics are back.

## Light and dark

The dashboard themes with **Catppuccin** — Latte (light) and Frappé (dark). A
toggle in the header switches between them; your choice is saved to
`localStorage`. With no saved choice, it follows your OS `prefers-color-scheme`.

![The dashboard in its dark Catppuccin Frappé theme](/images/dashboard-dark.png)

## Requires JSON metrics

::: warning Use the JSON format for the dashboard
The dashboard renders the **JSON** metrics format, which is the default. If you
set `metrics.format: prometheus` (or `--metrics-format prometheus`), the admin
server emits Prometheus text that the dashboard can't render — it shows a clear
"needs JSON metrics" message instead. Keep `metrics.format: json` for the UI.
Prometheus scraping still works at `/_metrics`.
:::

## Loopback only

Like the rest of the admin server, the dashboard binds **`127.0.0.1`** only —
even when the app binds `0.0.0.0`. It's reachable from your machine, not from the
network, so its metrics never leak to the outside. See
[why a separate, loopback-only port](/guides/observability#the-admin-port).

Disabling the admin server turns the dashboard off along with the endpoints:

```bash
radix serve --metrics=false          # no admin server, no /_metrics, no dashboard
```
