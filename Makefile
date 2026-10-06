# Makefile for kwatch
#
# `make verify` is the local gate. `make ci` is the exact gate the required
# CI job runs: verify plus the checks that need CI tooling (actionlint,
# ShellCheck) and module hygiene. Both run the test suite once, race-enabled
# with coverage, and the alert-quality gates run inside that same run; the
# wall-clock latency and memory budgets then run once more without -race.

.PHONY: help build test test-short vet lint verify-fmt clean \
	coverage coverage-check verify ci verify-focused verify-fast \
	verify-latency verify-race verify-security verify-manifests \
	verify-operational verify-scenarios verify-negative-regressions \
	verify-catalogs \
	docs-verify line-check diff-check mod-tidy-check architecture-check \
	test-layout-check lint-actions lint-shell alert-quality \
	alert-quality-gate docker-build docker-build-latest

# Binary names
BINARY_NAME := kwatch
CMD_DIR := cmd/kwatch

# Go parameters
GOCMD := go
GOBUILD := CGO_ENABLED=0 $(GOCMD) build
GOTEST := $(GOCMD) test
GOVET := $(GOCMD) vet
GOBIN_DIR := $(shell $(GOCMD) env GOPATH)/bin

# Build parameters
VERSION := $(shell \
	git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -ldflags \
	"-X github.com/abahmed/kwatch/internal/version.version=$(VERSION) \
	-X github.com/abahmed/kwatch/internal/version.gitCommitID=$(shell \
		git rev-parse --short HEAD 2>/dev/null || echo "none") \
	-X github.com/abahmed/kwatch/internal/version.buildDate=$(BUILD_TIME)"

# Output directory
OUTPUT_DIR := _output

# Ref that line-check and diff-check compare against. CI passes the pull
# request base; locally a missing ref falls back to the uncommitted diff.
BASE ?= origin/main
export BASE

# find_tool resolves a tool from PATH, then from the Go install directory.
find_tool = $(or $(shell command -v $(1) 2>/dev/null),$(wildcard \
	$(GOBIN_DIR)/$(1)))
GOLANGCI_LINT = $(call find_tool,golangci-lint)
ACTIONLINT = $(call find_tool,actionlint)
SHELLCHECK = $(call find_tool,shellcheck)
HELM = $(call find_tool,helm)

# tool_gate NAME PATH: a missing tool skips the check locally with a notice
# and fails under CI=true (set by GitHub Actions), so CI never skips a check.
tool_gate = if [ -z "$(2)" ]; then if [ "$${CI:-}" = true ]; then \
	echo "$(1) is required in CI" >&2; exit 1; fi; \
	echo "SKIP: $(1) is not installed (CI runs this check)"; exit 0; fi

# Alert-quality gates (docs/production-goals.md). The report lands here.
ALERT_QUALITY_REPORT ?= $(OUTPUT_DIR)/alert-quality.md
ALERT_QUALITY_ENV = KWATCH_SCORECARD_REPORT=$(abspath $(ALERT_QUALITY_REPORT))
ALERT_QUALITY_TEST := $(GOTEST) -count=1 -timeout 600s ./internal/scenarios \
	-run '^TestScorecardGates$$' -v

# Race tests are slower than the default per-package timeout allows for
# the scenario replay.
TEST_TIMEOUT ?= 30m

help:
	@echo "kwatch Makefile"
	@echo ""
	@echo "Gates:"
	@echo "  make verify        Local gate: build, vet, lint, race tests with"
	@echo "                     coverage and alert-quality gates, static checks,"
	@echo "                     manifests and negative regressions"
	@echo "  make ci            Everything the required CI job runs (verify plus"
	@echo "                     actionlint, ShellCheck and go mod tidy drift)"
	@echo "  make verify-fast PKGS=...    Package tests, vet and lint only"
	@echo "  make verify-focused PKGS=... Package gate plus cheap repo checks"
	@echo ""
	@echo "Individual checks:"
	@echo "  make build         Build the binary into $(OUTPUT_DIR)/"
	@echo "  make test          Run all tests"
	@echo "  make test-short    Run short tests only"
	@echo "  make vet           Run go vet, including the e2e build tag"
	@echo "  make lint          Run golangci-lint"
	@echo "  make verify-fmt    Verify gofmt formatting"
	@echo "  make coverage      Race tests with coverage into coverage.txt"
	@echo "  make coverage-check Coverage run plus threshold enforcement"
	@echo "  make verify-race   Serialized race run (-p 1) for milestones"
	@echo "  make verify-latency Decision-lag and memory budgets without -race"
	@echo "  make alert-quality Report the alert-quality gates (never fails)"
	@echo "  make alert-quality-gate Fail when any alert-quality gate misses"
	@echo "  make verify-catalogs Verify checked-in generated catalogs"
	@echo "  make docs-verify   Verify code-owned documentation metadata"
	@echo "  make verify-manifests Helm lint, chart tests, manifest parity"
	@echo "  make verify-negative-regressions Named safety regression tests"
	@echo "  make architecture-check Check package dependency boundaries"
	@echo "  make test-layout-check Check responsibility-based test filenames"
	@echo "  make line-check    80-column check on Go lines changed since BASE"
	@echo "  make diff-check    Whitespace check on changes since BASE"
	@echo "  make mod-tidy-check Fail when go mod tidy would change go.mod"
	@echo "  make lint-actions  Lint GitHub workflows (actionlint)"
	@echo "  make lint-shell    Lint shell scripts (ShellCheck)"
	@echo "  make verify-security Go dependency and optional image scan"
	@echo ""
	@echo "Clusters and images (need Docker and Kind):"
	@echo "  make verify-operational Disposable Kind production smoke test"
	@echo "  make verify-scenarios   Real-cluster regression scenarios"
	@echo "  make docker-build       Build the image tagged with the version"
	@echo "  make docker-build-latest Build the image tagged version and latest"
	@echo "  make clean         Remove build artifacts"

build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(OUTPUT_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(OUTPUT_DIR)/$(BINARY_NAME) ./$(CMD_DIR)

test:
	$(GOTEST) ./...

test-short:
	$(GOTEST) -short ./...

# The e2e suite only compiles under its build tag, so vet it explicitly.
vet:
	$(GOVET) ./...
	$(GOVET) -tags e2e ./test/...

lint:
	@test -n "$(GOLANGCI_LINT)" || { \
		echo "golangci-lint is required; install it with go install" >&2; \
		exit 1; \
	}
	$(GOLANGCI_LINT) run ./...

verify-fmt:
	@diff=$$(gofmt -l .); \
	if [ -n "$$diff" ]; then \
		echo "The following files are not formatted correctly:"; \
		echo "$$diff"; \
		exit 1; \
	fi
	@echo "All files are properly formatted."

# The single test run of the gate: race-enabled, with coverage, and with the
# alert-quality gates enforced inside it.
coverage:
	@mkdir -p $(dir $(ALERT_QUALITY_REPORT))
	KWATCH_SCORECARD_GATE=1 $(ALERT_QUALITY_ENV) \
		$(GOTEST) -race -timeout $(TEST_TIMEOUT) \
		--coverprofile=coverage.txt --covermode=atomic ./...

coverage-check: coverage
	./scripts/check-coverage.sh coverage.txt

verify: build vet lint coverage-check verify-latency line-check diff-check \
	architecture-check test-layout-check verify-catalogs docs-verify \
	verify-manifests verify-negative-regressions

ci: verify lint-actions lint-shell mod-tidy-check

# Package-scoped gate for incremental refactors, for example:
#   make verify-focused PKGS="./internal/incident ./internal/delivery/..."
verify-focused: verify-fast
	$(MAKE) line-check diff-check architecture-check test-layout-check

# Package tests, vet, and lint only, after a small local edit.
verify-fast:
	@test -n "$(PKGS)" || { \
		echo "PKGS is required; pass one or more Go package patterns"; \
		exit 2; \
	}
	$(GOTEST) $(PKGS)
	$(GOVET) $(PKGS)
	@test -n "$(GOLANGCI_LINT)" || { \
		echo "golangci-lint is required; install it with go install" >&2; \
		exit 1; \
	}
	$(GOLANGCI_LINT) run $(PKGS)

# Wall-clock budgets of docs/production-goals.md: the p99 decision lag at
# 5,000 pods and the memory budget. The race detector slows the engine
# several times over, so the lag test skips under -race and both run here
# once more without it, after the race run.
verify-latency:
	$(GOTEST) -count=1 \
		-run 'TestEngineDecisionLagAt5000Pods|TestMemoryBudget.*' \
		./internal/pipeline/ ./internal/app/

# Serialized race run for workstream or release milestones.
verify-race:
	$(GOTEST) -race -p 1 -timeout $(TEST_TIMEOUT) ./...

# A missing scanner fails this target, so a skipped scan never looks clean.
verify-security:
	./scripts/security-check.sh

# Chart lint and template tests need Helm; parity and release consistency
# are plain text checks. The Kind CRD lifecycle test runs in CI only.
verify-manifests:
	@$(call tool_gate,helm,$(HELM)); \
		$(HELM) lint deploy/chart && ./deploy/chart/test_helm.sh
	./scripts/check-manifest-parity.sh
	./scripts/check-release-consistency.sh
	@command -v kubectl >/dev/null 2>&1 || { \
		if [ "$${CI:-}" = true ]; then \
			echo "kubectl is required in CI" >&2; exit 1; fi; \
		echo "SKIP: kubectl is not installed (CI runs this check)"; \
		exit 0; }; \
		./scripts/test-require-kind-context.sh
	./scripts/test-check-e2e-artifacts.sh

verify-scenarios:
	./scripts/test-kind-scenarios.sh

# Fails when a named regression test is renamed or deleted.
verify-negative-regressions:
	./scripts/test-negative-regressions.sh

verify-operational:
	@for tool in kind kubectl helm docker; do \
		command -v $$tool > /dev/null || { \
			echo "$$tool is required; run this target in CI or a" \
				"disposable cluster" >&2; \
			exit 1; \
		}; \
	done
	./scripts/test-kind-production.sh

# Documentation published by kwatch.dev is sourced from the website
# repository. This target verifies the code-side contract and generated
# catalogs without pretending that the website is a second source of truth.
docs-verify: verify-catalogs
	./scripts/check-docs.sh
	$(GOCMD) run ./cmd/coveragedocs -check
	@test -s AGENTS.md
	@test -s CONTRIBUTING.md
	@grep -q "kwatch.dev/docs" AGENTS.md
	@grep -q "kwatch.dev/docs" CONTRIBUTING.md

verify-catalogs:
	@tmp=$$(mktemp -d); trap 'rm -rf "$$tmp"' EXIT; \
	$(GOCMD) run ./cmd/configcatalog -output "$$tmp/config.tsv" && \
	cmp deploy/config-catalog.tsv "$$tmp/config.tsv" && \
	$(GOCMD) run ./cmd/featurecatalog -output "$$tmp/feature.tsv" && \
	cmp deploy/feature-catalog.tsv "$$tmp/feature.tsv" && \
	$(GOCMD) run ./cmd/providercatalog -output "$$tmp/provider.tsv" && \
	cmp deploy/provider-catalog.tsv "$$tmp/provider.tsv"

# Go lines added since the merge base with BASE (committed, staged and
# unstaged) plus untracked Go files must fit in 80 columns.
line-check:
	@sh scripts/check-line-length.sh

# Whitespace errors in the same range line-check inspects.
diff-check:
	@if base=$$(git merge-base "$(BASE)" HEAD 2>/dev/null); then \
		git diff --check "$$base"; \
	else \
		echo "diff-check: $(BASE) not found; checking uncommitted changes"; \
		git diff --check HEAD; \
	fi

mod-tidy-check:
	$(GOCMD) mod tidy -diff

architecture-check:
	@sh scripts/check-architecture.sh
	@$(GOCMD) test ./internal/architecture

test-layout-check:
	@sh scripts/check-test-layout.sh

lint-actions:
	@$(call tool_gate,actionlint,$(ACTIONLINT)); $(ACTIONLINT)

lint-shell:
	@$(call tool_gate,shellcheck,$(SHELLCHECK)); \
		$(SHELLCHECK) scripts/*.sh deploy/chart/*.sh

# Replay the labelled and held-out scenarios, the 1,000-pod storms and the
# synthetic staging day, and report every gate. alert-quality only reports;
# alert-quality-gate fails on any miss. The gate also runs inside the
# coverage run of verify and ci. Results: internal/scenarios/SCORECARD.md.
alert-quality:
	@mkdir -p $(dir $(ALERT_QUALITY_REPORT))
	$(ALERT_QUALITY_ENV) $(ALERT_QUALITY_TEST)
	@echo "Alert-quality report: $(ALERT_QUALITY_REPORT)"

alert-quality-gate:
	@mkdir -p $(dir $(ALERT_QUALITY_REPORT))
	KWATCH_SCORECARD_GATE=1 $(ALERT_QUALITY_ENV) $(ALERT_QUALITY_TEST)

clean:
	@rm -rf $(OUTPUT_DIR)
	@rm -f coverage.out coverage.txt

docker-build:
	docker build -t kwatch:$(VERSION) .

docker-build-latest:
	docker build -t kwatch:latest -t kwatch:$(VERSION) .
