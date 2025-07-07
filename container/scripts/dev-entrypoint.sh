#!/bin/bash
# Development entrypoint that starts both Vite and Go server with hot reloading

set -e

# Source the catnip environment (skipping as we now run the regular entrypoint as well)
# source /etc/profile.d/catnip.sh

echo "🚀 Starting Catnip development environment..."

# Function to handle cleanup
cleanup() {
    echo "🛑 Shutting down development servers..."
    jobs -p | xargs -r kill
    wait
    exit 0
}

# Trap signals for cleanup
trap cleanup SIGTERM SIGINT

# Change to the mounted project directory
cd /workspace/catnip

# Install frontend dependencies (will be fast due to pre-warmed cache)
echo "📦 Installing frontend dependencies..."
pnpm install

# Start Vite dev server in background (config handled by vite.config.ts)
echo "🎨 Starting Vite development server on port 5173..."
pnpm dev &
VITE_PID=$!

# Wait a moment for Vite to start
sleep 3

# Change to container directory for Go server
cd /workspace/catnip/container

# Download Go dependencies (will be fast due to pre-warmed cache)
echo "📦 Installing Go dependencies..."
go mod download

# Start Go server with Air hot reloading
echo "⚡ Starting Go server with hot reloading on port 8080..."
air &
GO_PID=$!

echo "✅ Development environment ready!"
echo "   📱 Frontend: http://localhost:5173 (with HMR hot reloading)"
echo "   🔧 Backend:  http://localhost:8080 (with Air hot reloading)"
echo "   📚 API Docs: http://localhost:8080/swagger/"
echo ""
echo "🔥 Hot Module Replacement (HMR) enabled:"
echo "   • Frontend: File polling active for container compatibility"
echo "   • Backend: Air watching for Go file changes"
echo "   • Make changes to src/ or container/ files to see live updates!"

# Wait for both processes
wait