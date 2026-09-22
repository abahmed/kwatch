# Makefile for kwatch
# Following Kubernetes community conventions

.PHONY: build test test-short lint vet clean verify verify-fmt verify-unit \
	verify-all verify-catalogs docs-verify line-check docker-build \
	docker-build-latest architecture-check test-layout-check help \
	verify-focused verify-fast verify-race verify-security \
	verify-manifests verify-docs verify-operational coverage coverage-check

.PHONY: verify-scenarios verify-scenario verify-kind-extended-scenarios

# Binary names
BINARY_NAME := kwatch
CMD_DIR := cmd/kwatch

# Go parameters
GOCMD := go
GOBUILD := CGO_ENABLED=0 $(GOCMD) build
GOTEST := $(GOCMD) test
GOVET := $(GOCMD) vet

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

# Default target
help:
	@echo "kwatch Makefile"
	@echo ""
	@echo "Usage:"
	@echo "  make build         Build the binary"
	@echo "  make test          Run all tests"
	@echo "  make test-short    Run short tests only"
	@echo "  make vet           Run go vet"
	@echo "  make lint          Run linting (requires golangci-lint)"
	@echo "  make verify        Run the complete required validation gate"
	@echo "  make verify-race   Run the serialized race-test gate"
	@echo "  make verify-security Run dependency and image security checks"
	@echo "  make verify-manifests Validate Helm and Kubernetes manifests"
	@echo "  make verify-operational Run the disposable Kind production smoke test"
	@echo "  make verify-scenarios  Run real-cluster regression scenarios"
	@echo "  make verify-scenario   Run selected real-cluster scenarios"
	@echo "  make verify-kind-extended-scenarios Run extended Kind scenarios"
	@echo "  make verify-docs   Validate code-owned documentation metadata"
	@echo "  make verify-focused PKGS=... Validate only changed package groups"
	@echo "  make verify-catalogs Verify checked-in generated catalogs"
	@echo "  make docs-verify    Verify code-owned documentation metadata"
	@echo "  make architecture-check Check package dependency boundaries"
	@echo "  make test-layout-check Check responsibility-based test filenames"
	@echo "  make line-check    Check new Go lines for an 80-column maximum"
	@echo "  make verify-fmt    Verify code formatting"
	@echo "  make verify-unit   Run unit tests"
	@echo "  make coverage      Run race-enabled tests and write coverage.txt"
	@echo "  make coverage-check Run tests and enforce coverage thresholds"
	@echo "  make verify-all    Run all verification scripts"
	@echo "  make clean         Clean build artifacts"
	@echo ""

# Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(OUTPUT_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(OUTPUT_DIR)/$(BINARY_NAME) ./$(CMD_DIR)

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Run short tests
test-short:
	@echo "Running short tests..."
	$(GOTEST) -short ./...

# Run go vet
vet:
	@echo "Running go vet..."
	$(GOVET) ./...

# Run linting
lint:
	@echo "Running golangci-lint..."
	@command -v golangci-lint > /dev/null || { \
		echo "golangci-lint is required; install it with go install"; \
		exit 1; \
	}
	golangci-lint run ./...

# Verify code formatting
verify-fmt:
	@echo "Verifying code formatting..."
	@diff=$$(gofmt -l .); \
	if [ -n "$$diff" ]; then \
		echo "The following files are not formatted correctly:"; \
		echo "$$diff"; \
		exit 1; \
	fi
	@echo "All files are properly formatted."

# Run unit tests
verify-unit:
	@echo "Running unit tests..."
	$(GOTEST) -short ./...

coverage:
	@echo "Running race-enabled tests with coverage..."
	$(GOTEST) -race --coverprofile=coverage.txt \
		--covermode=atomic ./...

coverage-check: coverage
	./scripts/check-coverage.sh coverage.txt

# Run the repository's required validation gate. Keep this in the same order
# as AGENTS.md so local verification and CI verification cannot drift.
verify: build vet test lint coverage-check line-check architecture-check \
	test-layout-check \
	verify-catalogs \
	docs-verify
	@git diff --check

# Run the fast package-scoped gate during incremental refactors. The caller
# must provide Go package patterns, for example:
#   make verify-focused PKGS="./internal/incident ./internal/delivery/..."
verify-focused:
	@test -n "$(PKGS)" || { \
		echo "PKGS is required; pass one or more Go package patterns"; \
		exit 2; \
	}
	$(GOTEST) -run '^$$' $(PKGS)
	$(GOTEST) $(PKGS)
	$(GOVET) $(PKGS)
	@command -v golangci-lint > /dev/null || { \
		echo "golangci-lint is required; install it with go install"; \
		exit 1; \
	}
	golangci-lint run $(PKGS)
	$(MAKE) line-check architecture-check test-layout-check
	@git diff --check

