#!/bin/bash
# stop_test_servers.sh - Stop zs3 and sweb test servers

set -e

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${YELLOW}=== Stopping Test Servers ===${NC}"

# Stop zs3
echo "Stopping zs3..."
pkill -f "zs3.*9000" 2>/dev/null && echo -e "${GREEN}zs3 stopped${NC}" || echo -e "${YELLOW}zs3 not running${NC}"

# Stop sweb
echo "Stopping sweb..."
pkill -f "sweb.*8081" 2>/dev/null && echo -e "${GREEN}sweb stopped${NC}" || echo -e "${YELLOW}sweb not running${NC}"

echo ""
echo -e "${GREEN}=== Test Servers Stopped ===${NC}"
