BINARY := bin/scip-cli-go
CMD    := ./cmd/scip-cli
GO     ?= go
export GOTOOLCHAIN=auto

GOLANGCI_LINT_VERSION := v1.64.8

# Repo-local tools in ./bin — not global PATH.
export PATH := $(CURDIR)/bin:$(PATH)

.PHONY: build install test test-unit test-e2e fmt fmt-check vet typecheck lint tools setup pre-commit clean

build:
	$(GO) build -o $(BINARY) $(CMD)

# Local dev only — never install as scip-cli on PATH (PyPI / editable Python keeps that name).
install: build
	@echo "Built $(BINARY) — run via ./$(BINARY) or add $(CURDIR)/bin to PATH for this repo only"

tools:
	@mkdir -p bin
	GOBIN=$(CURDIR)/bin $(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	GOBIN=$(CURDIR)/bin $(GO) install golang.org/x/tools/cmd/goimports@latest

# One-time dev setup: local Go tools + venv pre-commit + git hooks.
setup: tools
	@test -d .venv || python3 -m venv .venv
	.venv/bin/pip install -q -r requirements-dev.txt
	.venv/bin/pre-commit install
	@echo "Ready. Hooks run via .venv/bin/pre-commit; Go tools in ./bin/"

test: build test-unit test-e2e

test-unit:
	$(GO) test $(shell $(GO) list ./internal/... | grep -v /e2e) -count=1

test-e2e:
	$(GO) test ./internal/e2e/... -count=1 -timeout 10m

fmt: tools
	gofmt -s -w .
	goimports -local github.com/sourcegraph/scip-cli-go -w .

fmt-check: tools
	@test -z "$$(gofmt -s -l .)" || (echo "gofmt needed on:"; gofmt -s -l .; exit 1)
	@test -z "$$(goimports -local github.com/sourcegraph/scip-cli-go -l .)" || (echo "goimports needed on:"; goimports -local github.com/sourcegraph/scip-cli-go -l .; exit 1)

typecheck:
	$(GO) build -o /dev/null ./...
	$(GO) vet ./...

vet: typecheck

lint: tools typecheck
	golangci-lint run --timeout=5m

pre-commit:
	@if [ -x .venv/bin/pre-commit ]; then .venv/bin/pre-commit run --all-files; \
	else pre-commit run --all-files; fi

clean:
	rm -f $(BINARY)
	$(GO) clean -testcache
