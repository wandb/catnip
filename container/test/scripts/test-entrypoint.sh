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

# If running with Air, prepare config (optional; default is disabled)
if [[ "${CATNIP_TEST_AIR}" == "1" ]]; then
  # Download Go dependencies (will be fast due to pre-warmed cache)
  echo "📦 Installing Go dependencies..."
  go mod download

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
fi

# Create test live repository for preview branch testing
echo "📂 Creating test live repository..."
TEST_LIVE_REPO="/live/test-live-repo"
mkdir -p "$TEST_LIVE_REPO"

# Initialize git repository
cd "$TEST_LIVE_REPO"
git init --initial-branch=main
git config user.name "Test User"
git config user.email "test@example.com"

# Create a basic project structure
cat > README.md << 'EOF'
# Test Live Repository

This is a test repository for integration testing.
It includes a setup.sh script for testing auto-setup functionality.
EOF

cat > package.json << 'EOF'
{
  "name": "test-live-repo",
  "version": "1.0.0",
  "description": "Test repository for integration tests",
  "main": "index.js",
  "scripts": {
    "start": "node index.js",
    "test": "echo \"Test passed\""
  }
}
EOF

cat > index.js << 'EOF'
console.log("Hello from test live repository!");
console.log("This file was created during test setup.");
EOF

# Create setup.sh script for testing auto-setup functionality
cat > setup.sh << 'EOF'
#!/bin/bash
echo "🔧 Running setup.sh for test-live-repo"
echo "📦 Installing dependencies..."
echo "✅ Setup completed successfully!"
echo "Setup timestamp: $(date)"
echo "Working directory: $(pwd)"
echo "Files in directory: $(ls -la)"
EOF

chmod +x setup.sh

# Add and commit all files
git add .
git commit -m "Initial commit with test files and setup.sh"

# Change ownership to catnip user
chown -R catnip:catnip "$TEST_LIVE_REPO"

echo "✅ Created test live repository at $TEST_LIVE_REPO"

# Go back to container directory
cd /live/catnip/container

# Set environment variables for test mode
export CATNIP_TEST_MODE=1
export PORT=8181

# Start server
if [[ "${CATNIP_TEST_AIR}" == "1" ]]; then
  echo "⚡ Starting Go test server with Air on port 8181..."
  air -c .air.test.toml &
  GO_PID=$!
  echo "✅ Test environment ready with Air"
  echo "   🔧 Test Server: http://localhost:8181 (Air hot reloading)"
  echo "   📚 API Docs:    http://localhost:8181/swagger/"
  echo ""
  echo "🔥 HMR enabled for backend via Air"
  wait
else
  echo "🚀 Starting Catnip server (no Air) on port 8181..."
  exec /opt/catnip/bin/catnip serve
fi