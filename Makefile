GO ?= go
APP ?= til-consensus
CMD_PKG ?= ./cmd/til-consensus
BIN_DIR ?= ./bin
DIST_DIR ?= ./dist
TARGET_GOOS ?= $(shell $(GO) env GOOS)
TARGET_GOARCH ?= $(shell $(GO) env GOARCH)
RELEASE_NAME ?= $(APP)_$(VERSION)_$(TARGET_GOOS)_$(TARGET_GOARCH)
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

.PHONY: fmt test test-e2e vet lint ci build build-debug build-release release-archive install run cover clean

fmt:
	$(GO) fmt ./...

test:
	$(GO) test ./...

test-e2e:
	$(GO) test ./internal/app -run '^TestE2E' -count=1

vet:
	$(GO) vet ./...

lint:
	golangci-lint run

ci:
	@output="$$(gofmt -l .)"; \
	if [ -n "$$output" ]; then \
		echo "以下文件未格式化:"; \
		echo "$$output"; \
		exit 1; \
	fi
	$(GO) test ./...
	$(GO) test -race ./...
	$(GO) vet ./...
	golangci-lint run
	$(MAKE) build VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_TIME=$(BUILD_TIME) DIRTY=$(DIRTY)

build:
	mkdir -p $(BIN_DIR)
	$(GO) build -ldflags "$(COMMON_LDFLAGS)" -o $(BIN_DIR)/$(APP) $(CMD_PKG)

build-debug:
	mkdir -p $(BIN_DIR)
	$(GO) build $(DEBUG_FLAGS) -ldflags "$(COMMON_LDFLAGS)" -o $(BIN_DIR)/$(APP)-debug $(CMD_PKG)

build-release:
	mkdir -p $(DIST_DIR)
	$(GO) build -trimpath -ldflags "$(RELEASE_LDFLAGS)" -o $(DIST_DIR)/$(APP) $(CMD_PKG)

release-archive:
	rm -rf $(DIST_DIR)/$(RELEASE_NAME) $(DIST_DIR)/$(RELEASE_NAME).tar.gz
	mkdir -p $(DIST_DIR)/$(RELEASE_NAME)
	GOOS=$(TARGET_GOOS) GOARCH=$(TARGET_GOARCH) $(GO) build -trimpath -ldflags "$(RELEASE_LDFLAGS)" -o $(DIST_DIR)/$(RELEASE_NAME)/$(APP) $(CMD_PKG)
	tar -C $(DIST_DIR) -czf $(DIST_DIR)/$(RELEASE_NAME).tar.gz $(RELEASE_NAME)

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
