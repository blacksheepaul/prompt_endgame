.PHONY: help build test test-race test-cover run clean lint fmt vet ci

# Default target
help:
	@echo "Available targets:"
	@echo "  make build      - Build the application"
	@echo "  make test       - Run all tests"
	@echo "  make test-race  - Run tests with race detector"
	@echo "  make test-cover - Run tests with coverage report"
	@echo "  make run        - Run the server"
	@echo "  make clean      - Clean build artifacts"
	@echo "  make fmt        - Format code"
	@echo "  make vet        - Run go vet"
	@echo "  make lint       - Run golangci-lint (if installed)"
	@echo "  make ci         - Run full CI pipeline locally"

# Build the application
build:
	@echo "Building..."
	@go build -o bin/server ./cmd/server

# Run all tests
test:
	@echo "Running tests..."
	@go test ./... -v

# Run tests with race detector
test-race:
	@echo "Running tests with race detector..."
	@go test ./... -race -v

# Run tests with coverage
test-cover:
	@echo "Running tests with coverage..."
	@go test ./... -coverprofile=coverage.out
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Run the server
run:
	@echo "Starting server..."
	@go run ./cmd/server/main.go

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf bin/
	@rm -f coverage.out coverage.html
	@go clean

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...

# Run go vet
vet:
	@echo "Running go vet..."
	@go vet ./...

# Run golangci-lint (if installed)
lint:
	@echo "Running golangci-lint..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not installed. Install: https://golangci-lint.run/usage/install/" && exit 1)
	@golangci-lint run ./...

# Full CI pipeline
ci: fmt vet test-race
	@echo "✅ CI pipeline passed!"
