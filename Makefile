# Taichi Test Orchestration Framework Makefile

BINARY   := taichi
BIN_DIR  := bin
CMD_DIR  := ./cmd/taichi
COVERAGE := 80

GO ?= go

# Version injected at build time via -ldflags "-X main.Version=...".
# Defaults to git describe output (e.g. v1.0.0, v1.0.0-3-gabc1234, abc1234-dirty);
# falls back to the default in cmd/taichi/version.go ("0.1.0-dev") when git is unavailable.
# Override with: make build VERSION=v1.2.3
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null)

# LDFLAGS strips debug info (-s -w) and injects the version string.
LDFLAGS := -s -w
ifneq ($(VERSION),)
	LDFLAGS += -X main.Version=$(VERSION)
endif

# BUILD_FLAGS combines -trimpath (removes local file paths for reproducible builds)
# with LDFLAGS (strip debug info + inject version).
BUILD_FLAGS := -trimpath -ldflags "$(LDFLAGS)"

.PHONY: all build test test-race test-cover test-integration lint fmt-check fmt clean install run help

all: build test

## build: Compile taichi binary to bin/taichi (version-injected, stripped debug symbols)
build:
	$(GO) build $(BUILD_FLAGS) -o $(BIN_DIR)/$(BINARY) $(CMD_DIR)

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

## install: Install to GOBIN (version-injected, stripped debug symbols)
install: build
	$(GO) install $(BUILD_FLAGS) $(CMD_DIR)

## run: Directly run (example configuration)
run: build
	./$(BIN_DIR)/$(BINARY) run --config configs/taichi.yaml

## clean: Clean up build artifacts
clean:
	rm -rf $(BIN_DIR) coverage.out tests/reports/*.json tests/reports/*.xml

## help: Display all targets
help:
	@awk '/^## /{sub(/^## /,""); print}' $(MAKEFILE_LIST)
