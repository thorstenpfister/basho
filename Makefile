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

clean: ## Remove bin/, dist/, completions/, manpages/ and coverage files (never data/ or the work cache)
	rm -rf $(BIN) dist completions manpages coverage.out coverage.html

## Test

test: ## Run all tests
	$(GO) test ./...

test-race: ## Run all tests with the race detector
	$(GO) test -race ./...

test-leaks: ## The screen walks (spoiler leaks, --ascii, ui.names), which skip under the race detector
	$(GO) test -run 'TestNoLeaks|TestWalkDay13|TestASCIIScreens|TestNameModes' ./internal/tui/

test-live: ## Real API smoke tests, manual only (phase 1)
	$(todo)

golden: ## Rewrite golden files with -update; review the diff (phase 6)
	$(todo)

bench: ## The performance budgets: first screen under 300 ms, grid frame under 16 ms
	$(GO) test -run '^$$' -bench 'BenchmarkStart|BenchmarkGrid' -benchmem ./internal/tui/...

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

check: fmt-check lint vuln test-race test-leaks ## All checks; run before committing

tools: ## Check that external tools are installed
	@command -v golangci-lint >/dev/null || { echo "missing golangci-lint: brew install golangci-lint"; exit 1; }
	@command -v goreleaser >/dev/null || { echo "missing goreleaser: brew install goreleaser"; exit 1; }
	@command -v gh >/dev/null || { echo "missing gh (for make release): brew install gh"; exit 1; }
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

calibrate: build ## Calibrate the rating model (no network): data/rating/params.json, docs/rating.md
	bin/basho-data rating calibrate

## Release and docs

vhs: build ## Render the README GIF from docs/demo.tape (needs VHS: brew install vhs; without Chrome, VHS downloads a headless Chromium to ~/.cache/rod)
	@command -v vhs >/dev/null || { echo "make vhs: needs vhs (brew install vhs)" >&2; exit 2; }
	vhs docs/demo.tape

release-snapshot: ## Local GoReleaser test build into dist/ (archives, completions, man page, checksums)
	goreleaser release --snapshot --clean

release: ## Publish the tag at HEAD: GitHub release and Homebrew cask (SPEC §14.3, docs/releasing.md)
	@tag=$$(git describe --tags --exact-match HEAD 2>/dev/null || echo "?"); printf 'Publishing %s to GitHub and committing Casks/basho.rb to main. ' "$$tag"
	$(call confirm,)
	./scripts/release.sh

portraits: build ## Fetch and build the pixel portraits, ARGS passed on (SPEC §8.9; CONTACT=you@example.org for the User-Agent)
	$(BIN)/basho-data portraits update --dry-run $(ARGS)
	$(call confirm,basho-data portraits update fetches the images above from Wikimedia at 1 request per second.)
	$(BIN)/basho-data portraits update $(if $(CONTACT),--contact "$(CONTACT)") $(ARGS)
	$(MAKE) build # bin/basho embeds data/portraits: build it again with the new set

portraits-preview: build ## Rebuild only the wrestlers in overrides.toml: .cache/basho-data/portraits-preview/sheet.png
	$(BIN)/basho-data portraits update --overrides-only --work .cache/basho-data/portraits-preview $(if $(CONTACT),--contact "$(CONTACT)") $(ARGS)
