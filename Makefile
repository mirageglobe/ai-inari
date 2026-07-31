# ================================================================ info ===== #

# project : inari
# author  : mirageglobe

# ======================================================= configuration ===== #

PROJECT  := inari
BIN_DIR  := bin
BINARY   := $(BIN_DIR)/inari

VERSION := $(shell git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//' || echo "dev")

NEXT_VERSION = \
	tag=$$(git describe --tags --abbrev=0 2>/dev/null); \
	if [ -z "$$tag" ]; then echo "v0.1.0"; \
	else \
		major=$$(echo $$tag | sed 's/^v//' | cut -d. -f1); \
		minor=$$(echo $$tag | sed 's/^v//' | cut -d. -f2); \
		patch=$$(echo $$tag | sed 's/^v//' | cut -d. -f3); \
		echo "v$$major.$$minor.$$((patch+1))"; fi

NEXT_MINOR_VERSION = \
	tag=$$(git describe --tags --abbrev=0 2>/dev/null); \
	if [ -z "$$tag" ]; then echo "v0.1.0"; \
	else \
		major=$$(echo $$tag | sed 's/^v//' | cut -d. -f1); \
		minor=$$(echo $$tag | sed 's/^v//' | cut -d. -f2); \
		echo "v$$major.$$((minor+1)).0"; fi

NEXT_MAJOR_VERSION = \
	tag=$$(git describe --tags --abbrev=0 2>/dev/null); \
	if [ -z "$$tag" ]; then echo "v1.0.0"; \
	else \
		major=$$(echo $$tag | sed 's/^v//' | cut -d. -f1); \
		echo "v$$((major+1)).0.0"; fi

.DEFAULT_GOAL := help

.SHELLFLAGS := -eu -o pipefail -c
.ONESHELL:

.PHONY: help all build clean fmt lint test curated-sync run-daemon run-tui start stop demo bump-patch bump-minor bump-major push-tags release release-reset release-dry

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
	go run ./cmd/inari daemon -f

run-tui: ## run TUI only (no build, assumes daemon running)
	go run ./cmd/inari tui

start: build ## build and run inari start (daemon + TUI); stops any running daemon first
	@-./$(BINARY) stop
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

curated-sync: ## regenerate SPEC.md §6.1 tables from tui/views/curated.go (single source)
	go test ./tui/views -run TestCuratedTablesInSync -update-curated -count=1

##@ release

bump-patch: ## tag next patch version (e.g. v0.1.2 -> v0.1.3)
	@next=$$($(NEXT_VERSION)); \
	read -p "tag $$next? [y/N] " ans && [ "$$ans" = "y" ] && \
		git tag $$next && echo "tagged $$next" || echo "aborted"

bump-minor: ## tag next minor version (e.g. v0.1.3 -> v0.2.0)
	@next=$$($(NEXT_MINOR_VERSION)); \
	read -p "tag $$next? [y/N] " ans && [ "$$ans" = "y" ] && \
		git tag $$next && echo "tagged $$next" || echo "aborted"

bump-major: ## tag next major version (e.g. v0.2.0 -> v1.0.0)
	@next=$$($(NEXT_MAJOR_VERSION)); \
	read -p "tag $$next? [y/N] " ans && [ "$$ans" = "y" ] && \
		git tag $$next && echo "tagged $$next" || echo "aborted"

push-tags: ## push local tags to origin (triggers CI goreleaser)
	git push origin --tags

release: ## publish via goreleaser (requires GITHUB_TOKEN)
	goreleaser release --clean

release-reset: ## delete GitHub release for current tag (use before retrying a failed release)
	gh release delete v$(VERSION) --yes 2>/dev/null || true

release-dry: ## dry-run goreleaser without publishing
	goreleaser release --snapshot --clean

##@ demo

demo: build ## generate vhs demo gif
	./$(BINARY) daemon &
	@sleep 1
	/opt/homebrew/bin/vhs demo.tape
	./$(BINARY) stop
