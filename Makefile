.DEFAULT_GOAL := help

BIN_DIR    := bin
DAEMON_BIN := 20 20 12 61 79 80 81 98 701 33 100 204 250 395 398 399 400 702BIN_DIR)/haniwad
CLIENT_BIN := $(BIN_DIR)/s9s

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
build: build-daemon build-client  ## Build both binaries

.PHONY: build-daemon
build-daemon:                ## Build haniwad (daemon)
	@mkdir -p $(BIN_DIR)
	go build -o $(DAEMON_BIN) ./cmd/haniwad

.PHONY: build-client
build-client:                ## Build s9s (TUI client)
	@mkdir -p $(BIN_DIR)
	go build -o $(CLIENT_BIN) ./cmd/s9s

# ============================================================
# Run (development)
# ============================================================

.PHONY: run-daemon
run-daemon:                  ## Run haniwad directly (no build)
	go run ./cmd/haniwad

.PHONY: run-client
run-client:                  ## Run s9s directly (no build)
	go run ./cmd/s9s

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
# Clean
# ============================================================

.PHONY: clean
clean:                       ## Remove build artefacts and socket
	rm -rf $(BIN_DIR)
	rm -f /tmp/haniwa.sock
	rm -f haniwa-audit.log
