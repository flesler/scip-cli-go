BINARY := bin/scip-cli-go
CMD    := ./cmd/scip-cli
GO     ?= go
export GOTOOLCHAIN=auto

.PHONY: build install test test-unit test-e2e fmt vet lint clean

build:
	$(GO) build -o $(BINARY) $(CMD)

# Local dev only — never install as scip-cli on PATH (PyPI / editable Python keeps that name).
install: build
	@echo "Built $(BINARY) — run via ./$(BINARY) or add $(CURDIR)/bin to PATH for this repo only"

test: build test-unit test-e2e

test-unit:
	$(GO) test $(shell $(GO) list ./internal/... | grep -v /e2e) -count=1

test-e2e:
	$(GO) test ./internal/e2e/... -count=1 -timeout 10m

fmt:
	gofmt -s -w .
	goimports -local github.com/sourcegraph/scip-cli-go -w . 2>/dev/null || true

vet:
	go vet ./...

lint: vet
	golangci-lint run --timeout=5m

pre-commit:
	pre-commit run --all-files

clean:
	rm -f $(BINARY)
	go clean -testcache

.PHONY: pre-commit
