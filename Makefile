SHELL := /usr/bin/env bash
.SHELLFLAGS := -eu -o pipefail -c

.DEFAULT_GOAL := help

GO          ?= go
GOLANGCILINT ?= golangci-lint
GORELEASER   ?= goreleaser

BIN_DIR := bin
BIN     := $(BIN_DIR)/omc

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
  -X github.com/SCB-SCREAM/oh-my-claude/internal/version.Version=$(VERSION) \
  -X github.com/SCB-SCREAM/oh-my-claude/internal/version.Commit=$(COMMIT) \
  -X github.com/SCB-SCREAM/oh-my-claude/internal/version.Date=$(DATE)

.PHONY: help
help: ## show this help
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: build
build: ## build the omc binary into ./bin
	@mkdir -p $(BIN_DIR)
	$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN) ./cmd/omc

.PHONY: install
install: ## go install the omc binary into $GOBIN
	$(GO) install -trimpath -ldflags '$(LDFLAGS)' ./cmd/omc

.PHONY: lint
lint: ## run golangci-lint
	$(GOLANGCILINT) run ./...

.PHONY: vet
vet: ## go vet
	$(GO) vet ./...

.PHONY: test
test: ## run unit tests with race detector
	$(GO) test -race ./...

.PHONY: test-cover
test-cover: ## run tests with coverage
	$(GO) test -race -coverprofile=coverage.out -covermode=atomic ./...

.PHONY: snapshot
snapshot: ## regenerate golden test files
	$(GO) test -update ./...

.PHONY: e2e
e2e: build ## end-to-end: run omc against each fixture
	@echo "e2e not yet implemented (milestone M3)"

.PHONY: tidy
tidy: ## go mod tidy
	$(GO) mod tidy

.PHONY: release-snapshot
release-snapshot: ## goreleaser release in snapshot mode (no publish)
	$(GORELEASER) release --snapshot --clean --skip=publish,sign

.PHONY: release-check
release-check: ## validate .goreleaser.yaml without building
	$(GORELEASER) check

.PHONY: clean
clean: ## remove build artifacts
	rm -rf $(BIN_DIR) dist coverage.out
