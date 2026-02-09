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

# Proxy API requests
radix proxy http://localhost:8080 --port 3000

# Echo server for debugging
radix echo --port 9000

# Mock API endpoints
radix mock --routes ./api-mocks.yml
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
