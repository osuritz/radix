# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **Dedicated admin port for metrics + `/healthz`** — every server command
  (`serve`, `proxy`, `echo`, `mock`) now exposes request-oriented metrics and a
  new `/healthz` liveness endpoint on a separate admin server (default port
  `9090`, configurable via `--metrics-port` / `metrics.port`). The admin server
  always binds loopback (`127.0.0.1`), even when the app binds `0.0.0.0`, so
  telemetry and health are not broadly exposed. `/healthz` returns `200` with
  `{"status":"ok","uptime":"<duration>","version":"<version>"}`. The metrics
  endpoint honors `--metrics-format` (JSON or Prometheus). The admin and main
  servers share one collector and shut down together on signal/context cancel;
  the admin listener is bound eagerly and always released (never leaked) even if
  the main server fails to start. `--metrics=false` starts no admin server.
  Config validation (`radix validate` and each command at startup) rejects an
  out-of-range `metrics.port` and a collision with the app port.

### Changed

- **Metrics moved off the application port (behavior change).** The application
  listener **no longer serves `/_metrics`**; it is served only on the admin port
  (default `127.0.0.1:9090`). Point scrapers and health checks at the admin port
  instead of the app port. `echo` and `mock` continue to serve their existing
  `/_health` and `/_ready` JSON endpoints on the app port.

## [0.4.0] - 2026-06-14

### Changed

- **Polished `dev` access-log format** — the developer-friendly log line is now a
  single, aligned, Vite-style row: a dimmed short timestamp (`HH:MM:SS`), the
  color-coded method (padded so paths align), the request path (padded, with
  long paths truncated by a single `…`), the color-coded status, the latency,
  and a human-readable size that is omitted entirely for zero-size responses.
  Color is now auto-disabled when `NO_COLOR` is set or when output is not a TTY
  (in addition to the existing `--no-color`), with `FORCE_COLOR` / `CLICOLOR_FORCE`
  available to force it on; the precedence is documented in the README. The
  logging middleware also gained an injectable `io.Writer` (`LoggingConfig.Output`,
  defaulting to stdout) and serializes writes so concurrent request lines never
  interleave. The CLF and Extended CLF formats are unchanged (byte-identical).

### Added

- **Echo client-certificate inspection** — under client-auth/mTLS, the `echo`
  response's `tls.client_cert` block now reports the presented client
  certificate: subject and issuer distinguished names (CN and O), serial,
  validity window (`not_before`/`not_after`, RFC3339), and DNS/IP
  subject-alternative names. The `client_cert` field is always present in the
  `tls` section and is `null` when no client certificate was presented.
- **Config-driven auth header values from env vars and the OS keychain** — proxy
  header values can now reference `${env:NAME}` and `${keychain:SERVICE/ACCOUNT}`
  tokens, resolved per request (with a short TTL cache for keychain reads) so a
  rotated token is picked up without restarting. This covers the common
  corporate "simulate the edge gateway locally" case with no fork required.
  Available in two equivalent surfaces: inline `${...}` tokens in `--header` /
  `proxy.headers` (Surface A), and a structured `proxy.auth.provider: headers`
  with a `config.headers` list (`value` / `env` / `keychain` + optional `prefix`,
  Surface B). Keychain access is backed by `github.com/zalando/go-keyring` (macOS
  Keychain, Windows Credential Manager, Linux Secret Service) behind a swappable
  `KeychainReader` interface. Resolution **fails loud** (an unresolved or
  set-but-empty source returns 502, never a silent unauthenticated proxy), and
  injected secret values are **never logged** (verbose injection logging emits
  header names only).

### Dependencies

- Added `github.com/zalando/go-keyring` for the keychain value source. Binary-size
  impact of OS keychain support is small (measured, stripped release builds):
  ~+85–100 KiB on macOS and ~+30 KiB on Windows; the Linux Secret Service backend
  (`godbus/dbus`) adds ~+524 KiB.

## [0.3.0] - 2026-06-14

### Added

