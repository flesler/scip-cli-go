BINARY := bin/scip-cli-go
CMD    := ./cmd/scip-cli
GO     ?= go
export GOTOOLCHAIN=auto

GOLANGCI_LINT_VERSION := v2.12.2

GO_SDK := $(HOME)/.local/go-sdk/go/bin

# Repo-local tools in ./bin; prefer user Go 1.25+ over /usr/bin/go.
ifneq ($(wildcard $(GO_SDK)/go),)
export PATH := $(GO_SDK):$(CURDIR)/bin:$(PATH)
else
export PATH := $(CURDIR)/bin:$(PATH)
endif

.PHONY: build install test test-unit test-e2e test-cross fmt fmt-check vet typecheck lint tools setup pre-commit clean sync-upstream check-upstream publish

build:
	$(GO) build -o $(BINARY) $(CMD)

# Local dev only — never install as scip-cli on PATH (PyPI / editable Python keeps that name).
install: build
	@echo "Built $(BINARY) — run via ./$(BINARY) or add $(CURDIR)/bin to PATH for this repo only"

tools:
	@mkdir -p bin
	GOBIN=$(CURDIR)/bin $(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	GOBIN=$(CURDIR)/bin $(GO) install golang.org/x/tools/cmd/goimports@latest

# One-time dev setup: local Go tools + git hooks (no Python required).
setup: tools
	@chmod +x scripts/hooks/pre-commit
	@git config core.hooksPath scripts/hooks
	@echo "Ready. Git hooks in scripts/hooks/; Go tools in ./bin/"

test: build test-unit test-e2e

test-unit:
	$(GO) test $(shell $(GO) list ./internal/... | grep -v /e2e | grep -v /cross) -count=1

test-e2e:
	$(GO) test ./internal/e2e/... -count=1 -timeout 10m

# Python (PATH scip-cli) vs Go output parity on the TS fixture. Requires npx + ../scip-cli venv or PATH scip-cli.
test-cross:
	$(GO) test ./internal/cross/... -count=1 -timeout 15m

fmt: tools
	gofmt -s -w .
	goimports -local github.com/flesler/scip-cli-go/v2 -w .

fmt-check: tools
	@test -z "$$(gofmt -s -l .)" || (echo "gofmt needed on:"; gofmt -s -l .; exit 1)
	@test -z "$$(goimports -local github.com/flesler/scip-cli-go/v2 -l .)" || (echo "goimports needed on:"; goimports -local github.com/flesler/scip-cli-go/v2 -l .; exit 1)

typecheck:
	$(GO) build -o /dev/null ./...
	$(GO) vet ./...

vet: typecheck

lint: tools typecheck
	golangci-lint run --timeout=5m

pre-commit: fmt-check lint
	@git diff --quiet go.mod go.sum || (echo "go.mod/go.sum not tidy — run: go mod tidy" >&2; exit 1)

# Mirror allowlisted paths from flesler/scip-cli (see scripts/upstream.manifest).
sync-upstream:
	@chmod +x scripts/sync-upstream.sh
	@scripts/sync-upstream.sh --apply

check-upstream:
	@chmod +x scripts/sync-upstream.sh
	@scripts/sync-upstream.sh --check

publish:
	@chmod +x scripts/publish.sh scripts/test.sh
	@scripts/publish.sh $(PUBLISH_BUMP)

clean:
	rm -f $(BINARY)
	$(GO) clean -testcache
