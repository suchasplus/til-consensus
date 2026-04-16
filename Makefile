GO ?= go
APP ?= til-consensus
CMD_PKG ?= ./cmd/til-consensus
BIN_DIR ?= ./bin
DIST_DIR ?= ./dist
DEBUG_FLAGS ?= -gcflags "all=-N -l"
VERSION_PKG ?= github.com/suchasplus/til-consensus/internal/buildinfo
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_TIME ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
DIRTY ?= $(shell if [ -n "$$(git status --porcelain 2>/dev/null)" ]; then echo true; else echo false; fi)
COMMON_LDFLAGS := -X $(VERSION_PKG).Version=$(VERSION) -X $(VERSION_PKG).Commit=$(COMMIT) -X $(VERSION_PKG).BuildTime=$(BUILD_TIME) -X $(VERSION_PKG).Dirty=$(DIRTY)
RELEASE_LDFLAGS := $(COMMON_LDFLAGS) -s -w
UNAME_S := $(shell uname -s)

ifeq ($(UNAME_S),Darwin)
INSTALL_DIR ?= $(HOME)/.local/bin
else
INSTALL_DIR ?= $(HOME)/.local/bin
endif

.PHONY: fmt test vet lint build build-debug build-release install run cover clean

fmt:
	$(GO) fmt ./...

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

lint:
	golangci-lint run

build:
	mkdir -p $(BIN_DIR)
	$(GO) build -ldflags "$(COMMON_LDFLAGS)" -o $(BIN_DIR)/$(APP) $(CMD_PKG)

build-debug:
	mkdir -p $(BIN_DIR)
	$(GO) build $(DEBUG_FLAGS) -ldflags "$(COMMON_LDFLAGS)" -o $(BIN_DIR)/$(APP)-debug $(CMD_PKG)

build-release:
	mkdir -p $(DIST_DIR)
	$(GO) build -trimpath -ldflags "$(RELEASE_LDFLAGS)" -o $(DIST_DIR)/$(APP) $(CMD_PKG)

install: build-release
	mkdir -p $(INSTALL_DIR)
	install -m 0755 $(DIST_DIR)/$(APP) $(INSTALL_DIR)/$(APP)
	@echo "installed $(APP) to $(INSTALL_DIR)/$(APP)"

run:
	$(GO) run -ldflags "$(COMMON_LDFLAGS)" $(CMD_PKG) $(ARGS)

cover:
	$(GO) test ./... -coverprofile=coverage.out
	$(GO) tool cover -func=coverage.out

clean:
	rm -rf $(BIN_DIR) $(DIST_DIR) coverage.out
