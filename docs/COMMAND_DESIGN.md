# Radix Command Design Document

This document provides comprehensive design specifications for all planned Radix commands. Each section details the command's purpose, CLI interface, configuration options, implementation considerations, and examples.

**Document Version**: 1.0.0
**Last Updated**: 2026-02-09
**Status**: Draft

---

## Review Status

| Section | Status | Reviewer | Notes |
|---------|--------|----------|-------|
| [Serve Command](#serve-command) | Approved | @osuritz | nginx-style index fallback, SPA routing |
| [Proxy Command](#proxy-command) | Approved | @osuritz | SSE/streaming support, no load balancing |
| [Echo Command](#echo-command) | Approved | @osuritz | |
| [Mock Command](#mock-command) | Approved | @osuritz | httpbin-style built-in endpoints, browser landing page |
| [GenCert Command](#gencert-command) | Approved | @osuritz | |
| [Shared Infrastructure](#shared-infrastructure) | Approved | @osuritz | Visual dashboard, no Prometheus |

---

## Table of Contents

1. [Overview](#overview)
2. [Serve Command](#serve-command)
3. [Proxy Command](#proxy-command)
4. [Echo Command](#echo-command)
5. [Mock Command](#mock-command)
6. [GenCert Command](#gencert-command)
7. [Shared Infrastructure](#shared-infrastructure)
8. [Configuration Reference](#configuration-reference)

---

## Overview

Radix provides five primary server commands, each designed for specific local development scenarios:

| Command | Purpose | Primary Use Case |
|---------|---------|------------------|
| `serve` | Static file server | Frontend development, SPA hosting |
| `proxy` | Reverse proxy | API development, backend integration |
| `echo` | Request echo/debug | HTTP debugging, webhook testing |
| `mock` | API mocking | Frontend development without backend |
| `gencert` | Certificate generation | TLS/HTTPS local development |

### Target Audience

Radix is designed for **local development use** by:
- Software engineers and developers
- Coding agents and AI assistants
- QA engineers testing frontend/backend integration

**Radix is NOT intended for production traffic.** This influences design decisions:
- Simplicity over maximum performance
- Developer experience over operational complexity
- Fast startup over extensive optimization
- Reasonable defaults over exhaustive configurability

### Design Principles

1. **Zero Configuration Start**: Every command works with sensible defaults
2. **Progressive Disclosure**: Simple CLI flags for common cases, config files for complex setups
3. **Composable**: Commands can be combined via config file for complex scenarios
4. **Observable**: Built-in metrics and logging for all commands
5. **Secure by Default**: TLS support, no directory traversal, safe defaults
6. **Development-First**: Optimize for developer workflows, not production scale

---

## Serve Command

### Purpose

Serve static files from a local directory. Replaces tools like Python's `SimpleHTTPServer`, Node's `http-server`, and `serve`.

### CLI Interface

```bash
radix serve [directory] [flags]
```

### Flags

#### Basic Options

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--dir` | `-d` | string | `.` | Directory to serve |
| `--port` | `-p` | int | `8080` | Port to listen on |
| `--host` | `-H` | string | `localhost` | Host/IP to bind to |
| `--browse` | `-b` | bool | `false` | Open browser automatically |

#### Index & Routing

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--index` | `-i` | []string | `index.html,index.htm` | Index files to try (in order) |
| `--spa` | | bool | `false` | SPA mode: serve spa-index for non-file 404s |
| `--spa-index` | | string | `index.html` | Index file for SPA fallback (from root) |
| `--spa-exclude` | | []string | `[]` | Paths to exclude from SPA fallback (return 404) |
| `--not-found` | | string | `404` | 404 behavior: `404`, `spa`, `page:/path` |
| `--trailing-slash` | | string | `auto` | Trailing slash handling: `add`, `remove`, `auto` |
| `--clean-urls` | | bool | `false` | Serve `/about` from `/about.html` |

**Index file resolution** (like nginx `index` directive):
- When a directory is requested, try each index file in order
- Default: try `index.html`, then `index.htm`
- Applies to root and all subdirectories
- Example: `/docs/` tries `/docs/index.html`, then `/docs/index.htm`

**SPA mode** (like nginx `try_files $uri $uri/ /index.html`):
- When enabled with `--spa`, non-existent paths serve the spa-index
- Files with extensions (`.js`, `.css`, `.png`) still return 404 if missing
- Paths without extensions (like `/users/123`) serve the SPA index
- API paths can be excluded (see `--spa-exclude`)

#### Directory Listing

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--dir-listing` | `-l` | bool | `true` | Enable directory listing |
| `--dir-listing-format` | | string | `html` | Format: `html`, `json`, `text` |
| `--hidden` | | bool | `false` | Show hidden files (dotfiles) |
| `--ignore` | | []string | `[]` | Glob patterns to ignore |

#### Caching & Headers

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--cache` | | string | `3600` | Cache-Control max-age (seconds), `-1` to disable |
| `--cache-immutable` | | []string | `[]` | Patterns for immutable cache (e.g., `*.hash.*`) |
| `--etag` | | bool | `true` | Generate ETag headers |
| `--last-modified` | | bool | `true` | Send Last-Modified headers |
| `--headers` | | []string | `[]` | Custom headers (`Header: Value`) |

#### Compression

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--gzip` | `-z` | bool | `false` | Enable gzip compression |
| `--brotli` | | bool | `false` | Enable brotli compression |
| `--compression-level` | | int | `6` | Compression level (1-9) |
| `--compression-min-size` | | int | `1024` | Minimum size to compress (bytes) |

#### CORS

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--cors` | | bool | `false` | Enable CORS with permissive defaults |
| `--cors-origin` | | string | `*` | Access-Control-Allow-Origin |
| `--cors-methods` | | string | `GET,HEAD,OPTIONS` | Allowed methods |
| `--cors-headers` | | string | `*` | Allowed headers |
| `--cors-credentials` | | bool | `false` | Allow credentials |
| `--cors-max-age` | | int | `86400` | Preflight cache duration (seconds) |

#### Security Headers

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--security-headers` | | bool | `false` | Enable recommended security headers |
| `--hsts` | | bool | `false` | Enable HSTS (requires TLS) |
| `--hsts-max-age` | | int | `31536000` | HSTS max-age (seconds) |
| `--x-frame-options` | | string | `DENY` | X-Frame-Options value |
| `--x-content-type-options` | | bool | `true` | Add X-Content-Type-Options: nosniff |

#### Error Pages

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--404-page` | | string | `` | Custom 404 page path |
| `--500-page` | | string | `` | Custom 500 page path |

#### Advanced

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--base-path` | | string | `/` | Base path prefix for all URLs |
| `--rewrites` | | []string | `[]` | URL rewrite rules (`from:to`) |
| `--redirects` | | []string | `[]` | Redirect rules (`from:to:status`) |
| `--range` | | bool | `true` | Support range requests (byte serving) |
| `--symlinks` | | bool | `true` | Follow symbolic links |
| `--max-age-override` | | []string | `[]` | Per-pattern max-age (`*.js:31536000`) |

### Configuration (YAML)

```yaml
serve:
  # Basic
  dir: ./public
  port: 8080
  host: localhost

  # Index & Routing (nginx-style: tries each in order per directory)
  index:
    - index.html
    - index.htm
  spa:
    enabled: true
    index: index.html        # Fallback file (relative to root)
    exclude:                 # Paths that should 404, not fallback
      - /api/*
      - /static/*
  not_found: spa             # 404, spa, or page:/errors/404.html
  trailing_slash: auto       # add, remove, auto
  clean_urls: false

  # Directory Listing
  dir_listing: true
  dir_listing_format: html  # html, json, text
  hidden: false
  ignore:
    - "*.tmp"
    - ".git"
    - "node_modules"

  # Caching
  cache:
    default: 3600
    immutable_patterns:
      - "*.hash.*"
      - "assets/*"
    overrides:
      "*.html": 0
      "*.js": 31536000
      "*.css": 31536000
      "images/*": 604800
  etag: true
  last_modified: true

  # Compression (on-the-fly, no pre-compressed files needed)
  compression:
    enabled: true
    gzip: true
    brotli: true
    level: 6
    min_size: 1024
    types:
      - text/*
      - application/json
      - application/javascript
      - image/svg+xml

  # CORS
  cors:
    enabled: true
    origin: "*"
    methods: "GET, HEAD, OPTIONS"
    headers: "*"
    credentials: false
    max_age: 86400

  # Security Headers
  security:
    enabled: false
    hsts:
      enabled: false
      max_age: 31536000
      include_subdomains: true
      preload: false
    x_frame_options: DENY
    x_content_type_options: true
    referrer_policy: strict-origin-when-cross-origin
    csp: ""  # Custom Content-Security-Policy

  # Custom Headers
  headers:
    X-Powered-By: Radix
    X-Custom-Header: value

  # Error Pages
  error_pages:
    404: ./errors/404.html
    500: ./errors/500.html

  # URL Handling
  rewrites:
    - from: /old-path/*
      to: /new-path/$1
    - from: /api/*
      to: /v2/api/$1

  redirects:
    - from: /legacy
      to: /modern
      status: 301
    - from: /temp
      to: /permanent
      status: 302

  # Advanced
  base_path: /
  range: true
  symlinks: true
```

### Implementation Notes

#### MIME Type Detection

Use Go's `mime` package with extended types:

```go
var extraMimeTypes = map[string]string{
    ".wasm":  "application/wasm",
    ".mjs":   "application/javascript",
    ".woff2": "font/woff2",
    ".avif":  "image/avif",
    ".webp":  "image/webp",
    ".webm":  "video/webm",
}
```

#### Index File Resolution

For directory requests (like nginx `index` directive):

```
Request: /docs/
1. Try /docs/index.html → if exists, serve it
2. Try /docs/index.htm  → if exists, serve it
3. If dir_listing enabled → show directory listing
4. Otherwise → 404
```

This applies to every subdirectory, not just root.

#### SPA Mode Behavior

Modeled after nginx `try_files $uri $uri/ /index.html`:

```
Request: /users/123/profile

1. Try exact path: /users/123/profile (file) → not found
2. Try as directory: /users/123/profile/ with index → not found
3. Check if path has file extension (.js, .css, etc.)
   - If YES and not found → 404 (it's a missing asset)
   - If NO → serve SPA index from root
4. Check exclusion patterns (e.g., /api/*)
   - If matches exclusion → 404 (let API handle its own errors)
   - Otherwise → serve SPA index
```

**Why extension matters**: Requests like `/app.js` or `/styles.css` are clearly
asset requests - if they're missing, the developer needs a 404 to debug.
Requests like `/users/123` are clearly routes that the SPA should handle.

#### Compression Strategy

1. If compression enabled, compress on-the-fly
2. Honor `Accept-Encoding` header priority (prefer brotli > gzip)
3. Skip compression for already-compressed formats (images, videos, archives)
4. Only compress above minimum size threshold

#### ETag Generation

Generate weak ETags based on file modification time and size:

```go
func generateETag(info os.FileInfo) string {
    return fmt.Sprintf(`W/"%x-%x"`, info.ModTime().Unix(), info.Size())
}
```

### Suggested Additions (Developer-Focused)

Based on common local development needs:

| Feature | Priority | Rationale |
|---------|----------|-----------|
| **Graceful shutdown** | High | Clean exit on Ctrl+C, no orphaned connections |
| **Live reload integration** | High | WebSocket endpoint for browser refresh tools |
| **Request logging** | High | See what's being requested during development |
| **Port conflict handling** | Medium | Auto-increment port if busy, or clear error message |
| **QR code for mobile** | Low | Display QR code for LAN URL (mobile testing) |
| **Request ID** | Low | X-Request-ID header for debugging |

**Explicitly NOT included** (production concerns):
- Pre-compressed file serving (files change constantly in dev)
- Virtual hosts (use different ports instead)
- Rate limiting (not needed for local development)
- Advanced caching strategies (usually want fresh files in dev)

### Examples

```bash
# Basic static server (current directory)
radix serve

# Serve build directory with SPA routing
radix serve ./dist --spa --port 3000

# Frontend dev with API on different port (CORS enabled)
radix serve ./src --cors --cache -1

# React/Vue/Angular development build
radix serve ./build --spa --port 3000 --browse

# Serve with compression (useful for testing bundle sizes)
radix serve ./dist --gzip --port 8080

# Share on local network (mobile testing)
radix serve ./public --host 0.0.0.0 --port 3000

# Serve with custom error pages
radix serve ./public \
  --404-page ./errors/404.html \
  --spa
```

---

## Proxy Command

### Purpose

Reverse proxy requests to backend services. Essential for local development when frontend and backend run on different ports, or for debugging API traffic.

### CLI Interface

```bash
radix proxy [target] [flags]
```

### Flags

#### Basic Options

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--target` | `-t` | string | *required* | Target URL to proxy to |
| `--port` | `-p` | int | `8080` | Port to listen on |
| `--host` | `-H` | string | `localhost` | Host/IP to bind to |

#### Path Handling

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--strip-prefix` | | string | `` | Remove prefix before forwarding |
| `--add-prefix` | | string | `` | Add prefix before forwarding |
| `--rewrite` | | []string | `[]` | Path rewrite rules (`from=to`) |

#### Headers

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--header` | | []string | `[]` | Add/override request headers |
| `--remove-header` | | []string | `[]` | Remove request headers |
| `--response-header` | | []string | `[]` | Add/override response headers |
| `--host-header` | | string | `auto` | Host header: `auto`, `preserve`, or custom |
| `--forward-host` | | bool | `false` | Forward original Host header |

#### Timeouts

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--timeout` | | duration | `30s` | Total request timeout |
| `--dial-timeout` | | duration | `10s` | Connection dial timeout |
| `--read-timeout` | | duration | `30s` | Response read timeout |
| `--write-timeout` | | duration | `30s` | Request write timeout |
| `--idle-timeout` | | duration | `90s` | Keep-alive idle timeout |

#### Streaming & SSE Support

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--streaming` | | bool | `true` | Enable streaming response support |
| `--no-buffering` | | bool | `false` | Disable response buffering globally |
| `--flush-interval` | | duration | `100ms` | Flush interval for streaming |
| `--sse-paths` | | []string | `[]` | Paths to treat as SSE (auto-detected) |
| `--sse-timeout` | | duration | `0` | SSE connection timeout (0=infinite) |

#### WebSocket Support

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--websocket` | `-w` | bool | `true` | Enable WebSocket proxying |
| `--ws-read-buffer` | | int | `4096` | WebSocket read buffer size |
| `--ws-write-buffer` | | int | `4096` | WebSocket write buffer size |
| `--ws-ping-interval` | | duration | `30s` | WebSocket ping interval |

#### Backend TLS

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--tls-skip-verify` | `-k` | bool | `false` | Skip TLS certificate verification |
| `--backend-ca` | | string | `` | CA certificate for backend verification |
| `--backend-cert` | | string | `` | Client certificate for backend mTLS |
| `--backend-key` | | string | `` | Client key for backend mTLS |

#### CORS (Proxy-Level)

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--cors` | | bool | `false` | Add CORS headers to responses |
| `--cors-origin` | | string | `*` | Access-Control-Allow-Origin |

#### Auth Extensions

Radix supports pluggable auth header injection via the `HeaderProvider` Go interface. This is designed for corporate forks that compile in a custom provider (e.g., Okta, Azure AD) so that auth headers are injected automatically — no per-engineer configuration needed.

**How it works:** If a fork registers exactly one custom `HeaderProvider`, it is used automatically for all proxied requests. No CLI flags or YAML config required. Engineers just run `radix proxy` and get auth headers. When multiple providers are registered, select one explicitly with `proxy.auth.provider: <name>` (an unregistered name is a hard startup error); if none is selected the registry does not auto-pick, and resolution falls back to the static provider.

**Built-in provider:** `static` — Injects fixed headers from the `--header` flag or config file. Used when no custom provider is registered, or when multiple are registered and none is explicitly selected.

See [IMPLEMENTATION_PLAN.md Section 15](../IMPLEMENTATION_PLAN.md#15-auth-extensions--middleware-extensibility) for the full `HeaderProvider` interface and fork integration pattern.

#### Logging & Debugging

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--log-requests` | | bool | `true` | Log proxied requests |
| `--log-body` | | bool | `false` | Log request/response bodies |
| `--log-body-max` | | int | `4096` | Max body bytes to log |

### Configuration (YAML)

```yaml
proxy:
  # Basic
  target: http://localhost:3000
  port: 8080
  host: localhost

  # Path handling
  strip_prefix: /api
  add_prefix: /v2
  rewrites:
    - from: /old/*
      to: /new/$1
    - from: /users/:id
      to: /api/users/$id

  # Headers
  headers:
    request:
      add:
        X-Forwarded-Proto: https
        X-Real-IP: $remote_addr
      remove:
        - Authorization  # Strip auth before forwarding
    response:
      add:
        X-Proxy: Radix
      remove:
        - Server
  host_header: auto  # auto, preserve, or custom value

  # Timeouts
  timeouts:
    total: 30s
    dial: 10s
    read: 30s
    write: 30s
    idle: 90s

  # Streaming & SSE
  streaming:
    enabled: true
    buffer_response: false
    flush_interval: 100ms
    sse:
      paths:
        - /events
        - /stream
        - /api/chat/*
      timeout: 0  # 0 = no timeout
      retry_interval: 3000  # SSE retry field (ms)

  # WebSocket
  websocket:
    enabled: true
    read_buffer: 4096
    write_buffer: 4096
    ping_interval: 30s
    pong_timeout: 60s
    origins:
      - localhost
      - "*.example.com"

  # Backend TLS
  backend_tls:
    skip_verify: false
    ca: ./certs/backend-ca.pem
    cert: ./certs/client.pem
    key: ./certs/client-key.pem
    server_name: api.internal

  # Auth extensions (HeaderProvider)
  # If a fork registers a custom HeaderProvider (e.g., Okta), it is used
  # automatically — no configuration needed. This section is only required
  # when multiple providers are registered and you need to select one,
  # or to pass provider-specific settings.
  # See IMPLEMENTATION_PLAN.md Section 15 for the HeaderProvider interface.
  # auth:
  #   provider: okta          # Only needed to disambiguate multiple providers
  #   config:                 # Provider-specific settings — read by the provider
  #     audience: "api.internal"   # itself; radix core does not consume/pass this.

  # CORS
  cors:
    enabled: true
    origin: "*"
    methods: "GET, POST, PUT, DELETE, PATCH, OPTIONS"
    headers: "Content-Type, Authorization"
    credentials: true
    max_age: 86400

  # Request/Response modifications
  request_modifiers:
    - type: header
      action: set
      name: X-Request-ID
      value: $uuid

  response_modifiers:
    - type: header
      action: set
      name: X-Response-Time
      value: $response_time

  # Retry policy
  retry:
    enabled: true
    attempts: 3
    per_try_timeout: 10s
    retry_on:
      - 502
      - 503
      - 504
      - connect-failure
      - reset

  # Circuit breaker
  circuit_breaker:
    enabled: false
    threshold: 5  # failures before opening
    timeout: 30s  # time before half-open

  # Logging
  logging:
    requests: true
    responses: true
    body: false
    body_max_size: 4096
```

### Implementation Notes

#### Streaming Response Support

Critical for SSE (Server-Sent Events) and AI chat applications:

```go
type streamingTransport struct {
    base http.RoundTripper
}

func (t *streamingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
    resp, err := t.base.RoundTrip(req)
    if err != nil {
        return nil, err
    }

    // Detect streaming responses
    contentType := resp.Header.Get("Content-Type")
    if isStreamingContentType(contentType) {
        // Ensure no buffering
        resp.Header.Set("X-Accel-Buffering", "no")
        resp.Header.Set("Cache-Control", "no-cache")
    }

    return resp, nil
}

func isStreamingContentType(ct string) bool {
    streamingTypes := []string{
        "text/event-stream",           // SSE
        "application/x-ndjson",         // Newline-delimited JSON
        "application/stream+json",      // JSON streaming
        "text/plain; charset=utf-8",    // Often used for streaming
    }
    for _, st := range streamingTypes {
        if strings.HasPrefix(ct, st) {
            return true
        }
    }
    return false
}
```

#### SSE-Specific Handling

```go
type sseProxy struct {
    target    *url.URL
    flusher   http.Flusher
    timeout   time.Duration
}

func (p *sseProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    // Set SSE headers
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")
    w.Header().Set("X-Accel-Buffering", "no")  // Disable nginx buffering

    // Ensure we can flush
    flusher, ok := w.(http.Flusher)
    if !ok {
        http.Error(w, "Streaming not supported", http.StatusInternalServerError)
        return
    }

    // Create upstream request
    proxyReq := p.createUpstreamRequest(r)

    // Use HTTP/1.1 for upstream (HTTP/2 can cause issues with SSE)
    client := &http.Client{
        Transport: &http.Transport{
            ForceAttemptHTTP2:     false,
            MaxIdleConns:          100,
            IdleConnTimeout:       90 * time.Second,
            ResponseHeaderTimeout: 0,  // No timeout for SSE
        },
    }

    resp, err := client.Do(proxyReq)
    if err != nil {
        http.Error(w, err.Error(), http.StatusBadGateway)
        return
    }
    defer resp.Body.Close()

    // Copy headers
    for k, vv := range resp.Header {
        for _, v := range vv {
            w.Header().Add(k, v)
        }
    }
    w.WriteHeader(resp.StatusCode)

    // Stream the response
    buf := make([]byte, 1024)
    for {
        n, err := resp.Body.Read(buf)
        if n > 0 {
            w.Write(buf[:n])
            flusher.Flush()
        }
        if err != nil {
            break
        }
    }
}
```

#### WebSocket Proxy

```go
func (p *Proxy) handleWebSocket(w http.ResponseWriter, r *http.Request) {
    // Upgrade connection
    upgrader := websocket.Upgrader{
        ReadBufferSize:  p.config.WSReadBuffer,
        WriteBufferSize: p.config.WSWriteBuffer,
        CheckOrigin:     p.checkWSOrigin,
    }

    clientConn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        return
    }
    defer clientConn.Close()

    // Connect to backend
    backendURL := p.wsTargetURL(r)
    backendConn, _, err := websocket.DefaultDialer.Dial(backendURL, nil)
    if err != nil {
        return
    }
    defer backendConn.Close()

    // Bidirectional copy
    errChan := make(chan error, 2)
    go p.copyWS(clientConn, backendConn, errChan)
    go p.copyWS(backendConn, clientConn, errChan)

    <-errChan
}
```

#### X-Forwarded Headers

Automatically add standard proxy headers:

```go
func addForwardedHeaders(req *http.Request, original *http.Request) {
    if clientIP, _, err := net.SplitHostPort(original.RemoteAddr); err == nil {
        if prior := original.Header.Get("X-Forwarded-For"); prior != "" {
            clientIP = prior + ", " + clientIP
        }
        req.Header.Set("X-Forwarded-For", clientIP)
    }

    if original.TLS != nil {
        req.Header.Set("X-Forwarded-Proto", "https")
    } else {
        req.Header.Set("X-Forwarded-Proto", "http")
    }

    req.Header.Set("X-Forwarded-Host", original.Host)
    req.Header.Set("X-Real-IP", strings.Split(original.RemoteAddr, ":")[0])
}
```

### Examples

```bash
# Basic proxy
radix proxy http://localhost:3000

# API proxy with path stripping
radix proxy http://api.internal:8080 \
  --strip-prefix /api \
  --port 8080

# SSE/Streaming proxy for AI chat
radix proxy http://localhost:11434 \
  --streaming \
  --no-buffering \
  --sse-paths "/api/chat,/api/generate" \
  --sse-timeout 0

# WebSocket proxy
radix proxy ws://localhost:8080 \
  --websocket \
  --ws-ping-interval 30s

# Proxy with CORS (for frontend dev)
radix proxy http://localhost:3000 \
  --cors \
  --cors-origin "http://localhost:5173"

# Proxy with custom headers
radix proxy http://api.example.com \
  --header "Authorization: Bearer token123" \
  --header "X-Custom: value"

# Development proxy with TLS skip
radix proxy https://self-signed.local:8443 \
  --tls-skip-verify

# If your fork compiles in an auth provider (e.g., Okta),
# it's used automatically — no flags needed:
radix proxy http://backend:8080
```

---

## Echo Command

### Purpose

Echo server that returns detailed information about incoming HTTP requests. Essential for debugging webhooks, testing HTTP clients, and understanding request structure.

### CLI Interface

```bash
radix echo [flags]
```

### Flags

#### Basic Options

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--port` | `-p` | int | `8080` | Port to listen on |
| `--host` | `-H` | string | `localhost` | Host/IP to bind to |

#### Response Control

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--status` | `-s` | int | `200` | Default response status code |
| `--delay` | | duration | `0` | Response delay |
| `--delay-jitter` | | duration | `0` | Random jitter added to delay |
| `--body` | | string | `` | Custom response body (overrides echo) |
| `--header` | | []string | `[]` | Custom response headers |
| `--content-type` | | string | `application/json` | Response Content-Type |

#### Echo Behavior

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--echo-body` | | bool | `true` | Include request body in response |
| `--echo-headers` | | bool | `true` | Include request headers in response |
| `--echo-query` | | bool | `true` | Include query parameters in response |
| `--body-limit` | | int | `1048576` | Max request body size (1MB) |
| `--pretty` | | bool | `true` | Pretty-print JSON response |

#### Path-Based Responses

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--status-from-path` | | bool | `false` | Use path as status (e.g., `/404` → 404) |
| `--delay-from-path` | | bool | `false` | Use path as delay (e.g., `/delay/500ms`) |

#### Logging

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--log-body` | | bool | `false` | Log request bodies |
| `--log-body-max` | | int | `4096` | Max body bytes to log |

### Configuration (YAML)

```yaml
echo:
  port: 8080
  host: localhost

  # Default response
  response:
    status: 200
    delay: 0
    delay_jitter: 0
    body: ""  # Empty = echo mode
    content_type: application/json
    headers:
      X-Echo-Server: Radix

  # Echo behavior
  echo:
    body: true
    headers: true
    query: true
    cookies: true
    tls: true  # Include TLS info when available

  # Limits
  limits:
    body_size: 1048576  # 1MB
    header_size: 65536  # 64KB

  # Path-based behavior
  path_handlers:
    status_from_path: false  # /404 returns 404
    delay_from_path: false   # /delay/500ms delays 500ms

  # Special endpoints
  endpoints:
    health: /_health
    ready: /_ready

  # Logging
  logging:
    body: false
    body_max_size: 4096
```

### Response Format

```json
{
  "request": {
    "method": "POST",
    "url": "/api/users?limit=10",
    "path": "/api/users",
    "query": {
      "limit": ["10"]
    },
    "headers": {
      "Content-Type": ["application/json"],
      "Authorization": ["Bearer xxx..."],
      "User-Agent": ["curl/7.88.1"],
      "Accept": ["*/*"]
    },
    "body": {
      "name": "John Doe",
      "email": "john@example.com"
    },
    "body_raw": "{\"name\":\"John Doe\",\"email\":\"john@example.com\"}",
    "body_size": 52,
    "cookies": {
      "session_id": "abc123"
    }
  },
  "client": {
    "ip": "127.0.0.1",
    "port": 54321,
    "remote_addr": "127.0.0.1:54321"
  },
  "server": {
    "hostname": "localhost",
    "port": 8080,
    "protocol": "HTTP/1.1"
  },
  "tls": {
    "enabled": false,
    "version": "",
    "cipher_suite": "",
    "server_name": "",
    "client_cert": null
  },
  "timing": {
    "timestamp": "2026-02-08T12:00:00.000Z",
    "unix": 1770681600,
    "unix_nano": 1770681600000000000
  },
  "echo": {
    "version": "1.0.0",
    "delay_applied": "0s",
    "request_id": "req_abc123xyz"
  }
}
```

### Implementation Notes

#### Body Parsing

Attempt to parse body as JSON for structured echo:

```go
func parseBody(body []byte, contentType string) (interface{}, string) {
    if len(body) == 0 {
        return nil, ""
    }

    raw := string(body)

    // Try JSON first
    if strings.Contains(contentType, "json") {
        var parsed interface{}
        if err := json.Unmarshal(body, &parsed); err == nil {
            return parsed, raw
        }
    }

    // Try form data
    if strings.Contains(contentType, "form-urlencoded") {
        if values, err := url.ParseQuery(raw); err == nil {
            return values, raw
        }
    }

    // Return as string
    return nil, raw
}
```

#### Path-Based Status Codes

```go
func statusFromPath(path string) (int, bool) {
    // Match /status/XXX or /XXX
    re := regexp.MustCompile(`^/(?:status/)?(\d{3})$`)
    if matches := re.FindStringSubmatch(path); matches != nil {
        status, _ := strconv.Atoi(matches[1])
        if status >= 100 && status < 600 {
            return status, true
        }
    }
    return 0, false
}
```

### Examples

```bash
# Basic echo server
radix echo

# Echo with delay (simulate slow API)
radix echo --delay 2s

# Echo with random jitter
radix echo --delay 500ms --delay-jitter 200ms

# Custom status code
radix echo --status 201

# Path-based status codes
radix echo --status-from-path
# Then: curl localhost:8080/404 → returns 404
# Then: curl localhost:8080/500 → returns 500

# Custom response body
radix echo --body '{"message": "OK"}' --status 200

# Webhook debugging
radix echo --log-body --port 9000
# Then configure webhook to POST to localhost:9000
```

---

## Mock Command

### Purpose

API mocking server with two modes:
1. **Built-in httpbin-style endpoints** - Zero-config testing endpoints (inspired by [httpbin.org](https://httpbin.org))
2. **Custom route configuration** - Project-specific mocks from YAML

### CLI Interface

```bash
radix mock [config-file] [flags]
```

### Flags

#### Basic Options

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--port` | `-p` | int | `8080` | Port to listen on |
| `--host` | `-H` | string | `localhost` | Host/IP to bind to |
| `--routes` | `-r` | string | `` | Custom routes config file (YAML) |
| `--watch` | `-w` | bool | `false` | Watch config file for changes |

#### Built-in Endpoints

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--builtin` | | bool | `true` | Enable built-in httpbin-style endpoints |
| `--prefix` | | string | `` | Prefix for built-in endpoints (e.g., `/_`) |

#### Behavior

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--latency` | | duration | `0` | Global artificial latency |
| `--latency-jitter` | | duration | `0` | Random latency jitter |
| `--fail-rate` | | float | `0` | Random failure rate (0-100%) |
| `--fail-status` | | int | `500` | Status code for random failures |
| `--cors` | | bool | `true` | Enable CORS by default |

### Built-in Endpoints (httpbin-style)

When `--builtin` is enabled (default), these endpoints are available with zero configuration:

#### HTTP Methods

| Endpoint | Description |
|----------|-------------|
| `GET /get` | Returns GET request data (headers, args, origin, url) |
| `POST /post` | Returns POST request data including body |
| `PUT /put` | Returns PUT request data |
| `PATCH /patch` | Returns PATCH request data |
| `DELETE /delete` | Returns DELETE request data |
| `* /anything` | Returns request data for any HTTP method |
| `* /anything/*` | Returns request data, including the URL path |

#### Request Inspection

| Endpoint | Description |
|----------|-------------|
| `GET /ip` | Returns the client's origin IP address |
| `GET /uuid` | Returns a UUID4 |
| `GET /user-agent` | Returns the User-Agent header |
| `GET /headers` | Returns all request headers |

#### Status Codes

| Endpoint | Description |
|----------|-------------|
| `* /status/:code` | Returns the specified HTTP status code |
| `* /status/:code1,:code2` | Returns random status from list |

#### Dynamic Data

| Endpoint | Description |
|----------|-------------|
| `GET /delay/:n` | Delays response by n seconds (max 10) |
| `GET /bytes/:n` | Returns n random bytes |
| `GET /stream-bytes/:n` | Streams n random bytes (chunked) |
| `GET /stream/:n` | Streams n JSON objects |
| `GET /range/:n` | Returns n bytes, supports Range header |
| `GET /drip` | Drips data: `?duration=2&numbytes=10&code=200` |

#### Response Formats

| Endpoint | Description |
|----------|-------------|
| `GET /html` | Returns an HTML page |
| `GET /xml` | Returns XML content |
| `GET /json` | Returns JSON content |
| `GET /robots.txt` | Returns robots.txt rules |
| `GET /deny` | Returns page denied by robots.txt |
| `GET /encoding/utf8` | Returns UTF-8 encoded content |

#### Compression

| Endpoint | Description |
|----------|-------------|
| `GET /gzip` | Returns gzip-encoded response |
| `GET /deflate` | Returns deflate-encoded response |
| `GET /brotli` | Returns brotli-encoded response |

#### Redirects

| Endpoint | Description |
|----------|-------------|
| `GET /redirect/:n` | 302 redirects n times, then returns |
| `GET /redirect-to?url=` | 302 redirects to the specified URL |
| `GET /redirect-to?url=&status_code=` | Redirects with custom status |
| `GET /relative-redirect/:n` | Relative 302 redirects n times |
| `GET /absolute-redirect/:n` | Absolute 302 redirects n times |

#### Cookies

| Endpoint | Description |
|----------|-------------|
| `GET /cookies` | Returns current cookies |
| `GET /cookies/set?name=value` | Sets cookies and redirects to /cookies |
| `GET /cookies/set/:name/:value` | Sets a cookie |
| `GET /cookies/delete?name` | Deletes specified cookies |

#### Authentication

| Endpoint | Description |
|----------|-------------|
| `GET /basic-auth/:user/:pass` | Challenges HTTP Basic Auth |
| `GET /hidden-basic-auth/:user/:pass` | 404 if auth fails (no challenge) |
| `GET /bearer` | Checks Bearer token auth |

#### Images

| Endpoint | Description |
|----------|-------------|
| `GET /image` | Returns image based on Accept header |
| `GET /image/png` | Returns a PNG image |
| `GET /image/jpeg` | Returns a JPEG image |
| `GET /image/webp` | Returns a WebP image |
| `GET /image/svg` | Returns an SVG image |

#### Caching

| Endpoint | Description |
|----------|-------------|
| `GET /cache` | Returns 304 if If-Modified-Since/If-None-Match |
| `GET /cache/:n` | Sets Cache-Control: max-age=n |
| `GET /etag/:etag` | Responds to If-None-Match/If-Match headers |
| `GET /response-headers?key=val` | Returns specified headers in response |

**Example built-in endpoint response (`GET /get?foo=bar`):**
```json
{
  "args": {"foo": "bar"},
  "headers": {
    "Accept": "*/*",
    "Host": "localhost:8080",
    "User-Agent": "curl/8.0.0"
  },
  "origin": "127.0.0.1",
  "url": "http://localhost:8080/get?foo=bar"
}
```

### Landing Page (Browser Access)

When accessing `http://localhost:8080/` in a browser, the mock server displays an
interactive landing page (inspired by httpbin.org) that lets developers explore
all available endpoints.

**Design Requirements:**
- Built with TailwindCSS (included via CDN for simplicity)
- Single self-contained HTML page (no external dependencies beyond Tailwind CDN)
- Dark/light mode support (respects `prefers-color-scheme`)
- Mobile responsive

**Layout:**

```
┌─────────────────────────────────────────────────────────────────┐
│  radix mock                                        [Dark Mode]  │
│  HTTP Request & Response Testing Service                        │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐ │
│  │ HTTP Methods    │  │ Request Inspect │  │ Status Codes    │ │
│  │ ─────────────── │  │ ─────────────── │  │ ─────────────── │ │
│  │ GET  /get       │  │ GET  /ip        │  │ GET  /status/:n │ │
│  │ POST /post      │  │ GET  /uuid      │  │                 │ │
│  │ PUT  /put       │  │ GET  /headers   │  │ Try: /status/418│ │
│  │ ...             │  │ GET  /user-agent│  │                 │ │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘ │
│                                                                 │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐ │
│  │ Dynamic Data    │  │ Redirects       │  │ Cookies         │ │
│  │ ─────────────── │  │ ─────────────── │  │ ─────────────── │ │
│  │ GET /delay/:n   │  │ GET /redirect/:n│  │ GET /cookies    │ │
│  │ GET /bytes/:n   │  │ GET /redirect-to│  │ GET /cookies/set│ │
│  │ GET /stream/:n  │  │ ...             │  │ ...             │ │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘ │
│                                                                 │
│  ── Custom Routes (from routes.yml) ──────────────────────────  │
│  POST /api/users          Create a new user                     │
│  GET  /api/users/:id      Get user by ID                        │
│  ...                                                            │
│                                                                 │
├─────────────────────────────────────────────────────────────────┤
│  Radix v1.0.0 · Local Development Server · Metrics: :8739      │
└─────────────────────────────────────────────────────────────────┘
```

**Features:**
- Endpoints are clickable (opens in new tab or shows response inline)
- Each endpoint shows HTTP method with color coding (GET=green, POST=blue, etc.)
- Description text for each endpoint
- Collapsible sections for each category
- If custom routes are loaded, they appear in a separate "Custom Routes" section
- Quick copy-to-clipboard for curl commands
- Link to metrics dashboard

**Implementation Notes:**
- Detect browser vs API client via `Accept` header
  - `Accept: text/html` → serve landing page
  - `Accept: application/json` or curl → serve JSON (for `/` endpoint)
- Landing page is generated at startup (not on every request)
- Tailwind CSS loaded from CDN: `https://cdn.tailwindcss.com`

### Custom Route Configuration (YAML)

For project-specific mocks, provide a routes file:

```yaml
# mock-routes.yml

# Global settings
settings:
  port: 8080
  host: localhost
  latency: 0
  latency_jitter: 0
  fail_rate: 0
  fail_status: 500
  cors:
    enabled: true
    origin: "*"

  # Fallback for unmatched routes
  fallback:
    type: "404"  # 404, echo, proxy
    proxy_target: ""

# Route definitions
routes:
  # Simple GET endpoint
  - path: /api/health
    method: GET
    response:
      status: 200
      body: '{"status": "healthy"}'
      headers:
        Content-Type: application/json

  # Path parameters
  - path: /api/users/:id
    method: GET
    response:
      status: 200
      body: |
        {
          "id": "{{params.id}}",
          "name": "User {{params.id}}",
          "email": "user{{params.id}}@example.com"
        }

  # Response from file
  - path: /api/products
    method: GET
    response:
      file: ./mocks/products.json
      headers:
        Content-Type: application/json

  # POST with request body access
  - path: /api/users
    method: POST
    response:
      status: 201
      body: |
        {
          "id": "{{uuid}}",
          "name": "{{body.name}}",
          "email": "{{body.email}}",
          "created_at": "{{now}}"
        }

  # Conditional responses
  - path: /api/auth/login
    method: POST
    conditions:
      - match:
          body.username: admin
          body.password: secret
        response:
          status: 200
          body: '{"token": "{{uuid}}", "user": "admin"}'
      - match:
          body.username: "*"
        response:
          status: 401
          body: '{"error": "Invalid credentials"}'

  # Query parameter matching
  - path: /api/search
    method: GET
    conditions:
      - match:
          query.q: ""
        response:
          status: 400
          body: '{"error": "Query parameter q is required"}'
      - match:
          query.q: "*"
        response:
          status: 200
          body: '{"results": [], "query": "{{query.q}}"}'

  # Header matching
  - path: /api/protected
    method: GET
    conditions:
      - match:
          headers.Authorization: "Bearer valid-token"
        response:
          status: 200
          body: '{"data": "secret"}'
      - match:
          headers.Authorization: ""
        response:
          status: 401
          body: '{"error": "Missing authorization"}'
      - default: true
        response:
          status: 403
          body: '{"error": "Invalid token"}'

  # Delayed response
  - path: /api/slow
    method: GET
    delay: 2s
    response:
      status: 200
      body: '{"message": "Finally!"}'

  # Random delay range
  - path: /api/variable
    method: GET
    delay: 100ms
    delay_jitter: 500ms
    response:
      status: 200
      body: '{"message": "Variable delay"}'

  # Multiple methods
  - path: /api/resource
    methods: [GET, POST, PUT, DELETE]
    response:
      status: 200
      body: '{"method": "{{method}}", "path": "{{path}}"}'

  # Regex path matching
  - path: "regex:/api/v[0-9]+/users"
    method: GET
    response:
      status: 200
      body: '{"users": []}'

  # Proxy fallback for specific route
  - path: /api/external/*
    proxy: https://api.external.com

  # Sequence responses (stateful)
  - path: /api/counter
    method: POST
    sequence:
      - body: '{"count": 1}'
      - body: '{"count": 2}'
      - body: '{"count": 3}'
      - repeat: true  # Loop back to first
        body: '{"count": 1, "reset": true}'

  # Random response selection
  - path: /api/random
    method: GET
    random:
      - weight: 70
        response:
          status: 200
          body: '{"result": "success"}'
      - weight: 20
        response:
          status: 500
          body: '{"error": "Random failure"}'
      - weight: 10
        response:
          status: 503
          body: '{"error": "Service unavailable"}'

  # WebSocket mock
  - path: /ws/chat
    websocket: true
    messages:
      - delay: 0
        data: '{"type": "connected", "id": "{{uuid}}"}'
      - delay: 1s
        data: '{"type": "message", "text": "Hello!"}'
      - delay: 2s
        data: '{"type": "message", "text": "How can I help?"}'
    echo: true  # Echo client messages back

  # SSE mock
  - path: /events
    sse: true
    events:
      - delay: 0
        event: connected
        data: '{"status": "ok"}'
      - delay: 1s
        event: update
        data: '{"value": {{random 1 100}}}'
        repeat: 10
        repeat_delay: 1s
```

### Template Functions

| Function | Description | Example |
|----------|-------------|---------|
| `{{uuid}}` | Generate UUID v4 | `550e8400-e29b-41d4-a716-446655440000` |
| `{{now}}` | Current ISO timestamp | `2026-02-08T12:00:00Z` |
| `{{now "RFC3339"}}` | Formatted timestamp | `2026-02-08T12:00:00Z` |
| `{{timestamp}}` | Unix timestamp | `1770681600` |
| `{{random min max}}` | Random integer | `42` |
| `{{randomFloat min max}}` | Random float | `3.14` |
| `{{randomString len}}` | Random alphanumeric | `aB3xY9` |
| `{{randomChoice "a" "b"}}` | Random selection | `a` or `b` |
| `{{lorem words}}` | Lorem ipsum text | `Lorem ipsum dolor...` |
| `{{params.name}}` | Path parameter | `123` |
| `{{query.key}}` | Query parameter | `value` |
| `{{body.field}}` | Request body field | `value` |
| `{{headers.Name}}` | Request header | `value` |
| `{{method}}` | HTTP method | `POST` |
| `{{path}}` | Request path | `/api/users` |
| `{{env "VAR"}}` | Environment variable | `value` |
| `{{file "path"}}` | File contents | `...` |
| `{{base64 "text"}}` | Base64 encode | `dGV4dA==` |
| `{{hash "sha256" "text"}}` | Hash value | `9f86d08...` |
| `{{seq}}` | Sequence counter | `1`, `2`, `3`... |
| `{{faker.name}}` | Fake name | `John Doe` |
| `{{faker.email}}` | Fake email | `john@example.com` |
| `{{faker.phone}}` | Fake phone | `555-1234` |
| `{{faker.address}}` | Fake address | `123 Main St` |

### Implementation Notes

#### Route Matching Priority

1. Exact path match with exact method
2. Exact path match with wildcard method
3. Parameterized path match (`:id`)
4. Regex path match (`regex:pattern`)
5. Glob path match (`/api/*`)
6. Fallback handler

#### Hot Reload

```go
type ConfigWatcher struct {
    path     string
    routes   atomic.Value  // *RouteConfig
    watcher  *fsnotify.Watcher
}

func (w *ConfigWatcher) Start() error {
    watcher, err := fsnotify.NewWatcher()
    if err != nil {
        return err
    }

    go func() {
        for {
            select {
            case event := <-watcher.Events:
                if event.Op&fsnotify.Write == fsnotify.Write {
                    if config, err := loadConfig(w.path); err == nil {
                        w.routes.Store(config)
                        log.Println("Routes reloaded")
                    }
                }
            case err := <-watcher.Errors:
                log.Println("Watcher error:", err)
            }
        }
    }()

    return watcher.Add(w.path)
}
```

### Examples

```bash
# Zero-config httpbin-style mock server
radix mock

# Test it immediately:
# curl localhost:8080/get
# curl localhost:8080/status/404
# curl localhost:8080/delay/2
# curl -X POST -d '{"foo":"bar"}' localhost:8080/post

# Built-in endpoints with prefix (avoid conflicts with custom routes)
radix mock --prefix /_test --routes ./api-mocks.yml
# Built-in: curl localhost:8080/_test/get
# Custom:   curl localhost:8080/api/users

# Custom routes only (disable built-in endpoints)
radix mock --builtin=false --routes ./api-mocks.yml

# Watch for config changes (hot reload)
radix mock --routes ./api-mocks.yml --watch

# Simulate slow API
radix mock --latency 200ms

# Chaos testing (10% random failures)
radix mock --fail-rate 10

# Combine with proxy fallback
radix mock --routes ./api-mocks.yml \
  --fallback proxy \
  --fallback-target http://localhost:3000
```

**Quick testing with curl:**
```bash
# Test status codes
curl -i localhost:8080/status/201
curl -i localhost:8080/status/500

# Test delays
curl localhost:8080/delay/3  # 3 second delay

# Inspect your request
curl -X POST -H "Authorization: Bearer token" \
  -d '{"name":"test"}' localhost:8080/anything

# Test redirects
curl -L localhost:8080/redirect/3  # follows 3 redirects

# Test basic auth
curl -u user:pass localhost:8080/basic-auth/user/pass

# Get random bytes
curl localhost:8080/bytes/1024 -o random.bin
```

---

## GenCert Command

### Purpose

Generate self-signed TLS certificates for local HTTPS development. Eliminates the need for external tools like `mkcert` or `openssl`.

### CLI Interface

```bash
radix gencert [flags]
```

### Flags

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--host` | | string | `localhost` | Comma-separated hostnames/IPs |
| `--output` | `-o` | string | `./certs` | Output directory |
| `--days` | | int | `365` | Certificate validity (days) |
| `--org` | | string | `Radix Development` | Organization name |
| `--key-size` | | int | `2048` | RSA key size (2048, 4096) |
| `--key-type` | | string | `rsa` | Key type: `rsa`, `ecdsa` |
| `--ecdsa-curve` | | string | `P-256` | ECDSA curve: `P-256`, `P-384`, `P-521` |
| `--ca` | | bool | `true` | Generate CA certificate |
| `--ca-cert` | | string | `` | Use existing CA certificate |
| `--ca-key` | | string | `` | Use existing CA private key |
| `--client` | | bool | `false` | Generate client certificate |
| `--pkcs12` | | bool | `false` | Also generate PKCS#12 bundle |
| `--pkcs12-password` | | string | `changeit` | PKCS#12 password |
| `--overwrite` | | bool | `false` | Overwrite existing files |

### Output Files

```
./certs/
├── ca.pem           # CA certificate (for browser import)
├── ca-key.pem       # CA private key (keep secure)
├── cert.pem         # Server certificate
├── key.pem          # Server private key
├── cert.p12         # PKCS#12 bundle (optional)
└── README.txt       # Usage instructions
```

### Configuration (YAML)

```yaml
gencert:
  hosts:
    - localhost
    - "127.0.0.1"
    - "::1"
    - "*.local.dev"
    - myapp.local

  output: ./certs
  days: 365

  organization: "Radix Development"
  organizational_unit: "Development"
  country: "US"
  province: ""
  locality: ""

  key:
    type: rsa  # rsa, ecdsa
    size: 2048  # RSA: 2048, 4096; ECDSA: ignored
    curve: P-256  # ECDSA: P-256, P-384, P-521

  ca:
    generate: true
    cert: ""  # Use existing CA
    key: ""

  pkcs12:
    generate: false
    password: changeit
```

### Implementation Notes

#### Certificate Generation

```go
func generateCertificate(config *CertConfig) (*Certificate, error) {
    // Generate private key
    var privateKey crypto.PrivateKey
    var publicKey crypto.PublicKey

    switch config.KeyType {
    case "rsa":
        key, err := rsa.GenerateKey(rand.Reader, config.KeySize)
        if err != nil {
            return nil, err
        }
        privateKey = key
        publicKey = &key.PublicKey
    case "ecdsa":
        curve := ellipticCurve(config.ECDSACurve)
        key, err := ecdsa.GenerateKey(curve, rand.Reader)
        if err != nil {
            return nil, err
        }
        privateKey = key
        publicKey = &key.PublicKey
    }

    // Create certificate template
    template := x509.Certificate{
        SerialNumber: big.NewInt(time.Now().UnixNano()),
        Subject: pkix.Name{
            Organization: []string{config.Organization},
            CommonName:   config.Hosts[0],
        },
        NotBefore:             time.Now(),
        NotAfter:              time.Now().AddDate(0, 0, config.Days),
        KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
        ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
        BasicConstraintsValid: true,
    }

    // Add SANs
    for _, host := range config.Hosts {
        if ip := net.ParseIP(host); ip != nil {
            template.IPAddresses = append(template.IPAddresses, ip)
        } else {
            template.DNSNames = append(template.DNSNames, host)
        }
    }

    // Sign certificate
    certDER, err := x509.CreateCertificate(
        rand.Reader,
        &template,
        config.CACert,  // Parent cert (or self for CA)
        publicKey,
        config.CAKey,   // Parent key (or self for CA)
    )
    if err != nil {
        return nil, err
    }

    return &Certificate{
        Cert:       certDER,
        PrivateKey: privateKey,
    }, nil
}
```

#### Trust Store Installation (Documentation)

```bash
# macOS - Add CA to System Keychain
sudo security add-trusted-cert -d -r trustRoot \
  -k /Library/Keychains/System.keychain ./certs/ca.pem

# Linux - Add to system certificates
sudo cp ./certs/ca.pem /usr/local/share/ca-certificates/radix-ca.crt
sudo update-ca-certificates

# Windows - Add to certificate store
certutil -addstore -f "ROOT" .\certs\ca.pem

# Firefox (all platforms) - Import manually
# Preferences → Privacy & Security → View Certificates → Import

# Chrome (uses system store on macOS/Windows, NSS on Linux)
# Linux: certutil -d sql:$HOME/.pki/nssdb -A -t "C,," -n "Radix CA" -i ./certs/ca.pem
```

### Examples

```bash
# Generate certs for localhost
radix gencert

# Multiple hosts
radix gencert --host "localhost,127.0.0.1,myapp.local,*.local.dev"

# Custom output directory
radix gencert --output ~/.radix/certs

# Longer validity
radix gencert --days 730

# ECDSA key (faster, smaller)
radix gencert --key-type ecdsa --ecdsa-curve P-256

# Using existing CA
radix gencert \
  --ca=false \
  --ca-cert ./my-ca/ca.pem \
  --ca-key ./my-ca/ca-key.pem

# Generate client certificate
radix gencert --client --host "client.local"

# With PKCS#12 for Java/browsers
radix gencert --pkcs12 --pkcs12-password mypassword
```

---

## Shared Infrastructure

### Metrics Dashboard

All server commands include a built-in metrics dashboard for live monitoring during development.

**Flags:**

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--metrics` | | bool | `true` | Enable metrics dashboard |
| `--metrics-port` | | int | `8739` | Port for metrics dashboard |
| `--no-metrics` | | bool | `false` | Disable metrics entirely |

**Dashboard Features:**

The metrics dashboard runs on a dedicated port (default: 8739) and provides:

1. **Status Code Distribution** (Pie Chart)
   - Color-coded segments: 2xx (green), 3xx (blue), 4xx (orange), 5xx (red)
   - Live updates as requests come in

2. **Request Trends** (Line Chart)
   - Requests over time with gridlines
   - Helps identify traffic patterns during testing

3. **Recent Requests Table**
   - Last 1000 requests (circular buffer for bounded memory)
   - Shows: method, path, status, response size, timestamp
   - Useful for debugging specific requests

**Implementation Notes:**
- SVG-based charts (no JavaScript dependencies)
- Thread-safe, non-blocking metrics collection
- Circular buffer ensures bounded memory usage
- Dashboard is a single static HTML page (server-rendered)

**Example:**
```bash
# Start server with metrics on custom port
radix serve --metrics-port 9000

# Disable metrics entirely
radix serve --no-metrics
```

**Why not Prometheus?**
Prometheus is designed for production monitoring infrastructure. For local development,
a simple visual dashboard is more useful - you can glance at it without setting up
Prometheus, Grafana, etc. If users need Prometheus, they can scrape the JSON endpoint.

### Logging

All commands support two independent logging outputs: **terminal** and **file**.

#### Terminal output (TTY)

The default terminal format is `dev` — a compact, colored format optimized for glanceability:

```
GET /index.html 200 2.3KB 12ms
POST /api/users 201 48B 340ms
GET /missing 404 - 1ms
```

- **Method** color-coded: GET=cyan, POST=green, PUT=yellow, DELETE=red, PATCH=magenta
- **Status** color-coded: 2xx=green, 3xx=cyan, 4xx=yellow, 5xx=red
- Colors disabled when `--no-color` flag is set **or** `NO_COLOR` env var is set (per [no-color.org](https://no-color.org))
- Colors also auto-disabled when stdout is not a TTY (e.g., piped to a file)
- Optional timestamp prefix with `--log-timestamp` (shows `HH:MM:SS`)

#### File output (access log)

When `--access-log <path>` is set, structured logs are written to the file **independently** of terminal output. The terminal continues showing the compact `dev` format while the file receives full detail.

```bash
# Terminal shows dev format, file gets combined (extended CLF)
radix serve --access-log ./access.log

# Override file format
radix serve --access-log ./access.log --access-log-format json
```

File format options:
- `combined` (default): Extended CLF with referrer and user-agent — standard format parseable by GoAccess, AWStats, etc.
- `common`: Standard CLF (no referrer/user-agent)
- `json`: One JSON object per line (structured, machine-parseable)

File rotation is **not** in scope — use OS-level tools (logrotate, newsyslog) for rotation.

#### Global logging flags

```yaml
--verbose, -v           # Verbose output (debug-level messages)
--quiet, -q             # Suppress terminal request logging
--no-color              # Disable colored output
--log-timestamp         # Show HH:MM:SS timestamp in terminal output
--log-level             # Level: debug, info, warn, error (default: info)
--access-log            # Path to access log file (enables file logging)
--access-log-format     # File format: common, combined, json (default: combined)
```

### TLS

All server commands support TLS via shared flags:

```yaml
--tls               # Enable HTTPS
--cert              # Certificate file path
--key               # Private key file path
--ca                # CA certificate (for client verification)
--client-auth       # Require client certificates
--tls-min-version   # Minimum TLS version (1.2, 1.3)
```

### Graceful Shutdown

All servers implement graceful shutdown:

```go
func (s *Server) Start(ctx context.Context) error {
    server := &http.Server{
        Addr:    s.addr,
        Handler: s.handler,
    }

    go func() {
        <-ctx.Done()
        shutdownCtx, cancel := context.WithTimeout(
            context.Background(),
            s.shutdownTimeout,
        )
        defer cancel()
        server.Shutdown(shutdownCtx)
    }()

    return server.ListenAndServe()
}
```

---

## Configuration Reference

### Config File Discovery

Radix searches for configuration files using upward directory traversal (like `.eslintrc`, `.prettierrc`, or `.git`):

```
Search order (first match wins):

1. --config flag         (explicit path, highest priority)
2. Current directory     ./radix.yml, ./radix.yaml, ./.radix.yml
3. Parent directories    Walk up looking for radix.yml (stops at filesystem root or home dir)
4. User home            ~/.radix.yml, ~/.config/radix/config.yml
5. System               /etc/radix/radix.yml (Linux/macOS only)
```

**Why upward traversal?**
- Run `radix serve` from any subdirectory and it finds the project's config
- Monorepos can have per-package configs that override the root
- Matches developer expectations from tools like ESLint, Prettier, Git

**Example:**
```
~/projects/myapp/
├── radix.yml              ← Found when running from any subdirectory
├── frontend/
│   ├── radix.yml          ← Overrides parent for frontend-specific settings
│   └── src/
│       └── (run radix serve here, finds frontend/radix.yml)
└── backend/
    └── (run radix serve here, finds root radix.yml)
```

### Global Configuration

```yaml
# radix.yml - Global configuration

# Global settings (apply to all commands)
global:
  port: 8080
  host: localhost
  verbose: false

  # TLS (shared)
  tls:
    enabled: false
    cert: ./certs/cert.pem
    key: ./certs/key.pem
    ca: ./certs/ca.pem
    client_auth: false
    min_version: "1.2"

  # Metrics dashboard (shared)
  metrics:
    enabled: true
    port: 8739  # Dedicated port for dashboard

  # Logging (shared)
  logging:
    level: info               # debug, info, warn, error
    no_color: false            # also respects NO_COLOR env var
    timestamp: false           # HH:MM:SS prefix in terminal
    access_log: ""             # path to access log file (empty = disabled)
    access_log_format: combined  # common, combined, json

# Command-specific settings
serve:
  # ... (see serve section)

proxy:
  # ... (see proxy section)

echo:
  # ... (see echo section)

mock:
  # ... (see mock section)

gencert:
  # ... (see gencert section)
```

### Environment Variables

All configuration options can be set via environment variables:

```bash
RADIX_PORT=3000
RADIX_HOST=0.0.0.0
RADIX_VERBOSE=true

# TLS
RADIX_TLS_ENABLED=true
RADIX_TLS_CERT=./certs/cert.pem
RADIX_TLS_KEY=./certs/key.pem

# Command-specific
RADIX_SERVE_DIR=./public
RADIX_SERVE_SPA=true
RADIX_PROXY_TARGET=http://localhost:3000
```

---

## References

### Research Sources

- [Caddy Web Server Features](https://caddyserver.com/features)
- [nginx Reverse Proxy Guide](https://www.getpagespeed.com/server-setup/nginx/nginx-reverse-proxy)
- [http-server (npm)](https://www.npmjs.com/package/http-server)
- [serve by Vercel](https://github.com/vercel/serve)
- [SSE Proxy Configuration](https://medium.com/@wang645788/troubleshooting-server-sent-events-sse-in-a-multi-service-architecture-5084ce155ea0)
- [nginx SSE Optimization](https://www.digitalocean.com/community/questions/nginx-optimization-for-server-sent-events-sse)

### Related Documentation

- [IMPLEMENTATION_PLAN.md](./IMPLEMENTATION_PLAN.md) - Project roadmap
- [CUSTOM_AUTH_PROVIDER.md](./CUSTOM_AUTH_PROVIDER.md) - Guide: adding a custom auth provider to a fork
- [CLAUDE.md](./CLAUDE.md) - AI assistant guide
- [CONTRIBUTING.md](./CONTRIBUTING.md) - Contributor guidelines

---

**Document Maintainers**: Project Contributors
**Feedback**: Open an issue at https://github.com/osuritz/radix/issues
