.PHONY: build test test-coverage integration install install-completion clean run fmt lint vuln release-snapshot help

BINARY_NAME=mox
BUILD_DIR=./build
CMD_DIR=./cmd/mox
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT=$(shell git rev-parse HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u -d "@$${SOURCE_DATE_EPOCH:-$$(date -u +%s)}" +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS=-ldflags "-X github.com/bthall/mox/pkg/version.Version=$(VERSION) \
                  -X github.com/bthall/mox/pkg/version.GitCommit=$(GIT_COMMIT) \
                  -X github.com/bthall/mox/pkg/version.BuildDate=$(BUILD_DATE)"

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-18s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the binary
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_DIR)
	@echo "Built: $(BUILD_DIR)/$(BINARY_NAME)"

test: ## Run tests
	@echo "Running tests..."
	go test -race -count=1 ./...

test-coverage: ## Run tests with coverage
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

integration: ## Run integration tests (requires tmux)
	@echo "Running integration tests..."
	go test -tags=integration -race ./...

install: ## Install the binary and shell completion for $$SHELL
	@echo "Installing $(BINARY_NAME)..."
	go install $(LDFLAGS) $(CMD_DIR)
	@echo "Installed to $(shell go env GOPATH)/bin/$(BINARY_NAME)"
	@$(MAKE) install-completion

install-completion: ## Install shell completion for $$SHELL (bash, zsh, or fish)
	@BIN=$(shell go env GOPATH)/bin/$(BINARY_NAME); \
	if [ ! -x "$$BIN" ]; then echo "$$BIN not found; run 'make install' first" >&2; exit 1; fi; \
	SHELL_NAME=$$(basename "$${SHELL:-/bin/bash}"); \
	case "$$SHELL_NAME" in \
	  bash) \
	    DIR="$${XDG_DATA_HOME:-$$HOME/.local/share}/bash-completion/completions"; \
	    mkdir -p "$$DIR"; \
	    "$$BIN" completion bash > "$$DIR/$(BINARY_NAME)"; \
	    echo "Installed bash completion: $$DIR/$(BINARY_NAME)"; \
	    echo "(Reload your shell or 'source $$DIR/$(BINARY_NAME)' to activate.)" ;; \
	  zsh) \
	    DIR="$$HOME/.zsh/completions"; \
	    mkdir -p "$$DIR"; \
	    "$$BIN" completion zsh > "$$DIR/_$(BINARY_NAME)"; \
	    echo "Installed zsh completion: $$DIR/_$(BINARY_NAME)"; \
	    echo "Ensure your ~/.zshrc has: fpath=($$DIR \$$fpath); autoload -U compinit && compinit" ;; \
	  fish) \
	    DIR="$$HOME/.config/fish/completions"; \
	    mkdir -p "$$DIR"; \
	    "$$BIN" completion fish > "$$DIR/$(BINARY_NAME).fish"; \
	    echo "Installed fish completion: $$DIR/$(BINARY_NAME).fish" ;; \
	  *) \
	    echo "Unknown shell '$$SHELL_NAME'; install completion manually:" >&2; \
	    echo "  $$BIN completion bash > <somewhere on your bash-completion path>" >&2; \
	    exit 1 ;; \
	esac

clean: ## Remove build artifacts
	@echo "Cleaning..."
	rm -rf $(BUILD_DIR) dist
	rm -f coverage.out coverage.html
	go clean

run: build ## Build and run with example session
	@echo "Running $(BINARY_NAME)..."
	$(BUILD_DIR)/$(BINARY_NAME)

fmt: ## Format Go code
	@echo "Formatting code..."
	go fmt ./...

lint: ## Run linter (requires golangci-lint)
	@echo "Running linter..."
	golangci-lint run

vuln: ## Scan for known vulnerabilities
	@echo "Running govulncheck..."
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

release-snapshot: ## Build a local snapshot release with goreleaser
	@echo "Building snapshot release..."
	goreleaser release --snapshot --clean

.DEFAULT_GOAL := build
