# Radix - Multi-Agent Execution Plan

Tracks implementation tasks for each command. Each task is designed to be a single PR with comprehensive unit tests. Update the status column as PRs are merged.

**Created**: 2026-02-09
**Based on**: [COMMAND_DESIGN.md](./COMMAND_DESIGN.md), [IMPLEMENTATION_PLAN.md](../IMPLEMENTATION_PLAN.md)

---

## Status Legend

| Symbol | Meaning |
|--------|---------|
| ⬜ | Not started |
| 🔵 | In progress |
| ✅ | Complete |

---

## Phase 4: TLS Infrastructure

### 4.1 GenCert Command

| # | Task | PR | Status | Description |
|---|------|----|--------|-------------|
| 4.1.1 | TLS key generation core | #20 | ✅ | `internal/tls/generator.go` — RSA (2048/4096) and ECDSA (P-256/P-384/P-521) key pair generation. Unit tests for key types, sizes, and error cases. |
| 4.1.2 | X.509 certificate creation | #20 | ✅ | CA and leaf certificate generation with SAN support (DNS names, IPs, wildcards). Serial number generation, validity periods, key usage extensions. Tests for all SAN types, CA vs leaf certs, expiry. |
| 4.1.3 | PEM encoding & file output | #20 | ✅ | Write cert.pem, key.pem, ca.pem, ca-key.pem to output directory. PKCS#12 bundle generation (optional). File permission handling (0600 for keys). README.txt generation with trust-store instructions. Tests for file creation, overwrite protection, permissions. |
| 4.1.4 | `gencert` CLI command | #20 | ✅ | `internal/cli/gencert.go` — Wire up Cobra command with all flags (`--host`, `--output`, `--days`, `--org`, `--key-size`, `--key-type`, `--ecdsa-curve`, `--ca`, `--ca-cert`, `--ca-key`, `--client`, `--pkcs12`, `--overwrite`). Register in root command. Config file integration. Tests for flag parsing, validation, end-to-end generation. |

### 4.2 TLS Configuration Loading

| # | Task | PR | Status | Description |
|---|------|----|--------|-------------|
| 4.2.1 | TLS config loader | | ⬜ | `internal/tls/loader.go` — Load cert/key/CA files, build `*tls.Config`. Support TLS 1.2/1.3 minimum version. Cipher suite selection. Certificate validation on load. Tests for valid/invalid certs, version constraints, mTLS configs. |
| 4.2.2 | TLS global flags | | ⬜ | Add persistent TLS flags to root command (`--tls`, `--cert`, `--key`, `--ca`, `--client-auth`, `--tls-min-version`). Wire into config system. Tests for flag-to-config binding. |

---

## Phase 5: Core Commands (HTTP)

### 5.1 Shared Server Infrastructure

