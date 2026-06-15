# Observability

Every server command (`serve`, `proxy`, `echo`, `mock`) exposes request metrics
and a liveness endpoint on a dedicated **admin port** — separate from the
application port.

```bash
radix serve                          # app on :8080, admin on 127.0.0.1:9090
curl http://127.0.0.1:9090/healthz
curl http://127.0.0.1:9090/_metrics
```

## The admin port

The admin server defaults to port `9090` and always binds **loopback
(`127.0.0.1`)** — even when the app binds `0.0.0.0`.

::: tip Why a separate, loopback-only port?
Keeping metrics and health off the application listener means telemetry never
leaks to the public when you bind the app to `0.0.0.0`, and your metrics path
can't collide with a real application route. Scrapers and health checks run on
the machine, so loopback is enough.
:::

Change or disable it:

```bash
radix proxy http://localhost:3000 --metrics-port 9091   # custom admin port
radix serve --metrics-path /internal/metrics            # custom path
radix echo --metrics=false                              # no admin server at all
```

::: warning Metrics moved off the app port
The application port **no longer serves `/_metrics`**. Point scrapers and health
checks at `127.0.0.1:9090` (or your `--metrics-port`), not the app port. As a
compatibility shim, `echo` and `mock` still serve legacy `/_health` and
`/_ready` on the app port.
:::

`--metrics-port` must differ from the app `--port`, must be `1..65535`, and the
metrics path must start with `/` and can't be the reserved `/healthz`.

::: tip Prefer a UI?
The admin server also serves a live web dashboard at the same address
(`http://127.0.0.1:9090`) that reads these same metrics. See the
[Metrics dashboard](/guides/dashboard) guide.
:::

## Health check

```bash
curl http://127.0.0.1:9090/healthz
```

```json
{"status":"ok","uptime":"1s","version":"dev"}
```

## Metrics: JSON or Prometheus

Default is JSON; switch with `--metrics-format prometheus`.

### JSON

```bash
curl http://127.0.0.1:9090/_metrics
```

```json
{
  "server": { "command": "echo", "uptime_seconds": 1.01, "start_time": "2026-06-14T21:33:03-07:00", "version": "dev" },
  "requests": { "total": 1, "success": 1, "errors": 0, "rate_per_second": 0.98 },
  "status_codes": { "OK": 1 },
  "methods": { "POST": 1 },
  "response_times": { "min_ms": 0.847, "max_ms": 0.847, "avg_ms": 0.847, "p50_ms": 0.847, "p95_ms": 0.847, "p99_ms": 0.847, "count": 1 },
  "bandwidth": { "bytes_sent": 1042, "bytes_received": 14, "avg_request_size_bytes": 14, "avg_response_size_bytes": 1042 },
  "command": { "echo": { "delays_applied": 0, "custom_body_responses": 0, "path_status_hits": 0 } }
}
```

The `command` block is per-command — here `echo`; `mock` and `proxy` have their
own. `serve` has no `command` block.

### Prometheus

```bash
radix mock --metrics-format prometheus
curl http://127.0.0.1:9090/_metrics
```

```
# HELP radix_requests_total Total number of HTTP requests
# TYPE radix_requests_total counter
radix_requests_total{command="mock"} 1
# HELP radix_mock_route_matches_total Mock route matches by kind
# TYPE radix_mock_route_matches_total counter
radix_mock_route_matches_total{command="mock",kind="builtin"} 1
radix_mock_route_matches_total{command="mock",kind="custom"} 0
```

Metric families: `radix_server_info`, `radix_server_uptime_seconds`,
`radix_requests_total`, `radix_requests_by_status_total`,
`radix_requests_by_method_total`, `radix_response_time_milliseconds` (summary),
`radix_bytes_sent_total`, `radix_bytes_received_total`,
`radix_request_rate_per_second`, plus the per-command families below.

## Per-command counters

On top of the shared request metrics, each command tracks its own activity
(`serve` has none):

| Command | Counters (JSON keys → Prometheus) |
|---------|-----------------------------------|
| `echo` | `delays_applied`, `custom_body_responses`, `path_status_hits` → `radix_echo_*_total` |
| `mock` | route matches (`builtin`/`custom`), `template_renders`, `template_errors`, `reloads`, `fail_injections`, fallback (`not_found`/`proxy`) → `radix_mock_*` |
| `proxy` | `auth_injections`, `stream_connections` → `radix_proxy_*` |
