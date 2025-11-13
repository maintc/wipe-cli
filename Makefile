.PHONY: build install clean test fmt vet lint deadcode check

# Build directories
BUILD_DIR := build
BIN_DIR := /usr/local/bin
SYSTEMD_DIR := /etc/systemd/system

# Binary names
CLI_BIN := wipe
DAEMON_BIN := wiped

# Build both binaries
build: check
	@echo "Building binaries..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(CLI_BIN) ./cmd/wipe
	go build -o $(BUILD_DIR)/$(DAEMON_BIN) ./cmd/wiped
	@echo "Build complete: $(BUILD_DIR)/$(CLI_BIN), $(BUILD_DIR)/$(DAEMON_BIN)"

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...
	@echo "✓ Format complete"

# Run go vet
vet:
	@echo "Running go vet..."
	@go vet ./...
	@echo "✓ Vet complete"

# Run staticcheck (if available)
lint:
	@echo "Running staticcheck..."
	@if command -v staticcheck >/dev/null 2>&1; then \
		staticcheck ./...; \
		echo "✓ Lint complete"; \
	else \
		echo "⚠ staticcheck not installed (install with: go install honnef.co/go/tools/cmd/staticcheck@latest)"; \
	fi

# Check for dead code (if available)
deadcode:
	@echo "Checking for dead code..."
	@if command -v deadcode >/dev/null 2>&1; then \
		deadcode -test ./...; \
		echo "✓ Deadcode check complete"; \
	else \
		echo "⚠ deadcode not installed (install with: go install golang.org/x/tools/cmd/deadcode@latest)"; \
	fi

# Run all checks
check: fmt vet lint deadcode
	@echo ""
	@echo "✓ All checks passed!"

# Install binaries and systemd service
install: check build
	@echo "Installing binaries..."
	sudo install -m 755 $(BUILD_DIR)/$(CLI_BIN) $(BIN_DIR)/
	sudo install -m 755 $(BUILD_DIR)/$(DAEMON_BIN) $(BIN_DIR)/
	@echo "Installing systemd service..."
	sudo install -m 644 systemd/wiped.service $(SYSTEMD_DIR)/wiped@.service
	sudo systemctl daemon-reload
	@echo "Installation complete!"
	@echo ""
	@echo "To enable and start the service for your user, run:"
	@echo "  sudo systemctl enable wiped@$$USER.service"
	@echo "  sudo systemctl start wiped@$$USER.service"

# Uninstall binaries and systemd service
uninstall:
	@echo "Stopping and disabling service..."
	-sudo systemctl stop wiped@$$USER.service
	-sudo systemctl disable wiped@$$USER.service
	@echo "Removing binaries..."
	sudo rm -f $(BIN_DIR)/$(CLI_BIN)
	sudo rm -f $(BIN_DIR)/$(DAEMON_BIN)
	@echo "Removing systemd service..."
	sudo rm -f $(SYSTEMD_DIR)/wiped@.service
	sudo systemctl daemon-reload
	@echo "Uninstall complete!"

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)
	@echo "Clean complete!"

# Run tests
test:
	go test ./...

# Development: build and run CLI
run-cli: build
	$(BUILD_DIR)/$(CLI_BIN)

# Development: build and run daemon
run-daemon: build
	$(BUILD_DIR)/$(DAEMON_BIN)