| # | Task | PR | Status | Description |
|---|------|----|--------|-------------|
| 5.1.1 | Base HTTP server | | ⬜ | `internal/server/server.go` — Shared server struct with graceful shutdown (context-based), signal handling (SIGINT/SIGTERM), startup banner, address binding. Configurable read/write/idle timeouts. Tests for startup, shutdown, signal handling, port conflicts. |
| 5.1.2 | CORS middleware | | ⬜ | `internal/server/middleware/cors.go` — Configurable CORS with origin, methods, headers, credentials, max-age. Preflight (OPTIONS) handling. Tests for simple requests, preflight, wildcard vs specific origins, credentials mode. |
| 5.1.3 | Recovery middleware | | ⬜ | `internal/server/middleware/recovery.go` — Panic recovery returning 500. Stack trace logging in verbose mode. Tests for panic recovery, response code, logging output. |
| 5.1.4 | Metrics dashboard (SVG) | | ⬜ | `internal/metrics/dashboard.go` — Visual dashboard on dedicated port (default 8739). Status code pie chart (SVG), request trend line chart (SVG), recent requests table (last 1000, circular buffer). `--metrics-port` and `--no-metrics` flags. Tests for dashboard rendering, buffer bounds, concurrent access. Based on radix-serve PR #3 design. |
| 5.1.5 | Logging enhancements | | ⬜ | Refactor `internal/server/middleware/logging.go`. **TTY output (dev format)**: compact colored output (`METHOD /path STATUS SIZE DURATION`), respects both `--no-color` flag and `NO_COLOR` env var ([no-color.org](https://no-color.org) standard), auto-detect TTY via `os.Stdout` isatty check, optional HH:MM:SS timestamp prefix in TTY mode (`--log-timestamp`). **File output**: `--access-log <path>` writes to file in extended CLF (combined) format independently of TTY output — when set, terminal shows dev format and file gets combined format simultaneously. Configurable file format override (`--access-log-format common|combined|json`). File rotation not in scope (use external logrotate). **Dual writer**: `io.Writer` abstraction supporting concurrent TTY + file output. Tests for: NO_COLOR env var, isatty detection, dual output to TTY + file, file format independence, concurrent write safety, all format variants. |

### 5.2 Serve Command

| # | Task | PR | Status | Description |
|---|------|----|--------|-------------|
| 5.2.1 | Static file handler | | ⬜ | `internal/server/static.go` — Serve files from directory with MIME type detection (including extended types: .wasm, .mjs, .woff2, .avif, .webp). ETag generation (weak, based on mtime+size). Last-Modified headers. Conditional request support (If-None-Match, If-Modified-Since → 304). Range request support (byte serving). Symlink following (configurable). Tests for all MIME types, conditional requests, range requests, path traversal prevention. |
| 5.2.2 | Directory listing | | ⬜ | HTML, JSON, and plain text directory listing. Hidden file filtering (dotfiles). Glob-based ignore patterns. Sortable entries (name, size, date). Breadcrumb navigation in HTML format. Tests for all formats, hidden files, ignore patterns, empty directories. |
| 5.2.3 | Index file resolution & SPA mode | | ⬜ | nginx-style index resolution: try each index file in order per directory (default: index.html, index.htm). SPA mode: serve spa-index for non-file 404s, respect file extensions (missing .js → 404, missing /route → SPA), exclusion patterns. Clean URLs (/about → /about.html). Trailing slash handling (add/remove/auto). Tests for index priority, SPA routing, extension detection, exclusions, clean URLs, trailing slashes. |
| 5.2.4 | Compression middleware | | ⬜ | `internal/server/middleware/compression.go` — On-the-fly gzip and brotli compression. Accept-Encoding negotiation (prefer brotli > gzip). Minimum size threshold. Skip already-compressed formats (images, archives). Configurable compression level (1-9). Content-type filtering. Tests for negotiation, threshold, skip logic, level settings. |
| 5.2.5 | Caching & custom headers | | ⬜ | Cache-Control header with configurable max-age. Per-pattern max-age overrides (e.g., `*.js:31536000`). Immutable cache patterns. Custom response headers. Tests for default cache, overrides, immutable, custom headers. |
| 5.2.6 | Security headers | | ⬜ | Optional security header injection: HSTS (with max-age, includeSubDomains, preload), X-Frame-Options, X-Content-Type-Options: nosniff, Referrer-Policy, Content-Security-Policy. Tests for each header, combinations, HSTS requiring TLS. |
| 5.2.7 | Error pages | | ⬜ | Custom 404 and 500 pages from user-provided HTML files. Built-in default styled error pages. Error page path validation. Tests for custom pages, fallback defaults, missing error page files. |
| 5.2.8 | URL rewrites & redirects | | ⬜ | Rewrite rules (`from:to` with glob/capture support). Redirect rules (`from:to:status`). Base path prefix for all URLs. Tests for rewrites, redirects, base path, capture groups. |
| 5.2.9 | `serve` CLI command | | ⬜ | `internal/cli/serve.go` — Wire up Cobra command with all flags. Positional `[directory]` argument. `--browse` flag (open browser). Integrate all middleware (logging, metrics, CORS, compression, security headers). Config file integration. Startup banner with URL, directory, features enabled. Tests for flag parsing, middleware chain, config loading, startup output. |

### 5.3 Proxy Command

| # | Task | PR | Status | Description |
|---|------|----|--------|-------------|
| 5.3.1 | Basic reverse proxy | | ⬜ | `internal/server/proxy.go` — Reverse proxy using `net/http/httputil.ReverseProxy`. X-Forwarded-For, X-Forwarded-Proto, X-Forwarded-Host, X-Real-IP header injection. Host header handling (auto/preserve/custom). Configurable timeouts (total, dial, read, write, idle). Error handling (502 on backend failure). Tests for proxying, header injection, host modes, timeouts, backend errors. |
| 5.3.2 | Path handling | | ⬜ | Strip prefix before forwarding. Add prefix before forwarding. Path rewrite rules with capture groups (`from=to`). Tests for strip, add, rewrite combinations, edge cases. |
| 5.3.3 | Header manipulation | | ⬜ | Add/override request headers. Remove request headers. Add/override response headers. Request/response modifier pipeline. Tests for add, remove, override, modifier ordering. |
| 5.3.4 | Streaming & SSE support | | ⬜ | Streaming response detection (text/event-stream, application/x-ndjson, etc.). Disable buffering for streaming content types. Configurable flush interval. SSE-specific path matching. SSE connection timeout (0=infinite). X-Accel-Buffering: no header. Tests for SSE detection, flushing, unbuffered streaming, timeout. |
| 5.3.5 | WebSocket proxy | | ⬜ | WebSocket upgrade detection and handling. Bidirectional message copying. Configurable read/write buffer sizes. Ping/pong interval. Origin checking. Tests for upgrade, bidirectional copy, ping/pong, origin validation. Note: evaluate whether to use gorilla/websocket or stdlib. |
| 5.3.6 | Backend TLS | | ⬜ | Skip TLS verification (`--tls-skip-verify`). Custom CA for backend verification. Client certificate for backend mTLS. Server name override. Tests for skip-verify, custom CA, mTLS, SNI. |
| 5.3.7 | Retry & circuit breaker | | ⬜ | Configurable retry policy (attempts, per-try timeout, retry-on status codes). Circuit breaker (threshold, timeout, half-open state). Tests for retry logic, circuit states, timeout behavior. |
| 5.3.8 | Request/response logging | | ⬜ | Optional body logging with size limits. Request/response body capture. Truncation at configurable max size. Tests for body logging, truncation, size limits. |
| 5.3.9 | `proxy` CLI command | | ⬜ | `internal/cli/proxy.go` — Wire up Cobra command with all flags. Positional `[target]` argument. Integrate middleware (logging, metrics, CORS). Config file integration. Startup banner. Tests for flag parsing, target validation, middleware chain. |

---

## Phase 6: Core Commands (HTTPS)

| # | Task | PR | Status | Description |
|---|------|----|--------|-------------|
| 6.1 | Serve TLS integration | | ⬜ | HTTPS static file serving. HTTP/2 support (automatic with TLS). Optional HTTP→HTTPS redirect with `--http-redirect` and `--http-port`. HSTS header injection. Tests for HTTPS serving, HTTP/2, redirect, HSTS. |
| 6.2 | Proxy TLS integration | | ⬜ | HTTPS frontend (accepting TLS connections). HTTPS backend proxying. Mutual TLS for backend connections. Tests for TLS frontend, TLS backend, mTLS, cert verification. |

---

## Phase 7: Advanced Commands (HTTP)

### 7.1 Echo Command

| # | Task | PR | Status | Description |
|---|------|----|--------|-------------|
| 7.1.1 | Echo handler core | | ⬜ | `internal/server/echo.go` — Build JSON response with: request info (method, URL, path, query, headers, cookies), body parsing (JSON, form-urlencoded, raw), client info (IP, port, remote_addr), server info (hostname, port, protocol), timing info (timestamp, unix, unix_nano), request ID generation. Body size limiting. Pretty-print option. Tests for all response fields, body parsing formats, size limits. |
| 7.1.2 | Response control | | ⬜ | Configurable default status code. Configurable response delay with jitter (random within range). Custom response body (overrides echo). Custom response headers. Content-Type control. Tests for status codes, delay timing, jitter range, custom body/headers. |
| 7.1.3 | Path-based behavior | | ⬜ | Status from path: `/404` or `/status/404` → returns 404. Delay from path: `/delay/500ms` → delays 500ms. Both configurable via flags. Tests for path parsing, status range validation (100-599), delay parsing, combined behavior. |
| 7.1.4 | `echo` CLI command | | ⬜ | `internal/cli/echo.go` — Wire up Cobra command with all flags (`--status`, `--delay`, `--delay-jitter`, `--body`, `--header`, `--content-type`, `--echo-body`, `--echo-headers`, `--echo-query`, `--body-limit`, `--pretty`, `--status-from-path`, `--delay-from-path`, `--log-body`). Integrate middleware. Config file integration. Tests for flag parsing, middleware chain, config loading. |

### 7.2 Mock Command

| # | Task | PR | Status | Description |
|---|------|----|--------|-------------|
| 7.2.1 | Route matching engine | | ⬜ | `internal/server/mock/router.go` — Route matching with priority: exact path+method → exact path+wildcard method → parameterized (`:id`) → regex (`regex:pattern`) → glob (`/api/*`) → fallback. Path parameter extraction. Method matching (single and multiple). Tests for all match types, priority ordering, parameter extraction, no-match fallback. |
| 7.2.2 | Template engine | | ⬜ | `internal/server/mock/template.go` — Template functions: `{{uuid}}`, `{{now}}`, `{{timestamp}}`, `{{random min max}}`, `{{randomFloat}}`, `{{randomString len}}`, `{{randomChoice}}`, `{{lorem words}}`, `{{params.name}}`, `{{query.key}}`, `{{body.field}}`, `{{headers.Name}}`, `{{method}}`, `{{path}}`, `{{env "VAR"}}`, `{{file "path"}}`, `{{base64 "text"}}`, `{{hash "sha256" "text"}}`, `{{seq}}`, `{{faker.name}}`, `{{faker.email}}`, `{{faker.phone}}`, `{{faker.address}}`. Tests for each function, error cases, nested access. |
| 7.2.3 | Route config loading | | ⬜ | `internal/server/mock/config.go` — Parse mock routes YAML: path, method(s), response (status, body, headers, file), delay, delay_jitter. Validation (valid paths, status codes, file existence). Tests for valid configs, missing fields, defaults, file-based responses. |
| 7.2.4 | Conditional responses | | ⬜ | Match conditions on body fields, query params, headers. Wildcard matching. Default fallback condition. Tests for body matching, query matching, header matching, wildcard, default, no-match. |
| 7.2.5 | Sequence & random responses | | ⬜ | Sequence responses: stateful counter, optional loop-back. Random weighted responses: weighted selection across response variants. Tests for sequence ordering, loop, reset, random distribution within tolerance. |
| 7.2.6 | Built-in httpbin endpoints (HTTP methods) | | ⬜ | `internal/server/mock/builtin.go` — `/get`, `/post`, `/put`, `/patch`, `/delete`, `/anything`, `/anything/*`. Returns httpbin-compatible JSON (args, headers, origin, url, data, json, form, files). Tests for each method endpoint, response format, query args, body parsing. |
| 7.2.7 | Built-in httpbin endpoints (inspection & status) | | ⬜ | `/ip`, `/uuid`, `/user-agent`, `/headers`, `/status/:code`, `/status/:code1,:code2` (random). Tests for each endpoint, random status selection. |
| 7.2.8 | Built-in httpbin endpoints (dynamic data) | | ⬜ | `/delay/:n` (max 10s), `/bytes/:n`, `/stream-bytes/:n` (chunked), `/stream/:n` (JSON objects), `/range/:n` (Range header), `/drip` (query params: duration, numbytes, code). Tests for delay capping, byte generation, chunked streaming, range support, drip parameters. |
| 7.2.9 | Built-in httpbin endpoints (formats & compression) | | ⬜ | `/html`, `/xml`, `/json`, `/robots.txt`, `/deny`, `/encoding/utf8`, `/gzip`, `/deflate`, `/brotli`. Tests for content types, encoding correctness. |
| 7.2.10 | Built-in httpbin endpoints (redirects & cookies) | | ⬜ | `/redirect/:n`, `/redirect-to`, `/relative-redirect/:n`, `/absolute-redirect/:n`, `/cookies`, `/cookies/set`, `/cookies/set/:name/:value`, `/cookies/delete`. Tests for redirect chains, cookie setting/reading/deletion, redirect limits. |
| 7.2.11 | Built-in httpbin endpoints (auth & caching) | | ⬜ | `/basic-auth/:user/:pass`, `/hidden-basic-auth/:user/:pass`, `/bearer`, `/cache`, `/cache/:n`, `/etag/:etag`, `/response-headers`. Tests for auth challenge, success/failure, bearer validation, conditional requests, cache headers. |
| 7.2.12 | Built-in httpbin endpoints (images) | | ⬜ | `/image` (Accept-based), `/image/png`, `/image/jpeg`, `/image/webp`, `/image/svg`. Minimal embedded test images. Tests for content negotiation, correct MIME types, binary output. |
| 7.2.13 | Hot reload (file watcher) | | ⬜ | `internal/server/mock/watcher.go` — Watch routes config file for changes. Atomic route swap on reload. Debounce rapid changes. Error logging on invalid config (keep previous routes). Tests for reload trigger, atomic swap, debounce, invalid config handling. Note: evaluate fsnotify dependency. |
| 7.2.14 | Landing page | | ⬜ | `internal/server/mock/landing.go` — Browser-facing HTML landing page (TailwindCSS via CDN). Lists all built-in endpoints grouped by category. Lists custom routes if loaded. Dark/light mode. Method color coding. Curl command copy-to-clipboard. Detect browser via Accept header (text/html → page, otherwise JSON). Generated at startup (cached). Tests for HTML generation, Accept detection, custom route inclusion. |
| 7.2.15 | Chaos testing features | | ⬜ | Global artificial latency (`--latency`, `--latency-jitter`). Random failure rate (`--fail-rate`, `--fail-status`). Tests for latency injection, failure rate distribution, combined with normal routes. |
| 7.2.16 | WebSocket & SSE mocking | | ⬜ | WebSocket mock: scripted messages with delays, echo mode. SSE mock: scripted events with delays, repeat with interval. Tests for WebSocket message sequence, echo, SSE event streaming, repeat. |
| 7.2.17 | `mock` CLI command | | ⬜ | `internal/cli/mock.go` — Wire up Cobra command with all flags (`--routes`, `--watch`, `--builtin`, `--prefix`, `--latency`, `--latency-jitter`, `--fail-rate`, `--fail-status`, `--cors`). Positional `[config-file]` argument. Integrate middleware. Config file integration. `--fallback` mode (404/echo/proxy). Tests for flag parsing, builtin toggle, prefix routing, fallback modes. |

---

## Phase 8: Advanced Commands (HTTPS)

| # | Task | PR | Status | Description |
|---|------|----|--------|-------------|
| 8.1 | Echo TLS integration | | ⬜ | HTTPS echo server. Include TLS connection info in echo response (version, cipher suite, server name, client cert details). Tests for TLS info fields, client cert inspection. |
| 8.2 | Mock TLS integration | | ⬜ | HTTPS mock server. Per-route `require_client_cert` option. `{{client.cn}}` template variable for client cert CN. Tests for TLS routes, client cert routing, template variables. |

---

## Phase 9: Polish & Release

| # | Task | PR | Status | Description |
|---|------|----|--------|-------------|
| 9.1 | Config upward traversal | | ⬜ | Config file discovery via upward directory traversal (like .eslintrc, .git). Search: `--config` → `./radix.yml` → parent dirs → `~/.radix.yml` → `~/.config/radix/` → `/etc/radix/`. Tests for traversal order, stop conditions, override behavior. |
| 9.2 | GPG signing setup | | ⬜ | GoReleaser GPG signing config. Checksum signing. Detached binary signatures. Public key in repo. Verification documentation. |
| 9.3 | Documentation & examples | | ⬜ | README updates for all commands. Example configs per command. TLS setup guide. Binary verification guide. |
| 9.4 | Performance & benchmarks | | ⬜ | Benchmark tests for static serving, proxy throughput, mock routing. Memory profiling. Startup time validation (<100ms). Binary size check (<10MB). |

---

## Task Dependencies

```
Phase 4 (TLS)
  4.1.1 → 4.1.2 → 4.1.3 → 4.1.4
  4.1.2 → 4.2.1 → 4.2.2

Phase 5 (Core HTTP)
  5.1.1 ──────────────────────────→ 5.2.9 (serve CLI)
  5.1.2 ──────────────────────────→ 5.2.9, 5.3.9
  5.1.3 ──────────────────────────→ 5.2.9, 5.3.9
  5.1.4 ──────────────────────────→ 5.2.9, 5.3.9
  5.1.5 ──────────────────────────→ 5.2.9, 5.3.9
  5.2.1 → 5.2.2 → 5.2.3 ────────→ 5.2.9
  5.2.4, 5.2.5, 5.2.6, 5.2.7 ───→ 5.2.9 (independent of each other)
  5.2.8 ─────────────────────────→ 5.2.9
  5.3.1 → 5.3.2, 5.3.3 ─────────→ 5.3.9
  5.3.4, 5.3.5 ─────────────────→ 5.3.9 (independent of each other)
  5.3.6 ─────────────────────────→ 5.3.9
  5.3.7, 5.3.8 ─────────────────→ 5.3.9 (independent of each other)

Phase 6 (Core HTTPS)
  4.2.1 + 5.2.9 → 6.1
  4.2.1 + 5.3.9 → 6.2

Phase 7 (Advanced HTTP)
  5.1.1 → 7.1.1 → 7.1.2 → 7.1.4
  7.1.3 ──────────────────→ 7.1.4
  5.1.1 → 7.2.1 → 7.2.3 → 7.2.17
  7.2.2 ──────────────────→ 7.2.3
  7.2.4, 7.2.5 ──────────→ 7.2.17
  7.2.6..7.2.12 ─────────→ 7.2.17 (all independent of each other)
  7.2.13..7.2.16 ────────→ 7.2.17 (all independent of each other)

Phase 8 (Advanced HTTPS)
  4.2.1 + 7.1.4 → 8.1
  4.2.1 + 7.2.17 → 8.2

Phase 9 (Polish)
  All prior phases → 9.1..9.4
```

---

## Parallelization Opportunities

Multiple agents can work concurrently on independent tasks within the same phase:

**Phase 5 parallel groups:**
- Group A: 5.1.1 (base server) — required first
- Group B (after 5.1.1): 5.1.2, 5.1.3, 5.1.4, 5.1.5 — all independent
- Group C (after 5.1.1): 5.2.1, 5.3.1 — serve and proxy cores in parallel
- Group D (after 5.2.1): 5.2.2, 5.2.4, 5.2.5, 5.2.6, 5.2.7, 5.2.8 — serve features in parallel
- Group E (after 5.3.1): 5.3.2, 5.3.3, 5.3.4, 5.3.5, 5.3.6, 5.3.7, 5.3.8 — proxy features in parallel

**Phase 7 parallel groups:**
- Group A: 7.1.1 and 7.2.1 — echo core and mock router in parallel
- Group B (after 7.2.1): 7.2.2, 7.2.6-7.2.12 — template engine and all builtin endpoints in parallel
- Group C (after 7.2.2+7.2.3): 7.2.4, 7.2.5, 7.2.13, 7.2.14, 7.2.15, 7.2.16 — advanced mock features in parallel

---

## Test Requirements Per Task

Every task PR must include:

1. **Unit tests** with table-driven test patterns
2. **Race detection** passing (`go test -race`)
3. **Edge cases** (empty input, max values, invalid input, concurrent access)
4. **Linting** clean (`make lint`)
5. **>80% coverage** for new code
6. **No regressions** (`make test` passes fully)

---

## Agent Prompt Template

Use this prompt to start or resume a task. Copy and customize the `{{placeholders}}`.

---

### Starting a new task

```
## Task

Implement task **{{TASK_ID}}** from the Radix execution plan: **{{TASK_TITLE}}**

## Context

- Read `docs/EXECUTION_PLAN.md` for the full plan and task dependencies
- Read `docs/COMMAND_DESIGN.md` for detailed design specs and expected behavior
- Read `CLAUDE.md` for project conventions, code patterns, and development workflow
- Read `IMPLEMENTATION_PLAN.md` for architecture context

## Task description

{{TASK_DESCRIPTION — copy the Description column from the execution plan table}}

## Dependencies

These tasks are already complete and their code is available:
{{LIST_COMPLETED_DEPENDENCY_TASKS — e.g., "- 5.1.1 Base HTTP server (internal/server/server.go)"}}

## Requirements

1. Implement the feature in the appropriate `internal/` package
2. Write comprehensive unit tests (table-driven, edge cases, race-safe)
3. Follow existing code patterns and conventions from CLAUDE.md
4. Run `make test && make lint` and fix any issues
5. Commit with conventional commit format: `feat(scope): description`
6. Push to the designated branch

## Constraints

- No new external dependencies without justification
- Use standard library where possible
- All exported functions must have doc comments
- Thread safety required for shared state (use atomic, sync.Map, or mutexes)
- >80% test coverage for new code
```

---

### Resuming / continuing a task

```
## Resume

Continue work on task **{{TASK_ID}}**: **{{TASK_TITLE}}**

## Current state

{{DESCRIBE_CURRENT_STATE — e.g.:
- "The handler struct is implemented but tests are incomplete"
- "PR feedback requested changes to X"
- "Linting passes but coverage is at 72%, need more edge case tests"}}

## What remains

{{LIST_REMAINING_ITEMS — e.g.:
- "Add tests for concurrent access"
- "Fix linter warning about unused parameter"
- "Update to match revised design in COMMAND_DESIGN.md section X"}}

## References

- Read `docs/EXECUTION_PLAN.md` for full plan context
- Read `docs/COMMAND_DESIGN.md` for design specs
- Read `CLAUDE.md` for conventions
- Previous work is on branch: `{{BRANCH_NAME}}`
```

---

### Starting a CLI command integration task (e.g., 5.2.9, 5.3.9, 7.1.4, 7.2.17)

These tasks wire everything together and are always the last task for a command.

```
## Task

Implement task **{{TASK_ID}}**: **{{COMMAND_NAME}} CLI command integration**

## Context

- Read `docs/EXECUTION_PLAN.md` for the full plan
- Read `docs/COMMAND_DESIGN.md` — the "{{COMMAND_NAME}} Command" section has the
  complete flag list, config YAML structure, and usage examples
- Read `CLAUDE.md` for conventions (especially "Adding a New Command" workflow)
- Study `internal/cli/version.go` and `internal/cli/validate.go` for the existing
  command patterns

## Completed dependencies

All feature tasks for this command are complete:
{{LIST_ALL_FEATURE_TASKS — e.g.:
- "5.2.1 Static file handler (internal/server/static.go)"
- "5.2.2 Directory listing"
- "5.2.3 Index resolution & SPA mode"
- "5.2.4 Compression middleware"
- ...}}

## Requirements

1. Create `internal/cli/{{command}}.go` with the Cobra command
2. Register all flags from COMMAND_DESIGN.md (basic, advanced, all categories)
3. Register the command in `internal/cli/root.go` via `rootCmd.AddCommand()`
4. In the `RunE` function:
   - Load and validate config
   - Build the handler by composing all feature implementations
   - Chain middleware: recovery → logging → metrics → CORS → command-specific
   - Start the base server with graceful shutdown
   - Start the metrics dashboard (if enabled)
   - Print startup banner (URL, features enabled, config source)
5. Write tests: flag parsing, config binding, middleware chain order, startup output
6. Run `make test && make lint` and ensure clean
7. Commit and push
```

---

### Starting multiple parallel tasks

When launching agents for tasks that can run concurrently (see "Parallelization Opportunities" section):

```
## Parallel task batch

Start the following independent tasks in parallel. Each should be on its own
feature branch and result in its own PR.

### Agent 1 — Task {{TASK_ID_A}}: {{TITLE_A}}
{{TASK_DESCRIPTION_A}}

### Agent 2 — Task {{TASK_ID_B}}: {{TITLE_B}}
{{TASK_DESCRIPTION_B}}

### Agent 3 — Task {{TASK_ID_C}}: {{TITLE_C}}
{{TASK_DESCRIPTION_C}}

## Shared context for all agents

- Read `docs/EXECUTION_PLAN.md`, `docs/COMMAND_DESIGN.md`, `CLAUDE.md`
- These tasks are independent — no code dependencies between them
- Completed prerequisites available: {{LIST_SHARED_DEPENDENCIES}}
- Each agent: implement, test, lint, commit, push
```

---

## Notes

- Task numbers (e.g., 5.2.1) are stable identifiers — do not renumber when inserting tasks
- Each task should result in a single focused PR
- Tasks within a group can be merged in any order as long as dependencies are satisfied
- The CLI command task (e.g., 5.2.9) for each command is always last — it integrates everything
