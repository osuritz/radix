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
  - Backend TLS support (--tls-skip-verify, backend CA/cert/key)
  - CORS support (`--cors`)
  - Auth header injection via HeaderProvider middleware
  - TLS/HTTPS listener support
  - Metrics integration
  - Graceful shutdown
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
