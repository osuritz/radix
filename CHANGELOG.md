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
- `radix serve` command for static file serving
  - SPA mode (--spa) for single page applications
  - CORS headers (--cors)
  - Gzip compression (--gzip)
  - Cache-Control header (--cache)
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
