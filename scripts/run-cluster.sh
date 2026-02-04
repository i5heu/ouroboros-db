#!/usr/bin/env bash
#
# Starts a 3-node OuroborosDB cluster for development/testing.
# Node 1: Dashboard + Upload enabled (port 4242)
# Node 2: Connects to Node 1 (port 4243)
# Node 3: Connects to Node 1 (port 4244)
#
# Usage: ./scripts/run-cluster.sh
# Stop:  Ctrl+C (gracefully stops all nodes)

set -euo pipefail

# Colors for node prefixes
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Data directories
DATA_DIR="${DATA_DIR:-./data-cluster}"
NODE1_DATA="$DATA_DIR/node1"
NODE2_DATA="$DATA_DIR/node2"
NODE3_DATA="$DATA_DIR/node3"

# Ports
NODE1_PORT="4242"
NODE2_PORT="4243"
NODE3_PORT="4244"
DASHBOARD_PORT="8420"

# PIDs for cleanup
PIDS=()

# Cleanup function
cleanup() {
    echo ""
    echo "Stopping cluster..."

    # Send SIGTERM to all child processes
    for pid in "${PIDS[@]}"; do
        if kill -0 "$pid" 2>/dev/null; then
            echo "Stopping process $pid..."
            kill -TERM "$pid" 2>/dev/null || true
        fi
    done

    # Wait for processes to exit gracefully
    local timeout=5
    local waited=0
    while [ $waited -lt $timeout ]; do
        local all_dead=true
        for pid in "${PIDS[@]}"; do
            if kill -0 "$pid" 2>/dev/null; then
                all_dead=false
                break
            fi
        done
        if $all_dead; then
            break
        fi
        sleep 1
        ((waited++))
    done

    # Force kill any remaining processes
    for pid in "${PIDS[@]}"; do
        if kill -0 "$pid" 2>/dev/null; then
            echo "Force killing process $pid..."
            kill -9 "$pid" 2>/dev/null || true
        fi
    done

    echo "Cluster stopped."
    exit 0
}

# Set up signal handlers
trap cleanup SIGINT SIGTERM EXIT

# Create data directories
mkdir -p "$NODE1_DATA" "$NODE2_DATA" "$NODE3_DATA"

echo "========================================"
echo "Starting OuroborosDB Cluster"
echo "========================================"
echo ""
echo "Node 1: localhost:$NODE1_PORT (Dashboard: http://localhost:$DASHBOARD_PORT)"
echo "Node 2: localhost:$NODE2_PORT"
echo "Node 3: localhost:$NODE3_PORT"
echo ""
echo "Press Ctrl+C to stop all nodes"
echo "========================================"
echo ""

# Start Node 1 (with dashboard)
echo -e "${GREEN}[NODE1]${NC} Starting with dashboard..."
(
    go run ./cmd/daemon/main.go \
        -data "$NODE1_DATA" \
        -listen ":$NODE1_PORT" \
        -UNSECURE-dashboard \
        -UNSECURE-dashboard-port "$DASHBOARD_PORT" \
        -UNSECURE-upload-via-dashboard \
        -debug \
        2>&1 | while IFS= read -r line; do
            echo -e "${GREEN}[NODE1]${NC} $line"
        done
) &
PIDS+=($!)

# Wait for Node 1 to start
echo "Waiting for Node 1 to initialize..."
sleep 3

# Start Node 2 (connects to Node 1)
echo -e "${BLUE}[NODE2]${NC} Starting and connecting to Node 1..."
(
    go run ./cmd/daemon/main.go \
        -data "$NODE2_DATA" \
        -listen ":$NODE2_PORT" \
        -bootstrap "localhost:$NODE1_PORT" \
        -debug \
        2>&1 | while IFS= read -r line; do
            echo -e "${BLUE}[NODE2]${NC} $line"
        done
) &
PIDS+=($!)

# Start Node 3 (connects to Node 1)
echo -e "${RED}[NODE3]${NC} Starting and connecting to Node 1..."
(
    go run ./cmd/daemon/main.go \
        -data "$NODE3_DATA" \
        -listen ":$NODE3_PORT" \
        -bootstrap "localhost:$NODE1_PORT" \
        -debug \
        2>&1 | while IFS= read -r line; do
            echo -e "${RED}[NODE3]${NC} $line"
        done
) &
PIDS+=($!)

echo ""
echo "========================================"
echo "Cluster is running!"
echo "Dashboard: http://localhost:$DASHBOARD_PORT"
echo "========================================"
echo ""

# Wait for any process to exit
wait