# Run only package tests, vet, and lint after a small local edit. The focused
# workstream target additionally runs repository-wide cheap checks.
verify-fast:
	@test -n "$(PKGS)" || { \
		echo "PKGS is required; pass one or more Go package patterns"; \
		exit 2; \
	}
	$(GOTEST) $(PKGS)
	$(GOVET) $(PKGS)
	@command -v golangci-lint > /dev/null || { \
		echo "golangci-lint is required; install it with go install"; \
		exit 1; \
	}
	golangci-lint run $(PKGS)

# Run the expensive race suite explicitly at a workstream or release
# milestone instead of making every focused edit pay its cost.
verify-race:
	$(GOTEST) -race -p 1 ./...

# Security tooling is intentionally explicit. A missing scanner fails the
# target so release automation cannot accidentally report an unscanned build.
verify-security:
	./scripts/security-check.sh

# Validate the chart without creating a cluster. The lifecycle test remains a
# CI/operational check because it requires kind and Docker.
verify-manifests:
	@command -v helm > /dev/null || { \
		echo "helm is required; install it before running verify-manifests"; \
		exit 1; \
	}
	helm lint deploy/chart
	./deploy/chart/test_helm.sh
	./scripts/check-manifest-parity.sh
	./scripts/check-release-consistency.sh

verify-scenarios:
	./scripts/test-kind-scenarios.sh

verify-scenario:
	./scripts/test-kind-scenarios.sh

verify-kind-extended-scenarios:
	./scripts/test-kind-extended-scenarios.sh

verify-installer:
	./scripts/test-kwatch-installer.sh

verify-operational:
	@command -v kind > /dev/null || { \
		echo "kind is required; run this target in CI or a disposable cluster"; \
		exit 1; \
	}
	@command -v kubectl > /dev/null || { \
		echo "kubectl is required; run this target in CI or a disposable cluster"; \
		exit 1; \
	}
	@command -v helm > /dev/null || { \
		echo "helm is required; run it in CI or a disposable cluster"; \
		exit 1; \
	}
	@command -v docker > /dev/null || { \
		echo "docker is required; run it in CI or a disposable cluster"; \
		exit 1; \
	}
	./scripts/test-kind-production.sh

verify-docs: docs-verify

# Documentation published by kwatch.dev is sourced from the website
# repository. This target verifies the code-side contract and generated
# catalogs without pretending that the website is a second source of truth.
docs-verify: verify-catalogs
	./scripts/check-docs.sh
	@test -s AGENTS.md
	@test -s CONTRIBUTING.md
	@grep -q "kwatch.dev/docs" AGENTS.md
	@grep -q "kwatch.dev/docs" CONTRIBUTING.md

verify-catalogs:
	@tmp=$$(mktemp -d); trap 'rm -rf "$$tmp"' EXIT; \
	$(GOCMD) run ./cmd/configcatalog -output "$$tmp/config.tsv"; \
	cmp deploy/config-catalog.tsv "$$tmp/config.tsv"; \
	$(GOCMD) run ./cmd/featurecatalog -output "$$tmp/feature.tsv"; \
	cmp deploy/feature-catalog.tsv "$$tmp/feature.tsv"; \
	$(GOCMD) run ./cmd/providercatalog -output "$$tmp/provider.tsv"; \
	cmp deploy/provider-catalog.tsv "$$tmp/provider.tsv"

# Backward-compatible alias retained for contributors using the old target.
verify-all: verify

# Check new and intentionally refactored Go lines without rewriting legacy
# generated code or unrelated provider payload literals.
line-check:
	@sh scripts/check-line-length.sh

# Check package direction and keep transitional implementation imports from
# spreading back into new production code.
architecture-check:
	@sh scripts/check-architecture.sh
	@$(GOCMD) test ./internal/architecture

# Keep test files discoverable by behavior instead of edit-history fragments.
test-layout-check:
	@sh scripts/check-test-layout.sh

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf $(OUTPUT_DIR)
	@rm -f coverage.out coverage.txt
	@echo "Clean complete."

# Docker build
docker-build:
	@echo "Building Docker image..."
	docker build -t kwatch:$(VERSION) .

# Docker build with latest tag
docker-build-latest:
	docker build -t kwatch:latest -t kwatch:$(VERSION) .
