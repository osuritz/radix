---
layout: home

hero:
  name: Radix
  text: Multi-mode HTTP server for local development
  tagline: Static file serving, reverse proxy, request echo, and API mocking — one self-contained Go binary, no external services.
  actions:
    - theme: brand
      text: Get started
      link: /getting-started
    - theme: alt
      text: View on GitHub
      link: https://github.com/osuritz/radix

features:
  - title: serve
    details: Serve a static directory (with SPA fallback) like a zero-config http-server.
    link: /commands/serve
  - title: proxy
    details: Reverse-proxy requests to a backend, with header injection and forwarded headers.
    link: /commands/proxy
  - title: echo
    details: Echo any HTTP request back as JSON to debug clients, webhooks, and payloads.
    link: /commands/echo
  - title: mock
    details: Mock an API from built-in httpbin-style endpoints or custom YAML routes with hot-reload.
    link: /commands/mock
  - title: gencert
    details: Generate self-signed CA, server, and client certificates for local HTTPS and mTLS.
    link: /commands/gencert
  - title: Observability
    details: Built-in metrics and a /healthz endpoint on a dedicated admin port, JSON or Prometheus.
    link: /guides/observability
---

> 🚧 Documentation in progress

Radix is a single self-contained binary that replaces several local development
tools. Pick a mode, point it at a directory or a backend, and go.

## Install

```bash
# Go install
go install github.com/osuritz/radix/cmd/radix@latest

# Homebrew, Scoop, and prebuilt binaries — see Getting started
```

See [Getting started](/getting-started) for full installation options and your
first `serve` and `proxy`.
