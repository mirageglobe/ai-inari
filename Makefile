# ================================================================ info ===== #

# project : ai-inari
# author  : mirageglobe

# ======================================================= configuration ===== #

PROJECT  := ai-inari
BIN_DIR  := bin
DAEMON   := $(BIN_DIR)/inarid
TUI      := $(BIN_DIR)/kitsune

.DEFAULT_GOAL := help

.SHELLFLAGS := -eu -o pipefail -c
.ONESHELL:

.PHONY: help all build clean fmt lint test run-daemon run-tui start stop demo

# ============================================================== targets ===== #

# ----------------------------------------------------------------- meta ----- #

help: ## show this menu
	@printf "\n  \033[33m$(PROJECT)\033[0m\n"
	@printf "\n  usage: make <target>\n\n"
	@awk '/^##@/ { printf "\n  \033[1m%s\033[0m\n", substr($$0, 5) } /^[a-zA-Z_-]+:.*##/ { printf "  \033[36m%-15s\033[0m %s\n", substr($$1, 1, length($$1)-1), substr($$0, index($$0, "##")+3) }' $(MAKEFILE_LIST)
	@printf "\n"

##@ build

all: lint test ## run lint and test

build: ## build all binaries
	@mkdir -p $(BIN_DIR)
	go build -o $(DAEMON) ./cmd/inarid
	go build -o $(TUI)    ./cmd/kitsune

clean: ## remove build artefacts and socket
	rm -rf $(BIN_DIR)
	rm -f /tmp/inari.sock inari-audit.log kitsune.log

##@ run

run-daemon: ## run inarid in foreground (no build)
	go run ./cmd/inarid

run-tui: ## run kitsune TUI (no build)
	go run ./cmd/kitsune

start: build ## build, start inarid in background, launch kitsune
	@pgrep ollama > /dev/null || (printf "starting ollama...\n" && ollama serve > /dev/null 2>&1 &)
	@sleep 1
	@pgrep inarid > /dev/null && printf "inarid already running\n" || (./$(DAEMON) & printf "$$!\n" > /tmp/inarid.pid)
	@sleep 0.5
	@./$(TUI)
	@$(MAKE) --no-print-directory stop

stop: ## stop background inarid
	@-kill $$(cat /tmp/inarid.pid 2>/dev/null) 2>/dev/null && rm -f /tmp/inarid.pid && printf "inarid stopped\n" || true

##@ verify

lint: ## run vet and staticcheck
	go vet ./...
	@command -v staticcheck >/dev/null && staticcheck ./... || printf "  staticcheck not found — skipping\n"

test: ## run all tests
	go vet ./...
	go test ./...

##@ demo

demo: build ## generate vhs demo gif
	@pgrep inarid > /dev/null && printf "inarid already running\n" || (./$(DAEMON) & printf "$$!\n" > /tmp/inarid.pid)
	@sleep 1
	/opt/homebrew/bin/vhs demo.tape
	@$(MAKE) --no-print-directory stop
