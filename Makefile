.PHONY: all build build-linux test run clean fmt ci help

# Binary name
BINARY_NAME=server
BUILD_DIR=bin

# Default target
all: build

# Build for current platform
build:
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/server

# Build for Docker (Linux AMD64 and ARM64)
build-linux:
	@mkdir -p $(BUILD_DIR)/linux
	GOOS=linux GOARCH=amd64 go build -o $(BUILD_DIR)/linux/$(BINARY_NAME)-amd64 ./cmd/server

# Run tests
test:
	go test ./...

run: build-linux
	docker compose down
	docker compose up -d --build

# Clean build artifacts
clean:
	rm -rf $(BUILD_DIR)/
	go clean

# Format code
fmt:
	go fmt ./...

# CI pipeline: format + test
ci: fmt test

# Show help
help:
	@echo "Available targets:"
	@echo "  build       - Build the server binary"
	@echo "  build-linux - Build Linux binaries for Docker (amd64)"
	@echo "  test        - Run all tests"
	@echo "  run         - Run the server"
	@echo "  clean       - Clean build artifacts"
	@echo "  fmt         - Format Go code"
	@echo "  ci          - Run CI checks (fmt + test)"
