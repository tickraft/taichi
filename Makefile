# Taichi Test Orchestration Framework Makefile

BINARY   := taichi
BIN_DIR  := bin
CMD_DIR  := ./cmd/taichi
COVERAGE := 80

GO ?= go

.PHONY: all build test test-race test-cover test-integration lint fmt-check fmt clean install run help

all: build test

## build: Compile taichi binary to bin/taichi
build:
	$(GO) build -o $(BIN_DIR)/$(BINARY) $(CMD_DIR)

## test: Run unit tests
test:
	$(GO) test ./...

## test-race: Run tests with race detector enabled
test-race:
	$(GO) test -race ./...

## test-cover: Generate coverage report (threshold $(COVERAGE)%)
test-cover:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out | awk '/^total:/ {gsub("%",""); if ($$3 < $(COVERAGE)) exit 1}'

## test-integration: Run integration tests (tag: integration)
test-integration:
	$(GO) test -tags=integration ./...

## lint: Run golangci-lint (fails on gofmt issues too)
lint: fmt-check
	golangci-lint run --timeout 5m ./...

## fmt-check: Verify all Go files are gofmt-compliant (exit 1 if not)
fmt-check:
	@out=$$(gofmt -l $(shell find . -name '*.go' -not -path './vendor/*')); \
	if [ -n "$$out" ]; then \
		echo "gofmt issues found in the following files:"; \
		echo "$$out" | sed 's/^/  /'; \
		echo "Run 'make fmt' to fix."; \
		exit 1; \
	fi

## fmt: Format code
fmt:
	$(GO) fmt ./...
	gofmt -s -w .

## install: Install to GOBIN
install: build
	$(GO) install $(CMD_DIR)

## run: Directly run (example configuration)
run: build
	./$(BIN_DIR)/$(BINARY) run --config configs/taichi.yaml

## clean: Clean up build artifacts
clean:
	rm -rf $(BIN_DIR) coverage.out tests/reports/*.json tests/reports/*.xml

## help: Display all targets
help:
	@awk '/^## /{sub(/^## /,""); print}' $(MAKEFILE_LIST)
