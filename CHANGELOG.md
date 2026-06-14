# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
  - Pluggable auth header injection via the `HeaderProvider` interface.
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
  middleware, a `StaticProvider` for fixed headers, and a provider registry with
  auto-detection (a single compiled-in provider is used without config;
  resolution is explicit config → auto-detect → static fallback). Designed for
  corporate forks that inject tokens (Okta, Azure AD, etc.) into proxied requests.
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

[Unreleased]: https://github.com/osuritz/radix/compare/v0.1.0-alpha.1...HEAD
[0.1.0-alpha.1]: https://github.com/osuritz/radix/releases/tag/v0.1.0-alpha.1
