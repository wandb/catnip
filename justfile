# Catnip Development Container Management

# Build the catnip container for the current platform  
build-container:
    @echo "🏗️  Building catnip container for current platform..."
    docker build -f container/Dockerfile -t catnip:latest -t ghcr.io/wandb/catnip:latest .
    @echo "✅ Build complete! Run with: docker run -it catnip:latest"

# Update language versions to latest stable and rebuild
update-versions:
    @echo "🔄 Updating language versions..."
    ./scripts/update-versions.sh

# Build for multiple architectures (requires buildx)
build-multi:
    @echo "🏗️  Building catnip container for multiple architectures..."
    docker buildx build -f container/Dockerfile --platform linux/amd64,linux/arm64 -t catnip:latest --load .
    @echo "✅ Multi-arch build complete!"

# Run the container interactively
run:
    @echo "🚀 Starting catnip container..."
    docker run -it --rm -v catnip-state:/volume -e ANTHROPIC_API_KEY=$ANTHROPIC_API_KEY -p 8080:8080 catnip-dev

# Run the container in development mode with hot reloading (interactive)
run-dev: build-dev
    @echo "🚀 Starting catnip full-stack development environment..."
    @echo "   📱 Frontend: http://localhost:5173"
    @echo "   🔧 Backend:  http://localhost:8080"
    @echo "   📚 API Docs: http://localhost:8080/swagger/"
    @echo "   Press Ctrl+C to stop both servers"
    docker run -it --rm \
        --name catnip-dev \
        -v catnip-state:/volume \
        -v ~/.claude/ide:/volume/.claude/ide \
        -v $(pwd):/live/catnip \
        -v catnip-dev-node-modules:/live/catnip/node_modules \
        -e CLAUDE_CODE_IDE_HOST_OVERRIDE=host.docker.internal \
        -e CATNIP_SESSION=catnip \
        -e CATNIP_USERNAME=$USER \
        -e ANTHROPIC_API_KEY=$ANTHROPIC_API_KEY \
        -p 8080:8080 \
        -p 5173:5173 \
        catnip-dev:dev

# Run the container in development mode (non-interactive, for testing)
test-dev: build-dev
    @echo "🧪 Testing catnip development environment..."
    docker run --rm \
        -v $(pwd):/live/catnip \
        -v catnip-dev-node-modules:/live/catnip/node_modules \
        -e ANTHROPIC_API_KEY=$ANTHROPIC_API_KEY \
        -p 8080:8080 \
        -p 5173:5173 \
        catnip-dev:dev &
    @echo "✅ Development servers started in background"

# Build development container with Air support
build-dev: build-container
    @echo "🏗️  Building catnip development container..."
    docker build -f container/Dockerfile.dev -t catnip-dev:dev --build-arg BUILDKIT_INLINE_CACHE=1 .
    @echo "✅ Development build complete!"

# Clean development node_modules volume
clean-dev-volumes:
    @echo "🧹 Cleaning up development volumes..."
    docker volume rm catnip-dev-node-modules 2>/dev/null || true
    @echo "✅ Development volumes cleaned!"

# Force rebuild development container (clears cache layers)
rebuild-dev: clean-containers clean-dev-volumes
    @echo "🔄 Force rebuilding development container..."
    docker build --no-cache -f container/Dockerfile.dev -t catnip-dev:dev .
    @echo "✅ Development container rebuilt!"

# Run the container with a custom command
run-cmd CMD:
    @echo "🚀 Running command in catnip container: {{CMD}}"
    docker run -it --rm catnip-dev {{CMD}}

# Format all TypeScript/JavaScript files
format-ts:
    pnpm format

# Format only changed TypeScript/JavaScript files
format-ts-changed:
    pnpm format:changed

# Format all Go files in container
format-go:
    cd container && just format-go

# Format only changed Go files in container
format-go-changed:
    cd container && just format-go-changed

# Format all code (TypeScript and Go)
format-all: format-ts format-go
    @echo "✅ All code formatted!"

# Format only changed files (TypeScript and Go)
format-changed: format-ts-changed format-go-changed
    @echo "✅ Changed files formatted!"

# Clean up container images
clean-containers:
    @echo "🧹 Cleaning up catnip container images..."
    docker rmi catnip-dev catnip-dev:dev 2>/dev/null || true
    @echo "✅ Cleanup complete!"

