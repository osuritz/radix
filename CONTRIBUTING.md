# Contributing to radix

Thank you for your interest in contributing to radix! This document provides guidelines and instructions for contributing.

## Development Setup

### Prerequisites

- Go 1.25 or higher
- Git
- Make

### Getting Started

1. Fork the repository
2. Clone your fork:
   ```bash
   git clone https://github.com/YOUR_USERNAME/radix.git
   cd radix
   ```
3. Add upstream remote:
   ```bash
   git remote add upstream https://github.com/osuritz/radix.git
   ```
4. Install dependencies:
   ```bash
   go mod download
   ```

## Development Workflow

### Creating a Branch

Create a feature branch from `main`:

```bash
git checkout main
git pull upstream main
git checkout -b feature/your-feature-name
```

Branch naming conventions:
- `feature/` - New features
- `fix/` - Bug fixes
- `docs/` - Documentation changes
- `refactor/` - Code refactoring
- `test/` - Test additions or modifications

### Making Changes

1. Make your changes in your feature branch
2. Add tests for new functionality
3. Ensure all tests pass:
   ```bash
   make test
   ```
4. Run linters:
   ```bash
   make lint
   ```
5. Format your code:
   ```bash
   go fmt ./...
   goimports -w .
   ```

### Commit Messages

Follow conventional commit format:

```
type(scope): subject

body (optional)

footer (optional)
```

**Types:**
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `style`: Code style changes (formatting, etc.)
- `refactor`: Code refactoring
- `test`: Adding or modifying tests
- `chore`: Maintenance tasks
- `perf`: Performance improvements
- `ci`: CI/CD changes

**Examples:**
```
feat(serve): add gzip compression support

fix(proxy): correct header forwarding for WebSocket upgrade

docs: update README with installation instructions

test(cli): add tests for version command
```

### Running Tests

```bash
# Run all tests
make test

# Run tests with coverage
make coverage

# Run tests with race detection
go test -race ./...

# Run specific package tests
go test -v ./internal/cli/...
```

### End-to-End Smoke Tests

`make smoke` builds the binary and exercises every command end-to-end (version,
validate, gencert, serve, echo, mock built-ins and custom routes, and proxy)
against a real running server on fixed high ports, printing a PASS/FAIL line per
check and exiting non-zero on any failure:

```bash
make smoke
# or directly:
bash scripts/smoke.sh
```

Run it before opening a PR that touches command behavior. It needs `curl` and a
free port range starting at 18080.

### Building the Web UI

`radix` embeds a metrics dashboard — a Vite + React + TypeScript single-page app
under `ui/` — into the binary via a `//go:embed` directive (see `assets.go`). It
compiles to `ui/dist`; building it requires Node (CI uses Node 24) and is a
frontend-only toolchain, not a Go dependency.

```bash
make ui   # runs `npm ci && npm run build` in ui/, producing ui/dist
```

`make build` depends on `make ui`, so a normal build embeds the real dashboard.
When npm or `ui/` is unavailable the UI build is skipped gracefully: a committed
`ui/dist/.gitkeep` placeholder satisfies the embed, so a Node-less `go build
./...` still compiles — the binary then serves a "run `make ui`" page in place of
the dashboard.

For a fast UI-only dev loop, run Vite directly against a separately running
radix (e.g. `radix mock`) so the dashboard has live data:

```bash
cd ui && npm run dev   # Vite on :5173, proxies /_metrics to :9090
```

Frontend tests use [Vitest](https://vitest.dev/) and also run in the CI **UI
Tests** job:

```bash
cd ui && npm test
```

### Code Quality

Before submitting a PR:

1. **Linting**: Run `make lint` and fix all issues
2. **Testing**: Ensure coverage remains above 80%
3. **Documentation**: Update relevant docs and code comments
4. **Formatting**: Run `go fmt` and `goimports`

### Submitting a Pull Request

1. Push your changes to your fork:
   ```bash
   git push origin feature/your-feature-name
   ```

2. Create a pull request against the `main` branch

3. Fill out the PR template with:
   - Description of changes
   - Related issue number
   - Type of change
   - Testing performed
   - Checklist completion

4. Wait for CI checks to pass

5. Address review feedback

## Code Style

- Follow [Effective Go](https://golang.org/doc/effective_go.html) guidelines
- Use meaningful variable and function names
- Keep functions small and focused
- Add comments for exported functions and types
- Use package-level documentation

## Testing Guidelines

- Write table-driven tests where applicable
- Test both success and error cases
- Use subtests for related test cases
- Mock external dependencies
- Aim for >80% code coverage

Example:
```go
func TestServeCommand(t *testing.T) {
    tests := []struct {
        name    string
        args    []string
        wantErr bool
    }{
        {
            name:    "valid directory",
            args:    []string{"--dir", "./testdata"},
            wantErr: false,
        },
        {
            name:    "invalid directory",
            args:    []string{"--dir", "/nonexistent"},
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

## Documentation

- Update README.md for user-facing changes
- Update command help text for CLI changes
- Add code comments for complex logic
- Update CHANGELOG.md (will be automated)

### Documentation site

The public documentation site (https://osuritz.github.io/radix/) is built with
[VitePress](https://vitepress.dev/) and lives entirely under `docs/`. It is a
docs-only Node toolchain — it is **not** a Go dependency and does not affect the
`radix` binary or `go.mod`.

To run it locally:

```bash
cd docs
npm install
npm run docs:dev
```

`npm run docs:build` produces the static site under `docs/.vitepress/dist`.
Page content lives in `docs/site/`; site config and navigation live in
`docs/.vitepress/config.mts`. The site is built and deployed to GitHub Pages by
`.github/workflows/docs.yml` on pushes to `main` (pull requests build for
validation only).

## Release Process

Releases are automated via GitHub Actions:

1. Ensure all changes are merged to `main`
2. Create and push a version tag:
   ```bash
   git tag -a v1.0.0 -m "Release v1.0.0"
   git push origin v1.0.0
   ```
3. GitHub Actions will:
   - Build binaries for all platforms
   - Create a GitHub release
   - Generate changelog
   - Publish to package managers

## Getting Help

- Open an issue for bugs or feature requests
- Start a discussion for questions
- Review existing issues and PRs

## Code of Conduct

Be respectful, inclusive, and professional. We follow the [Contributor Covenant Code of Conduct](CODE_OF_CONDUCT.md).

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
