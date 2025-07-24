#!/bin/bash
set -e

# Script to find the latest stable versions of development languages
# and update the Dockerfile ARG defaults

echo "🔍 Finding latest stable versions..."

# Function to get latest Node.js LTS version
get_latest_node() {
    curl -s https://nodejs.org/dist/index.json | jq -r '[.[] | select(.lts != false)] | .[0].version' | sed 's/v//'
}

# Function to get latest Python version
get_latest_python() {
    curl -s https://endoflife.date/api/python.json | jq -r '.[0].latest'
}

# Function to get latest Rust version
get_latest_rust() {
    curl -s https://api.github.com/repos/rust-lang/rust/releases/latest | jq -r '.tag_name' | sed 's/^[^0-9]*//'
}

# Function to get latest Go version
get_latest_go() {
    curl -s "https://go.dev/dl/?mode=json" | jq -r '.[0].version' | sed 's/go//'
}

# Function to get latest NVM version
get_latest_nvm() {
    curl -s https://api.github.com/repos/nvm-sh/nvm/releases/latest | jq -r '.tag_name' | sed 's/v//'
}


echo "📦 Fetching version information..."

echo "  Fetching Node.js version..."
NODE_VERSION=$(get_latest_node)
echo "  Fetching Python version..."
PYTHON_VERSION=$(get_latest_python)
echo "  Fetching Rust version..."
RUST_VERSION=$(get_latest_rust)
echo "  Fetching Go version..."
GO_VERSION=$(get_latest_go)
echo "  Fetching NVM version..."
NVM_VERSION=$(get_latest_nvm)

echo "✅ Latest versions found:"
echo "  Node.js: $NODE_VERSION"
echo "  Python: $PYTHON_VERSION"
echo "  Rust: $RUST_VERSION"
echo "  Go: $GO_VERSION"
echo "  NVM: $NVM_VERSION"

# Update Dockerfile with new versions
DOCKERFILE_PATH="$(dirname "$0")/../container/Dockerfile"

if [ -f "$DOCKERFILE_PATH" ]; then
    echo "🔧 Updating Dockerfile ARG defaults..."
    
    # Create a backup
    cp "$DOCKERFILE_PATH" "$DOCKERFILE_PATH.backup"
    
    # Update versions using sed
    sed -i.tmp \
        -e "s/ARG NODE_VERSION=.*/ARG NODE_VERSION=$NODE_VERSION/" \
        -e "s/ARG PYTHON_VERSION=.*/ARG PYTHON_VERSION=$PYTHON_VERSION/" \
        -e "s/ARG RUST_VERSION=.*/ARG RUST_VERSION=$RUST_VERSION/" \
        -e "s/ARG GO_VERSION=.*/ARG GO_VERSION=$GO_VERSION/" \
        -e "s/ARG NVM_VERSION=.*/ARG NVM_VERSION=$NVM_VERSION/" \
        "$DOCKERFILE_PATH"
    
    # Remove temporary file
    rm "$DOCKERFILE_PATH.tmp"
    
    echo "✅ Dockerfile updated successfully!"
    echo "📄 Backup saved as: $DOCKERFILE_PATH.backup"
else
    echo "❌ Dockerfile not found at: $DOCKERFILE_PATH"
    exit 1
fi

echo "🎉 Version update complete!"
echo ""
echo "💡 To build with these versions:"
echo "   docker build -t catnip-dev container/"
echo ""
echo "💡 To restore previous version:"
echo "   mv $DOCKERFILE_PATH.backup $DOCKERFILE_PATH"