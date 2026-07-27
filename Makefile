.PHONY: all build run run-oneshot test test-coverage lint fmt tidy clean help

# Binary configuration
BINARY_NAME=bsal-engine
BUILD_DIR=bin
MAIN_PATH=main.go

# Default target
all: tidy fmt lint test build

## build: Build the Go binary
build:
	@echo "==> Building binary..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "==> Build successful: $(BUILD_DIR)/$(BINARY_NAME)"

## run: Run engine in continuous daemon mode
run:
	go run $(MAIN_PATH) -mode=daemon

## run-oneshot: Run engine in single-pass oneshot mode
run-oneshot:
	go run $(MAIN_PATH) -mode=oneshot

## test: Run unit tests
test:
	@echo "==> Running unit tests..."
	go test -v ./...

## test-coverage: Run unit tests with code coverage report
test-coverage:
	@echo "==> Running unit tests with coverage..."
	go test -v -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -func=coverage.out

## lint: Run golangci-lint (uses local binary or fallback to go run)
lint:
	@echo "==> Running golangci-lint..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "[INFO] golangci-lint not found in PATH, running via go run..."; \
		go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run ./...; \
	fi

## fmt: Format Go source files
fmt:
	@echo "==> Formatting code..."
	go fmt ./...

## tidy: Tidy Go module dependencies
tidy:
	@echo "==> Tidying dependencies..."
	go mod tidy

## clean: Remove build artifacts and test coverage profiles
clean:
	@echo "==> Cleaning build artifacts..."
	rm -rf $(BUILD_DIR) coverage.out

## help: Display available Makefile targets
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Available targets:"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':'
