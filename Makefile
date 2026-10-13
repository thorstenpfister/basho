# basho — see SPEC.md §14.1. Targets carry a "## description" for `make help`.
# Written for GNU Make 3.81 (the macOS default).

GO   ?= go
BIN  := bin
ARGS ?=

# Only the version is injected. Commit, dirty flag and commit time come from
# the VCS stamping of the Go toolchain (internal/buildinfo). Deferred (=), so
# only build and install run git.
LDFLAGS = -X github.com/thorstenpfister/basho/internal/buildinfo.Version=$(shell git describe --tags --always 2>/dev/null || echo devel)

# todo is the recipe of a target whose phase (see its description) has not arrived yet.
todo = @echo "make $@: not implemented yet, see make help" >&2; exit 2

# confirm asks before a target that calls the APIs; YES=1 skips the question.
confirm = @[ "$(YES)" = 1 ] || { printf '%s Continue? [y/N] ' "$(1)"; read a; [ "$$a" = y ] || [ "$$a" = Y ]; }

.DEFAULT_GOAL := help
.PHONY: $(shell sed -n 's/^\([a-z][a-z-]*\):.*\#\# .*/\1/p' $(MAKEFILE_LIST))

help: ## List all targets
	@awk 'BEGIN { FS = ":.*## " } /^## / { printf "\n%s\n", substr($$0, 4) } /^[a-z][a-z-]*:.*## / { printf "  %-17s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

## Build and run

build: ## Build bin/basho and bin/basho-data with version info
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BIN)/ ./cmd/...

install: ## Install basho with version info (go install)
	$(GO) install -ldflags "$(LDFLAGS)" ./cmd/basho

run: ## Run basho with ARGS
	$(GO) run ./cmd/basho $(ARGS)

clean: ## Remove bin/, dist/ and coverage files (never data/ or the work cache)
	rm -rf $(BIN) dist coverage.out coverage.html

## Test

test: ## Run all tests
	$(GO) test ./...

test-race: ## Run all tests with the race detector
	$(GO) test -race ./...

test-live: ## Real API smoke tests, manual only (phase 1)
	$(todo)

golden: ## Rewrite golden files with -update; review the diff (phase 6)
	$(todo)

cover: ## Coverage profile and HTML report (coverage.html)
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

## Code quality

fmt: ## Format with gofumpt
	$(GO) tool gofumpt -w .

fmt-check: ## Fail if gofumpt would change anything
	@out=$$($(GO) tool gofumpt -l .); if [ -n "$$out" ]; then echo "needs gofumpt:"; echo "$$out"; exit 1; fi

lint: ## Run golangci-lint
	golangci-lint run

vuln: ## Check for known vulnerabilities
	$(GO) tool govulncheck ./...

tidy: ## go mod tidy and verify
	$(GO) mod tidy
	$(GO) mod verify

check: fmt-check lint vuln test-race ## All checks; run before committing

tools: ## Check that external tools are installed
	@command -v golangci-lint >/dev/null || { echo "missing golangci-lint: brew install golangci-lint"; exit 1; }
	@command -v goreleaser >/dev/null || echo "optional until phase 13: goreleaser (brew install goreleaser)"
	@echo "tools ok"

## Data (network targets will ask before fetching unless YES=1)

fixtures: build ## Record the test fixtures into testdata/ (SPEC §13)
	$(call confirm,basho-data fixtures record fetches each fixture once from sumo-api.com and osada.us.)
	$(BIN)/basho-data fixtures record

record: build ## Live recorder for tournament days, ARGS passed on (SPEC Appendix E)
	$(call confirm,basho-data record calls osada.us and sumo-api.com about 5 times per tick during the live window.)
	$(BIN)/basho-data record $(ARGS)

snapshot: build ## Update and verify the embedded snapshot, ARGS passed on (SPEC §4.3)
	$(BIN)/basho-data snapshot update --dry-run $(ARGS)
	$(call confirm,basho-data snapshot update fetches the tournaments above from sumo-api.com at 1 request per second.)
	$(BIN)/basho-data snapshot update $(ARGS)
	$(BIN)/basho-data snapshot verify

snapshot-verify: build ## Verify the embedded snapshot (phase 3)
	$(BIN)/basho-data snapshot verify

snapshot-stats: build ## Snapshot sizes and coverage (phase 3)
	$(BIN)/basho-data snapshot stats

calibrate: ## Calibrate the rating model (phase 5)
	$(todo)

## Release and docs

vhs: ## Render the README GIF (phase 12)
	$(todo)

release-snapshot: ## Local GoReleaser test build into dist/ (phase 13)
	$(todo)

portraits: ## Build pixel portraits, after 1.0 (phase 14)
	$(todo)
