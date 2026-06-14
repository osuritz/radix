# radix

[![CI](https://github.com/osuritz/radix/actions/workflows/ci.yml/badge.svg)](https://github.com/osuritz/radix/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/osuritz/radix)](https://goreportcard.com/report/github.com/osuritz/radix)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Multi-mode HTTP server for local development. Provides static file serving, reverse proxy, request echo, and API mocking capabilities—all running locally with no external services or data leakage. Built in Go and distributed as a single self-contained binary across platforms, with a deliberately small, curated set of dependencies.

## Who Is This For?

Radix is designed for **local development**, not production traffic. It's built for:

- **Software engineers** who need a quick static file server or API proxy during development
- **Frontend developers** working on SPAs who need to mock backend APIs
- **Coding agents and AI assistants** that need reliable HTTP tooling for automated workflows
- **QA engineers** testing frontend/backend integration locally

If you need a production-grade web server, use nginx, Caddy, or a cloud load balancer instead.

## Features

| Command | Purpose | Example Use Case |
|---------|---------|------------------|
| `serve` | Static file server | Serve a React/Vue build folder |
| `proxy` | Reverse proxy | Forward `/api/*` to a backend service |
| `echo` | Request debugger | Inspect webhook payloads |
| `mock` | API mocking | Develop frontend without a running backend |
| `gencert` | TLS certificates | Generate self-signed certs for HTTPS |

## Quick Start

```bash
# Install
go install github.com/osuritz/radix/cmd/radix@latest

# Serve current directory
radix serve

# Serve with SPA routing
radix serve ./dist --spa --port 3000

# Proxy to a backend service
radix proxy http://localhost:8080 --port 3000

# Echo server for debugging
radix echo --port 9000

# Mock API endpoints
radix mock --routes ./api-mocks.yml
```

## Static File Serving

The `serve` command serves a directory of static files over HTTP(S), with optional SPA routing, CORS, gzip, and cache headers.

```bash
# Serve the current directory on :8080
radix serve

# Serve a build folder with SPA fallback to index.html
radix serve ./dist --spa --port 3000

# Enable CORS and gzip
radix serve --cors --gzip
```

### HTTPS security headers and redirect

When serving over HTTPS (`--tls`), `serve` can emit an HSTS header and run a
plain-HTTP listener that redirects to HTTPS:

```bash
# Send Strict-Transport-Security on every response (requires --tls)
radix serve --tls --cert cert.pem --key key.pem --hsts

# Custom HSTS max-age (default is 31536000 = 1 year)
radix serve --tls --cert cert.pem --key key.pem --hsts --hsts-max-age 86400

# Also run an HTTP→HTTPS redirect listener on :8080 (308 Permanent Redirect)
radix serve --tls --cert cert.pem --key key.pem --port 8443 \
  --http-redirect --http-port 8080
```

Notes:
- `--hsts` and `--http-redirect` both require `--tls` (HSTS is ignored by
  browsers over plain HTTP, so sending it would be misleading).
- `--http-port` must differ from `--port` (the redirect listener and the HTTPS
  server cannot share a port).
- The redirect preserves the request method, path, and query (308 Permanent
  Redirect).

## Reverse Proxy

The `proxy` command starts a reverse proxy that forwards all incoming requests to a backend target. This is useful when your frontend dev server needs to talk to a local or remote API without dealing with CORS issues, or when you need to inject auth headers into every request.

### Basic usage

```bash
# Forward all traffic on :8080 to a backend on :3000
radix proxy http://localhost:3000

# Same, on a custom port
radix proxy http://localhost:3000 --port 9000

# Using the --target flag instead of a positional argument
radix proxy --target http://localhost:3000
```

### Path manipulation

```bash
# Strip a prefix: /api/users → /users on the backend
radix proxy http://localhost:3000 --strip-prefix /api

# Rewrite paths: /v1/users → /v2/users on the backend
radix proxy http://localhost:3000 --rewrite /v1:/v2
```

### Headers and CORS

```bash
# Inject custom headers into every proxied request
radix proxy http://localhost:3000 --header "X-Api-Key: dev-123" --header "X-Env: local"

# Enable permissive CORS headers (useful for browser-based frontends)
radix proxy http://localhost:3000 --cors
```

### TLS / HTTPS

The proxy supports TLS on both the frontend (clients connect via HTTPS) and the backend (proxy connects to an HTTPS target).

```bash
# Serve the proxy itself over HTTPS
radix proxy http://localhost:3000 --tls --cert cert.pem --key key.pem

# Proxy to an HTTPS backend (skip certificate verification for self-signed certs)
radix proxy https://api.internal:443 --tls-skip-verify

# Both: HTTPS frontend proxying to an HTTPS backend with mTLS
radix proxy https://api.internal:443 \
  --tls --cert server.pem --key server-key.pem \
  --tls-skip-verify
```

### Auth header injection

Useful when you're developing locally against a backend that normally sits behind
an edge/zero-trust gateway: the proxy can inject the same auth headers the gateway
would, so you don't hand-pass `--header` flags on every run.

**Header values from the environment and the OS keychain.** Values in `--header`
and `proxy.headers` may contain `${...}` tokens, resolved per request:

- `${env:NAME}` — value of an environment variable
- `${keychain:SERVICE/ACCOUNT}` — a secret from the OS keychain (macOS Keychain,
  Windows Credential Manager, or Linux Secret Service)

```bash
radix proxy http://localhost:3000 \
  --header "X-Auth-Request-Email: ${env:USER_EMAIL}" \
  --header "Authorization: Bearer ${keychain:work-cli/jwt}"
```

Keychain reads are cached briefly (~10s), so a token rotated by another tool is
picked up without restarting radix; note the flip side is that a revoked secret
may still be served until that window elapses. Resolution fails loud — an unset
or empty variable, or a keychain miss, returns `502` rather than silently
proxying without credentials — and injected values are never written to logs.

For a structured, validatable alternative to inline `${...}` tokens, set
`proxy.auth.provider: headers` and list each header (`value` / `env` / `keychain`
plus an optional `prefix`) under `proxy.auth.config.headers`; see
`examples/radix.example.yml`.

**Forks.** For credentials that need real refresh logic (OAuth/OIDC, Vault, STS),
compile in a custom `HeaderProvider` (e.g. for Okta or Azure AD); a single
registered provider is auto-detected with no config. See `IMPLEMENTATION_PLAN.md`
Section 15 and `internal/server/middleware/auth.go`.

### All proxy flags

| Flag | Default | Description |
|------|---------|-------------|
| `--target` / positional arg | *(required)* | Backend URL (must include `http://` or `https://`) |
| `--port` | `8080` | Port to listen on |
| `--strip-prefix` | | Strip this prefix from paths before forwarding |
| `--rewrite` | | Path rewrite rule in `from:to` format |
| `--header` | | Add header to proxied requests (`Key: Value`, repeatable) |
| `--cors` | `false` | Enable permissive CORS headers |
| `--timeout` | `30s` | Backend response timeout |
| `--flush-interval` | `-1ns` | Response flush interval for streaming; negative flushes immediately (SSE), `0` uses default buffering |
| `--tls` | `false` | Serve the proxy over HTTPS |
| `--cert` | | TLS certificate file (for HTTPS frontend) |
| `--key` | | TLS private key file (for HTTPS frontend) |
| `--tls-skip-verify` | `false` | Skip TLS certificate verification for backend |
| `--websocket` | `false` | Enable explicit WebSocket support |

## Request Echo

The `echo` command starts a server that answers every request with a JSON
description of that request — method, URL, path, query, headers, cookies, body,
client/server info, TLS state, and timing. It is handy for inspecting webhook
payloads and debugging HTTP clients.

```bash
# Echo server on :8080
radix echo

# Inspect a POST: the response JSON contains the method and the parsed body
curl -X POST localhost:8080/anything -d '{"hi":"there"}' -H 'Content-Type: application/json'

# Simulate a slow API (fixed delay + random jitter)
radix echo --delay 500ms --delay-jitter 200ms

# Return a fixed status / body instead of the echo JSON
radix echo --status 201 --body '{"ok":true}'

# Derive status or delay from the request path
radix echo --status-from-path   # GET /404 -> 404, GET /status/500 -> 500
radix echo --delay-from-path    # GET /delay/2 or /delay/500ms
```

The body parses JSON and form-urlencoded content (with `body_raw`/`body_size`
always included). Toggle echoed sections with `--echo-body`, `--echo-headers`,
and `--echo-query`; cap the request body with `--body-limit` (413 on exceed) and
control formatting with `--pretty`. `/_health` and `/_ready` are served at the
root and are not echoed. Use `--cors` for permissive CORS, and the global TLS
flags for HTTPS (the response's `tls` block reports the negotiated version,
cipher, and server name).

## Mock Server

The `mock` command starts a zero-config API mock server exposing httpbin-style built-in endpoints, plus global latency and chaos (random failure) knobs. It is useful for frontend development without a backend, and for exercising HTTP clients against predictable responses.

### Basic usage

```bash
# Built-in endpoints on :8080
radix mock

# Inspect a request (returns args, headers, origin, url, method)
curl localhost:8080/get?foo=bar

# Echo a POST body back as JSON
curl -X POST localhost:8080/post -d '{"hi":"there"}' -H 'Content-Type: application/json'
```

### Built-in endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /get`, `POST /post`, `PUT /put`, `PATCH /patch`, `DELETE /delete` | httpbin-style request description (body methods also include `data`/`json`/`form`) |
| `ANY /anything`, `ANY /anything/` | Same description for any method (`/anything/` matches any sub-path) |
| `GET /headers` | Request headers |
| `GET /ip` | Client origin IP |
| `GET /user-agent` | User-Agent header |
| `GET /uuid` | A random v4 UUID |
| `ANY /status/{code}` | Respond with that status (comma list picks one at random) |
| `ANY /delay/{n}` | Delay n seconds (max 10), then return the `/get`-style JSON |
| `GET /bytes/{n}` | n random bytes (max 100KB) as `application/octet-stream` |
| `GET /json`, `GET /html`, `GET /xml` | Sample document with the matching Content-Type |

### Latency and chaos

```bash
# Add 200ms latency (with optional jitter) to every response
radix mock --latency 200ms --latency-jitter 100ms

# Fail 10% of requests with a 503
radix mock --fail-rate 10 --fail-status 503
```

### Custom routes (YAML)

Provide a YAML routes file to define custom routes that take precedence over the
built-ins. Pass it positionally (`radix mock <file>`) or via `--routes`/`-r`
(giving both with different values is an error), and add `--watch`/`-w` to
hot-reload on save. `--watch` reloads the routes, the `fallback`, and the global
`latency`/`latency_jitter`/`fail_rate`/`fail_status` settings; a broken edit is
rejected and the previous good config keeps serving. `cors` is applied once at
startup and is **not** hot-reloaded. Explicitly-set CLI flags always win over the
file and survive reloads:

```bash
# Serve custom routes (positional or --routes)
radix mock examples/mock-routes.yml
radix mock --routes examples/mock-routes.yml --watch
```

```yaml
# routes.yml (see examples/mock-routes.yml for the full schema)
settings:
  fallback:
    type: "404"            # "404" (default) or "proxy" (forward to proxy_target)
routes:
  - path: /api/health      # exact path
    method: GET
    response:
      status: 200
      headers: { Content-Type: application/json }
      body: '{"status":"ok"}'
  - path: /api/users/:id    # :param path params -> {{.params.id}}
    method: GET
    response: { status: 200, body: '{"id":"{{.params.id}}"}' }
  - path: "regex:^/api/v[0-9]+/x$"   # regex: prefix
    response: { status: 200, body: "ok" }
  - path: /assets/*         # trailing /* glob (prefix) match
    response: { status: 200, body: "asset" }
  - path: /api/products
    method: GET
    response:
      file: ./mocks/products.json     # body read from file (relative to routes file), templated
```

**Matching priority (first match wins):** exact+method → exact+any-method →
`:param` → `regex:` → `/*` glob → built-in endpoint → fallback (`404`/`proxy`).

`regex:` patterns use Go [`regexp`](https://pkg.go.dev/regexp) semantics and are
**not** auto-anchored — they match if found anywhere in the path. Use `^...$` to
match the whole path (e.g. `regex:^/api/v[0-9]+/x$`).

**Templating** uses idiomatic Go `text/template` syntax. Request data is
dot-accessible: `{{.method}}`, `{{.path}}`, `{{.params.id}}`, `{{.query.q}}`,
`{{.headers.Authorization}}`, `{{.body.field}}` (a **top-level** field of the
parsed JSON object, or a form-urlencoded first value). JSON numbers keep their
exact source text (so `{"id":1000000}` renders `1000000`, not `1e+06`), and a
form field renders its single value (`admin`, not `[admin]`) — the same value
condition matching reads. Header names containing `-` need `index`, e.g.
`{{index .headers "Content-Type"}}`. Generator functions: `{{uuid}}`, `{{now}}`,
`{{timestamp}}`, `{{random low high}}`, `{{randomString n}}`, `{{env "VAR"}}`,
`{{base64 "s"}}`.

**Template validation timing.** An **inline** `body` template is parsed at load
time, so a malformed inline template fails fast (the routes file is rejected). A
`file:` body, by contrast, is read and templated **per request** — this is
intentional so edits to the referenced data file are reflected live without a
reload. A missing `file:` target or a malformed file template therefore surfaces
as a `500` at request time (the server stays up); a render error on any template
likewise yields a `500`.

**Conditional responses** let a route pick its response by matching request
content. Add a `conditions:` block of arms; the **first** arm whose every
`match` entry is satisfied wins (and a `default: true` arm always matches):

```yaml
routes:
  - path: /api/auth/login
    method: POST
    conditions:
      - match:
          body.username: admin
          body.password: secret      # all entries must match
        response: { status: 200, body: '{"token":"{{uuid}}"}' }
      - match:
          body.username: "*"          # "*" = present with any non-empty value
        response: { status: 401, body: '{"error":"invalid"}' }
      - default: true                 # matches unconditionally (place last)
        response: { status: 400, body: '{"error":"username required"}' }
```

Match keys are dotted and must be prefixed with `body.` (a **top-level** field
of the parsed JSON object or a form-urlencoded value — nested paths like
`body.a.b` are not supported, and only **scalar** fields are useful match
targets; a field whose value is a nested object/array is not a meaningful match
target), `query.` (first query value), or `headers.` (canonical-cased, first
header value). A value of `"*"` matches when the key is present with any
non-empty value; any other value requires an exact match. JSON `null` and an
empty string `""` both compare equal to an exact `""` match, and both are treated
as absent/empty by the wildcard `"*"`. The selected arm's body is templated
exactly like a plain response.

**Precedence when serving:** winning arm → `default: true` arm → the route's
top-level `response` (only if one was explicitly provided) → `404`. A route with
**no** conditions always has an effective response: an absent or empty
`response: {}` defaults to `200` with an empty body (so a path-only route like
`- path: /x` is valid). A route **with** conditions falls back to its top-level
`response` only when one was provided; otherwise a no-match request is a `404`.

Each non-`default` arm must have at least one `match` rule (use `default: true`
for an unconditional arm). Bad match-key prefixes, empty-match non-default arms,
and malformed **inline** templates all fail at load; `file:` arm bodies follow
the per-request validation described above.

> Not yet supported (ignored gracefully if present): stateful `sequence`,
> weighted `random`, `websocket`, and `sse`.

### All mock flags

| Flag | Default | Description |
|------|---------|-------------|
| `--builtin` | `true` | Register the built-in httpbin-style endpoints |
| `--prefix` | | Mount built-ins under a path prefix (e.g. `/_test` → `/_test/get`) |
| `--routes`, `-r` | | YAML routes file defining custom routes (also positional) |
| `--watch`, `-w` | `false` | Reload the routes file on change (routes, fallback, latency, fail-rate; CORS is set at startup) |
| `--latency` | `0` | Fixed artificial latency (e.g. `200ms`, `1s`) |
| `--latency-jitter` | `0` | Random jitter added to latency |
| `--fail-rate` | `0` | Random failure rate, percentage 0-100 |
| `--fail-status` | `500` | Status code returned for random failures |
| `--cors` | `false` | Enable permissive CORS headers |
| `--port` | `8080` | Port to listen on |
| `--tls` | `false` | Serve the mock over HTTPS |

> Note: `/_metrics`, `/_health`, and `/_ready` stay at the root regardless of `--prefix`. Routes-file `settings` are overridden by explicitly-set CLI flags. A file value of `cors: false` or `fail_rate: 0` is honored as written (an explicit zero/false is distinct from an omitted field).

## Logging

Every command logs each request through a small access-log middleware. Three
formats are available:

- **`dev`** (default) — a polished, single-line, color-coded format tuned for
  reading at a glance during local development. Columns, in order: a dimmed
  short timestamp (`HH:MM:SS`), the HTTP method (color-coded, padded so paths
  align), the request path (padded, with overly long paths truncated by a
  single `…` so the columns stay aligned), the status code (color-coded), the
  latency, and — only when the response had a body — a human-readable size. A
  zero-size response omits the size column entirely.

  ```
  14:23:01 GET     /index.html                  200 12ms 2.3KB
  14:23:01 POST    /api/users                   201 8ms 142B
  14:23:01 DELETE  /users/123                    204 5ms
  ```

- **`clf`** — Common Log Format.
- **`extended_clf`** — Common Log Format plus referrer and user-agent. This is
  what `--verbose` selects.

Both CLF formats are byte-for-byte the classic layouts and are unaffected by the
color settings below.

### Color control

Color for the `dev` format is decided once at startup, in this precedence
(first match wins):

1. `--no-color` (or `no_color` in config) → color **off**.
2. else the [`NO_COLOR`](https://no-color.org) environment variable is set and
   non-empty → color **off**.
3. else `FORCE_COLOR` or `CLICOLOR_FORCE` is set and non-empty → color **on**
   (this only overrides the TTY auto-detection in the next step; it can never
   re-enable color past steps 1–2).
4. else the output is not a TTY (e.g. redirected to a file or a pipe) → color
   **off**. (TTY detection is a stdlib-only character-device heuristic, not a
   true `isatty`; character devices such as `/dev/null` read as a TTY, so use the
   overrides above for those edge cases.)
5. otherwise → color **on**.

## TLS Certificates

The `gencert` command generates self-signed certificates for local HTTPS. By
default it creates a CA plus a server certificate signed by that CA; trust the CA
once and your browser/OS stops warning about the generated certs.

```bash
# Generate certs for localhost into ./certs (cert.pem, key.pem, ca.pem, ca-key.pem)
radix gencert

# Multiple hostnames/IPs and a custom output directory
radix gencert --host localhost,127.0.0.1,myapp.test --output ./certs

# ECDSA key, 2-year validity
radix gencert --key-type ecdsa --ecdsa-curve P-384 --days 730

# Reuse an existing CA, or generate a client certificate
radix gencert --ca-cert ./certs/ca.pem --ca-key ./certs/ca-key.pem
radix gencert --client

# Then serve over HTTPS with the generated cert
radix serve --tls --cert ./certs/cert.pem --key ./certs/key.pem
```

Key flags: `--host` (comma-separated SANs), `--output`/`-o` (directory, default
`./certs`), `--days`, `--org`, `--key-type` (`rsa`|`ecdsa`), `--key-size`
(`2048`|`4096`), `--ecdsa-curve` (`P-256`|`P-384`|`P-521`), `--ca`/`--ca-cert`/
`--ca-key`, `--client`, and `--overwrite`.

## Configuration & Validation

All commands read defaults from a config file (`./radix.yml`, `~/.radix.yml`, or
`/etc/radix/radix.yml`, overridable with `--config`/`-c`), then `RADIX_*`
environment variables, then CLI flags (CLI flags win). See
[`examples/radix.example.yml`](examples/radix.example.yml) for every available
key.

The `validate` command checks a config file's syntax, schema, port range, TLS
certificate paths, and serve TLS-coupling rules before you run a server:

```bash
radix validate                         # validate ./radix.yml
radix validate examples/radix.example.yml
radix validate --strict                # fail on warnings
```

## Development

### Building

```bash
# Build the binary
make build

# Run tests
make test

# Run tests with coverage
make coverage

# Run linters
make lint

# Install locally
make install
```

### CI/CD

This project uses GitHub Actions for continuous integration and automated releases:

- **CI Workflow**: Runs on every push and pull request
  - Linting with golangci-lint
  - Security scanning with gosec and govulncheck
  - Tests across multiple platforms (macOS, Linux, Windows)
  - Requires Go 1.25+
  - Code coverage reporting

- **Release Workflow**: Triggered on version tags (e.g., `v1.0.0`)
  - Builds binaries for multiple platforms using GoReleaser
  - Creates GitHub releases with changelogs
  - Publishes to Homebrew, Scoop, and package managers

### Versioning

This project uses semantic versioning. Version information is injected at build time:

```bash
# Check version
./bin/radix version

# Short version
./bin/radix version --short

# JSON output
./bin/radix version --json
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development guidelines.
