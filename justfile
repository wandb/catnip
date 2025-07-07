# Catnip Development Container Management

# Build the catnip container for the current platform
build:
    @echo "🏗️  Building catnip container for current platform..."
    docker build -t catnip-dev container/
    @echo "✅ Build complete! Run with: docker run -it catnip-dev"

# Update language versions to latest stable and rebuild
update-versions:
    @echo "🔄 Updating language versions..."
    ./scripts/update-versions.sh

# Build for multiple architectures (requires buildx)
build-multi:
    @echo "🏗️  Building catnip container for multiple architectures..."
    docker buildx build --platform linux/amd64,linux/arm64 -t catnip-dev container/
    @echo "✅ Multi-arch build complete!"

# Run the container interactively
run:
    @echo "🚀 Starting catnip container..."
    docker run -it --rm catnip-dev

# Run the container with a custom command
run-cmd CMD:
    @echo "🚀 Running command in catnip container: {{CMD}}"
    docker run -it --rm catnip-dev {{CMD}}

# Clean up container images
clean:
    @echo "🧹 Cleaning up catnip container images..."
    docker rmi catnip-dev 2>/dev/null || true
    @echo "✅ Cleanup complete!"

# Show container information
info:
    @echo "📋 Catnip Container Information:"
    @echo "  Image: catnip-dev"
    @echo "  Platform: $(uname -m)"
    @echo "  Build context: ./container/"
    @echo ""
    @echo "Available commands:"
    @echo "  just build         - Build container"
    @echo "  just update-versions - Update versions and rebuild"
    @echo "  just run           - Run container interactively"
    @echo "  just clean         - Remove container images"

# Default recipe
default:
    @just --list