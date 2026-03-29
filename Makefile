# Makefile for kwatch
# Following Kubernetes community conventions

.PHONY: build test test-short lint vet clean verify-fmt verify-unit verify-all help

# Binary names
BINARY_NAME := kwatch
CMD_DIR := cmd/kwatch

# Go parameters
GOCMD := go
GOBUILD := CGO_ENABLED=0 $(GOCMD) build
GOTEST := $(GOCMD) test
GOVET := $(GOCMD) vet
GOFMT := $(GOCMD) fmt

# Build parameters
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -ldflags "-X github.com/abahmed/kwatch/internal/version.version=$(VERSION) -X github.com/abahmed/kwatch/internal/version.gitCommitID=$(shell git rev-parse --short HEAD 2>/dev/null || echo "none") -X github.com/abahmed/kwatch/internal/version.buildDate=$(BUILD_TIME)"

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
	@echo "  make verify-fmt    Verify code formatting"
	@echo "  make verify-unit   Run unit tests"
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
	@which golangci-lint > /dev/null || (echo "golangci-lint not found. Install: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest" && exit 1)
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

# Run all verification scripts
verify-all: verify-fmt vet verify-unit

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
