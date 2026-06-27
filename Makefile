# Makefile for the MLWH MCP server.
#
# Run `make help` for the target list. The quality gates (lint, test) are the
# same ones CI runs (see .github/workflows/ci.yml).

.PHONY: help build install lint format test config start clean

SHELL := bash

BINARY := mcp-server
PKG := ./cmd/mcp-server
GOBIN := $(shell go env GOBIN)
GOBIN := $(if $(GOBIN),$(GOBIN),$(shell go env GOPATH)/bin)

# Server version baked into the binary. Overrides the internal/core "dev"
# default and is surfaced by --version, the mcp-server://version resource, and
# the startup log line. Override with `make build VERSION=1.2.3`.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X github.com/wtsi-hgi/llm-knowledge-base/internal/core.ServerVersion=$(VERSION)

# Pinned golangci-lint used only when one is not already on PATH.
GOLANGCI_LINT_VERSION := v2.12.2

help:
	@printf 'MLWH MCP server -- make targets:\n\n'
	@printf '  build      Build ./%s (version: %s)\n' '$(BINARY)' '$(VERSION)'
	@printf '  install    go install %s into %s\n' '$(BINARY)' '$(GOBIN)'
	@printf '  lint       Run golangci-lint over all packages\n'
	@printf '  format     Apply gofmt (and cleanorder, if installed)\n'
	@printf '  test       Run the hermetic test suite\n'
	@printf '  config     Create .env from .env.example for local runs\n'
	@printf '  start      Load .env and serve over stdio (Ctrl-C to stop)\n'
	@printf '  clean      Remove the built %s binary\n' '$(BINARY)'
	@printf '\nSet MLWH_BASE_URL (env or --mlwh-base-url) to your "wa mlwh serve"\n'
	@printf 'instance. See the README for Claude Code / Codex CLI setup.\n'

build:
	go build -ldflags '$(LDFLAGS)' -o $(BINARY) $(PKG)

install:
	go install -ldflags '$(LDFLAGS)' $(PKG)

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run ./...; \
	fi

format:
	@git ls-files '*.go' | xargs -r gofmt -w
	@if command -v cleanorder >/dev/null 2>&1; then git ls-files '*.go' | xargs -r cleanorder -min-diff; fi

test:
	go test -count=1 ./...

config:
	@if [ -f .env ]; then \
		printf '%s\n' '.env already exists; leaving it untouched.'; \
	else \
		cp .env.example .env && printf '%s\n' 'Created .env from .env.example -- edit MLWH_BASE_URL to point at your "wa mlwh serve" instance.'; \
	fi

start:
	@set -a; [ -f .env ] && . ./.env || true; set +a; \
		if [ -z "$${MLWH_BASE_URL:-}" ]; then \
			printf '%s\n' 'MLWH_BASE_URL is not set. Run "make config" and edit .env, or export MLWH_BASE_URL.' >&2; \
			exit 1; \
		fi; \
		exec go run -ldflags '$(LDFLAGS)' $(PKG)

clean:
	rm -f $(BINARY)
