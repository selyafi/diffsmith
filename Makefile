BINARY := diffsmith
BIN_DIR := bin
PKG := ./...

# VERSION is stamped into the binary via -ldflags. Defaults to the most
# recent annotated tag plus a short SHA suffix so local builds carry a
# traceable version string; CI/release builds should pass VERSION=vX.Y.Z
# explicitly.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

.PHONY: help build test lint fmt tidy clean run version

help: ## Show this help.
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

build: ## Build the diffsmith binary into ./bin/ (stamps version via -ldflags).
	@mkdir -p $(BIN_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY) ./cmd/diffsmith

version: build ## Print the stamped version of the just-built binary.
	@$(BIN_DIR)/$(BINARY) --version

test: ## Run unit tests.
	go test $(PKG)

lint: ## Run golangci-lint (install: https://golangci-lint.run/).
	golangci-lint run $(PKG)

fmt: ## Format Go source files.
	gofmt -w .

tidy: ## Update go.mod / go.sum.
	go mod tidy

run: build ## Build then run with --help.
	$(BIN_DIR)/$(BINARY) --help

clean: ## Remove build outputs.
	rm -rf $(BIN_DIR) coverage.out coverage.html
