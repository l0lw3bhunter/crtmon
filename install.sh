#!/bin/bash

# crtmon VPS Installation Script
# This script automates the installation of crtmon on a VPS

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
GITHUB_REPO="${GITHUB_REPO:-}"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
CONFIG_DIR="${CONFIG_DIR:-/home/crtmon/.config/crtmon}"
SERVICE_USER="${SERVICE_USER:-crtmon}"
SERVICE_GROUP="${SERVICE_GROUP:-crtmon}"

# Helper functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[✓]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

check_requirements() {
    log_info "Checking system requirements..."
    
    # Check if running as root or with sudo
    if [[ $EUID -ne 0 ]]; then
        log_error "This script must be run as root (use: sudo ./install.sh)"
        exit 1
    fi
    
    # Check for Go
    if ! command -v go &> /dev/null; then
        log_error "Go is not installed. Please install Go 1.18+ first."
        echo "  Download from: https://golang.org/dl/"
        exit 1
    fi
    
    # Check Go version
    GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
    log_success "Go $GO_VERSION is installed"
    
    # Check for git
    if ! command -v git &> /dev/null; then
        log_error "Git is not installed. Please install git first."
        exit 1
    fi
    log_success "Git is installed"
    
    # Check for curl
    if ! command -v curl &> /dev/null; then
        log_warn "curl is not installed, but may be needed for some features"
    fi
}

setup_service_user() {
    log_info "Setting up service user '$SERVICE_USER'..."
    
    if id "$SERVICE_USER" &>/dev/null 2>&1; then
        log_warn "User '$SERVICE_USER' already exists"
    else
        useradd -r -s /bin/bash -d "/home/$SERVICE_USER" -m "$SERVICE_USER" || {
            log_error "Failed to create user '$SERVICE_USER'"
            exit 1
        }
        log_success "User '$SERVICE_USER' created"
    fi
}

clone_or_pull_repo() {
    log_info "Cloning/updating crtmon repository..."
    
    if [ -z "$GITHUB_REPO" ]; then
        log_error "GITHUB_REPO environment variable not set"
        echo "Usage: GITHUB_REPO=https://github.com/your-org/crtmon.git sudo ./install.sh"
        exit 1
    fi
    
    REPO_DIR="/opt/crtmon"
    
    if [ -d "$REPO_DIR" ]; then
        log_info "Repository already exists, pulling latest changes..."
        cd "$REPO_DIR"
        git pull origin main || {
            log_error "Failed to pull latest changes"
            exit 1
        }
    else
        log_info "Cloning repository from $GITHUB_REPO..."
        git clone "$GITHUB_REPO" "$REPO_DIR" || {
            log_error "Failed to clone repository"
            exit 1
        }
        cd "$REPO_DIR"
    fi
    
    log_success "Repository ready at $REPO_DIR"
    echo "$REPO_DIR"
}

build_binary() {
    local repo_dir=$1
    log_info "Building crtmon binary..."
    
    cd "$repo_dir"
    
    if ! go build -o crtmon; then
        log_error "Failed to build crtmon"
        exit 1
    fi
    
    log_success "Binary built successfully"
    echo "$repo_dir/crtmon"
}

install_binary() {
    local binary_path=$1
    log_info "Installing binary to $INSTALL_DIR..."
    
    # Backup existing binary if it exists
    if [ -f "$INSTALL_DIR/crtmon" ]; then
        cp "$INSTALL_DIR/crtmon" "$INSTALL_DIR/crtmon.backup"
        log_warn "Backed up existing binary to $INSTALL_DIR/crtmon.backup"
    fi
    
    install -D -m 755 "$binary_path" "$INSTALL_DIR/crtmon" || {
        log_error "Failed to install binary"
        exit 1
    }
    
    log_success "Binary installed to $INSTALL_DIR/crtmon"
}

setup_config_dir() {
    log_info "Setting up configuration directory..."
    
    mkdir -p "$CONFIG_DIR" || {
        log_error "Failed to create config directory"
        exit 1
    }
    
    chown -R "$SERVICE_USER:$SERVICE_GROUP" "/home/$SERVICE_USER" || {
        log_error "Failed to set directory ownership"
        exit 1
    }
    
    chmod 750 "$CONFIG_DIR"
    
    log_success "Config directory created at $CONFIG_DIR"
}