- **Mock conditional responses** — a custom route may carry a `conditions:` block
  that selects its response by matching request content. Arms are evaluated in
  file order; the first arm whose every `match` entry is satisfied wins (a
  `default: true` arm always matches). Match keys are dotted and prefixed with
  `body.` (top-level JSON field or form value), `query.`, or `headers.`; a value
  of `"*"` means "present with any non-empty value", any other value is an exact
  match. JSON numbers match their exact source text (e.g. `{"id":1000000}`
  matches `body.id: "1000000"`, not `1e+06`), booleans match `"true"`/`"false"`,
  and JSON `null`/`""` both compare equal to an exact `""` (and are absent to a
  `"*"` wildcard). Only scalar top-level body fields are meaningful match targets.
  Precedence when serving: winning arm → default arm → the route's top-level
  `response` **only when explicitly provided** → `404`. A route with no
  conditions always has an effective response — an absent or empty `response: {}`
  serves `200` empty (path-only routes are valid). A non-`default` arm must have
  at least one match rule. Each arm's body template (inline or `file:`) keeps the
  same traversal guard; the request body is parsed once (numbers as exact text,
  form fields as first-value strings) and shared by matching and templating, so
  `{{.body.field}}` renders exactly what a condition matches. **Inline** `body`
  templates are validated at load; `file:` bodies are read and templated **per
  request** (so edits to the data file are reflected live), and a missing file or
  malformed file template surfaces as a `500` at request time.
- **`proxy.auth.provider` selection** — the proxy now honors `proxy.auth.provider`
  to choose among multiple compiled-in `HeaderProvider`s by name. A configured
  name that isn't registered is a hard startup error (rather than silently
  injecting no headers). Auto-detection of a single provider and the static
  `--header` fallback are unchanged.

### Changed

- `middleware.ResolveProvider` now returns `(HeaderProvider, error)`: an explicit
  but unregistered provider name yields an error instead of a silent `nil`.
  Empty-name auto-detection (single provider, static fallback, or none) is
  unchanged and never errors.

## [0.2.0] - 2026-06-14

This release completes the core command set. Radix now provides `serve`, `proxy`,
`echo`, `mock`, and `gencert` alongside the existing `version` and `validate`
commands, all with TLS/HTTPS support, metrics, structured logging, and graceful
shutdown.

### Added

- **`radix serve`** — static file server.
  - SPA mode (`--spa`), CORS (`--cors`), gzip (`--gzip`), `Cache-Control`
    (`--cache`), directory listing, and a configurable index file (`--index`).
  - HTTPS via the global TLS flags, with optional HSTS (`--hsts`,
    `--hsts-max-age`) and an HTTP→HTTPS redirect listener (`--http-redirect`,
    `--http-port`) issuing 308 Permanent Redirect. HSTS and the redirect both
    require `--tls`; `--http-port` must differ from `--port`.
- **`radix proxy`** — reverse proxy to a backend target.
  - Target via positional arg or `--target`; path rewriting (`--rewrite from:to`),
    prefix stripping (`--strip-prefix`), header injection (`--header "Key: Value"`),
    CORS (`--cors`), and a configurable backend timeout (`--timeout`).
  - Streaming/SSE support with a configurable flush interval (`--flush-interval`,
    defaulting to immediate flush for SSE / agent-chat backends).
  - HTTPS frontend plus backend TLS, including mTLS (`--tls-skip-verify`,
    backend CA/cert/key).
  - Pluggable auth header injection via the `HeaderProvider` interface: a
    single compiled-in provider is auto-detected and used, otherwise the static
    `--header` values are injected. Explicit `auth.provider` selection among
    multiple providers landed in a later change (see Unreleased).
- **`radix echo`** — echoes each request back as JSON (method, URL, path, query,
  headers, cookies, body, client/server info, TLS state, timing).
  - JSON/form body parsing; configurable status (`--status`), delay with jitter
    (`--delay`, `--delay-jitter`), literal body (`--body`), content type, and
    headers; section toggles (`--echo-body`, `--echo-headers`, `--echo-query`);
    body-size limit (`--body-limit`, 413 on exceed) and pretty-printing
    (`--pretty`).
  - Path-based status (`--status-from-path`) and delay (`--delay-from-path`),
    CORS (`--cors`), and `/_health` / `/_ready` endpoints.
