.PHONY: build test lint install clean run coverage help

# Version information
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE ?= $(shell date -u +"%Y-%m-%d")
LDFLAGS = -ldflags "-X github.com/osuritz/radix/internal/version.Version=$(VERSION) \
                     -X github.com/osuritz/radix/internal/version.Commit=$(COMMIT) \
                     -X github.com/osuritz/radix/internal/version.Date=$(DATE)"

# Binary name
BINARY = radix
BUILD_DIR = bin

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the binary
	@echo "Building $(BINARY)..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) ./cmd/radix

test: ## Run tests
	@echo "Running tests..."
	go test -v -race ./...

coverage: ## Run tests with coverage
	@echo "Running tests with coverage..."
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

lint: ## Run linters
	@echo "Running linters..."
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed. Install with:"; \
		echo "  curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b \$$(go env GOPATH)/bin"; \
		exit 1; \
	fi

install: ## Install the binary to GOPATH/bin
	@echo "Installing $(BINARY)..."
	go install $(LDFLAGS) ./cmd/radix

run: build ## Build and run the binary
	@echo "Running $(BINARY)..."
	$(BUILD_DIR)/$(BINARY)

clean: ## Remove build artifacts
	@echo "Cleaning..."
	rm -rf $(BUILD_DIR)/ dist/ coverage.out coverage.html

.DEFAULT_GOAL := help
