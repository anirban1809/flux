# ============================================================================
# ZipCode – Makefile
# TUI-first, single-binary Go application
# ============================================================================

# ---- Project metadata ------------------------------------------------------
APP_NAME       := zipcode
CMD_DIR        := .
BIN_DIR        := ./bin
DIST_DIR       := ./dist
INSTALL_DIR    := $(HOME)/.zipcode/bin

GO             := go
GOFLAGS        :=
LDFLAGS        := -s -w

VERSION        := $(shell git describe --tags --dirty --always 2>/dev/null || echo dev)
COMMIT         := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_TIME     := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Inject build metadata (optional but useful for UI footer)
LD_META        := -X main.version=$(VERSION) \
                  -X main.commit=$(COMMIT) \
                  -X main.buildTime=$(BUILD_TIME)

# ---- Build targets ----------------------------------------------------------
.DEFAULT_GOAL := help

.PHONY: help
help:
	@echo "ZipCode – available targets:"
	@echo ""
	@echo "  build        Build zipcode binary (local)"
	@echo "  run          Run zipcode in dev mode"
	@echo "  dev          Build + run with race detector"
	@echo "  test         Run all tests"
	@echo "  lint         Run static checks"
	@echo "  fmt          Format Go sources"
	@echo "  clean        Remove build artifacts"
	@echo "  dist         Build release binaries"
	@echo ""

# ---- Local build ------------------------------------------------------------
.PHONY: build
build:
	@mkdir -p $(BIN_DIR)
	$(GO) build $(GOFLAGS) \
		-ldflags "$(LDFLAGS) $(LD_META)" \
		-o $(BIN_DIR)/$(APP_NAME) \
		$(CMD_DIR)
	@mkdir -p $(INSTALL_DIR)
	@cp $(BIN_DIR)/$(APP_NAME) $(INSTALL_DIR)/$(APP_NAME)
	@echo "installed $(APP_NAME) -> $(INSTALL_DIR)/$(APP_NAME)"

# ---- Run --------------------------------------------------------------------
.PHONY: run
run:
	$(GO) run $(CMD_DIR)

# ---- Dev mode (race + debug) ------------------------------------------------
.PHONY: dev
dev:
	$(GO) run -race $(CMD_DIR)

# ---- Tests ------------------------------------------------------------------
.PHONY: test
test:
	$(GO) test ./...

# ---- Lint -------------------------------------------------------------------
.PHONY: lint
lint:
	@command -v golangci-lint >/dev/null 2>&1 || \
		{ echo "golangci-lint not installed"; exit 1; }
	golangci-lint run

# ---- Format -----------------------------------------------------------------
.PHONY: fmt
fmt:
	$(GO) fmt ./...

# ---- Clean ------------------------------------------------------------------
.PHONY: clean
clean:
	rm -rf $(BIN_DIR) $(DIST_DIR)

# ---- Release builds ---------------------------------------------------------
.PHONY: dist
dist:
	@mkdir -p $(DIST_DIR)
	GOOS=linux   GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS) $(LD_META)" -o $(DIST_DIR)/$(APP_NAME)-linux-amd64   $(CMD_DIR)
	GOOS=darwin  GOARCH=arm64 $(GO) build -ldflags "$(LDFLAGS) $(LD_META)" -o $(DIST_DIR)/$(APP_NAME)-darwin-arm64  $(CMD_DIR)
	GOOS=darwin  GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS) $(LD_META)" -o $(DIST_DIR)/$(APP_NAME)-darwin-amd64  $(CMD_DIR)
