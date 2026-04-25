.DEFAULT_GOAL := help

BIN_DIR     := bin
DAEMON_BIN  := $(BIN_DIR)/inarid
TUI_BIN     := $(BIN_DIR)/kitsune

# ============================================================
# Help
# ============================================================

.PHONY: help
help:                        ## Show this help
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*##/ { printf "  %-18s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

# ============================================================
# Build
# ============================================================

.PHONY: build
build: build-daemon build-tui  ## Build all binaries

.PHONY: build-daemon
build-daemon:                ## Build inarid (daemon)
	@mkdir -p $(BIN_DIR)
	go build -o $(DAEMON_BIN) ./cmd/inarid

.PHONY: build-tui
build-tui:                   ## Build kitsune (TUI)
	@mkdir -p $(BIN_DIR)
	go build -o $(TUI_BIN) ./cmd/kitsune

# ============================================================
# Run
# ============================================================

.PHONY: start
start: build                 ## Build, start ollama + inarid in background, then launch kitsune TUI
	@pgrep ollama > /dev/null || (echo "starting ollama..." && ollama serve > /dev/null 2>&1 &)
	@sleep 1
	@pgrep inarid > /dev/null && echo "inarid already running" || (./$(DAEMON_BIN) & echo $$! > /tmp/inarid.pid)
	@sleep 0.5
	@./$(TUI_BIN)
	@$(MAKE) --no-print-directory stop

.PHONY: stop
stop:                        ## Stop inarid background process
	@-kill $$(cat /tmp/inarid.pid 2>/dev/null) 2>/dev/null && rm -f /tmp/inarid.pid && echo "inarid stopped" || true

.PHONY: run-daemon
run-daemon:                  ## Run inarid directly (no build)
	go run ./cmd/inarid

.PHONY: run-tui
run-tui:                     ## Run kitsune TUI directly (no build)
	go run ./cmd/kitsune

# ============================================================
# Code quality
# ============================================================

.PHONY: fmt
fmt:                         ## Format all Go source files
	go fmt ./...

.PHONY: vet
vet:                         ## Run go vet
	go vet ./...

.PHONY: tidy
tidy:                        ## Tidy and verify go.mod / go.sum
	go mod tidy

.PHONY: lint
lint:                        ## Run staticcheck (install: go install honnef.co/go/tools/cmd/staticcheck@latest)
	staticcheck ./...

# ============================================================
# Test
# ============================================================

.PHONY: test
test:                        ## Run all tests
	go test ./...

.PHONY: test-v
test-v:                      ## Run all tests (verbose)
	go test -v ./...

# ============================================================
# Demo
# ============================================================

.PHONY: demo
demo: build-daemon build-tui      ## Generate VHS demo GIF
	@pgrep inarid > /dev/null && echo "inarid already running" || (./$(DAEMON_BIN) & echo $$! > /tmp/inarid.pid)
	@sleep 1
	/opt/homebrew/bin/vhs demo.tape
	@$(MAKE) --no-print-directory stop

# ============================================================
# Clean
# ============================================================

.PHONY: clean
clean:                       ## Remove build artefacts and socket
	rm -rf $(BIN_DIR)
	rm -f /tmp/inari.sock
	rm -f inari-audit.log
