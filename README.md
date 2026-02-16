# radix

[![CI](https://github.com/osuritz/radix/actions/workflows/ci.yml/badge.svg)](https://github.com/osuritz/radix/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/osuritz/radix)](https://goreportcard.com/report/github.com/osuritz/radix)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Multi-mode HTTP server for local development. Provides static file serving, reverse proxy, request echo, and API mocking capabilities—all running locally with no external services or data leakage. Built in Go for zero-dependency deployment across platforms.

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

Radix supports pluggable auth header providers via a Go interface. Corporate forks can compile in a custom `HeaderProvider` (e.g., for Okta or Azure AD) that automatically injects auth tokens into every proxied request — no per-engineer configuration needed.

For static headers, use `--header`. For dynamic token injection, see `IMPLEMENTATION_PLAN.md` Section 15 and `internal/server/middleware/auth.go`.

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
| `--tls` | `false` | Serve the proxy over HTTPS |
| `--cert` | | TLS certificate file (for HTTPS frontend) |
| `--key` | | TLS private key file (for HTTPS frontend) |
| `--tls-skip-verify` | `false` | Skip TLS certificate verification for backend |
| `--websocket` | `false` | Enable explicit WebSocket support |

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
  - Requires Go 1.24+
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
