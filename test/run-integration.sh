#!/bin/bash

# Integration Test Runner for Torua Distributed Storage System
# This script runs end-to-end tests against the distributed system

set -e  # Exit on error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
COORDINATOR_PORT=18080
NODE1_PORT=18081
NODE2_PORT=18082

# Cleanup function to ensure processes are killed
cleanup() {
    echo -e "${YELLOW}Cleaning up processes...${NC}"

    # Kill any running test processes
    pkill -f "bin/coordinator" 2>/dev/null || true
    pkill -f "bin/node" 2>/dev/null || true

    # Also check for processes on our test ports
    lsof -ti:$COORDINATOR_PORT | xargs kill -9 2>/dev/null || true
    lsof -ti:$NODE1_PORT | xargs kill -9 2>/dev/null || true
    lsof -ti:$NODE2_PORT | xargs kill -9 2>/dev/null || true

    echo -e "${GREEN}Cleanup complete${NC}"
}

# Trap to ensure cleanup runs on exit
trap cleanup EXIT INT TERM

# Function to check if a port is in use
check_port() {
    local port=$1
    if lsof -Pi :$port -sTCP:LISTEN -t >/dev/null 2>&1; then
        echo -e "${RED}Error: Port $port is already in use${NC}"
        echo "Please stop any existing services on ports $COORDINATOR_PORT, $NODE1_PORT, $NODE2_PORT"
        exit 1
    fi
}

# Main execution
main() {
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}Torua Integration Test Runner${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo ""

    # Check if we're in the right directory
    if [ ! -f "go.mod" ] || [ ! -d "cmd/coordinator" ]; then
        echo -e "${RED}Error: Must run from torua project root${NC}"
        exit 1
    fi

    # Check ports are available
    echo "Checking ports..."
    check_port $COORDINATOR_PORT
    check_port $NODE1_PORT
    check_port $NODE2_PORT
    echo -e "${GREEN}Ports are available${NC}"
    echo ""

    # Build the system
    echo "Building system components..."
    echo "  Building coordinator..."
    go build -o bin/coordinator ./cmd/coordinator
    if [ $? -ne 0 ]; then
        echo -e "${RED}Failed to build coordinator${NC}"
        exit 1
    fi

    echo "  Building node..."
    go build -o bin/node ./cmd/node
    if [ $? -ne 0 ]; then
        echo -e "${RED}Failed to build node${NC}"
        exit 1
    fi
    echo -e "${GREEN}Build complete${NC}"
    echo ""

    # Run unit tests first
    echo "Running unit tests..."
    go test ./internal/... -cover -short
    if [ $? -ne 0 ]; then
        echo -e "${RED}Unit tests failed${NC}"
        exit 1
    fi
    echo -e "${GREEN}Unit tests passed${NC}"
    echo ""

    # Run integration tests
    echo "Running integration tests..."
    echo "This will start a coordinator and 2 nodes for testing"
    echo ""

    # Set timeout for integration tests (default 2 minutes)
    INTEGRATION_TIMEOUT=${INTEGRATION_TIMEOUT:-120s}

    # Run the integration test
    go test ./test/integration -v -timeout $INTEGRATION_TIMEOUT
    TEST_RESULT=$?

    echo ""
    if [ $TEST_RESULT -eq 0 ]; then
        echo -e "${GREEN}========================================${NC}"
        echo -e "${GREEN}✓ All integration tests passed!${NC}"
        echo -e "${GREEN}========================================${NC}"
    else
        echo -e "${RED}========================================${NC}"
        echo -e "${RED}✗ Integration tests failed${NC}"
        echo -e "${RED}========================================${NC}"
        exit 1
    fi

    # Optional: Run feature validation
    if [ "$1" == "--features" ]; then
        echo ""
        echo "Validating feature specifications..."
        validate_features
    fi
}

# Function to validate feature files against implementation
validate_features() {
    echo "Checking feature coverage..."

    # Count scenarios in feature files
    if [ -d "features" ]; then
        total_scenarios=$(grep -h "Scenario:" features/*.feature | wc -l)
        echo "  Total scenarios defined: $total_scenarios"

        # List all scenarios
        echo ""
        echo "Defined scenarios:"
        grep -h "Scenario:" features/*.feature | sed 's/^/  - /'

        echo ""
        echo -e "${YELLOW}Note: Not all scenarios may be implemented yet${NC}"
    else
        echo -e "${YELLOW}No feature files found${NC}"
    fi
}

# Function to run a specific test scenario
run_scenario() {
    local scenario=$1
    echo "Running scenario: $scenario"
    go test ./test/integration -v -run "TestDistributedStorage/$scenario" -timeout 30s
}

# Parse command line arguments
case "$1" in
    --help|-h)
        echo "Usage: $0 [options]"
        echo ""
        echo "Options:"
        echo "  --help, -h       Show this help message"
        echo "  --features       Validate feature specifications"
        echo "  --scenario NAME  Run a specific test scenario"
        echo "  --quick          Run only quick tests (skip performance)"
        echo ""
        echo "Environment variables:"
        echo "  INTEGRATION_TIMEOUT  Test timeout (default: 120s)"
        echo ""
        echo "Examples:"
        echo "  $0                          # Run all integration tests"
        echo "  $0 --features               # Run tests and validate features"
        echo "  $0 --scenario StoreAndRetrieve  # Run specific scenario"
        exit 0
        ;;
    --scenario)
        if [ -z "$2" ]; then
            echo -e "${RED}Error: --scenario requires a scenario name${NC}"
            exit 1
        fi
        cleanup
        run_scenario "$2"
        exit $?
        ;;
    --quick)
        export SKIP_PERFORMANCE_TESTS=1
        main
        ;;
    --features)
        main "--features"
        ;;
    *)
        main
        ;;
esac
