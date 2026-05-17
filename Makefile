# ================================================================ info ===== #

# project : ai-inari
# author  : mirageglobe

# ======================================================= configuration ===== #

PROJECT  := ai-inari
BIN_DIR  := bin
BINARY   := $(BIN_DIR)/inari

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

build: ## build inari binary
	@mkdir -p $(BIN_DIR)
	go build -o $(BINARY) ./cmd/inari

clean: ## remove build artefacts and socket
	rm -rf $(BIN_DIR)
	rm -f /tmp/inari.sock inari-audit.log inari.log

##@ run

run-daemon: ## run daemon in foreground (no build)
	go run ./cmd/inari daemon

run-tui: ## run TUI only (no build, assumes daemon running)
	go run ./cmd/inari tui

start: build ## build and run inari start (daemon + TUI)
	@pgrep ollama > /dev/null || (printf "starting ollama...\n" && ollama serve > /dev/null 2>&1 &)
	@sleep 1
	./$(BINARY) start

stop: ## stop the running daemon
	./$(BINARY) stop

##@ verify

lint: ## run vet and staticcheck
	go vet ./...
	@command -v staticcheck >/dev/null && staticcheck ./... || printf "  staticcheck not found — skipping\n"

test: ## run all tests
	go vet ./...
	go test ./...

##@ demo

demo: build ## generate vhs demo gif
	./$(BINARY) daemon &
	@sleep 1
	/opt/homebrew/bin/vhs demo.tape
	./$(BINARY) stop
