#!/bin/bash

# Integration Test Runner for Catnip
# This script builds the test Docker container and runs integration tests

set -e

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONTAINER_DIR="$(dirname "$SCRIPT_DIR")"
PROJECT_ROOT="$(dirname "$CONTAINER_DIR")"
TEST_IMAGE="catnip:test"
TEST_CONTAINER="catnip-integration-test"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to cleanup containers and images
cleanup() {
    log_info "Cleaning up test containers and images..."
    
    # Stop and remove test container if it exists
    if docker ps -a --format '{{.Names}}' | grep -q "^${TEST_CONTAINER}$"; then
        log_info "Stopping and removing test container: $TEST_CONTAINER"
        docker stop "$TEST_CONTAINER" >/dev/null 2>&1 || true
        docker rm "$TEST_CONTAINER" >/dev/null 2>&1 || true
    fi
    
    # Optionally remove test image (uncomment if desired)
    # if docker images --format '{{.Repository}}:{{.Tag}}' | grep -q "^${TEST_IMAGE}$"; then
    #     log_info "Removing test image: $TEST_IMAGE"
    #     docker rmi "$TEST_IMAGE" >/dev/null 2>&1 || true
    # fi
}

# Function to build the main catnip image if it doesn't exist
build_main_image() {
    if ! docker images --format '{{.Repository}}:{{.Tag}}' | grep -q "^catnip:latest$"; then
        log_info "Building main catnip image..."
        cd "$PROJECT_ROOT"
        docker build -t catnip:latest -f container/Dockerfile .
        log_success "Main catnip image built successfully"
    else
        log_info "Main catnip image already exists"
    fi
}

# Function to build the test image
build_test_image() {
    log_info "Building test image: $TEST_IMAGE"
    cd "$PROJECT_ROOT"
    
    # Build the test image
    docker build -t "$TEST_IMAGE" -f container/Dockerfile.test .
    
    log_success "Test image built successfully: $TEST_IMAGE"
}

# Function to run integration tests
run_tests() {
    log_info "Running integration tests..."
    
    # Create a temporary container to run tests
    docker run --rm \
        --name "$TEST_CONTAINER" \
        -v "$SCRIPT_DIR/integration:/opt/catnip/test/src/test/integration" \
        -e CATNIP_TEST_MODE=1 \
        -e CATNIP_TEST_DATA_DIR=/opt/catnip/test/data \
        -w /opt/catnip/test/src \
        --user root \
        "$TEST_IMAGE" \
        bash -c "
            # Run the tests from the integration subdirectories
            go test -v -timeout 30m ./test/integration/common/... ./test/integration/api/... 2>&1
        "
    
    local test_exit_code=$?
    
    if [ $test_exit_code -eq 0 ]; then
        log_success "All integration tests passed!"
    else
        log_error "Integration tests failed with exit code: $test_exit_code"
        return $test_exit_code
    fi
}

# Function to run specific test
run_specific_test() {
    local test_name="$1"
    log_info "Running specific test: $test_name"
    
    docker run --rm \
        --name "$TEST_CONTAINER" \
        -v "$SCRIPT_DIR/integration:/opt/catnip/test/src/test/integration" \
        -e CATNIP_TEST_MODE=1 \
        -e CATNIP_TEST_DATA_DIR=/opt/catnip/test/data \
        -w /opt/catnip/test/src \
        --user root \
        "$TEST_IMAGE" \
        bash -c "
            go test -v -timeout 30m -run '$test_name' ./test/integration/common/... ./test/integration/api/...
        "
}

# Function to run benchmarks
run_benchmarks() {
    log_info "Running benchmarks..."
    
    docker run --rm \
        --name "$TEST_CONTAINER" \
        -v "$SCRIPT_DIR/integration:/opt/catnip/test/src/test/integration" \
        -e CATNIP_TEST_MODE=1 \
        -e CATNIP_TEST_DATA_DIR=/opt/catnip/test/data \
        -w /opt/catnip/test/src \
        --user root \
        "$TEST_IMAGE" \
        bash -c "
            go test -v -bench=. -benchmem ./test/integration/common/... ./test/integration/api/...
        "
}

