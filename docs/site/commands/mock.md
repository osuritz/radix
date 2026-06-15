# radix mock

Run a mock API: built-in httpbin-style endpoints, plus optional custom YAML
routes that take precedence over them.

```bash
radix mock [config-file] [flags]
```

With no arguments you get the built-in endpoints on
`http://localhost:8080`. Pass a routes file (positionally
or with `--routes`) to add your own.

## When to use it

Stand up a fake API in seconds: hit `/uuid` or `/status/503` while testing a
client, add latency or random failures to exercise retries, or define custom
routes with templated bodies to mimic a real endpoint your frontend depends on.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--builtin` | `true` | Register the built-in endpoints |
| `--prefix` | | Mount built-ins under a prefix (`/_test` → `/_test/get`) |
| `--routes`, `-r` | | YAML routes file (also accepted positionally) |
| `--watch`, `-w` | `false` | Hot-reload the routes file on change |
| `--latency` | `0` | Fixed artificial latency (e.g. `200ms`, `1s`) |
| `--latency-jitter` | `0` | Random jitter added to the latency |
| `--fail-rate` | `0` | Random failure rate, percentage 0–100 |
| `--fail-status` | `500` | Status returned for random failures |
| `--cors` | `false` | Add permissive CORS headers |

Global flags apply too — see [Configuration](/configuration#global-flags).

::: warning
Pass the routes file *either* positionally *or* via `--routes`, not both.
Explicitly-set CLI flags always override the same setting in a routes file's
`settings:` block.
:::

## Built-in endpoints

| Endpoint | Returns |
|----------|---------|
| `/get` `/post` `/put` `/patch` `/delete` | Request echo for that method |
| `/anything` | Request echo for any method |
| `/headers` | The request headers |
| `/ip` | The client IP |
| `/user-agent` | The `User-Agent` header |
| `/uuid` | A random v4 UUID |
| `/status/{code}` | A response with that status code |
| `/delay/{n}` | Responds after `n` seconds |
| `/bytes/{n}` | `n` bytes of data |
| `/json` `/html` `/xml` | A sample payload in that format |

## Custom routes

Custom routes take precedence over the built-ins and support exact, `:param`,
`regex:`, and trailing `/*` glob paths; templated bodies; per-route delays;
conditional responses; sequenced and weighted-random responses; SSE; and a `404`
or `proxy` fallback.

```bash
radix mock examples/mock-routes.yml
radix mock --routes examples/mock-routes.yml --watch   # hot-reload on save
```

See the [Mock guide](/guides/mock) for matching priority, the full template
function set, conditions, sequences, weighted random, and SSE.

## Examples

### Add latency and chaos

```bash
radix mock --latency 200ms --latency-jitter 100ms --fail-rate 10
```

10% of responses fail with `500` (change with `--fail-status`); the rest get
200–300ms of latency.

### Mount built-ins under a prefix

```bash
radix mock --prefix /_test
curl localhost:8080/_test/get
```

::: tip
`/_health` and `/_ready` stay at the root regardless of `--prefix`. Metrics and
`/healthz` live on the admin port — see [Observability](/guides/observability).
:::
