.PHONY: build install install-vps update clean help

# Variables
BINARY_NAME=crtmon
VERSION=$(shell cat version.go | grep 'const version' | awk -F'"' '{print $$2}')
BUILD_TIME=$(shell date -u '+%Y-%m-%d %H:%M:%S UTC')
INSTALL_DIR?=/usr/local/bin
CONFIG_DIR?=$(HOME)/.config/crtmon
SERVICE_NAME=crtmon

help:
	@echo "crtmon Makefile targets:"
	@echo ""
	@echo "  make build          - Build crtmon binary"
	@echo "  make install        - Install crtmon locally (requires sudo)"
	@echo "  make install-vps    - Install crtmon on VPS with systemd service"
	@echo "  make update         - Update crtmon binary (git pull + rebuild)"
	@echo "  make service-start  - Start the systemd service"
	@echo "  make service-stop   - Stop the systemd service"
	@echo "  make service-status - Check service status"
	@echo "  make service-logs   - View service logs"
	@echo "  make clean          - Remove binary and temporary files"
	@echo "  make version        - Show version"
	@echo ""

version:
	@echo "crtmon version: $(VERSION)"

build:
	@echo "Building crtmon..."
	go build -o $(BINARY_NAME)
	@echo "✓ Build successful: ./$(BINARY_NAME)"

install: build
	@echo "Installing crtmon to $(INSTALL_DIR)..."
	install -D -m 755 $(BINARY_NAME) $(INSTALL_DIR)/$(BINARY_NAME)
	@echo "✓ Installation successful"
	@echo ""
	@echo "First run configuration:"
	@echo "  $(BINARY_NAME) -config ~/.config/crtmon/provider.yaml"
	@echo ""
	@echo "Edit the generated config file:"
	@echo "  nano ~/.config/crtmon/provider.yaml"

install-vps: build
	@echo "Installing crtmon on VPS..."
	@echo ""
	@echo "Step 1: Installing binary to $(INSTALL_DIR)..."
	sudo install -D -m 755 $(BINARY_NAME) $(INSTALL_DIR)/$(BINARY_NAME)
	@echo "✓ Binary installed"
	@echo ""
	@echo "Step 2: Creating config directory..."
	sudo mkdir -p $(CONFIG_DIR)
	sudo chown $(USER):$(USER) $(CONFIG_DIR)
	@echo "✓ Config directory created"
	@echo ""
	@echo "Step 3: Generating initial config..."
	$(INSTALL_DIR)/$(BINARY_NAME) -config $(CONFIG_DIR)/provider.yaml || true
	@echo "✓ Config template generated"
	@echo ""
	@echo "Step 4: Installing systemd service..."
	sudo cp crtmon.service /etc/systemd/system/
	sudo systemctl daemon-reload
	@echo "✓ Systemd service installed"
	@echo ""
	@echo "Next steps:"
	@echo "  1. Edit configuration:"
	@echo "     nano $(CONFIG_DIR)/provider.yaml"
	@echo ""
	@echo "  2. Start the service:"
	@echo "     make service-start"
	@echo ""
	@echo "  3. Check status:"
	@echo "     make service-status"
	@echo ""
	@echo "  4. View logs:"
	@echo "     make service-logs"

update:
	@echo "Updating crtmon..."
	git pull origin main
	@echo "✓ Repository updated"
	@$(MAKE) build
	@echo ""
	@echo "Update complete!"
	@echo "If running as systemd service, restart with:"
	@echo "  sudo systemctl restart $(SERVICE_NAME)"

service-start:
	@echo "Starting crtmon service..."
	sudo systemctl start $(SERVICE_NAME)
	@echo "✓ Service started"

service-stop:
	@echo "Stopping crtmon service..."
	sudo systemctl stop $(SERVICE_NAME)
	@echo "✓ Service stopped"

service-status:
	@echo "crtmon service status:"
	sudo systemctl status $(SERVICE_NAME) --no-pager

service-logs:
	@echo "crtmon service logs (last 50 lines):"
	sudo journalctl -u $(SERVICE_NAME) -n 50 --no-pager

service-logs-follow:
	@echo "Following crtmon service logs..."
	sudo journalctl -u $(SERVICE_NAME) -f

clean:
	@echo "Cleaning up..."
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_NAME).backup
	@echo "✓ Cleanup complete"
