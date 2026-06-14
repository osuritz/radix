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
