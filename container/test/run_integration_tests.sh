#!/bin/bash

# Integration Test Runner for Catnip
# This script manages a test container running the Catnip server and runs integration tests against it

set -e

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONTAINER_DIR="$(dirname "$SCRIPT_DIR")"
PROJECT_ROOT="$(dirname "$CONTAINER_DIR")"
TEST_IMAGE="catnip:test"
TEST_CONTAINER="catnip-test"
TEST_PORT="8181"

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

# Function to cleanup containers and images (only when explicitly requested)
cleanup() {
    # Only perform cleanup for specific commands or when explicitly requested
    case "$COMMAND" in
        clean)
            log_info "Cleaning up test containers and images..."
            # Stop test container using docker-compose
            cd "$SCRIPT_DIR"
            docker-compose -f docker-compose.test.yml down >/dev/null 2>&1 || true
            
            # Optionally remove test image (uncomment if desired)
            # if docker images --format '{{.Repository}}:{{.Tag}}' | grep -q "^${TEST_IMAGE}$"; then
            #     log_info "Removing test image: $TEST_IMAGE"
            #     docker rmi "$TEST_IMAGE" >/dev/null 2>&1 || true
            # fi
            ;;
        *)
            # No cleanup for other commands - keep container running
            ;;
    esac
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
    
    # Build the test image directly with Docker using BuildKit
    # This ensures we use the local catnip:latest image
    DOCKER_BUILDKIT=1 docker build \
        -t "$TEST_IMAGE" \
        -f container/test/Dockerfile.test \
        --build-arg BUILDKIT_INLINE_CACHE=1 \
        .
    
    log_success "Test image built successfully: $TEST_IMAGE"
}

# Function to start the test container
start_test_container() {
    log_info "Starting test container on port $TEST_PORT..."
    cd "$SCRIPT_DIR"
    
    # Start the container using docker-compose
    # No need to build, just use the existing catnip:test image
    docker-compose -f docker-compose.test.yml up -d --no-build
    
    # Wait for the container to be healthy
    log_info "Waiting for test server to be ready..."
    local max_attempts=30
    local attempt=1
    
    while [ $attempt -le $max_attempts ]; do
        if curl -f http://localhost:$TEST_PORT/health >/dev/null 2>&1; then
            log_success "Test server is ready on port $TEST_PORT"
            return 0
        fi
        
        if [ $attempt -eq $max_attempts ]; then
            log_error "Test server failed to start after $max_attempts attempts"
            docker-compose -f docker-compose.test.yml logs
            return 1
        fi
        
        log_info "Attempt $attempt/$max_attempts: Waiting for test server..."
        sleep 2
        ((attempt++))
    done
}

# Function to stop the test container
stop_test_container() {
    log_info "Stopping test container..."
    cd "$SCRIPT_DIR"
    docker-compose -f docker-compose.test.yml down
}

# Function to check if test container is running
is_test_container_running() {
    if curl -f http://localhost:$TEST_PORT/health >/dev/null 2>&1; then
        return 0
    else
        return 1
    fi
}

# Function to ensure test container is running
ensure_test_container() {
    if ! is_test_container_running; then
        log_info "Test container not running, starting it..."
        start_test_container
    else
        log_info "Test container is already running"
    fi
}

# Function to run integration tests
run_tests() {
    log_info "Running integration tests..."
    
    # Ensure test container is running
    ensure_test_container
    
    # Run the tests from the host using Go installed locally or in a runner container
    # For now, we'll use a simple approach - create a minimal test runner
    cd "$SCRIPT_DIR/integration"
    
    # Set environment variables for tests to point to our test server
    export CATNIP_TEST_MODE=1
    export CATNIP_TEST_SERVER_URL="http://localhost:$TEST_PORT"
    export CATNIP_TEST_DATA_DIR="$SCRIPT_DIR/data"
    
    # Check if go is available locally
    if command -v go >/dev/null 2>&1; then
        log_info "Running tests with local Go installation..."
        go test -v -timeout 30m ./... 2>&1
    else
        log_info "Go not found locally, using Docker to run tests..."
        # Use a Go container to run the tests, connected to our test server
        docker run --rm \
            -v "$SCRIPT_DIR/integration:/test" \
            -v "$SCRIPT_DIR/data:/data" \
            -e CATNIP_TEST_MODE=1 \
            -e CATNIP_TEST_SERVER_URL="http://host.docker.internal:$TEST_PORT" \
            -e CATNIP_TEST_DATA_DIR="/data" \
            -w /test \
            --add-host=host.docker.internal:host-gateway \
            golang:1.21 \
            go test -v -timeout 30m ./...
    fi
    
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
    
    # Ensure test container is running
    ensure_test_container
    
    cd "$SCRIPT_DIR/integration"
    
    # Set environment variables for tests to point to our test server
    export CATNIP_TEST_MODE=1
    export CATNIP_TEST_SERVER_URL="http://localhost:$TEST_PORT"
    export CATNIP_TEST_DATA_DIR="$SCRIPT_DIR/data"
    
    # Check if go is available locally
    if command -v go >/dev/null 2>&1; then
        log_info "Running test with local Go installation..."
        go test -v -timeout 30m -run "$test_name" ./...
    else
        log_info "Go not found locally, using Docker to run test..."
        docker run --rm \
            -v "$SCRIPT_DIR/integration:/test" \
            -v "$SCRIPT_DIR/data:/data" \
            -e CATNIP_TEST_MODE=1 \
            -e CATNIP_TEST_SERVER_URL="http://host.docker.internal:$TEST_PORT" \
            -e CATNIP_TEST_DATA_DIR="/data" \
            -w /test \
            --add-host=host.docker.internal:host-gateway \
            golang:1.21 \
            go test -v -timeout 30m -run "$test_name" ./...
    fi
}