# Function to show help
show_help() {
    cat << EOF
Integration Test Runner for Catnip

Usage: $0 [COMMAND] [OPTIONS]

Commands:
    build       Build the test Docker image
    test        Run all integration tests (default)
    test <name> Run specific test by name
    bench       Run benchmark tests
    clean       Clean up test containers and images
    shell       Open interactive shell in test container
    help        Show this help message

Options:
    --no-build  Skip building the test image (use existing)
    --rebuild   Force rebuild of both main and test images

Examples:
    $0                              # Run all tests
    $0 test                         # Run all tests
    $0 test TestWorktreeCreation    # Run specific test
    $0 bench                        # Run benchmarks
    $0 build                        # Just build the test image
    $0 clean                        # Clean up containers/images
    $0 shell                        # Interactive shell for debugging

EOF
}

# Function to open interactive shell in test container
open_shell() {
    log_info "Opening interactive shell in test container..."
    
    docker run --rm -it \
        --name "$TEST_CONTAINER" \
        -v "$SCRIPT_DIR/integration:/opt/catnip/test/src/test/integration" \
        -e CATNIP_TEST_MODE=1 \
        -e CATNIP_TEST_DATA_DIR=/opt/catnip/test/data \
        -w /opt/catnip/test/src \
        --user root \
        "$TEST_IMAGE" \
        bash
}

# Parse command line arguments
COMMAND="test"
SKIP_BUILD=false
FORCE_REBUILD=false

while [[ $# -gt 0 ]]; do
    case $1 in
        build)
            COMMAND="build"
            shift
            ;;
        test)
            COMMAND="test"
            shift
            if [[ $# -gt 0 && ! "$1" =~ ^-- ]]; then
                TEST_NAME="$1"
                shift
            fi
            ;;
        bench)
            COMMAND="bench"
            shift
            ;;
        clean)
            COMMAND="clean"
            shift
            ;;
        shell)
            COMMAND="shell" 
            shift
            ;;
        help|--help|-h)
            show_help
            exit 0
            ;;
        --no-build)
            SKIP_BUILD=true
            shift
            ;;
        --rebuild)
            FORCE_REBUILD=true
            shift
            ;;
        *)
            if [[ "$COMMAND" == "test" && -z "$TEST_NAME" ]]; then
                TEST_NAME="$1"
            else
                log_error "Unknown option: $1"
                show_help
                exit 1
            fi
            shift
            ;;
    esac
done

# Main execution
main() {
    log_info "Starting Catnip Integration Test Runner"
    log_info "Working directory: $SCRIPT_DIR"
    
    # Handle cleanup command early
    if [ "$COMMAND" = "clean" ]; then
        cleanup
        exit 0
    fi
    
    # Build images if needed
    if [ "$FORCE_REBUILD" = true ]; then
        log_info "Force rebuild requested"
        cleanup
        build_main_image
        build_test_image
    elif [ "$SKIP_BUILD" = false ]; then
        build_main_image
        build_test_image
    else
        log_info "Skipping image build (--no-build specified)"
        # Check if test image exists
        if ! docker images --format '{{.Repository}}:{{.Tag}}' | grep -q "^${TEST_IMAGE}$"; then
            log_error "Test image $TEST_IMAGE not found. Run without --no-build or run '$0 build' first."
            exit 1
        fi
    fi
    
    # Execute the requested command
    case "$COMMAND" in
        build)
            log_success "Test image build completed"
            ;;
        test)
            if [ -n "$TEST_NAME" ]; then
                run_specific_test "$TEST_NAME"
            else
                run_tests
            fi
            ;;
        bench)
            run_benchmarks
            ;;
        shell)
            open_shell
            ;;
        *)
            log_error "Unknown command: $COMMAND"
            show_help
            exit 1
            ;;
    esac
}

# Trap to cleanup on exit
trap cleanup EXIT

# Run main function
main "$@"