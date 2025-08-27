# Catnip Development Container Management

# Container Build System:
# - Set BUILDER=container to use Apple Container SDK instead of Docker
# - Use 'build-container' for flexible building (Docker by default, Apple Container SDK with BUILDER=container)
# - Use 'build-apple' as a convenience alias for Apple Container SDK
# - Use 'container-start/stop/status' to manage Apple Container system

# Build the catnip container for the current platform - supports both Docker and Apple Container SDK
# Usage: just build-container [TAG] [CPUS] [MEMORY] [ARGS...]
# Examples:
#   just build-container                           # Use defaults (includes ghcr.io tag)
#   just build-container catnip:dev               # Custom tag (local only)
#   just build-container catnip:dev 8 8192MB     # Custom resources
#   BUILDER=container just build-container        # Use Apple Container SDK
build-container TAG="catnip:latest" CPUS="4" MEMORY="4096MB" *ARGS="":
    #!/usr/bin/env bash
    set -euo pipefail
    
    # Determine if we should add remote registry tags
    # Only add ghcr.io tag for production builds (catnip:latest)
    if [ "{{TAG}}" = "catnip:latest" ]; then
        REMOTE_TAG="-t ghcr.io/wandb/catnip:latest"
        echo "🏷️  Including remote registry tag: ghcr.io/wandb/catnip:latest"
    else
        REMOTE_TAG=""
        echo "📍 Local build only, no remote registry tags"
    fi
    
    if [ "${BUILDER:-}" = "container" ]; then
        echo "🍎 Building with Apple Container SDK..."
        echo "   Tag: {{TAG}}"
        echo "   CPUs: {{CPUS}}"
        echo "   Memory: {{MEMORY}}"
        echo "   Args: {{ARGS}}"
        container build -f container/Dockerfile -t "{{TAG}}" $REMOTE_TAG --cpus {{CPUS}} --memory {{MEMORY}} {{ARGS}} .
    else
        echo "🐳 Building with Docker..."
        echo "   Tag: {{TAG}}"
        echo "   Args: {{ARGS}}"
        docker build -f container/Dockerfile -t "{{TAG}}" $REMOTE_TAG {{ARGS}} .
    fi
    echo "✅ Build complete! Run with: docker run -it {{TAG}}"

# Build using Apple Container SDK (convenience alias)
build-apple TAG="catnip:latest" CPUS="4" MEMORY="4096MB" *ARGS="":
    #!/usr/bin/env bash
    set -euo pipefail
    export BUILDER=container
    just build-container "{{TAG}}" "{{CPUS}}" "{{MEMORY}}" {{ARGS}}

# Build for local development (no remote registry tags)
build-local TAG="catnip:dev" CPUS="4" MEMORY="4096MB" *ARGS="":
    #!/usr/bin/env bash
    set -euo pipefail
    echo "🏠 Building for local development only..."
    export BUILDER=container
    just build-container "{{TAG}}" "{{CPUS}}" "{{MEMORY}}" {{ARGS}}

# Build with Docker and transfer to Apple Container SDK
build-docker-to-apple TAG="catnip:base" *ARGS="":
    #!/usr/bin/env bash
    set -euo pipefail
    echo "🐳→🍎 Building with Docker and transferring to Apple Container SDK..."
    
    # Build with Docker
    echo "🐳 Building with Docker..."
    docker build -f container/Dockerfile -t "{{TAG}}" {{ARGS}} .
    
    # Save to tar
    echo "💾 Saving Docker image to tar..."
    docker save "{{TAG}}" -o "/tmp/{{TAG}}.tar"
    
    # Load into Apple Container SDK
    echo "🍎 Loading into Apple Container SDK..."
    container images load --input "/tmp/{{TAG}}.tar"
    
    # Clean up
    rm "/tmp/{{TAG}}.tar"
    echo "✅ Image {{TAG}} now available in Apple Container SDK!"

# Start the Apple Container system service
container-start:
    @echo "🍎 Starting Apple Container system service..."
    container system start

# Stop the Apple Container system service
container-stop:
    @echo "🍎 Stopping Apple Container system service..."
    container system stop

# Check Apple Container system status
container-status:
    @echo "🍎 Apple Container system status:"
    container system status

# List local container images
container-images:
    @echo "📦 Local container images:"
    container images list

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
build-dev:
    #!/usr/bin/env bash
    set -euo pipefail
    echo "🏗️  Building catnip development container..."
    
    if [ "${BUILDER:-}" = "container" ]; then
        # Check if catnip:base exists in Apple Container SDK
        if ! container images list | grep -q "^catnip.*base"; then
            echo "❌ catnip:base not found in Apple Container SDK"
            echo "💡 Run: just build-docker-to-apple catnip:base"
            echo "   This will build catnip:base with Docker and transfer it to Apple Container SDK"
            exit 1
        fi
        echo "✅ Found catnip:base in Apple Container SDK"
        echo "🍎 Building dev container with Apple Container SDK..."
        container build -f container/Dockerfile.dev -t catnip-dev:dev --build-arg BUILDKIT_INLINE_CACHE=1 .
    else
        echo "🐳 Building base catnip image with Docker..."
        just build-container catnip:base
        echo "🐳 Building dev container with Docker..."
        docker build -f container/Dockerfile.dev -t catnip-dev:dev --build-arg BUILDKIT_INLINE_CACHE=1 .
    fi
    echo "✅ Development build complete!"

