# Radix Multi-Command Application - Implementation Plan

## Overview

Build a multi-command CLI tool in Go providing local development HTTP capabilities: static file serving, reverse proxy, request echo, and API mocking.

### Target Audience

Radix is designed exclusively for **local development use**:

- **Software engineers and developers** needing quick HTTP tooling during development
- **Coding agents and AI assistants** requiring reliable, scriptable HTTP utilities
- **Frontend developers** who need to mock APIs or serve SPA builds
- **QA engineers** testing frontend/backend integration locally

### Explicit Non-Goals

Radix is **NOT intended for production traffic**. This means:

- No focus on high-concurrency performance optimization
- No clustering, load balancing across machines, or distributed features
- No advanced security hardening (WAF, DDoS protection, etc.)
- No pre-compressed file serving (files change constantly in development)
- No complex caching strategies (developers usually want fresh files)

For production use cases, users should use nginx, Caddy, or cloud load balancers.

### Design Philosophy

1. **Zero-config startup** - `radix serve` just works
2. **Developer experience first** - Clear errors, sensible defaults
3. **Single binary** - No runtime dependencies, easy to install
4. **Fast startup** - Sub-100ms to serving requests
5. **Observable** - Built-in metrics and logging for debugging

## 1. Project Structure (Go Best Practices)

```
radix/
├── cmd/
│   └── radix/
│       └── main.go              # Entry point
├── internal/
│   ├── cli/
│   │   ├── root.go              # Root command
│   │   ├── version.go           # Version command
│   │   ├── validate.go          # Config validation command
│   │   ├── serve.go             # Static file serving command
│   │   ├── proxy.go             # Reverse proxy command
│   │   ├── echo.go              # Request echo command
│   │   ├── mock.go              # API mocking command
│   │   └── gencert.go           # Certificate generation command
│   ├── server/
│   │   ├── static.go            # Static file server implementation
│   │   ├── proxy.go             # Reverse proxy implementation
│   │   ├── echo.go              # Echo server implementation
│   │   ├── mock.go              # Mock server implementation
│   │   └── middleware/
│   │       ├── logging.go       # Request logging
│   │       ├── cors.go          # CORS handling
│   │       └── recovery.go      # Panic recovery
│   ├── tls/
│   │   ├── generator.go         # Self-signed certificate generation
│   │   ├── loader.go            # Certificate loading and validation
│   │   └── config.go            # TLS configuration helpers
│   ├── metrics/
│   │   ├── collector.go         # Metrics collection
│   │   ├── prometheus.go        # Prometheus format exporter
│   │   └── histogram.go         # Response time histogram
│   ├── config/
│   │   ├── config.go            # Configuration structs
│   │   ├── loader.go            # Config file loading
│   │   └── validator.go         # Configuration validation
│   └── version/
│       └── version.go           # Version info
├── pkg/                         # Public libraries (if needed)
├── testdata/                    # Test fixtures
├── scripts/                     # Build/release scripts
├── docs/                        # Documentation
├── examples/                    # Example configurations
├── .gitignore
├── .goreleaser.yml             # Release automation
├── Makefile                     # Build tasks
├── go.mod
├── go.sum
├── LICENSE
└── README.md
```