# Clean everything (containers + dev volumes)
clean-all: clean-containers clean-dev-volumes
    @echo "✅ Complete cleanup finished!"

# Show container information
info:
    @echo "📋 Catnip Container Information:"
    @echo "  Image: catnip-dev"
    @echo "  Platform: $(uname -m)"
    @echo "  Build context: ./container/"
    @echo ""
    @echo "Available commands:"
    @echo ""
    @echo "Container Management:"
    @echo "  just build-container   - Build production container"
    @echo "  just run               - Run container interactively"
    @echo ""
    @echo "Development:"
    @echo "  just dev               - Local dev mode (frontend + backend with port allocation)"
    @echo "  just run-dev           - Full-stack dev (interactive, Ctrl+C to stop)"
    @echo "  just test-dev          - Test development environment (background)"
    @echo "  just build-dev         - Build development container (with pre-warmed cache)"
    @echo "  just rebuild-dev       - Force rebuild dev container (clears cache)"
    @echo ""
    @echo "Go Server (use container:: prefix):"
    @echo "  just container::build  - Build Go server binary"
    @echo "  just container::dev    - Run Go server locally with Air"
    @echo "  just container::test   - Run Go tests"
    @echo ""
    @echo "Code Formatting:"
    @echo "  just format-all        - Format all TypeScript and Go files"
    @echo "  just format-changed    - Format only changed files"
    @echo "  just format-ts         - Format all TypeScript/JavaScript files"
    @echo "  just format-ts-changed - Format only changed TS/JS files"
    @echo "  just format-go         - Format all Go files"
    @echo "  just format-go-changed - Format only changed Go files"
    @echo ""
    @echo "Release Management:"
    @echo "  just release           - Create minor release (local tag)"
    @echo "  just release --patch   - Create patch release"
    @echo "  just release --major   - Create major release"
    @echo "  just release --dev     - Create dev release"
    @echo "  Add --push --message=\"...\" to actually release"
    @echo ""
    @echo "Cleanup:"
    @echo "  just clean-containers  - Remove container images"
    @echo "  just clean-dev-volumes - Remove development volumes"
    @echo "  just clean-all         - Clean everything"

# Release management (defaults to minor version bump)
release *ARGS="":
    @echo "🚀 Creating release..."
    npx tsx scripts/release.ts {{ARGS}}

# Development mode - runs both frontend and backend with proper port allocation
dev:
    #!/usr/bin/env bash
    set -euo pipefail
    
    # Extract first port from PORTZ if available, otherwise use default
    if [ -n "${PORTZ:-}" ]; then
        VITE_PORT=$(echo "$PORTZ" | jq -r '.[0] // 5173')
        echo "🌐 Using VITE_PORT=$VITE_PORT from PORTZ array"
    else
        VITE_PORT=5173
        echo "🌐 Using default VITE_PORT=$VITE_PORT (no PORTZ found)"
    fi
    
    # Show port info
    echo "🔗 Backend PORT: ${PORT:-8080}"
    echo "🔗 Frontend VITE_PORT: $VITE_PORT"
    echo ""
    
    # Export VITE_PORT for both processes
    export VITE_PORT
    
    # Function to cleanup background processes
    cleanup() {
        echo "🛑 Stopping development servers..."
        kill $(jobs -p) 2>/dev/null || true
        wait
        echo "✅ Development servers stopped"
    }
    
    # Set up signal handlers
    trap cleanup EXIT INT TERM

    # Start pnpm dev (frontend) in background
    echo "🚀 Starting pnpm dev (frontend) on port $VITE_PORT..."
    pnpm dev &
    PNPM_PID=$!
    # Give vite a moment to start
    sleep 2
    
    # Start Air (backend) in background
    echo "🚀 Starting Air (backend) on port ${PORT:-8080}..."
    export CATNIP_DEV=true
    cd container && air &
    AIR_PID=$!
    
    # Display helpful info
    echo ""
    echo "🎉 Development servers started!"
    echo "   📱 Frontend: http://localhost:$VITE_PORT"
    echo "   🔧 Backend:  http://localhost:${PORT:-8080}"
    echo "   📚 API Docs: http://localhost:${PORT:-8080}/docs/"
    echo ""
    echo "Press Ctrl+C to stop both servers"
    echo ""
    
    # Wait for either process to exit
    wait

# Default recipe
default:
    @just --list