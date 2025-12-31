# Radix Multi-Command Application - Implementation Plan

## Overview
Build a multi-command CLI tool in Go providing local development HTTP capabilities: static file serving, reverse proxy, request echo, and API mocking.

## 1. Project Structure (Go Best Practices)

```
radix/
├── cmd/
│   └── radix/
│       └── main.go              # Entry point
├── internal/
│   ├── cli/
│   │   ├── root.go              # Root command
│   │   ├── serve.go             # Static file serving command
│   │   ├── proxy.go             # Reverse proxy command
│   │   ├── echo.go              # Request echo command
│   │   └── mock.go              # API mocking command
│   ├── server/
│   │   ├── static.go            # Static file server implementation
│   │   ├── proxy.go             # Reverse proxy implementation
│   │   ├── echo.go              # Echo server implementation
│   │   ├── mock.go              # Mock server implementation
│   │   └── middleware/
│   │       ├── logging.go       # Request logging
│   │       ├── cors.go          # CORS handling
│   │       └── recovery.go      # Panic recovery
│   ├── config/
│   │   ├── config.go            # Configuration structs
│   │   └── loader.go            # Config file loading
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

## 3. Command Structure & Design

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

#### 3.1 Serve Command
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

#### 3.2 Proxy Command
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

#### 3.3 Echo Command
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

#### 3.4 Mock Command
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

# Command-specific configs
serve:
  dir: ./public
  spa: true
  cors: true
  gzip: true

proxy:
  target: http://localhost:3000
  timeout: 30s
  websocket: true

echo:
  delay: 0
  status: 200

mock:
  routes: ./mocks/routes.yml
  watch: true
```

## 5. Implementation Phases

### Phase 1: Foundation (Week 1)
- [ ] Initialize Go module
- [ ] Set up project structure
- [ ] Implement root command with Cobra
- [ ] Add version command
- [ ] Set up configuration loading (Viper)
- [ ] Implement logging middleware
- [ ] Set up testing framework

### Phase 2: Core Commands (Week 2-3)
- [ ] Implement `serve` command
  - [ ] Basic static file serving
  - [ ] Directory listing
  - [ ] Compression support
  - [ ] SPA mode
- [ ] Implement `proxy` command
  - [ ] Basic reverse proxy
  - [ ] Header manipulation
  - [ ] Path rewriting
  - [ ] WebSocket support

### Phase 3: Advanced Commands (Week 3-4)
- [ ] Implement `echo` command
  - [ ] Request inspection
  - [ ] Configurable responses
  - [ ] Delay simulation
- [ ] Implement `mock` command
  - [ ] YAML config parsing
  - [ ] Route matching
  - [ ] Template responses
  - [ ] File watching

### Phase 4: Polish & Release (Week 4-5)
- [ ] Comprehensive testing (unit + integration)
- [ ] Documentation
- [ ] Examples
- [ ] CI/CD setup (GitHub Actions)
- [ ] Release automation (GoReleaser)
- [ ] Cross-platform builds (Linux, macOS, Windows)

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

### GoReleaser
- Cross-platform builds (Linux, macOS, Windows, ARM)
- Archive generation (.tar.gz, .zip)
- Checksums & signatures
- Homebrew tap (optional)
- Docker images (optional)
- GitHub release automation

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

## 10. Success Metrics

- **Installation**: `go install github.com/osuritz/radix/cmd/radix@latest`
- **Usage**: `radix serve` starts server in <100ms
- **Size**: Binary <10MB (uncompressed)
- **Performance**: Handle 1000+ req/s for static serving
- **Compatibility**: Go 1.21+ on Linux, macOS, Windows

## 11. Example Usage Scenarios

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

## 12. Next Steps

1. **Review and approve this plan**
2. **Set up initial project structure**
3. **Initialize Go module and dependencies**
4. **Implement root command and configuration**
5. **Start with `serve` command (most straightforward)**
6. **Iterate based on testing and feedback**

---

**Questions to Consider:**
1. Do you want HTTPS/TLS support in v1.0?
2. Should there be a "combo" mode running multiple servers?
3. Any specific platform priority (Linux/macOS/Windows)?
4. Homebrew/apt/snap distribution plans?
5. Docker image needed?
