# CLAUDE.md - AI Assistant Guide for Radix

This document provides comprehensive guidance for AI assistants (like Claude Code) working on the Radix codebase. It covers project structure, development workflows, conventions, and best practices.

**Last Updated**: 2026-06-14
**Project Version**: v0.5.0
**Go Version**: 1.25+

---

## Table of Contents

1. [Project Overview](#project-overview)
2. [Codebase Structure](#codebase-structure)
3. [Development Environment](#development-environment)
4. [Key Conventions](#key-conventions)
5. [Development Workflows](#development-workflows)
6. [Testing Practices](#testing-practices)
7. [Common Tasks](#common-tasks)
8. [Code Patterns & Architecture](#code-patterns--architecture)
9. [CI/CD & Release Process](#cicd--release-process)
10. [Important Files Reference](#important-files-reference)
11. [AI Assistant Best Practices](#ai-assistant-best-practices)

---

## Project Overview

**Radix** is a multi-mode HTTP server for local development built in Go. It ships as a single self-contained binary that replaces multiple development tools. It keeps a deliberately small, curated set of well-known Go dependencies (CLI/config framework, file watching, YAML, and an OS-keychain reader) rather than pulling in transitive sprawl — every dependency is a conscious, auditable choice.

### Purpose
Provides local development HTTP capabilities:
- **Static File Serving** - Like Python's SimpleHTTPServer or `http-server`
- **Reverse Proxy** - Forward requests to backend services
- **Request Echo** - Debug HTTP requests by echoing them back
- **API Mocking** - Mock API endpoints from YAML configuration
- **TLS/HTTPS** - Self-signed certificate generation and TLS support
- **Metrics** - Built-in observability with JSON and Prometheus formats

### Project Status
- **Phase**: Feature-complete for the alpha/beta (v0.1.0-alpha.1)
- **Completed**: CLI framework, config system, metrics infrastructure, CI/CD,
  and all commands — `serve`, `proxy`, `echo`, `mock` (built-in httpbin-style
  endpoints + custom YAML routes + hot-reload), `gencert`, `version`,
  `validate` — plus TLS/HTTPS support and the middleware suite (logging,
  metrics, CORS, gzip, auth header injection, security/HSTS). All are
  implemented and tested.
- **Remaining (Phase 9 polish)**: release hardening (binary signing, distribution
  channels), expanded integration/benchmark coverage, and a few nice-to-haves
  noted in `IMPLEMENTATION_PLAN.md` (e.g. per-command auth/proxy/mock metrics,
  per-route TLS, client-cert inspection in echo).

### Key Characteristics
- **Language**: Go 1.25+
- **CLI Framework**: Cobra + Viper
- **Architecture**: Single self-contained binary; no external services needed to run (the optional keychain value source reads the OS credential store at runtime)
- **Platforms**: macOS, Linux, Windows (amd64, arm64)
- **License**: MIT

---

## Codebase Structure

```
radix/
├── cmd/
│   └── radix/
│       └── main.go                   # Application entry point
│
├── internal/                         # Private application code (~6,200 LOC, non-test)
│   ├── cli/                          # Command-line interface commands
│   │   ├── root.go                  # Root command with global/TLS/metrics flags
│   │   ├── version.go               # Version information command
│   │   ├── validate.go              # Configuration validation command
│   │   ├── gencert.go               # TLS certificate generation command
│   │   ├── serve.go                 # Static file serving command
│   │   ├── proxy.go                 # Reverse proxy command
│   │   ├── echo.go                  # Request echo command
│   │   └── mock.go                  # API mock command (built-ins + routes)
│   │
│   ├── config/                       # Configuration management
│   │   └── config.go                # Config structs, loading, validation
│   │
│   ├── server/                       # HTTP server implementations
│   │   ├── server.go                # Shared base HTTP server + graceful shutdown
│   │   ├── fileserver.go            # Static file server (SPA, index)
│   │   ├── proxy.go                 # Reverse proxy handler (streaming, fwd headers)
│   │   ├── echo.go                  # Echo handler (request → JSON)
│   │   ├── mock.go                  # Built-in httpbin-style mock endpoints
│   │   ├── mock_routes.go           # Custom YAML routes: load/compile/match/template
│   │   ├── mock_watch.go            # Routes-file hot-reload (fsnotify)
│   │   ├── redirect.go              # HTTP→HTTPS redirect handler
│   │   └── middleware/
│   │       ├── logging.go           # Request logging (CLF, Extended CLF, Dev)
│   │       ├── metrics.go           # Metrics collection middleware
│   │       ├── cors.go              # CORS headers middleware
│   │       ├── gzip.go              # Gzip compression middleware
│   │       ├── security.go          # Security headers (HSTS)
│   │       ├── auth.go              # HeaderProvider interface + InjectHeaders
│   │       ├── auth_registry.go     # Provider registry + ResolveProvider
│   │       └── auth_static.go       # StaticProvider (fixed headers)
│   │
│   ├── metrics/                      # Metrics & observability
│   │   ├── collector.go             # Metrics aggregation
│   │   ├── histogram.go             # Response time histogram
│   │   └── prometheus.go            # Prometheus exporter
│   │
│   ├── tls/                          # TLS certificate generation & loading
│   │   ├── generator.go             # Self-signed CA/server/client cert generation
│   │   └── loader.go                # Cert loading + server/client TLS config helpers
│   │
│   └── version/                      # Version information
│       └── version.go               # Build-time version data
│
├── .github/
│   ├── workflows/
│   │   ├── ci.yml                   # Continuous Integration
│   │   └── release.yml              # Release automation
│   └── ISSUE_TEMPLATE/
│
├── scripts/
│   └── smoke.sh                      # End-to-end smoke test (build + exercise all commands)
│
├── examples/
│   ├── radix.example.yml            # Example configuration (all keys)
│   ├── mock-routes.yml              # Example custom mock routes
│   └── mocks/                       # Sample mock response bodies
│
├── Makefile                         # Build automation
├── .golangci.yml                    # Linter configuration
├── .goreleaser.yml                  # Release configuration
├── go.mod                           # Go module definition
├── go.sum                           # Dependency checksums
├── README.md                        # Project overview
├── CHANGELOG.md                     # Release history
├── CONTRIBUTING.md                  # Contributor guidelines
├── IMPLEMENTATION_PLAN.md           # Detailed feature roadmap
└── LICENSE                          # MIT License
```

### Directory Purposes

| Directory | Purpose | Import Path |
|-----------|---------|-------------|
| `cmd/radix/` | Main entry point | `github.com/osuritz/radix/cmd/radix` |
| `internal/cli/` | CLI commands (Cobra) | `github.com/osuritz/radix/internal/cli` |
| `internal/config/` | Config management | `github.com/osuritz/radix/internal/config` |
| `internal/server/` | HTTP servers & handlers | `github.com/osuritz/radix/internal/server` |
| `internal/server/middleware/` | HTTP middleware (logging, metrics, CORS, gzip, auth, security) | `github.com/osuritz/radix/internal/server/middleware` |
| `internal/metrics/` | Metrics collection | `github.com/osuritz/radix/internal/metrics` |
| `internal/tls/` | TLS cert generation & loading | `github.com/osuritz/radix/internal/tls` |
| `internal/version/` | Version info | `github.com/osuritz/radix/internal/version` |

**Note**: The `internal/` directory means these packages cannot be imported by external projects (Go convention).

---

## Development Environment

### Prerequisites

- **Go**: 1.25 or higher
- **Make**: GNU Make for build automation
- **golangci-lint**: For code linting
- **Git**: Version control

### Setup

```bash
# Clone the repository
git clone https://github.com/osuritz/radix.git
cd radix

# Download dependencies
go mod download

# Verify setup
make build
./bin/radix version
```

### Development Tools

- **Code Editor**: Any editor with Go support (VS Code, GoLand, Vim, etc.)
- **Linter**: golangci-lint v1.x (installed via `make lint`)
- **Testing**: Go's built-in testing framework
- **Release**: GoReleaser for cross-platform builds

---

## Key Conventions

### File Naming

- **Go files**: `lowercase.go` (e.g., `collector.go`, `logging.go`)
- **Test files**: `*_test.go` (e.g., `collector_test.go`)
- **Markdown docs**: `UPPERCASE.md` (e.g., `README.md`, `CONTRIBUTING.md`)
- **Config files**: Lowercase with extension (e.g., `.golangci.yml`, `radix.yml`)

### Code Style

- **Formatting**: Use `gofmt` and `goimports` (enforced by CI)
- **Naming**:
  - Exported: `CamelCase` (e.g., `NewCollector`, `Config`)
  - Unexported: `camelCase` (e.g., `loadConfig`, `httpClient`)
  - Constants: `PascalCase` or `SCREAMING_SNAKE_CASE` (e.g., `DefaultPort`, `MAX_RETRIES`)
- **Line length**: No hard limit, but keep reasonable (~120 chars)
- **Comments**: Required for exported functions, types, and packages

### Package Organization

```go
// Package structure example
package cli

import (
    "github.com/spf13/cobra"
    "github.com/osuritz/radix/internal/config"
    "github.com/osuritz/radix/internal/version"
)

// Exported functions have doc comments
// NewRootCmd creates the root command for the CLI
func NewRootCmd() *cobra.Command {
    // ...
}
```

### Error Handling

- **Always check errors**: No unchecked error returns
- **Wrap errors**: Use `fmt.Errorf("context: %w", err)` for error wrapping
- **Return early**: Prefer early returns over deeply nested if-statements

```go
// Good error handling
func loadFile(path string) ([]byte, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("failed to read file %s: %w", path, err)
    }
    return data, nil
}
```

### Commit Message Format

Follow **Conventional Commits**:

```
<type>(<scope>): <subject>

[optional body]

[optional footer]
```

**Types**:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `test`: Adding or modifying tests
- `refactor`: Code refactoring
- `perf`: Performance improvements
- `ci`: CI/CD changes
- `chore`: Maintenance tasks
- `style`: Code formatting (no logic changes)

**Examples**:
```
feat(cli): add serve command for static file serving

fix(metrics): correct response time calculation in histogram

docs: update README with installation instructions

test(collector): add concurrent access tests
```

---

## Development Workflows

### Working on a New Feature

```bash
# 1. Create a feature branch
git checkout -b feature/your-feature-name

# 2. Make changes
# Edit code, add tests

# 3. Run tests
make test

# 4. Run linters
make lint

# 5. Build to verify compilation
make build

# 6. Commit changes
git add .
git commit -m "feat(scope): description"

# 7. Push to remote
git push origin feature/your-feature-name

# 8. Create a pull request on GitHub
```

### Adding a New Command

When adding a new CLI command (e.g., `radix serve`):

1. **Create command file**: `internal/cli/serve.go`
2. **Define command structure**:
   ```go
   package cli

   import "github.com/spf13/cobra"

   func newServeCmd() *cobra.Command {
       cmd := &cobra.Command{
           Use:   "serve [directory]",
           Short: "Serve static files",
           Long:  `Detailed description...`,
           RunE:  runServe,
       }

       // Add flags
       cmd.Flags().StringP("dir", "d", ".", "Directory to serve")

       return cmd
   }

   func runServe(cmd *cobra.Command, args []string) error {
       // Implementation
       return nil
   }
   ```

3. **Register in root command**: Add to `internal/cli/root.go`
   ```go
   func NewRootCmd() *cobra.Command {
       // ...
       rootCmd.AddCommand(newServeCmd())
       // ...
   }
   ```

4. **Add tests**: Create `internal/cli/serve_test.go`
5. **Update documentation**: Add to README.md

### Modifying Configuration

When adding new configuration options:

1. **Update struct**: Modify `internal/config/config.go`
   ```go
   type Config struct {
       // ... existing fields
       NewFeature NewFeatureConfig `mapstructure:"new_feature"`
   }

   type NewFeatureConfig struct {
       Enabled bool   `mapstructure:"enabled"`
       Timeout string `mapstructure:"timeout"`
   }
   ```

2. **Add defaults**: In `LoadConfig()` or `setDefaults()`
3. **Add validation**: In `Validate()` method
4. **Update example**: Modify `examples/radix.example.yml`
5. **Add tests**: Test loading and validation

### Adding Metrics

To add new metrics:

1. **Update collector**: Modify `internal/metrics/collector.go`
   ```go
   type Collector struct {
       // ... existing fields
       NewMetric atomic.Uint64
   }

   func (c *Collector) RecordNewMetric(value uint64) {
       c.NewMetric.Add(value)
   }
   ```

2. **Update snapshot**: Include in `Snapshot()` method
3. **Update Prometheus**: Add to `prometheus.go` exporter
4. **Add tests**: Test metric recording and export

---

## Testing Practices

### Test Organization

- **File naming**: `*_test.go` in the same package
- **Test functions**: `func TestFunctionName(t *testing.T)`
- **Benchmark tests**: `func BenchmarkFunctionName(b *testing.B)`

### Table-Driven Tests

Use table-driven tests for testing multiple scenarios:

```go
func TestCollector_RecordRequest(t *testing.T) {
    tests := []struct {
        name           string
        status         int
        method         string
        duration       time.Duration
        wantRequests   uint64
        wantStatusCode bool
    }{
        {
            name:           "success request",
            status:         200,
            method:         "GET",
            duration:       100 * time.Millisecond,
            wantRequests:   1,
            wantStatusCode: true,
        },
        {
            name:           "error request",
            status:         500,
            method:         "POST",
            duration:       50 * time.Millisecond,
            wantRequests:   1,
            wantStatusCode: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            collector := NewCollector("test", "1.0.0")
            collector.RecordRequest(tt.status, tt.method, tt.duration, 100, 200)

            // Assertions
            if got := collector.TotalRequests.Load(); got != tt.wantRequests {
                t.Errorf("TotalRequests = %d, want %d", got, tt.wantRequests)
            }
        })
    }
}
```

### Test Coverage

- **Target**: >80% code coverage
- **Run coverage**: `make coverage`
- **View report**: Open `coverage.html` in browser
- **CI enforcement**: Coverage uploaded to Codecov

### Race Detection

Always run tests with race detection:

```bash
# Enabled in make test
go test -race ./...
```

### HTTP Testing

Use `net/http/httptest` for testing HTTP handlers:

```go
func TestLoggingMiddleware(t *testing.T) {
    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte("OK"))
    })

    middleware := LoggingMiddleware(handler, "dev", os.Stdout)

    req := httptest.NewRequest("GET", "/test", nil)
    rec := httptest.NewRecorder()

    middleware.ServeHTTP(rec, req)

    if rec.Code != http.StatusOK {
        t.Errorf("unexpected status code: got %d, want %d", rec.Code, http.StatusOK)
    }
}
```

---

## Common Tasks

### Building

```bash
# Build for current platform
make build

# Binary output: ./bin/radix
./bin/radix version

# Build with custom version
VERSION=v1.0.0 make build

# Install to GOPATH/bin
make install
```

### Testing

```bash
# Run all tests
make test

# Run tests with coverage
make coverage

# Run specific package tests
go test -v ./internal/metrics/...

# Run specific test
go test -v -run TestCollector_RecordRequest ./internal/metrics/

# Benchmark tests
go test -bench=. ./internal/metrics/
```

### Linting

```bash
# Run all linters
make lint

# Auto-fix some issues
golangci-lint run --fix

# Run specific linter
golangci-lint run --disable-all -E errcheck
```

### Cleaning

```bash
# Remove build artifacts
make clean

# Remove dependencies cache
go clean -modcache
```

### Running

```bash
# Build and run
make run

# Run directly (after build)
./bin/radix version
./bin/radix validate --config examples/radix.example.yml

# Run with custom flags
./bin/radix version --json
./bin/radix version --short
```

---

## Code Patterns & Architecture

### Dependency Injection

Pass dependencies instead of creating them:

```go
// Good - testable
func NewServer(cfg *config.Config, logger *log.Logger) *Server {
    return &Server{
        config: cfg,
        logger: logger,
    }
}

// Bad - hard to test
func NewServer() *Server {
    cfg := config.Load() // Hard-coded dependency
    return &Server{config: cfg}
}
```

### Interface Design

Define interfaces for abstraction and testing:

```go
// Define interface in consumer package
type MetricsCollector interface {
    RecordRequest(status int, method string, duration time.Duration, bytesIn, bytesOut int64)
    Snapshot() *Metrics
}

// Use interface in middleware
func MetricsMiddleware(next http.Handler, collector MetricsCollector) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // ... implementation using collector
    })
}
```

### Thread Safety

Use atomic operations and sync primitives:

```go
type Collector struct {
    TotalRequests atomic.Uint64  // Atomic for lock-free increments
    StatusCodes   sync.Map        // sync.Map for concurrent map access
    mu            sync.RWMutex    // Mutex for complex operations
}

// Lock-free increment
func (c *Collector) IncrementRequests() {
    c.TotalRequests.Add(1)
}

// Protected read
func (c *Collector) GetData() Data {
    c.mu.RLock()
    defer c.mu.RUnlock()
    return c.data
}
```

### Context Usage

Always pass `context.Context` for cancellation:

```go
func (s *Server) Start(ctx context.Context) error {
    server := &http.Server{
        Addr:    s.addr,
        Handler: s.handler,
    }

    // Graceful shutdown on context cancellation
    go func() {
        <-ctx.Done()
        shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()
        server.Shutdown(shutdownCtx)
    }()

    return server.ListenAndServe()
}
```

### Configuration Loading

Use Viper with override hierarchy:

```go
// Priority: CLI flags > Env vars > Config file > Defaults
func LoadConfig(cfgFile string) (*Config, error) {
    v := viper.New()

    // Set defaults
    v.SetDefault("port", 8080)

    // Environment variables
    v.SetEnvPrefix("RADIX")
    v.AutomaticEnv()

    // Config file
    if cfgFile != "" {
        v.SetConfigFile(cfgFile)
    } else {
        v.SetConfigName("radix")
        v.AddConfigPath(".")
        v.AddConfigPath("$HOME")
        v.AddConfigPath("/etc/radix/")
    }

    if err := v.ReadInConfig(); err != nil {
        // Config file not required
    }

    // Unmarshal into struct
    var cfg Config
    if err := v.Unmarshal(&cfg); err != nil {
        return nil, err
    }

    return &cfg, nil
}
```

### Middleware Pattern

Chain HTTP middleware using standard `http.Handler`:

```go
func Chain(handler http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
    for i := len(middlewares) - 1; i >= 0; i-- {
        handler = middlewares[i](handler)
    }
    return handler
}

// Usage
handler := Chain(
    finalHandler,
    LoggingMiddleware,
    MetricsMiddleware,
    RecoveryMiddleware,
)
```

### Auth Extensions (HeaderProvider)

Radix supports pluggable auth header injection via the `HeaderProvider` interface, designed for corporate forks that need to inject tokens (Okta, Azure AD, etc.) into proxied requests:

```go
// internal/server/middleware/auth.go
type HeaderProvider interface {
    Headers(ctx context.Context, req *http.Request) (http.Header, error)
    Name() string
}
```

**Key design points:**
- Providers are registered at compile time via `RegisterHeaderProvider()` — no dynamic plugins
- **Auto-detection**: if a fork registers one provider, it's used automatically — no config needed
- Built-in `StaticProvider` handles fixed headers as a fallback
- Forks implement the interface and register via `init()` — engineers just run `radix proxy`
- Providers must be thread-safe (`Headers()` is called concurrently)
- Full design: see `IMPLEMENTATION_PLAN.md` Section 15

**Built-in config-driven header values (no fork required):** header values can be
sourced from environment variables and the OS keychain straight from config —
this covers the common corporate case without writing a provider. Two equivalent
authoring surfaces, both backed by the same per-request resolver
(`internal/server/middleware/auth_resolver.go`):

- **Surface A — `${...}` tokens** in `proxy.headers` / `--header` values:
  `${env:NAME}` and `${keychain:SERVICE/ACCOUNT}` (e.g.
  `"Authorization: Bearer ${keychain:work-cli/jwt}"`).
- **Surface B — structured** `proxy.auth.provider: headers` with a
  `config.headers` list, each entry naming exactly one source
  (`value` / `env` / `keychain`) plus an optional `prefix`. Compiles down to the
  same templates as Surface A.

Resolution is **per request** with a short TTL cache for keychain reads, so a
rotated token is picked up without restarting radix. Two security invariants:
**fail loud** (an unset env var or keychain miss returns 502, never a silent
unauthenticated proxy) and **never log secrets** (verbose injection logging emits
header names only). The keychain backend is `github.com/zalando/go-keyring`
(macOS Keychain, Windows Credential Manager, Linux Secret Service).

---

## CI/CD & Release Process

### Continuous Integration

**Workflow**: `.github/workflows/ci.yml`

**Triggers**:
- Push to `main` branch
- Pull requests to `main`

**Jobs**:
1. **Lint** (Ubuntu):
   - golangci-lint with 25+ linters
   - Timeout: 5 minutes

2. **Security** (Ubuntu):
   - gosec: Security scanning
   - govulncheck: Vulnerability scanning

3. **Test** (Ubuntu, macOS, Windows):
   - Run tests with race detection
   - Generate coverage report
   - Upload to Codecov (Ubuntu only)

4. **Build** (Ubuntu, macOS, Windows):
   - Build binary for each platform
   - Upload artifacts (7-day retention)

### Release Process

**Workflow**: `.github/workflows/release.yml`

**Triggers**:
- Git tags matching `v*.*.*` (e.g., `v1.0.0`)

**Process**:
1. Tag a release:
   ```bash
   git tag -a v1.0.0 -m "Release v1.0.0"
   git push origin v1.0.0
   ```

2. GitHub Actions automatically:
   - Runs GoReleaser
   - Builds binaries for all platforms (Linux, macOS, Windows × amd64, arm64)
   - Generates checksums (SHA256)
   - Creates GitHub release
   - Generates changelog from commits
   - Uploads release assets

**Release Assets**:
- `radix_<version>_<os>_<arch>.tar.gz` (Linux, macOS)
- `radix_<version>_<os>_<arch>.zip` (Windows)
- `checksums.txt` (SHA256 hashes)
- `CHANGELOG.md`

### Versioning

- **Scheme**: Semantic Versioning (SemVer) - `vMAJOR.MINOR.PATCH`
- **Injection**: Version info injected at build time via ldflags
- **Build variables**:
  - `Version`: Git tag (e.g., `v1.0.0`)
  - `Commit`: Git commit hash (short)
  - `Date`: Build date (YYYY-MM-DD)
  - `BuiltBy`: `goreleaser` or `dev`

### Pre-Release Checklist

Before creating a release:
- [ ] All tests passing
- [ ] Linters passing
- [ ] CHANGELOG.md updated
- [ ] Version bumped in relevant places
- [ ] Documentation updated
- [ ] Example configs updated

---

## Important Files Reference

### Configuration Files

| File | Purpose | Key Settings |
|------|---------|--------------|
| `go.mod` | Go module definition | Dependencies, Go version |
| `.golangci.yml` | Linter configuration | 25+ enabled linters, exclusions |
| `.goreleaser.yml` | Release automation | Build matrix, archives, checksums |
| `Makefile` | Build tasks | Build, test, lint, install targets |
| `examples/radix.example.yml` | Config example | All available options |

### Documentation

| File | Purpose |
|------|---------|
| `README.md` | Project overview, quick start |
| `CONTRIBUTING.md` | Contributor guidelines, workflow |
| `IMPLEMENTATION_PLAN.md` | Detailed feature roadmap (1,306 lines) |
| `CHANGELOG.md` | Release history |
| `LICENSE` | MIT License |
| `CLAUDE.md` | This file - AI assistant guide |

### Source Code Entry Points

| File | Purpose | Lines |
|------|---------|-------|
| `cmd/radix/main.go` | Application entry | 17 |
| `internal/cli/root.go` | Root command, global flags | 108 |
| `internal/config/config.go` | Config loading, validation | 213 |
| `internal/metrics/collector.go` | Metrics aggregation | 242 |

### Test Files

| Pattern | Purpose |
|---------|---------|
| `*_test.go` | Unit tests in same package |
| `testdata/` | Test fixtures (if needed) |

---

## AI Assistant Best Practices

### When Working on This Codebase

1. **Read Before Modifying**
   - Always read existing code before making changes
   - Understand the current implementation patterns
   - Check for similar code elsewhere in the project

2. **Maintain Consistency**
   - Follow existing code style and patterns
   - Use the same error handling approach
   - Match naming conventions

3. **Test Everything**
   - Add tests for new functionality
   - Run `make test` before committing
   - Ensure race detection passes
   - Check that coverage remains >80%

4. **Document Changes**
   - Add/update doc comments for exported functions
   - Update README.md for user-facing changes
   - Update IMPLEMENTATION_PLAN.md for roadmap changes
   - Update this file (CLAUDE.md) for workflow changes

5. **Check All CI Steps**
   - Run `make lint` before committing
   - Run `make test` before committing
   - Run `make build` to verify compilation
   - Fix any linter warnings

6. **Respect the Architecture**
   - Keep `internal/` packages private
   - Don't add external dependencies without justification
   - Use standard library when possible
   - Follow Go best practices and idioms

7. **Handle Errors Properly**
   - Never ignore errors
   - Wrap errors with context
   - Return errors, don't panic (except in truly exceptional cases)

8. **Be Thread-Safe**
   - Use atomic operations for simple counters
   - Use `sync.Map` for concurrent map access
   - Use mutexes for complex shared state
   - Test concurrent access patterns

9. **Keep It Simple**
   - Don't over-engineer solutions
   - Prefer clarity over cleverness
   - Write self-documenting code
   - Add comments only when necessary

10. **Follow the Roadmap**
    - Check `IMPLEMENTATION_PLAN.md` for planned features
    - Implement features in the planned order
    - Update the plan when deviating from it

### Common AI Assistant Pitfalls to Avoid

1. **Don't add dependencies casually** - This project keeps a small, curated dependency set; justify any new dependency (measure its real cost, e.g. binary-size impact, before adding)
2. **Don't skip tests** - Every new feature needs tests
3. **Don't ignore linter warnings** - They're there for a reason
4. **Don't create `pkg/` packages** - Use `internal/` for now
5. **Don't write to stdout directly** - Use the logging system
6. **Don't use `os.Exit()` in library code** - Return errors instead
7. **Don't hardcode values** - Use configuration or constants
8. **Don't forget thread safety** - This is a concurrent application

### Useful Commands for AI Assistants

```bash
# Check current state
make build && make test && make lint

# Quick verification
go test -short ./...

# Find all TODOs
grep -r "TODO" internal/

# Check test coverage for specific package
go test -cover ./internal/metrics/

# Run a single test
go test -v -run TestCollector_RecordRequest ./internal/metrics/

# Format all code
go fmt ./...
goimports -w .

# Check for security issues
golangci-lint run --enable gosec

# View available make targets
make help
```

### When Adding New Features

1. **Check IMPLEMENTATION_PLAN.md** for the feature design
2. **Create a feature branch** following naming conventions
3. **Write tests first** (TDD approach preferred)
4. **Implement the feature** following existing patterns
5. **Add documentation** to relevant files
6. **Run full CI locally** (`make test && make lint`)
7. **Update CHANGELOG.md** with the change
8. **Create a descriptive commit** using conventional commits

### When Fixing Bugs

1. **Add a failing test** that reproduces the bug
2. **Fix the bug** with minimal changes
3. **Verify the test passes** now
4. **Check for similar bugs** elsewhere
5. **Update documentation** if behavior changed
6. **Add regression test** if needed

### When Refactoring

1. **Ensure all tests pass** before starting
2. **Make small, incremental changes** one at a time
3. **Run tests after each change** to catch regressions
4. **Keep commits small and focused** on one change
5. **Don't change behavior** unless necessary
6. **Update documentation** to match refactored code

---

## Quick Reference

### Project Links

- **Repository**: https://github.com/osuritz/radix
- **Issues**: https://github.com/osuritz/radix/issues
- **Releases**: https://github.com/osuritz/radix/releases

### Key Commands

```bash
make build      # Build binary to ./bin/radix
make test       # Run tests with race detection
make coverage   # Generate coverage report
make lint       # Run all linters
make install    # Install to GOPATH/bin
make clean      # Remove build artifacts
make help       # Show all targets
```

### Important Paths

- **Binary**: `./bin/radix`
- **Config**: `./radix.yml`, `~/.radix.yml`, `/etc/radix/radix.yml`
- **Coverage**: `./coverage.html`
- **Examples**: `./examples/radix.example.yml`

### Environment Variables

- `RADIX_*` - Override config values (e.g., `RADIX_PORT=3000`)
- `GOPATH` - Go workspace path
- `CGO_ENABLED=0` - Required for static builds

---

## Conclusion

This guide should help AI assistants navigate and contribute to the Radix codebase effectively. When in doubt:

1. **Read the existing code** - It's the source of truth
2. **Check IMPLEMENTATION_PLAN.md** - For planned features
3. **Run tests frequently** - Catch issues early
4. **Follow Go conventions** - Standard library patterns
5. **Keep it simple** - Clarity over complexity

**Remember**: This is a tool for developers. Prioritize reliability, clarity, and ease of use over clever abstractions or premature optimization.

---

**Document maintained by**: AI Assistants and Contributors
**For questions**: Open an issue at https://github.com/osuritz/radix/issues
