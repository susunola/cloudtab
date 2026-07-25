.PHONY: build build-dev test test-race lint clean release

# Go toolchain
GO           ?= go
GOFLAGS      := -trimpath
LDFLAGS      := -s -w
VERSION      ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS      += -X main.Version=$(VERSION)

# Default target
build: build-dev

# Development build (fast, stripped, local arch only)
build-dev:
	$(GO) build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o cloudtab ./cmd/cloudtab

# Cross-compile for release (used by scripts/release.sh)
build-linux:
	GOOS=linux   GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o cloudtab_linux_amd64   ./cmd/cloudtab
	GOOS=linux   GOARCH=arm64 $(GO) build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o cloudtab_linux_arm64   ./cmd/cloudtab
build-darwin:
	GOOS=darwin  GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o cloudtab_darwin_amd64  ./cmd/cloudtab
	GOOS=darwin  GOARCH=arm64 $(GO) build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o cloudtab_darwin_arm64  ./cmd/cloudtab

test:
	$(GO) test ./...

test-race:
	$(GO) test -race -count=1 ./...

lint:
	$(GO) vet ./...
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run || true

clean:
	rm -f cloudtab cloudtab_* coverage.out

# Run full check (CI equivalent)
check: lint test-race
