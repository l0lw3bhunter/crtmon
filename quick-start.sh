#!/bin/bash

# Quick start script for crtmon local development
# Usage: ./quick-start.sh [config-path]

set -e

CONFIG_PATH="${1:-$HOME/.config/crtmon/provider.yaml}"
BINARY_NAME="crtmon"

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${BLUE}╔════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║       crtmon Quick Start                               ║${NC}"
echo -e "${BLUE}╚════════════════════════════════════════════════════════╝${NC}"
echo ""

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo -e "${YELLOW}[WARN]${NC} Go is not installed"
    exit 1
fi

echo -e "${BLUE}[1/3]${NC} Building crtmon..."
if go build -o $BINARY_NAME; then
    echo -e "${GREEN}[✓]${NC} Build successful"
else
    echo -e "${YELLOW}[ERROR]${NC} Build failed"
    exit 1
fi

echo ""
echo -e "${BLUE}[2/3]${NC} Checking configuration..."

if [ ! -f "$CONFIG_PATH" ]; then
    echo -e "${YELLOW}[WARN]${NC} Config not found at $CONFIG_PATH"
    echo -e "${YELLOW}[WARN]${NC} Generating template..."
    
    # Create config directory
    mkdir -p "$(dirname "$CONFIG_PATH")"
    
    # Run once to generate template
    ./$BINARY_NAME -config "$CONFIG_PATH" 2>/dev/null || true
    
    if [ -f "$CONFIG_PATH" ]; then
        echo -e "${GREEN}[✓]${NC} Config template created"
        echo ""
        echo "Edit the configuration file:"
        echo "  nano $CONFIG_PATH"
        echo ""
        exit 0
    fi
fi

echo -e "${GREEN}[✓]${NC} Config found at $CONFIG_PATH"

echo ""
echo -e "${BLUE}[3/3]${NC} Starting crtmon..."
echo ""
echo "Configuration: $CONFIG_PATH"
echo "Targets: $(grep -A5 'targets:' "$CONFIG_PATH" | grep '- ' | wc -l) configured"
echo ""
echo -e "${GREEN}Press Ctrl+C to stop${NC}"
echo ""

# Run crtmon
./$BINARY_NAME -config "$CONFIG_PATH"
