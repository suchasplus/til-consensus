GO ?= go
APP ?= til-consensus
CMD_PKG ?= ./cmd/til-consensus
BIN_DIR ?= ./bin
DIST_DIR ?= ./dist
TARGET_GOOS ?= $(shell $(GO) env GOOS)
TARGET_GOARCH ?= $(shell $(GO) env GOARCH)
RELEASE_NAME ?= $(APP)_$(VERSION)_$(TARGET_GOOS)_$(TARGET_GOARCH)
DEBUG_FLAGS ?= -gcflags "all=-N -l"
COVERAGE_DIR ?= ./tmp/coverage
COVERPROFILE ?= $(COVERAGE_DIR)/cover.out
COVERAGE_SVG ?= $(COVERAGE_DIR)/coverage.svg
GO_COVER_TREEMAP_PKG ?= github.com/nikolaydubina/go-cover-treemap@latest
VERSION_PKG ?= github.com/suchasplus/til-consensus/internal/buildinfo
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_TIME ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
BUILD_MACHINE ?= $(shell hostname 2>/dev/null || echo unknown)
GIT_HOOKS_DIR ?= .git/hooks
DIRTY ?= $(shell if [ -n "$$(git status --porcelain 2>/dev/null)" ]; then echo true; else echo false; fi)
COMMON_LDFLAGS := -X $(VERSION_PKG).Version=$(VERSION) -X $(VERSION_PKG).Commit=$(COMMIT) -X $(VERSION_PKG).BuildTime=$(BUILD_TIME) -X $(VERSION_PKG).BuildMachine=$(BUILD_MACHINE) -X $(VERSION_PKG).Dirty=$(DIRTY)
RELEASE_LDFLAGS := $(COMMON_LDFLAGS) -s -w
UNAME_S := $(shell uname -s)

ifeq ($(UNAME_S),Darwin)
INSTALL_DIR ?= $(HOME)/.local/bin
else
INSTALL_DIR ?= $(HOME)/.local/bin
endif

.PHONY: fmt fmt-check test test-e2e test-e2e-real test-e2e-real-api vet lint pre-push install-git-hooks ci build build-debug build-release release-archive install run cover coverage-treemap clean

fmt:
	$(GO) fmt ./...

fmt-check:
	@output="$$(gofmt -l .)"; \
	if [ -n "$$output" ]; then \
		echo "以下文件未格式化:"; \
		echo "$$output"; \
		exit 1; \
	fi

test:
	$(GO) test ./...

test-e2e:
	$(GO) test ./internal/app -run '^TestE2E' -count=1

test-e2e-real:
	TIL_CONSENSUS_E2E_REAL=1 $(GO) test ./internal/app -run '^TestE2ERealCLIFixtureMatrix$$' -count=1 -timeout 180m

test-e2e-real-api:
	TIL_CONSENSUS_E2E_REAL_API=1 $(GO) test ./internal/app -run '^TestE2EReal(APIFixtureMatrix|APIProviderReadinessPreflight)$$' -count=1 -timeout 180m

vet:
	$(GO) vet ./...

lint:
	golangci-lint run

pre-push: fmt-check coverage-treemap vet lint build

install-git-hooks:
	@test -d .git || (echo "当前目录不是 git worktree 根目录"; exit 1)
	mkdir -p $(GIT_HOOKS_DIR)
	cp scripts/git-hooks/pre-push $(GIT_HOOKS_DIR)/pre-push
	chmod +x $(GIT_HOOKS_DIR)/pre-push
	@echo "installed git pre-push hook to $(GIT_HOOKS_DIR)/pre-push"

ci:
	$(MAKE) fmt-check
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
	mkdir -p $(COVERAGE_DIR)
	$(GO) test ./... -coverprofile=$(COVERPROFILE)
	$(GO) tool cover -func=$(COVERPROFILE)

coverage-treemap:
	mkdir -p $(COVERAGE_DIR)
	$(GO) test ./... -coverprofile=$(COVERPROFILE)
	@tool="$$(command -v go-cover-treemap 2>/dev/null || true)"; \
	if [ -z "$$tool" ]; then \
		gobin="$$( $(GO) env GOBIN )"; \
		if [ -z "$$gobin" ]; then gobin="$$( $(GO) env GOPATH )/bin"; fi; \
		if [ -x "$$gobin/go-cover-treemap" ]; then tool="$$gobin/go-cover-treemap"; fi; \
	fi; \
	if [ -z "$$tool" ]; then \
		echo "installing $(GO_COVER_TREEMAP_PKG)"; \
		$(GO) install $(GO_COVER_TREEMAP_PKG); \
		gobin="$$( $(GO) env GOBIN )"; \
		if [ -z "$$gobin" ]; then gobin="$$( $(GO) env GOPATH )/bin"; fi; \
		tool="$$gobin/go-cover-treemap"; \
	fi; \
	"$$tool" -coverprofile=$(COVERPROFILE) > $(COVERAGE_SVG)
	@echo "coverage treemap: $(COVERAGE_SVG)"

clean:
	rm -rf $(BIN_DIR) $(DIST_DIR) $(COVERAGE_DIR) coverage.out cover.out
