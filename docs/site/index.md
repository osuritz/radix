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

Radix is one binary that does the job of several local-dev tools. Instead of
juggling `http-server`, a hand-rolled proxy, `httpbin`, and an echo endpoint,
you run one command per mode and point it at a directory or a backend.

```bash
radix serve .                       # static files on http://localhost:8080
radix proxy http://localhost:3000   # reverse proxy to your backend
radix echo                          # echo every request back as JSON
radix mock                          # httpbin-style mock API
```

Every mode shares the same plumbing: TLS, CORS, gzip, access logging, and
metrics on a dedicated admin port — no extra wiring.

## Install

```bash
go install github.com/osuritz/radix/cmd/radix@latest
```

Prebuilt binaries for Linux, macOS, and Windows are on the
[releases page](https://github.com/osuritz/radix/releases). See
[Getting started](/getting-started) for download steps and your first server.

::: tip
Homebrew and Scoop packaging are planned but not available yet — use
`go install` or a release binary today.
:::

## Pick your mode

- [serve](/commands/serve) — static files, SPA mode, HTTPS
- [proxy](/commands/proxy) — reverse proxy, path rewrite, header injection
- [echo](/commands/echo) — request-to-JSON for debugging
- [mock](/commands/mock) — built-in + custom-route API mocking
- [gencert](/commands/gencert) — self-signed certs for local HTTPS/mTLS
- [validate](/commands/validate) — check a config or routes file
- [version](/commands/version) — build and version info
