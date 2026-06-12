BINARY  := shoplazza
MODULE  := shoplazza-cli-v2
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
DATE    := $(shell date -u +%Y-%m-%d)

# DEV_PKG_ROOT bakes this checkout in as the jsbuild package root (scripts/jsbuild/
# + node_modules/), so a `make install`-ed binary outside the npm layout can still
# run `checkout build`/`app deploy`. Ignored at runtime if the path goes stale;
# override with DEV_PKG_ROOT= (empty) to omit.
DEV_PKG_ROOT ?= $(CURDIR)

LDFLAGS := -s -w \
           -X $(MODULE)/internal/build.Version=$(VERSION) \
           -X $(MODULE)/internal/build.Date=$(DATE) \
           -X $(MODULE)/internal/build.DevPkgRoot=$(DEV_PKG_ROOT)
# Default to a user-level install (no sudo, no root-owned files). Override with
# `make install PREFIX=/usr/local` (+ sudo) for a system-wide, all-users install.
PREFIX  ?= $(HOME)/.local

.PHONY: build vet unit-test integration-test test install uninstall clean

build:
	@echo "Building $(BINARY) $(VERSION)"
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) .

vet:
	go vet ./...

unit-test:
	go test -race -gcflags="all=-N -l" -count=1 \
	  ./cmd/... ./internal/... ./shortcuts/...

integration-test: build
	go test -v -count=1 ./tests/...

test: vet unit-test integration-test

# Builds straight into $(PREFIX)/bin — leaves no binary in the working directory.
install:
	@echo "Building $(BINARY) $(VERSION) → $(PREFIX)/bin"
	install -d $(PREFIX)/bin
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(PREFIX)/bin/$(BINARY) .
	@echo "OK: $(PREFIX)/bin/$(BINARY) ($(VERSION))"
	@case ":$$PATH:" in *":$(PREFIX)/bin:"*) ;; *) echo "warning: $(PREFIX)/bin is not on your PATH — add: export PATH=\"$(PREFIX)/bin:\$$PATH\"" >&2 ;; esac

uninstall:
	rm -f $(PREFIX)/bin/$(BINARY)

clean:
	rm -f $(BINARY)
