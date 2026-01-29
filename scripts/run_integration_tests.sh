#!/bin/bash
# run_integration_tests.sh - Run all integration tests with automatic server management

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${GREEN}=== ClearVault Integration Tests ===${NC}"
echo ""

# Check if tools are installed
check_tools() {
    local missing=()
    
    if [ ! -f "$PROJECT_ROOT/tools/zs3" ]; then
        missing+=("zs3")
    fi
    
    if [ ! -f "$PROJECT_ROOT/tools/sweb" ]; then
        missing+=("sweb")
    fi
    
    if [ ${#missing[@]} -gt 0 ]; then
        echo -e "${RED}Error: Missing required tools:${NC}"
        for tool in "${missing[@]}"; do
            echo "  - $tool"
        done
        echo ""
        echo "Please install missing tools:"
        for tool in "${missing[@]}"; do
            echo "  ./scripts/install_${tool}.sh"
        done
        exit 1
    fi
}

# Parse arguments
TEST_PATTERN=""
VERBOSE=""
COVERAGE=""

while [[ $# -gt 0 ]]; do
    case $1 in
        -v|--verbose)
            VERBOSE="-v"
            shift
            ;;
        -cover|--coverage)
            COVERAGE="-cover"
            shift
            ;;
        -p|--pattern)
            TEST_PATTERN="-run=$2"
            shift 2
            ;;
        -h|--help)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  -v, --verbose       Enable verbose output"
            echo "  -cover, --coverage  Generate coverage report"
            echo "  -p, --pattern       Run tests matching pattern (e.g., -p TestUpload)"
            echo "  -h, --help          Show this help message"
            echo ""
            echo "Examples:"
            echo "  $0                  Run all integration tests"
            echo "  $0 -v               Run with verbose output"
            echo "  $0 -p TestS3        Run S3 tests only"
            echo "  $0 -cover           Run with coverage"
            exit 0
            ;;
        *)
            echo -e "${RED}Unknown option: $1${NC}"
            exit 1
            ;;
    esac
done

# Check tools
check_tools

# Run tests
echo -e "${GREEN}Running integration tests...${NC}"
echo ""

cd "$PROJECT_ROOT"

# Run S3 integration tests
echo -e "${YELLOW}--- S3 Integration Tests (zs3) ---${NC}"
go test ./internal/remote/s3 $VERBOSE $COVERAGE $TEST_PATTERN -count=1 -timeout 120s
S3_RESULT=$?

echo ""
echo -e "${YELLOW}--- WebDAV Integration Tests (sweb) ---${NC}"
go test ./internal/remote/webdav $VERBOSE $COVERAGE $TEST_PATTERN -count=1 -timeout 120s
WEBDAV_RESULT=$?

echo ""
echo -e "${YELLOW}--- API Tests ---${NC}"
go test ./internal/api $VERBOSE $COVERAGE $TEST_PATTERN -count=1 -timeout 60s
API_RESULT=$?

echo ""
echo -e "${YELLOW}--- Crypto Tests ---${NC}"
go test ./internal/crypto $VERBOSE $COVERAGE $TEST_PATTERN -count=1 -timeout 60s
CRYPTO_RESULT=$?

echo ""
echo -e "${YELLOW}--- Key Manager Tests ---${NC}"
go test ./internal/key $VERBOSE $COVERAGE $TEST_PATTERN -count=1 -timeout 60s
KEY_RESULT=$?

# Summary
echo ""
echo -e "${GREEN}=== Test Summary ===${NC}"
if [ $S3_RESULT -eq 0 ]; then
    echo -e "S3 Tests:         ${GREEN}PASS${NC}"
else
    echo -e "S3 Tests:         ${RED}FAIL${NC}"
fi

if [ $WEBDAV_RESULT -eq 0 ]; then
    echo -e "WebDAV Tests:     ${GREEN}PASS${NC}"
else
    echo -e "WebDAV Tests:     ${RED}FAIL${NC}"
fi

if [ $API_RESULT -eq 0 ]; then
    echo -e "API Tests:        ${GREEN}PASS${NC}"
else
    echo -e "API Tests:        ${RED}FAIL${NC}"
fi

if [ $CRYPTO_RESULT -eq 0 ]; then
    echo -e "Crypto Tests:     ${GREEN}PASS${NC}"
else
    echo -e "Crypto Tests:     ${RED}FAIL${NC}"
fi

if [ $KEY_RESULT -eq 0 ]; then
    echo -e "Key Manager:      ${GREEN}PASS${NC}"
else
    echo -e "Key Manager:      ${RED}FAIL${NC}"
fi

# Exit with overall result
if [ $S3_RESULT -eq 0 ] && [ $WEBDAV_RESULT -eq 0 ] && [ $API_RESULT -eq 0 ] && [ $CRYPTO_RESULT -eq 0 ] && [ $KEY_RESULT -eq 0 ]; then
    echo ""
    echo -e "${GREEN}All tests passed!${NC}"
    exit 0
else
    echo ""
    echo -e "${RED}Some tests failed!${NC}"
    exit 1
fi