- **`radix mock`** — API mock server with built-in httpbin-style endpoints and
  optional custom YAML routes.
  - Built-ins: `/get`, `/post`, `/put`, `/patch`, `/delete`, `/anything[/...]`,
    `/headers`, `/ip`, `/user-agent`, `/uuid`, `/status/{code}`, `/delay/{n}`,
    `/bytes/{n}`, `/json`, `/html`, `/xml`. Toggle with `--builtin`, mount under
    `--prefix`, add global latency (`--latency`, `--latency-jitter`) and chaos
    (`--fail-rate`, `--fail-status`).
  - Custom routes via positional `radix mock <file>` or `--routes`/`-r`, taking
    precedence over the built-ins. Matching priority: exact+method >
    exact+any-method > `:param` > `regex:` > trailing `/*` glob. Templated
    response/file bodies via Go `text/template` (`{{.params.id}}`, `{{.query.q}}`,
    `{{.body.field}}`, `{{uuid}}`, `{{now}}`, `{{random low high}}`, etc.),
    per-route `delay`/`delay_jitter`, a `settings` block, and a `404`/`proxy`
    fallback for unmatched requests.
  - Hot-reload with `--watch`/`-w` (fsnotify): a broken edit is rejected and the
    previous good config keeps serving (lock-free atomic config swap).
  - Example routes at `examples/mock-routes.yml`.
- **`radix gencert`** — generate self-signed TLS certificates (CA + server/client,
  RSA or ECDSA, SAN support) for local HTTPS.
- **TLS infrastructure** — global TLS flags (`--tls`, `--cert`, `--key`, `--ca`,
  `--client-auth`, `--tls-min-version`) and a TLS config loader shared by all
  server commands.
- **Auth extensions** — `HeaderProvider` interface with `InjectHeaders`
  middleware, a `StaticProvider` for fixed headers, and a provider registry.
  The `proxy` command auto-detects a single compiled-in provider (used without
  any config) and otherwise falls back to injecting the static
  `--header`/`proxy.headers` values; explicit `proxy.auth.provider` selection
  among multiple registered providers is supported by the registry (wired into
  the command in a later change — see Unreleased). Designed for corporate forks
  that inject tokens (Okta, Azure AD, etc.) into proxied requests.
- **Middleware** — CORS and gzip compression middleware; HSTS security-headers
  middleware.
- **`scripts/smoke.sh`** and a `make smoke` target — end-to-end smoke test that
  builds the binary and exercises every command.

### Changed

- Bumped the minimum Go version to 1.25.
- Proxy now sets `X-Forwarded-For`/`-Host`/`-Proto` from the inbound request and
  strips client-supplied (spoofable) values rather than trusting or appending
  them; streaming content types additionally get `X-Accel-Buffering: no` and
  `Cache-Control: no-cache`.

### Fixed

- CI: pinned golangci-lint and gosec versions; switched to a path-based lint
  exclusion for the metrics package; skipped a flaky Windows port-conflict test
  to keep `main` green.

## [0.1.0-alpha.1] - 2025-12-31

### Added
- Initial alpha release for testing CI/CD pipeline
- `radix version` command with build info display
- `radix validate` command for configuration file validation
- Metrics infrastructure (JSON and Prometheus formats)
- Logging middleware with multiple formats (CLF, dev)
- Full CI/CD automation with GitHub Actions
- GoReleaser configuration for multi-platform releases
- Comprehensive linting with golangci-lint (25+ linters)
- Security scanning with gosec and govulncheck

### Note
This is an alpha release to test the release workflow. Server commands (serve, proxy, echo, mock) are not yet implemented.

[Unreleased]: https://github.com/osuritz/radix/compare/v0.4.0...HEAD
[0.4.0]: https://github.com/osuritz/radix/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/osuritz/radix/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/osuritz/radix/compare/v0.1.0-alpha.1...v0.2.0
[0.1.0-alpha.1]: https://github.com/osuritz/radix/releases/tag/v0.1.0-alpha.1