generate_config() {
    log_info "Generating initial configuration..."
    
    # Run crtmon once to generate template (ignore errors)
    sudo -u "$SERVICE_USER" "$INSTALL_DIR/crtmon" -config "$CONFIG_DIR/provider.yaml" 2>/dev/null || true
    
    if [ -f "$CONFIG_DIR/provider.yaml" ]; then
        log_success "Configuration template generated"
        log_warn "Please edit: $CONFIG_DIR/provider.yaml"
    else
        log_warn "Config template not found, you'll need to create it manually"
    fi
}

install_systemd_service() {
    local repo_dir=$1
    log_info "Installing systemd service..."
    
    if [ ! -f "$repo_dir/crtmon.service" ]; then
        log_error "crtmon.service file not found in repository"
        exit 1
    fi
    
    # Backup existing service if it exists
    if [ -f "/etc/systemd/system/crtmon.service" ]; then
        cp "/etc/systemd/system/crtmon.service" "/etc/systemd/system/crtmon.service.backup"
        log_warn "Backed up existing service to crtmon.service.backup"
    fi
    
    cp "$repo_dir/crtmon.service" "/etc/systemd/system/crtmon.service" || {
        log_error "Failed to install systemd service"
        exit 1
    }
    
    systemctl daemon-reload || {
        log_error "Failed to reload systemd"
        exit 1
    }
    
    log_success "Systemd service installed and reloaded"
}

install_logrotate() {
    log_info "Setting up log rotation..."
    
    cat > /etc/logrotate.d/crtmon << 'EOF'
/var/log/crtmon/*.log {
    daily
    rotate 14
    compress
    delaycompress
    notifempty
    create 0640 crtmon crtmon
    sharedscripts
    postrotate
        systemctl reload crtmon > /dev/null 2>&1 || true
    endscript
}
EOF
    
    log_success "Logrotate configuration installed"
}

print_summary() {
    local repo_dir=$1
    
    echo ""
    echo -e "${GREEN}╔════════════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║${NC}     crtmon Installation Complete                      ${GREEN}║${NC}"
    echo -e "${GREEN}╚════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo "Installation Summary:"
    echo "  Binary Location:   $INSTALL_DIR/crtmon"
    echo "  Config Directory:  $CONFIG_DIR"
    echo "  Repository:        $repo_dir"
    echo "  Service User:      $SERVICE_USER"
    echo "  Service Status:    Installed"
    echo ""
    echo "Next Steps:"
    echo ""
    echo "  1. Edit the configuration file:"
    echo "     nano $CONFIG_DIR/provider.yaml"
    echo ""
    echo "  2. Start the service:"
    echo "     systemctl start crtmon"
    echo ""
    echo "  3. Enable auto-start on boot:"
    echo "     systemctl enable crtmon"
    echo ""
    echo "  4. Check service status:"
    echo "     systemctl status crtmon"
    echo ""
    echo "  5. View logs:"
    echo "     journalctl -u crtmon -f"
    echo ""
    echo "Useful Commands:"
    echo "  Start:    systemctl start crtmon"
    echo "  Stop:     systemctl stop crtmon"
    echo "  Restart:  systemctl restart crtmon"
    echo "  Logs:     journalctl -u crtmon -f"
    echo "  Status:   systemctl status crtmon"
    echo ""
    echo "For updates, run from the repository:"
    echo "  cd $repo_dir"
    echo "  make update"
    echo ""
}

# Main installation flow
main() {
    echo -e "${BLUE}"
    echo "╔════════════════════════════════════════════════════════╗"
    echo "║       crtmon VPS Installation Script                   ║"
    echo "║     Certificate Transparency Monitor                   ║"
    echo "╚════════════════════════════════════════════════════════╝"
    echo -e "${NC}"
    echo ""
    
    check_requirements
    setup_service_user
    
    # Get repo path
    repo_dir=$(clone_or_pull_repo)
    
    # Build and install
    binary_path=$(build_binary "$repo_dir")
    install_binary "$binary_path"
    setup_config_dir
    generate_config
    install_systemd_service "$repo_dir"
    install_logrotate
    
    # Print summary
    print_summary "$repo_dir"
}

# Run main function
main "$@"
