#!/bin/bash
# Test entrypoint that starts the Go server on port 8181 with hot reloading

set -e

# Prepend test bin directory to PATH to ensure mocks are used
export PATH="/opt/catnip/test/bin:$PATH"

echo "🧪 Starting Catnip test environment..."

# Function to handle cleanup
cleanup() {
    echo "🛑 Shutting down test server..."
    jobs -p | xargs -r kill
    wait
    exit 0
}

# Trap signals for cleanup
trap cleanup SIGTERM SIGINT

# Change to the mounted project directory
cd /live/catnip/container

# Download Go dependencies (will be fast due to pre-warmed cache)
echo "📦 Installing Go dependencies..."
go mod download

# Create a test-specific .air.toml configuration
cat > .air.test.toml << 'EOF'
root = "."
testdata_dir = "testdata"
tmp_dir = "tmp"

[build]
  args_bin = []
  bin = "./tmp/catnip-test"
  cmd = "go build -buildvcs=false -o ./tmp/catnip-test ./cmd/server"
  delay = 1000
  exclude_dir = ["assets", "tmp", "vendor", "testdata", "docs", "bin", "dist", "internal/tui"]
  exclude_file = []
  exclude_regex = ["_test.go"]
  exclude_unchanged = false
  follow_symlink = false
  full_bin = ""
  include_dir = []
  include_ext = ["go", "tpl", "tmpl", "html"]
  include_file = []
  kill_delay = "0s"
  log = "build-errors.log"
  poll = false
  poll_interval = 0
  post_cmd = []
  pre_cmd = []
  rerun = false
  rerun_delay = 500
  send_interrupt = false
  stop_on_error = false

[color]
  app = ""
  build = "yellow"
  main = "magenta"
  runner = "green"
  watcher = "cyan"

[log]
  main_only = false
  time = false

[misc]
  clean_on_exit = false

[screen]
  clear_on_rebuild = false
  keep_scroll = true
EOF

# Set environment variables for test mode
export CATNIP_TEST_MODE=1
export PORT=8181

# Start Go server with Air hot reloading on test port
echo "⚡ Starting Go test server with hot reloading on port 8181..."
air -c .air.test.toml &
GO_PID=$!

echo "✅ Test environment ready!"
echo "   🔧 Test Server: http://localhost:8181 (with Air hot reloading)"
echo "   📚 API Docs:    http://localhost:8181/swagger/"
echo ""
echo "🔥 Hot Module Replacement (HMR) enabled:"
echo "   • Backend: Air watching for Go file changes"
echo "   • Make changes to container/ files to see live updates!"

# Wait for the process
wait