# Clean development node_modules volume
clean-dev-volumes:
    @echo "🧹 Cleaning up development volumes..."
    docker volume rm catnip-dev-node-modules 2>/dev/null || true
    @echo "✅ Development volumes cleaned!"

# Force rebuild development container (clears cache layers)
rebuild-dev: clean-containers clean-dev-volumes
    #!/usr/bin/env bash
    set -euo pipefail
    echo "🔄 Force rebuilding development container..."
    
    if [ "${BUILDER:-}" = "container" ]; then
        # Check if catnip:base exists in Apple Container SDK
        if ! container images list | grep -q "^catnip.*base"; then
            echo "❌ catnip:base not found in Apple Container SDK"
            echo "💡 Run: just build-docker-to-apple catnip:base"
            echo "   This will build catnip:base with Docker and transfer it to Apple Container SDK"
            exit 1
        fi
        echo "✅ Found catnip:base in Apple Container SDK"
        echo "🍎 Force rebuilding dev container with Apple Container SDK..."
        container build --no-cache -f container/Dockerfile.dev -t catnip-dev:dev .
    else
        echo "🐳 Force rebuilding base catnip image with Docker..."
        just build-container catnip:base 4 4096MB --no-cache
        echo "🐳 Force rebuilding dev container with Docker..."
        docker build --no-cache -f container/Dockerfile.dev -t catnip-dev:dev .
    fi
    echo "✅ Development container rebuilt!"

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
    @echo "  just build-container   - Build production container (Docker by default)"
    @echo "  just build-apple       - Build with Apple Container SDK"
    @echo "  just build-local       - Build for local dev only (no remote registry)"
    @echo "  just run               - Run container interactively"
    @echo "  just container-start   - Start Apple Container system service"
    @echo "  just container-stop    - Stop Apple Container system service" 
    @echo "  just container-status  - Check Apple Container system status"
    @echo "  just container-images  - List local container images"
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
    pnpm tsx scripts/release.ts {{ARGS}}

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
    echo "🔗 Backend PORT: ${PORT:-6369}"
    echo "🔗 Frontend VITE_PORT: $VITE_PORT"
    echo ""
    
    # Export VITE_PORT for both processes
    export VITE_PORT=$VITE_PORT
    export VITE_DEV_SERVER=http://localhost:$VITE_PORT
    
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
    echo "🚀 Starting Air (backend) on port ${PORT:-6369}..."
    export CATNIP_DEV=true
    cd container && air &
    AIR_PID=$!
    
    # Display helpful info
    echo ""
    echo "🎉 Development servers started!"
    echo "   📱 Frontend: http://localhost:$VITE_PORT"
    echo "   🔧 Backend:  http://localhost:${PORT:-6369}"
    echo "   📚 API Docs: http://localhost:${PORT:-6369}/docs/"
    echo ""
    echo "Press Ctrl+C to stop both servers"
    echo ""
    
    # Wait for either process to exit
    wait

# Upgrade container image to latest version and update wrangler.jsonc
upgrade-image:
    #!/usr/bin/env bash
    set -euo pipefail
    
    # Get latest version from git tags
    VERSION=$(git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//' || echo "0.1.0")
    echo "🏷️  Latest version: $VERSION"
    
    # Push container to Cloudflare registry
    echo "🚀 Pushing wandb/catnip:$VERSION to Cloudflare registry..."
    wrangler containers push wandb/catnip:$VERSION
    
    # Get the new registry URL
    NEW_IMAGE_URL="registry.cloudflare.com/0081e9dfbeb0e1a212ec5101e3c8768a/wandb/catnip:$VERSION"
    echo "📝 New image URL: $NEW_IMAGE_URL"
    
    # Update wrangler.jsonc with new image URL
    echo "🔄 Updating wrangler.jsonc..."
    
    # Create a temporary file for the updated content
    TMP_FILE=$(mktemp)
    
    # Use sed to replace all image URLs in wrangler.jsonc
    sed "s|registry\.cloudflare\.com/0081e9dfbeb0e1a212ec5101e3c8768a/wandb/catnip:[^\"]*|$NEW_IMAGE_URL|g" wrangler.jsonc > "$TMP_FILE"
    
    # Replace the original file
    mv "$TMP_FILE" wrangler.jsonc
    
    echo "✅ Updated container image references in wrangler.jsonc to version $VERSION"
    echo "🔍 Review changes with: git diff wrangler.jsonc"

# Default recipe
default:
    @just --list