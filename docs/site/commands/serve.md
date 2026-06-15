# radix serve

Serve a directory of static files over HTTP or HTTPS.

```bash
radix serve [directory] [flags]
```

The directory is positional (defaults to the current directory) or set with
`--dir`. Files are served live at `http://localhost:8080`.

## When to use it

A zero-config replacement for `python -m http.server` or `npx http-server`:
preview a static site, serve a built frontend bundle, or host an SPA with
client-side routing.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--dir`, `-d` | current directory | Directory to serve |
| `--index` | `index.html` | Index file served for directory requests |
| `--spa` | `false` | SPA mode: serve the index for unknown paths instead of 404 |
| `--cors` | `false` | Add permissive CORS headers |
| `--gzip` | `false` | Gzip-compress responses |
| `--cache` | | `Cache-Control` header value (e.g. `max-age=3600`) |
| `--hsts` | `false` | Send `Strict-Transport-Security` (requires `--tls`) |
| `--hsts-max-age` | `31536000` | HSTS `max-age` in seconds (1 year) |
| `--http-redirect` | `false` | Run a plain-HTTP listener that 308-redirects to HTTPS (requires `--tls`) |
| `--http-port` | `8080` | Port for the redirect listener (must differ from `--port`) |

Global flags (`--tls`, `--port`, `--host`, `--metrics`, …) apply too — see
[Configuration](/configuration#global-flags).

## Examples

### Serve a directory

```bash
radix serve ./public --port 3000
```

### Single-page app

In SPA mode, any path that doesn't map to a real file is served `index.html`,
so client-side routers (React Router, Vue Router) handle deep links:

```bash
radix serve ./dist --spa
```

A request for `/dashboard` with no such file returns the index. The access log
marks it with a `→ fallback` target so you can see when the fallback kicked in
(see [Logging](/guides/logging)).

### CORS and compression

```bash
radix serve ./public --cors --gzip --cache "max-age=3600"
```

### HTTPS with redirect and HSTS

`--hsts` and `--http-redirect` both require `--tls`, and `--http-port` must
differ from the HTTPS `--port`:

```bash
radix serve ./public \
  --tls --cert ./certs/cert.pem --key ./certs/key.pem \
  --port 8443 \
  --http-redirect --http-port 8080 \
  --hsts
```

HTTPS runs on `8443`; plain HTTP on `8080` 308-redirects to it. See the
[TLS guide](/guides/tls) for generating the certs.

::: warning
`--hsts` and `--http-redirect` are no-ops without `--tls` — radix rejects the
combination at startup. `--http-port` must not equal `--port`.
:::

::: details Why no per-command metrics for serve?
`serve` exposes the shared request metrics (totals, status codes, latency) but
has no per-command counters. `echo`, `mock`, and `proxy` each add their own —
see [Observability](/guides/observability).
:::
