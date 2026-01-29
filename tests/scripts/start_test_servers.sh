#!/bin/bash
# start_test_servers.sh - Start zs3 (S3) and sweb (WebDAV) test servers

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
ZS3_BIN="$PROJECT_ROOT/tools/zs3"
SWEB_BIN="$PROJECT_ROOT/tools/sweb"

# Test server directories
ZS3_DATA_DIR="$PROJECT_ROOT/data/test-s3"
SWEB_DATA_DIR="$PROJECT_ROOT/data/test-webdav"

# PIDs
ZS3_PID=""
SWEB_PID=""

# Log files
ZS3_LOG="$PROJECT_ROOT/tests/testdata/zs3.log"
SWEB_LOG="$PROJECT_ROOT/tests/testdata/sweb.log"

# Cleanup function
cleanup() {
    echo "Cleaning up..."
    if [ -n "$ZS3_PID" ]; then
        echo "Stopping zs3 (PID: $ZS3_PID)"
        kill "$ZS3_PID" 2>/dev/null || true
    fi
    if [ -n "$SWEB_PID" ]; then
        echo "Stopping sweb (PID: $SWEB_PID)"
        kill "$SWEB_PID" 2>/dev/null || true
    fi
    exit 0
}

trap cleanup EXIT INT TERM

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}=== Starting Test Servers ===${NC}"

# Check if binaries exist
if [ ! -f "$ZS3_BIN" ]; then
    echo -e "${YELLOW}Warning: zs3 not found at $ZS3_BIN${NC}"
    echo "Run: ./scripts/install_zs3.sh"
    echo "Using system zs3 if available..."
    ZS3_BIN="zs3"
fi

if [ ! -f "$SWEB_BIN" ]; then
    echo -e "${YELLOW}Warning: sweb not found at $SWEB_BIN${NC}"
    echo "Run: ./scripts/install_sweb.sh"
    echo "Using system sweb if available..."
    SWEB_BIN="sweb"
fi

# Create data directories
mkdir -p "$ZS3_DATA_DIR"
mkdir -p "$SWEB_DATA_DIR"
mkdir -p "$(dirname "$ZS3_LOG")"
mkdir -p "$(dirname "$SWEB_LOG")"

# Clean up old data (optional)
# rm -rf "$ZS3_DATA_DIR"/*
# rm -rf "$SWEB_DATA_DIR"/*

# Start zs3 (S3-compatible server)
echo "Starting zs3 S3 server on port 9000..."
echo "Data directory: $ZS3_DATA_DIR"
echo "Log file: $ZS3_LOG"

if [ -f "$ZS3_BIN" ]; then
    "$ZS3_BIN" \
        --port 9000 \
        --dir "$ZS3_DATA_DIR" \
        --access-key minioadmin \
        --secret-key minioadmin \
        > "$ZS3_LOG" 2>&1 &
else
    # Try system zs3 with different parameters
    zs3 serve \
        --port 9000 \
        --data "$ZS3_DATA_DIR" \
        --access-key minioadmin \
        --secret-key minioadmin \
        > "$ZS3_LOG" 2>&1 &
fi

ZS3_PID=$!
echo "zs3 started with PID: $ZS3_PID"

# Wait for zs3 to start
echo "Waiting for zs3 to be ready..."
for i in {1..30}; do
    if curl -s http://localhost:9000/minio/health/live > /dev/null 2>&1 || \
       curl -s http://localhost:9000/ > /dev/null 2>&1; then
        echo -e "${GREEN}zs3 is ready!${NC}"
        break
    fi
    if [ $i -eq 30 ]; then
        echo -e "${YELLOW}Warning: zs3 may not be ready (timeout)${NC}"
    fi
    sleep 1
done

# Start sweb (WebDAV server)
echo ""
echo "Starting sweb WebDAV server on port 8081..."
echo "Data directory: $SWEB_DATA_DIR"
echo "Log file: $SWEB_LOG"

if [ -f "$SWEB_BIN" ]; then
    "$SWEB_BIN" \
        --port 8081 \
        --dir "$SWEB_DATA_DIR" \
        --webdav-path "/webdav" \
        --auth admin:admin123 \
        > "$SWEB_LOG" 2>&1 &
else
    # Try system sweb
    sweb serve \
        --port 8081 \
        --dir "$SWEB_DATA_DIR" \
        --webdav /webdav \
        --auth admin:admin123 \
        > "$SWEB_LOG" 2>&1 &
fi

SWEB_PID=$!
echo "sweb started with PID: $SWEB_PID"

# Wait for sweb to start
echo "Waiting for sweb to be ready..."
for i in {1..30}; do
    if curl -s http://localhost:8081/webdav/ > /dev/null 2>&1; then
        echo -e "${GREEN}sweb is ready!${NC}"
        break
    fi
    if [ $i -eq 30 ]; then
        echo -e "${YELLOW}Warning: sweb may not be ready (timeout)${NC}"
    fi
    sleep 1
done

echo ""
echo -e "${GREEN}=== Test Servers Started ===${NC}"
echo "S3 (zs3):  http://localhost:9000"
echo "WebDAV:    http://localhost:8081/webdav"
echo ""
echo "Server PIDs:"
echo "  zs3:  $ZS3_PID"
echo "  sweb: $SWEB_PID"
echo ""
echo "Log files:"
echo "  zs3:  $ZS3_LOG"
echo "  sweb: $SWEB_LOG"
echo ""
echo "Press Ctrl+C to stop servers"

# Wait for interrupt signal
wait