# Function to run benchmarks
run_benchmarks() {
    log_info "Running benchmarks..."
    
    # Ensure test container is running
    ensure_test_container
    
    cd "$SCRIPT_DIR/integration"
    
    # Set environment variables for tests to point to our test server
    export CATNIP_TEST_MODE=1
    export CATNIP_TEST_SERVER_URL="http://localhost:$TEST_PORT"
    export CATNIP_TEST_DATA_DIR="$SCRIPT_DIR/data"
    
    # Check if go is available locally
    if command -v go >/dev/null 2>&1; then
        log_info "Running benchmarks with local Go installation..."
        go test -v -bench=. -benchmem ./...
    else
        log_info "Go not found locally, using Docker to run benchmarks..."
        docker run --rm \
            -v "$SCRIPT_DIR/integration:/test" \
            -v "$SCRIPT_DIR/data:/data" \
            -e CATNIP_TEST_MODE=1 \
            -e CATNIP_TEST_SERVER_URL="http://host.docker.internal:$TEST_PORT" \
            -e CATNIP_TEST_DATA_DIR="/data" \
            -w /test \
            --add-host=host.docker.internal:host-gateway \
            golang:1.21 \
            go test -v -bench=. -benchmem ./...
    fi
}

# Function to show help
show_help() {
    cat << EOF
Integration Test Runner for Catnip

This script manages a test container running the Catnip server on port $TEST_PORT
and runs integration tests against it from the host machine.

Usage: $0 [COMMAND] [OPTIONS]

Commands:
    build       Build the test Docker image
    start       Start the test container
    stop        Stop the test container
    status      Check if test container is running
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
    $0                              # Run all tests (starts container if needed)
    $0 start                        # Start the test container
    $0 status                       # Check container status
    $0 test                         # Run all tests
    $0 test TestWorktreeCreation    # Run specific test
    $0 bench                        # Run benchmarks
    $0 build                        # Just build the test image
    $0 stop                         # Stop the test container
    $0 clean                        # Clean up containers/images
    $0 shell                        # Interactive shell for debugging

The test container runs the Catnip server with hot reloading, so you can
edit server code and see changes reflected during testing.

EOF
}

# Function to open interactive shell in test container
open_shell() {
    log_info "Opening interactive shell in test container..."
    
    # Ensure test container is running
    ensure_test_container
    
    # Connect to the running test container
    cd "$SCRIPT_DIR"
    docker-compose -f docker-compose.test.yml exec catnip-test bash
}

# Parse command line arguments
COMMAND="test"
SKIP_BUILD=false
FORCE_REBUILD=false

while [[ $# -gt 0 ]]; do
    case $1 in
        build)
            COMMAND="build"
            FORCE_REBUILD=true
            shift
            ;;
        start)
            COMMAND="start"
            shift
            ;;
        stop)
            COMMAND="stop"
            shift
            ;;
        status)
            COMMAND="status"
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
    
    # Build images if needed (except for start/stop/status/shell commands)
    if [[ "$COMMAND" != "start" && "$COMMAND" != "stop" && "$COMMAND" != "status" && "$COMMAND" != "shell" ]]; then
        if [ "$FORCE_REBUILD" = true ]; then
            log_info "Force rebuild requested"
            # Stop existing container before rebuilding
            if is_test_container_running; then
                log_info "Stopping existing test container for rebuild"
                stop_test_container
            fi
            build_main_image
            build_test_image
        elif [ "$SKIP_BUILD" = false ]; then
            # Check if main image exists and build if needed
            build_main_image
            # Check if test image exists and build if needed
            if ! docker images --format '{{.Repository}}:{{.Tag}}' | grep -q "^${TEST_IMAGE}$"; then
                build_test_image
            else
                log_info "Test image already exists"
            fi
        else
            log_info "Skipping image build (--no-build specified)"
            # Check if test image exists
            if ! docker images --format '{{.Repository}}:{{.Tag}}' | grep -q "^${TEST_IMAGE}$"; then
                log_error "Test image $TEST_IMAGE not found. Run without --no-build or run '$0 build' first."
                exit 1
            fi
        fi
    fi
    
    # Execute the requested command
    case "$COMMAND" in
        build)
            log_success "Test image build completed"
            ;;
        start)
            start_test_container
            ;;
        stop)
            stop_test_container
            ;;
        status)
            if is_test_container_running; then
                log_success "Test container is running on port $TEST_PORT"
                echo "Access test server at: http://localhost:$TEST_PORT"
            else
                log_info "Test container is not running"
                exit 1
            fi
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

# Trap to cleanup on exit (cleanup function will decide what to do based on command)
trap cleanup EXIT

# Run main function
main "$@"