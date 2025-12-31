# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0-alpha.1] - 2025-12-31

### Added
- Initial alpha release for testing CI/CD pipeline
- `radix version` command with build info display
- `radix validate` command for configuration file validation
- Metrics infrastructure (JSON and Prometheus formats)
- Logging middleware with multiple formats (CLF, dev)
- Full CI/CD automation with GitHub Actions
- GoReleaser configuration for multi-platform releases
- Comprehensive linting with golangci-lint (25+ linters)
- Security scanning with gosec and govulncheck

### Note
This is an alpha release to test the release workflow. Server commands (serve, proxy, echo, mock) are not yet implemented.

[Unreleased]: https://github.com/osuritz/radix/compare/v0.1.0-alpha.1...HEAD
[0.1.0-alpha.1]: https://github.com/osuritz/radix/releases/tag/v0.1.0-alpha.1
