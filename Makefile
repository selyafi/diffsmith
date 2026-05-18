BINARY := diffsmith
BIN_DIR := bin
PKG := ./...

.PHONY: help build test lint fmt tidy clean run

help: ## Show this help.
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

build: ## Build the diffsmith binary into ./bin/.
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/$(BINARY) ./cmd/diffsmith

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
