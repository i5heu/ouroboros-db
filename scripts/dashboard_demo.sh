#!/bin/bash
# Dashboard Demo Script
# Starts 3 OuroborosDB nodes connected to each other
# Node 1: Dashboard + Upload enabled (port 8420)
# Node 2: Regular node
# Node 3: Regular node

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}  OuroborosDB Dashboard Demo${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""

# Build the daemon first
echo -e "${YELLOW}Building daemon...${NC}"
cd "$PROJECT_DIR"
go build -o bin/daemon ./cmd/daemon
echo -e "${GREEN}Build complete!${NC}"
echo ""

# Create data directories
echo -e "${YELLOW}Creating data directories...${NC}"
mkdir -p /tmp/ouroboros-demo/node1
mkdir -p /tmp/ouroboros-demo/node2
mkdir -p /tmp/ouroboros-demo/node3
echo -e "${GREEN}Data directories created!${NC}"
echo ""

# Cleanup function
cleanup() {
    echo ""
    echo -e "${YELLOW}Shutting down nodes...${NC}"
    
    if [ ! -z "$PID1" ]; then
        kill $PID1 2>/dev/null || true
    fi
    if [ ! -z "$PID2" ]; then
        kill $PID2 2>/dev/null || true
    fi
    if [ ! -z "$PID3" ]; then
        kill $PID3 2>/dev/null || true
    fi
    
    # Wait a moment for cleanup
    sleep 1
    
    echo -e "${GREEN}All nodes stopped.${NC}"
    echo ""
    echo -e "${YELLOW}Cleaning up data directories...${NC}"
    rm -rf /tmp/ouroboros-demo
    echo -e "${GREEN}Cleanup complete!${NC}"
    
    exit 0
}

# Set up trap for cleanup on exit
trap cleanup SIGINT SIGTERM EXIT

# Start Node 1 (with dashboard and upload)
echo -e "${BLUE}Starting Node 1 (Dashboard + Upload)...${NC}"
echo -e "  Data: /tmp/ouroboros-demo/node1"
echo -e "  Listen: :4242"
echo -e "  Dashboard: ${GREEN}http://localhost:8420${NC}"
echo -e "  Upload: ${GREEN}Enabled${NC}"
echo ""

$PROJECT_DIR/bin/daemon \
    --data /tmp/ouroboros-demo/node1 \
    --listen :4242 \
    --UNSECURE-dashboard \
    --UNSECURE-dashboard-port 8420 \
    --UNSECURE-upload-via-dashboard \
    --debug \
    2>&1 | sed 's/^/[Node1] /' &
PID1=$!

sleep 1

# Start Node 2
echo -e "${BLUE}Starting Node 2...${NC}"
echo -e "  Data: /tmp/ouroboros-demo/node2"
echo -e "  Listen: :4243"
echo -e "  Bootstrap: localhost:4242"
echo ""

$PROJECT_DIR/bin/daemon \
    --data /tmp/ouroboros-demo/node2 \
    --listen :4243 \
    --bootstrap localhost:4242 \
    --debug \
    2>&1 | sed 's/^/[Node2] /' &
PID2=$!

sleep 1

# Start Node 3
echo -e "${BLUE}Starting Node 3...${NC}"
echo -e "  Data: /tmp/ouroboros-demo/node3"
echo -e "  Listen: :4244"
echo -e "  Bootstrap: localhost:4242"
echo ""

$PROJECT_DIR/bin/daemon \
    --data /tmp/ouroboros-demo/node3 \
    --listen :4244 \
    --bootstrap localhost:4242 \
    --debug \
    2>&1 | sed 's/^/[Node3] /' &
PID3=$!

sleep 2

echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  All nodes started!${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
echo -e "Dashboard URL: ${BLUE}http://localhost:8420${NC}"
echo ""
echo -e "Press ${YELLOW}Ctrl+C${NC} to stop all nodes and cleanup."
echo ""
echo -e "${YELLOW}--- Node Logs ---${NC}"
echo ""

# Wait for all background processes
wait
