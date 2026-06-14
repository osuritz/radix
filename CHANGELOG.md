# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- `radix gencert` command for TLS certificate generation
- TLS config loader with global CLI flags (--tls, --cert, --key, --ca, --client-auth, --tls-min-version)
- Auth extensions infrastructure with HeaderProvider interface
  - `InjectHeaders` middleware for injecting headers into proxied requests
  - `StaticProvider` for fixed headers from configuration
  - Provider registry with auto-detection (single registered provider used automatically)
  - `ResolveProvider` resolution: explicit config → auto-detect → static fallback
- `radix proxy` command for reverse proxying to backend servers
  - Target URL via positional arg or `--target` flag
  - Path rewriting (`--rewrite from:to`)
  - Path prefix stripping (`--strip-prefix`)
  - Custom header injection (`--header "Key: Value"`)
  - Configurable timeout (`--timeout`)
  - Streaming response support with configurable flush interval (`--flush-interval`, defaults to immediate flush for SSE / agent-chat backends; sets `X-Accel-Buffering: no` and `Cache-Control: no-cache` on detected streaming content types)
  - Secure forwarded headers: sets X-Forwarded-For/-Host/-Proto from the inbound request and strips client-supplied (spoofable) values rather than trusting or appending them
  - Backend TLS support (--tls-skip-verify, backend CA/cert/key)
  - CORS support (`--cors`)
  - Auth header injection via HeaderProvider middleware
  - TLS/HTTPS listener support
  - Metrics integration
  - Graceful shutdown
- `radix echo` command for echoing HTTP requests back as JSON
  - JSON description of each request: method, URL, path, query, headers, cookies, body, client/server info, TLS state, and timing
  - Body parsing for JSON and form-urlencoded content (with `body_raw`/`body_size` always included)
  - Configurable default status (`--status`) and response delay with jitter (`--delay`, `--delay-jitter`)
  - Literal response body override (`--body`) and custom `--content-type` / `--header`
  - Toggle echoed sections (`--echo-body`, `--echo-headers`, `--echo-query`)
  - Request body size limit returning 413 (`--body-limit`) and pretty-printing (`--pretty`)
  - Path-based status (`--status-from-path`, e.g. `/404`, `/status/500`) and delay (`--delay-from-path`, e.g. `/delay/2`, `/delay/500ms`, capped at 10s)
  - `/_health` and `/_ready` endpoints, CORS support (`--cors`)
  - TLS/HTTPS listener support, metrics integration, and graceful shutdown
- `radix mock` command with built-in httpbin-style endpoints
  - HTTP method endpoints: `/get`, `/post`, `/put`, `/patch`, `/delete` returning httpbin-style JSON (args, headers, origin, url, method; plus data/json/form for body methods)
  - `/anything` and `/anything/` (subtree, any sub-path) for any HTTP method
  - Request inspection: `/headers`, `/ip`, `/user-agent`, `/uuid` (RFC 4122 v4)
  - `/status/{code}` (single or comma-separated random choice, validated to [200,599])
  - `/delay/{n}` (Go duration or bare seconds, capped at 10s, context-aware)
  - `/bytes/{n}` (random bytes, capped at 100KB, with correct Content-Length)
  - Response formats: `/json`, `/html`, `/xml`
  - Global latency (`--latency`, `--latency-jitter`) and chaos (`--fail-rate`, `--fail-status`)
  - Endpoint toggling (`--builtin`), path prefix (`--prefix`), CORS (`--cors`)
  - `/_health` and `/_ready` endpoints (kept at root), TLS/HTTPS, metrics, and graceful shutdown
  - Custom YAML routes via positional `radix mock <file>` or `--routes`/`-r`, taking precedence over the built-ins
    - Path matching priority: exact+method > exact+any-method > `:param` > `regex:` > trailing `/*` glob
    - Templated response bodies (and file bodies) using Go `text/template` syntax: data access via `{{.method}}`, `{{.path}}`, `{{.params.id}}`, `{{.query.q}}`, `{{.headers.Name}}`, `{{.body.field}}`, and generators `{{uuid}}`, `{{now}}`, `{{timestamp}}`, `{{random low high}}`, `{{randomString n}}`, `{{env "VAR"}}`, `{{base64 "s"}}`
    - Note: uses idiomatic dot-access Go template syntax (`{{.params.id}}`), deviating from the dot-less examples in `docs/COMMAND_DESIGN.md`
    - Per-route `delay`/`delay_jitter`; file response bodies resolved relative to the routes file with path-traversal protection; request-body parsing bounded to 1MB (413 on exceed)
    - Settings block (`latency`, `latency_jitter`, `fail_rate`, `fail_status`, `cors`, `fallback`); CLI flags override file settings
    - Fallback for unmatched requests: `404` (default) or `proxy` to a configured target
    - Hot-reload with `--watch`/`-w` (fsnotify): a broken edit is rejected and the previous good config keeps serving; config swaps are lock-free via an atomic pointer
    - Not yet supported (ignored gracefully if present): `conditions`, `sequence`, weighted `random`, `websocket`, `sse`
    - Example routes file at `examples/mock-routes.yml`
- `radix serve` command for static file serving
  - SPA mode (--spa) for single page applications
  - CORS headers (--cors)
  - Gzip compression (--gzip)
  - Cache-Control header (--cache)
  - HSTS header (--hsts, --hsts-max-age) over HTTPS (requires --tls)
  - HTTP→HTTPS redirect listener (--http-redirect, --http-port) issuing 308 Permanent Redirect (requires --tls; --http-port must differ from --port)
  - TLS/HTTPS support
  - Metrics integration
  - Graceful shutdown
  - Directory listing
- CORS middleware for cross-origin request handling
- Gzip compression middleware

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