### Why This Structure?
- **cmd/**: Application entry points (Go standard)
- **internal/**: Private application code (cannot be imported by other projects)
- **pkg/**: Reusable packages (none needed initially, use internal/)
- **testdata/**: Test fixtures (Go testing convention)
- Follows [project-layout](https://github.com/golang-standards/project-layout) standards

## 2. CLI Framework & Technology Stack

### Primary Framework: Cobra + Viper
```go
// Cobra: De facto standard for Go CLI applications
// Used by: kubectl, gh, hugo, docker, etc.
github.com/spf13/cobra

// Viper: Configuration management
github.com/spf13/viper
```

### Additional Dependencies
```go
// Minimal, focused dependencies:
- Standard library (net/http, net/http/httputil, etc.)
- github.com/spf13/cobra (CLI)
- github.com/spf13/viper (config)
- github.com/fatih/color (optional: colored output)
- gopkg.in/yaml.v3 (config files)
```

### Why Cobra?
- Industry standard for Go CLIs
- Excellent flag handling (persistent + local flags)
- Built-in help generation
- Subcommand support
- POSIX-compliant flags
- Auto-completion generation (bash, zsh, fish, powershell)

## 3. HTTPS/TLS Support Strategy

HTTPS support will be implemented in phases to provide comprehensive TLS capabilities:

### Phase 1: Certificate Generation
**Command:** `radix gencert [flags]`

Generate self-signed certificates for local development:

```bash
# Generate certificate for localhost
radix gencert --host localhost --output ./certs

# Generate for multiple domains/IPs
radix gencert --host "localhost,127.0.0.1,*.local.dev" --output ./certs

# Specify validity period
radix gencert --host localhost --days 365 --output ./certs
```

Flags:
- `--host`: Comma-separated list of hostnames/IPs (default: localhost)
- `--output, -o`: Output directory for cert files (default: ./certs)
- `--days`: Certificate validity in days (default: 365)
- `--org`: Organization name (default: "Radix Development")
- `--key-size`: RSA key size (default: 2048)

Output files:
- `cert.pem`: Certificate file
- `key.pem`: Private key file
- `ca.pem`: CA certificate (for importing into browser/system)

Features:
- RSA key generation (2048/4096 bit)
- X.509 certificate generation
- Subject Alternative Names (SAN) for multiple domains
- CA certificate generation for trust chain

### Phase 2: TLS Configuration Loading
Add TLS support to all server commands via shared flags:

Global TLS flags (available to serve, proxy, echo, mock):
- `--tls`: Enable HTTPS (default: false)
- `--cert`: Path to certificate file (required if --tls)
- `--key`: Path to private key file (required if --tls)
- `--ca`: Path to CA certificate (optional, for client cert verification)
- `--client-auth`: Require client certificates (default: false)
- `--tls-min-version`: Minimum TLS version (1.2, 1.3; default: 1.2)

Example:
```bash
# Serve with HTTPS
radix serve --tls --cert ./certs/cert.pem --key ./certs/key.pem

# Proxy with client certificate verification
radix proxy --target https://api.local \
  --tls --cert ./certs/cert.pem --key ./certs/key.pem \
  --ca ./certs/ca.pem --client-auth
```

### Phase 3: Per-Command TLS Integration

#### 3.1 Serve Command TLS
- HTTPS static file serving
- HTTP/2 support (automatic with TLS 1.2+)
- Optional HTTP-to-HTTPS redirect
- HSTS header support

Additional flags:
- `--http-redirect`: Redirect HTTP to HTTPS (requires --http-port)
- `--http-port`: HTTP port for redirects (default: 8080)
- `--hsts`: Enable HSTS headers

#### 3.2 Proxy Command TLS
- HTTPS frontend (accepting requests)
- HTTPS backend support (forwarding to HTTPS targets)
- Mutual TLS (mTLS) for backend connections
- Certificate pinning (optional)

Additional flags:
- `--backend-ca`: CA for verifying backend certificates
- `--backend-cert`: Client cert for backend mTLS
- `--backend-key`: Client key for backend mTLS

#### 3.3 Echo Command TLS
- HTTPS echo server
- Display TLS connection info in response
- Client certificate inspection

Response includes:
```json
{
  "tls": {
    "enabled": true,
    "version": "TLS 1.3",
    "cipher_suite": "TLS_AES_128_GCM_SHA256",
    "server_name": "localhost",
    "client_cert": {
      "subject": "CN=client",
      "issuer": "CN=Radix CA",
      "not_before": "2025-01-01T00:00:00Z",
      "not_after": "2026-01-01T00:00:00Z"
    }
  }
}
```

#### 3.4 Mock Command TLS
- HTTPS mock server
- Per-route TLS requirements
- Client certificate-based routing

Config enhancements:
```yaml
tls:
  enabled: true
  cert: ./certs/cert.pem
  key: ./certs/key.pem
  client_auth: optional

routes:
  - path: /api/public
    method: GET
    response:
      body: '{"public": true}'

  - path: /api/secure
    method: GET
    require_client_cert: true
    response:
      body: '{"authenticated": true, "cn": "{{client.cn}}"}'
```

### TLS Implementation Details

**Package Structure:**
```go
// internal/tls/generator.go
type CertGenerator struct {
    Hosts    []string
    ValidFor time.Duration
    KeySize  int
    Org      string
}

func (g *CertGenerator) Generate() (*Certificate, error)

// internal/tls/loader.go
type TLSLoader struct {
    CertFile   string
    KeyFile    string
    CAFile     string
    MinVersion uint16
}

func (l *TLSLoader) LoadConfig() (*tls.Config, error)

// internal/tls/config.go
func NewServerConfig(certFile, keyFile, caFile string, clientAuth bool) (*tls.Config, error)
func NewClientConfig(certFile, keyFile, caFile string) (*tls.Config, error)
```

**Security Considerations:**
- Strong cipher suites by default
- TLS 1.2 minimum (configurable to 1.3-only)
- Perfect Forward Secrecy (PFS) cipher suites preferred
- No SSLv3, TLS 1.0, TLS 1.1 support
- Certificate validation by default (skip-verify only with flag)

### Metrics & Observability

All server commands (serve, proxy, echo, mock) will expose a metrics endpoint for monitoring and observability.

**Metrics Endpoint:** `/_metrics` or `/__radix/metrics`

**Global Flags:**
- `--metrics`: Enable metrics endpoint (default: true)
- `--metrics-path`: Metrics endpoint path (default: /_metrics)
- `--metrics-format`: Output format (json, prometheus; default: json)

**Metrics Collected:**
```json
{
  "server": {
    "command": "serve",
    "uptime_seconds": 3600,
    "start_time": "2025-12-31T00:00:00Z",
    "version": "1.0.0"
  },
  "requests": {
    "total": 1500,
    "success": 1450,
    "errors": 50,
    "rate_per_second": 0.42
  },
  "status_codes": {
    "200": 1200,
    "304": 250,
    "404": 40,
    "500": 10
  },
  "methods": {
    "GET": 1400,
    "POST": 80,
    "PUT": 15,
    "DELETE": 5
  },
  "response_times": {
    "min_ms": 1,
    "max_ms": 523,
    "avg_ms": 45,
    "p50_ms": 32,
    "p95_ms": 120,
    "p99_ms": 280
  },
  "bandwidth": {
    "bytes_sent": 15728640,
    "bytes_received": 524288,
    "avg_request_size_bytes": 349,
    "avg_response_size_bytes": 10485
  }
}
```

**Command-Specific Metrics:**

*Serve Command:*
- File cache hit/miss ratio
- Compression ratios
- Most requested files
- Static assets served

*Proxy Command:*
- Backend response times
- Backend errors
- Connection pool stats
- WebSocket connections (active, total)

*Echo Command:*
- Average delay applied
- Request body sizes
- Custom response usage

*Mock Command:*
- Route match statistics
- Template rendering times
- Config reload count
- Failed route matches

**Prometheus Format Support:**
```prometheus
# HELP radix_requests_total Total number of HTTP requests
# TYPE radix_requests_total counter
radix_requests_total{command="serve",status="200",method="GET"} 1200

# HELP radix_response_time_seconds HTTP request duration
# TYPE radix_response_time_seconds histogram
radix_response_time_seconds_bucket{le="0.01"} 500
radix_response_time_seconds_bucket{le="0.05"} 1100
radix_response_time_seconds_bucket{le="0.1"} 1350
```

**Implementation:**
```go
// internal/metrics/collector.go
type Collector struct {
    StartTime      time.Time
    TotalRequests  atomic.Uint64
    StatusCodes    sync.Map  // map[int]uint64
    Methods        sync.Map  // map[string]uint64
    ResponseTimes  *Histogram
}

func (c *Collector) RecordRequest(status int, method string, duration time.Duration)
func (c *Collector) Snapshot() *Metrics
func (c *Collector) Handler() http.HandlerFunc

// internal/metrics/prometheus.go
func (c *Collector) PrometheusHandler() http.HandlerFunc
```

## 4. Command Structure & Design

### Root Command
```bash
radix [command] [flags]
```

Global flags (available to all commands):
- `--port, -p`: Port to listen on (default: 8080)
- `--host`: Host to bind to (default: localhost)
- `--verbose, -v`: Verbose logging
- `--config, -c`: Config file path
- `--no-color`: Disable colored output

### Commands

#### 3.1 Version Command
```bash
radix version [flags]
```

Purpose: Display version information

Flags:
- `--short`: Show only version number
- `--json`: Output as JSON

Output:
```json
{
  "version": "1.0.0",
  "commit": "abc123",
  "build_date": "2025-12-31",
  "go_version": "go1.22.0",
  "platform": "linux/amd64"
}
```

#### 3.2 Validate Command
```bash
radix validate [config-file] [flags]
```

Purpose: Validate configuration files

Flags:
- `--config, -c`: Config file to validate (default: ./radix.yml)
- `--type`: Config type (main, mock-routes; default: auto-detect)
- `--strict`: Strict validation mode (fail on warnings)

Features:
- YAML/JSON syntax validation
- Schema validation
- Check file paths exist
- Validate TLS certificate paths
- Validate port ranges
- Check for conflicting settings
- Warnings for deprecated options

Output:
```
✓ Configuration is valid: ./radix.yml

Checked:
  - Syntax: OK
  - Schema: OK
  - File paths: OK (cert.pem, key.pem found)
  - Port: 8080 (available)
  - TLS config: Valid

Warnings:
  - Consider setting tls.min_version to "1.3" for better security
```

#### 3.3 Serve Command
```bash
radix serve [directory] [flags]
```

Purpose: Static file server (like Python's SimpleHTTPServer)

Flags:
- `--dir, -d`: Directory to serve (default: current directory)
- `--index`: Index file (default: index.html)
- `--spa`: SPA mode (always serve index.html for 404s)
- `--browse, -b`: Open browser automatically
- `--cors`: Enable CORS
- `--gzip`: Enable gzip compression
- `--cache`: Cache-Control header value

Features:
- Directory listing (optional)
- MIME type detection
- Range requests support
- ETag generation
- Compression (gzip, brotli)

#### 3.4 Proxy Command
```bash
radix proxy [target] [flags]
```

Purpose: Reverse proxy to backend services

Flags:
- `--target, -t`: Target URL (required)
- `--rewrite`: Path rewrite rules (e.g., /api=/v1)
- `--strip-prefix`: Remove path prefix before forwarding
- `--timeout`: Request timeout (default: 30s)
- `--websocket`: Enable WebSocket proxying
- `--tls-skip-verify`: Skip TLS verification (dev only)
- `--headers`: Add/modify headers

Features:
- WebSocket support
- Request/response logging
- Header manipulation
- Path rewriting
- Load balancing (future: multiple targets)

#### 3.5 Echo Command
```bash
radix echo [flags]
```

Purpose: Echo server for debugging HTTP requests

Flags:
- `--delay`: Response delay (e.g., 500ms, 2s)
- `--status`: Default status code (default: 200)
- `--body`: Custom response body
- `--headers`: Custom response headers

Features:
- Return full request details (method, headers, body, query, etc.)
- Configurable delay (simulate slow servers)
- Custom responses
- JSON formatted output

Response format:
```json
{
  "method": "POST",
  "url": "/api/test",
  "headers": {...},
  "query": {...},
  "body": "...",
  "timestamp": "2025-12-31T00:00:00Z",
  "client_ip": "127.0.0.1"
}
```

#### 3.6 Mock Command
```bash
radix mock [config-file] [flags]
```

Purpose: API mocking server

Flags:
- `--routes, -r`: Routes config file (YAML/JSON)
- `--watch, -w`: Watch config file for changes
- `--latency`: Add artificial latency
- `--fail-rate`: Random failure percentage (chaos testing)

Config file format (YAML):
```yaml
routes:
  - path: /api/users
    method: GET
    status: 200
    response:
      body: '{"users": []}'
      headers:
        Content-Type: application/json
    delay: 100ms

  - path: /api/users/:id
    method: GET
    status: 200
    response:
      file: ./mocks/user.json

  - path: /api/users
    method: POST
    status: 201
    response:
      body: '{"id": "{{uuid}}", "created_at": "{{now}}"}'
```

Features:
- Route pattern matching (path parameters)
- Template responses ({{uuid}}, {{now}}, {{random}})
- File-based responses
- Hot reload on config changes
- Request matching (method, headers, query, body)

#### 3.7 GenCert Command
```bash
radix gencert [flags]
```

Purpose: Generate self-signed TLS certificates for local development

See **Section 3: HTTPS/TLS Support Strategy - Phase 1** for full details.

## 4. Shared Configuration System

### Configuration Priority (highest to lowest):
1. Command-line flags
2. Environment variables (RADIX_PORT, RADIX_HOST, etc.)
3. Config file (./radix.yml, ~/.radix.yml, /etc/radix/radix.yml)
4. Defaults

### Config File Format (radix.yml):
```yaml
# Global settings
port: 8080
host: localhost
verbose: false

# TLS settings (applies to all commands)
tls:
  enabled: false
  cert: ./certs/cert.pem
  key: ./certs/key.pem
  ca: ./certs/ca.pem
  client_auth: false
  min_version: "1.2"  # or "1.3"

# Command-specific configs
serve:
  dir: ./public
  spa: true
  cors: true
  gzip: true
  # TLS-specific for serve
  http_redirect: false
  http_port: 8080
  hsts: false

proxy:
  target: http://localhost:3000
  timeout: 30s
  websocket: true
  # Backend TLS settings
  backend_ca: ./certs/backend-ca.pem
  backend_cert: ./certs/client-cert.pem
  backend_key: ./certs/client-key.pem

echo:
  delay: 0
  status: 200

mock:
  routes: ./mocks/routes.yml
  watch: true
```

## 5. Implementation Phases

### Phase 1: Foundation
- [ ] Initialize Go module
- [ ] Set up project structure
- [ ] Implement root command with Cobra
- [ ] Implement version command
  - [ ] Version info struct
  - [ ] Build-time variable injection
  - [ ] JSON output format
  - [ ] Short format flag
- [ ] Set up configuration loading (Viper)
  - [ ] YAML/JSON parsing
  - [ ] Environment variable support
  - [ ] Config file discovery (./, ~/, /etc/)
- [ ] Implement validate command
  - [ ] Config file schema validation
  - [ ] File path existence checks
  - [ ] Port range validation
  - [ ] TLS certificate validation
  - [ ] Warning system for best practices
- [ ] Implement logging middleware
- [ ] Set up testing framework

### Phase 2: Metrics Infrastructure
- [ ] Implement `internal/metrics/collector.go`
  - [ ] Request counter (atomic operations)
  - [ ] Status code tracking
  - [ ] HTTP method tracking
  - [ ] Response time histogram
  - [ ] Bandwidth tracking
- [ ] Implement `internal/metrics/histogram.go`
  - [ ] Percentile calculations (p50, p95, p99)
  - [ ] Min/max/avg tracking
- [ ] Implement `internal/metrics/prometheus.go`
  - [ ] Prometheus text format exporter
  - [ ] Counter and histogram metrics
  - [ ] Label support
- [ ] Add metrics middleware
  - [ ] Request wrapping
  - [ ] Response wrapping for size tracking
  - [ ] Automatic metric recording
- [ ] Create metrics endpoint handler
- [ ] Add global metrics flags (--metrics, --metrics-path, --metrics-format)
- [ ] Tests for metrics collection

### Phase 3: CI/CD Pipeline & Automation
- [ ] **GitHub Actions Workflows**
  - [ ] Create `.github/workflows/ci.yml` for continuous integration
  - [ ] Create `.github/workflows/release.yml` for releases
  - [ ] Set up workflow triggers (push, pull_request, tags)

- [ ] **Build Automation**
  - [ ] Multi-platform build matrix (macOS, Windows, Linux)
  - [ ] Multiple Go versions (1.21, 1.22, 1.23)
  - [ ] Build caching for faster CI runs
  - [ ] Artifact uploading for builds

- [ ] **Code Quality & Linting**
  - [ ] Integrate golangci-lint with multiple linters
  - [ ] Configure linters (.golangci.yml)
  - [ ] Run `go vet` and `staticcheck`
  - [ ] Check code formatting with `gofmt`
  - [ ] Enforce consistent imports with `goimports`

- [ ] **Testing Automation**
  - [ ] Run tests on all platforms
  - [ ] Enable race detection (`-race` flag)
  - [ ] Generate coverage reports
  - [ ] Upload coverage to Codecov or similar
  - [ ] Set minimum coverage threshold

- [ ] **Security Scanning**
  - [ ] Integrate gosec for security issues
  - [ ] Run govulncheck for vulnerability scanning
  - [ ] Dependency security scanning
  - [ ] SAST (Static Application Security Testing)

- [ ] **Semantic Versioning**
  - [ ] Implement version detection from git tags
  - [ ] Support semantic versioning (vX.Y.Z)
  - [ ] Generate changelog from commits
  - [ ] Version bump automation

- [ ] **Release Automation (GoReleaser)**
  - [ ] Configure `.goreleaser.yml`
  - [ ] Cross-platform binary builds
  - [ ] Archive generation (.tar.gz, .zip)
  - [ ] SHA256 checksum generation
  - [ ] GitHub release creation
  - [ ] Release notes generation
  - [ ] Homebrew formula (future)

- [ ] **Additional Automation**
  - [ ] Dependabot configuration for dependency updates
  - [ ] Pull request templates
  - [ ] Issue templates
  - [ ] CODEOWNERS file
  - [ ] Branch protection rules documentation

### Phase 4: TLS Infrastructure
- [ ] **TLS Phase 1: Certificate Generation**
  - [ ] Implement `internal/tls/generator.go` for self-signed cert generation
  - [ ] Create `radix gencert` command
  - [ ] Support RSA key generation (2048/4096 bit)
  - [ ] Implement X.509 certificate with SAN support
  - [ ] Generate CA certificate for trust chain
  - [ ] Add tests for certificate generation
  - [ ] Document certificate usage

- [ ] **TLS Phase 2: Configuration & Loading**
  - [ ] Implement `internal/tls/loader.go` for cert loading
  - [ ] Implement `internal/tls/config.go` for TLS config helpers
  - [ ] Add global TLS flags (--tls, --cert, --key, --ca, --client-auth)
  - [ ] Support TLS version configuration (1.2, 1.3)
  - [ ] Implement cipher suite configuration
  - [ ] Add certificate validation
  - [ ] Create TLS configuration tests

### Phase 5: Core Commands (HTTP)
- [ ] Implement `serve` command (HTTP)
  - [ ] Basic static file serving
  - [ ] Directory listing
  - [ ] Compression support
  - [ ] SPA mode
  - [ ] CORS support
  - [ ] Integrate metrics middleware
  - [ ] Add serve-specific metrics (cache hits, file types, etc.)
  - [ ] Tests for serve command

- [ ] Implement `proxy` command (HTTP)
  - [ ] Basic reverse proxy
  - [ ] Header manipulation
  - [ ] Path rewriting
  - [ ] WebSocket support
  - [ ] Integrate metrics middleware
  - [ ] Add proxy-specific metrics (backend response times, errors, etc.)
  - [ ] Tests for proxy command

### Phase 6: Core Commands (HTTPS)
- [ ] **TLS Phase 3a: Serve Command TLS**
  - [ ] Add HTTPS support to serve command
  - [ ] Implement HTTP/2 support
  - [ ] Add HTTP-to-HTTPS redirect option
  - [ ] Implement HSTS headers
  - [ ] Integration tests with TLS

- [ ] **TLS Phase 3b: Proxy Command TLS**
  - [ ] Add HTTPS frontend to proxy
  - [ ] Implement HTTPS backend support
  - [ ] Add mutual TLS (mTLS) for backends
  - [ ] Backend certificate verification
  - [ ] Integration tests with TLS

### Phase 7: Advanced Commands (HTTP)
- [ ] Implement `echo` command (HTTP)
  - [ ] Request inspection and formatting
  - [ ] Configurable responses
  - [ ] Delay simulation
  - [ ] JSON output formatting
  - [ ] Integrate metrics middleware
  - [ ] Add echo-specific metrics (delays, custom responses, etc.)
  - [ ] Tests for echo command

- [ ] Implement `mock` command (HTTP)
  - [ ] YAML config parsing
  - [ ] Route matching engine
  - [ ] Template responses ({{uuid}}, {{now}}, etc.)
  - [ ] File watching and hot reload
  - [ ] Integrate metrics middleware
  - [ ] Add mock-specific metrics (route matches, template renders, config reloads)
  - [ ] Tests for mock command

### Phase 8: Advanced Commands (HTTPS)
- [ ] **TLS Phase 3c: Echo Command TLS**
  - [ ] Add HTTPS support to echo
  - [ ] Display TLS connection info in responses
  - [ ] Client certificate inspection
  - [ ] Integration tests with client certs

- [ ] **TLS Phase 3d: Mock Command TLS**
  - [ ] Add HTTPS support to mock
  - [ ] Per-route TLS requirements
  - [ ] Client certificate-based routing
  - [ ] Template support for client cert fields
  - [ ] Integration tests with complex TLS scenarios

### Phase 9: Polish & Release
- [ ] **Binary Signing & Security (Priority: GPG first)**
  - [ ] **GPG Signing (All Platforms)** - Cross-platform, no cost
    - [ ] Set up GPG key for release signing
    - [ ] Add GPG_FINGERPRINT to GitHub secrets
    - [ ] Configure GoReleaser for GPG signing of checksums
    - [ ] Update release workflow with GPG signing
    - [ ] Document signature verification in README
    - [ ] Add public key to repository/docs
  - [ ] macOS: Apple Developer ID codesigning setup (requires Apple Developer account)
  - [ ] macOS: Notarization with gon (requires notarization service)
  - [ ] Windows: Authenticode signing (requires code signing certificate)
  - [ ] Update release workflow with all signing methods
  - [ ] Create comprehensive signature verification documentation
- [ ] Comprehensive testing (unit + integration)
  - [ ] Achieve >80% code coverage
  - [ ] End-to-end TLS testing
  - [ ] Cross-platform testing
- [ ] Documentation
  - [ ] README with quickstart
  - [ ] Command documentation
  - [ ] TLS setup guide
  - [ ] Example configurations
  - [ ] Contributing guidelines
  - [ ] Binary verification guide (GPG)
- [ ] Examples
  - [ ] Basic usage examples for each command
  - [ ] TLS configuration examples
  - [ ] Mock API examples
  - [ ] Real-world use case examples
- [ ] Distribution enhancements
  - [ ] Homebrew tap setup
  - [ ] Chocolatey package (Windows)
  - [ ] Scoop manifest (Windows)
  - [ ] Installation documentation
  - [ ] Signature verification guide
- [ ] Final polish
  - [ ] Performance benchmarks
  - [ ] Memory profiling
  - [ ] Error message improvements
  - [ ] Help text refinement

## 6. Code Quality Standards

### Testing
- **Unit tests**: All packages in `internal/`
- **Integration tests**: End-to-end command testing
- **Coverage target**: >80%
- **Table-driven tests**: Use subtests for clarity

Example:
```go
func TestStaticServer(t *testing.T) {
    tests := []struct {
        name       string
        path       string
        wantStatus int
        wantBody   string
    }{
        {"index", "/", 200, "index.html content"},
        {"404", "/nonexistent", 404, ""},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // test implementation
        })
    }
}
```

### Code Organization
- **Interfaces**: Define interfaces for testability
- **Dependency injection**: Pass dependencies, don't create them
- **Error handling**: Always handle errors, use fmt.Errorf with %w
- **Context**: Use context.Context for cancellation
- **Logging**: Structured logging (consider zerolog or slog)

### Documentation
- **Package docs**: Every package has package comment
- **Function docs**: Exported functions have doc comments
- **Examples**: Provide runnable examples
- **README**: Comprehensive with badges, usage, installation

## 7. Build & Release Strategy

### Makefile
```makefile
.PHONY: build test lint install clean

build:
	go build -o bin/radix ./cmd/radix

test:
	go test -v -race -coverprofile=coverage.out ./...

lint:
	golangci-lint run

install:
	go install ./cmd/radix

clean:
	rm -rf bin/ dist/

release:
	goreleaser release --clean
```

### GoReleaser Configuration

**Build Matrix:**
- **macOS**: darwin/amd64, darwin/arm64
- **Windows**: windows/amd64, windows/arm64
- **Linux**: linux/amd64, linux/arm64, linux/arm

**Features:**
- Cross-platform builds with CGO disabled
- Archive generation (.tar.gz for Unix, .zip for Windows)
- Automatic checksums (SHA256)
- GitHub release automation
- Release notes generation from commits

**Binary Signing:**

1. **macOS Code Signing**
   ```yaml
   # .goreleaser.yml
   signs:
     - cmd: gon
       args: ["gon.hcl"]
       signature: "${artifact}.dmg"  # or .pkg
       artifacts: checksum
   ```
   - Use `gon` tool for notarization
   - Requires Apple Developer ID certificate
   - Environment variables: `AC_USERNAME`, `AC_PASSWORD`
   - Notarize for Gatekeeper compatibility

2. **Windows Authenticode Signing**
   ```yaml
   signs:
     - cmd: osslsigncode
       args:
         - sign
         - -certs
         - "${certificate}"
         - -key
         - "${key}"
         - -in
         - "${artifact}"
         - -out
         - "${signature}"
       artifacts: checksum
   ```
   - Use `osslsigncode` or Windows SDK `signtool`
   - Requires valid code signing certificate
   - Timestamp server for long-term validity

3. **GPG Signing (All Platforms)**
   ```yaml
   signs:
     - artifacts: checksum
       cmd: gpg
       args:
         - "--batch"
         - "-u"
         - "{{ .Env.GPG_FINGERPRINT }}"
         - "--output"
         - "${signature}"
         - "--detach-sign"
         - "${artifact}"
   ```
   - Sign SHA256SUMS file
   - Create detached signatures for binaries
   - Public key published in repository

**Example .goreleaser.yml structure:**
```yaml
before:
  hooks:
    - go mod tidy
    - go generate ./...

builds:
  - env:
      - CGO_ENABLED=0
    goos:
      - linux
      - windows
      - darwin
    goarch:
      - amd64
      - arm64
      - arm
    ignore:
      - goos: windows
        goarch: arm
    ldflags:
      - -s -w
      - -X main.version={{.Version}}
      - -X main.commit={{.Commit}}
      - -X main.date={{.Date}}

archives:
  - format: tar.gz
    format_overrides:
      - goos: windows
        format: zip
    files:
      - LICENSE
      - README.md
      - docs/*

checksum:
  name_template: 'checksums.txt'

snapshot:
  name_template: "{{ incpatch .Version }}-next"

changelog:
  sort: asc
  filters:
    exclude:
      - '^docs:'
      - '^test:'
```

### CI/CD (GitHub Actions)
```yaml
# .github/workflows/ci.yml
- Lint (golangci-lint)
- Test (multiple Go versions: 1.21, 1.22, 1.23)
- Build (multiple platforms)
- Security scan (gosec, govulncheck)
- Release (on tags)
```

## 8. Key Design Decisions

### 1. No External Dependencies for Core Logic
- Use standard library for HTTP serving
- Only use external deps for CLI framework and config

### 2. Single Binary
- No runtime dependencies
- Easy distribution and installation
- Cross-platform support

### 3. Configuration Flexibility
- Support flags, env vars, and config files
- Sensible defaults
- Override priority: flags > env > config > defaults

### 4. Developer Experience
- Clear error messages
- Colored output (when appropriate)
- Progress indicators for long operations
- Auto-completion support

### 5. Security Considerations
- No sensitive data in logs by default
- TLS support for proxy
- Request size limits
- Timeout configurations
- Rate limiting (future enhancement)

## 9. Non-Goals (v1.0)

To keep scope manageable:
- ❌ Authentication/Authorization
- ❌ Database integration
- ❌ Clustering/distributed mode
- ❌ GUI/Web UI
- ❌ Plugins/extensions
- ❌ Advanced load balancing

These can be considered for future versions based on user feedback.

## 10. Platform Support & Priorities

### Platform Priority (in order):
1. **macOS** (primary development platform)
   - Intel (amd64)
   - Apple Silicon (arm64)
2. **Windows** (secondary)
   - Windows 10/11 (amd64)
   - Windows on ARM (arm64)
3. **Linux** (tertiary)
   - amd64
   - arm64 (Raspberry Pi, cloud instances)
   - arm (32-bit Raspberry Pi)

### Testing Priority:
- macOS: Comprehensive testing on every build
- Windows: Regular testing, CI validation
- Linux: CI validation, community testing

### Distribution:
**Primary: GitHub Releases with Signed Binaries**

All platforms will be distributed via GitHub releases with downloadable, signed binaries:

- **macOS**:
  - `.tar.gz` archives with codesigned binaries
  - Apple Developer ID signature (notarization for Gatekeeper)
  - Universal binaries (amd64 + arm64) or separate builds

- **Windows**:
  - `.zip` archives with Authenticode-signed executables
  - Microsoft certificate signing
  - SmartScreen reputation building

- **Linux**:
  - `.tar.gz` archives
  - GPG-signed checksums file (SHA256SUMS.asc)
  - Detached GPG signatures for binaries

- **All platforms**:
  - `go install github.com/osuritz/radix/cmd/radix@latest` support
  - SHA256 checksums for all artifacts
  - SLSA provenance (optional, for supply chain security)

**Future Considerations (post-v1.0):**
- Homebrew tap (macOS)
- Chocolatey/Scoop (Windows)
- APT/Snap repositories (Linux)

## 11. Success Metrics

- **Installation**: `go install github.com/osuritz/radix/cmd/radix@latest`
- **Usage**: `radix serve` starts server in <100ms
- **Size**: Binary <10MB (uncompressed)
- **Performance**: Handle 1000+ req/s for static serving
- **Compatibility**: Go 1.21+ on macOS, Windows, Linux

## 12. Binary Verification Guide

Users should verify downloaded binaries before installation. Documentation will include:

### macOS Verification
```bash
# Verify codesigned binary
codesign -v -v radix

# Check signature details
codesign -d -vvv radix

# Verify notarization
spctl -a -v radix
```

### Windows Verification
```bash
# Using PowerShell
Get-AuthenticodeSignature radix.exe

# Check signature details
signtool verify /pa radix.exe
```

### Linux/All Platforms - GPG Verification
```bash
# Import public key (published in repository)
curl -sL https://github.com/osuritz/radix/releases/download/v1.0.0/radix.asc | gpg --import

# Verify checksums file signature
gpg --verify checksums.txt.asc checksums.txt

# Verify binary checksum
sha256sum -c checksums.txt --ignore-missing
```

### Quick Verification Script
```bash
#!/bin/bash
# verify-radix.sh
VERSION="v1.0.0"
BINARY="radix_${VERSION}_$(uname -s)_$(uname -m).tar.gz"

# Download files
curl -LO "https://github.com/osuritz/radix/releases/download/${VERSION}/${BINARY}"
curl -LO "https://github.com/osuritz/radix/releases/download/${VERSION}/checksums.txt"
curl -LO "https://github.com/osuritz/radix/releases/download/${VERSION}/checksums.txt.asc"

# Import GPG key
curl -sL "https://github.com/osuritz/radix/releases/download/${VERSION}/radix.asc" | gpg --import

# Verify
gpg --verify checksums.txt.asc checksums.txt
sha256sum -c checksums.txt --ignore-missing

echo "✓ Verification successful!"
```

## 13. Example Usage Scenarios

### Static Development Server
```bash
# Serve current directory
radix serve

# Serve specific directory with SPA routing
radix serve --dir ./dist --spa --port 3000

# With CORS and compression
radix serve --cors --gzip
```

### API Development Proxy
```bash
# Proxy API calls to backend
radix proxy --target http://localhost:3000

# With path rewriting
radix proxy --target http://api.example.com --rewrite /api=/v2

# WebSocket support
radix proxy --target ws://localhost:8080 --websocket
```

### Request Debugging
```bash
# Echo server
radix echo --port 9000

# With delay simulation
radix echo --delay 2s
```

### API Mocking
```bash
# Start mock server
radix mock --routes ./api-mocks.yml --watch

# With chaos testing
radix mock --routes ./api-mocks.yml --fail-rate 10
```

## 14. Next Steps

1. **Review and approve this plan** ✓
2. **Phase 1: Foundation** ✓ (completed)
3. **Phase 2: Metrics Infrastructure** ✓ (completed)
4. **Phase 3: CI/CD Pipeline & Automation** ✓ (completed)
   - Set up GitHub Actions workflows ✓
   - Configure linters and code quality tools ✓
   - Implement semantic versioning ✓
   - Set up GoReleaser for automated releases ✓
5. **Phase 4: TLS Infrastructure** ← Current
   - gencert command implementation
   - TLS configuration loading
6. **Phase 5: Core Commands (HTTP)** (serve, proxy)
7. **Phase 6: Core Commands (HTTPS)**
8. **Phase 7: Advanced Commands (HTTP)** (echo, mock)
9. **Phase 8: Advanced Commands (HTTPS)**
10. **Phase 9: Polish & Release**
    - Priority: GPG signing first (cross-platform, no cost)
    - Optional: Platform-specific signing (macOS, Windows)

---

## 15. Design Decisions Summary

**✓ Confirmed for v1.0:**
- HTTPS/TLS support (phased: generate certs → load certs → integrate per command)
- Metrics endpoint (all commands, JSON + Prometheus formats)
- Config validation command
- Platform priority: macOS > Windows > Linux
- Distribution: GitHub releases with signed binaries
  - macOS: Apple Developer ID codesigning + notarization
  - Windows: Authenticode signing
  - Linux: GPG-signed checksums
- `go install` support for all platforms

**✗ Deferred to future versions:**
- Interactive config generation mode
- Combo mode (multiple servers simultaneously)
- Package managers (Homebrew, Chocolatey, Scoop, apt, snap)
- Docker images
- Authentication/authorization
- Database integration
- Clustering/distributed mode
