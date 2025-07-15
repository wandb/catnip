#!/usr/bin/env bash

# Catnip CLI (catctrl) Installation Script
# 
# This script downloads and installs the latest version of catctrl
# from GitHub releases with SHA256 verification.
#
# Usage:
#   curl -sfL https://raw.githubusercontent.com/wandb/catnip/main/public/install.sh | sh
#   curl -sfL https://raw.githubusercontent.com/wandb/catnip/main/public/install.sh | sh -s -- --version v1.0.0
#   curl -sfL https://raw.githubusercontent.com/wandb/catnip/main/public/install.sh | INSTALL_DIR=/usr/local/bin sh

set -euo pipefail

# Default configuration
GITHUB_REPO="wandb/catnip"
BINARY_NAME="catctrl"
INSTALL_DIR="${INSTALL_DIR:-${HOME}/.local/bin}"
VERSION="${1:-latest}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log() {
    echo -e "${BLUE}[INFO]${NC} $*" >&2
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $*" >&2
}

error() {
    echo -e "${RED}[ERROR]${NC} $*" >&2
}

success() {
    echo -e "${GREEN}[SUCCESS]${NC} $*" >&2
}

fatal() {
    error "$@"
    exit 1
}

# Check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Verify required commands are available
verify_dependencies() {
    local missing_deps=()
    
    for cmd in curl tar; do
        if ! command_exists "$cmd"; then
            missing_deps+=("$cmd")
        fi
    done
    
    if [[ ${#missing_deps[@]} -gt 0 ]]; then
        fatal "Missing required dependencies: ${missing_deps[*]}"
    fi
    
    # Check for checksum command (sha256sum or shasum)
    if ! command_exists "sha256sum" && ! command_exists "shasum"; then
        fatal "Missing required checksum command: sha256sum or shasum"
    fi
}

# Detect operating system
detect_os() {
    local os
    case "$(uname -s)" in
        Linux*)     os="linux";;
        Darwin*)    os="darwin";;
        CYGWIN*)    fatal "Windows is not currently supported. Catnip requires Docker and containers, which work best on Linux/macOS. Consider using WSL2 on Windows.";;
        MINGW*)     fatal "Windows is not currently supported. Catnip requires Docker and containers, which work best on Linux/macOS. Consider using WSL2 on Windows.";;
        MSYS*)      fatal "Windows is not currently supported. Catnip requires Docker and containers, which work best on Linux/macOS. Consider using WSL2 on Windows.";;
        *)          fatal "Unsupported operating system: $(uname -s)";;
    esac
    echo "$os"
}

# Detect architecture
detect_arch() {
    local arch
    case "$(uname -m)" in
        x86_64|amd64)   arch="amd64";;
        i386|i686)      arch="386";;
        aarch64|arm64)  arch="arm64";;
        armv7l)         arch="armv7";;
        armv6l)         arch="armv6";;
        *)              fatal "Unsupported architecture: $(uname -m)";;
    esac
    echo "$arch"
}

# Get latest release version from GitHub API
get_latest_version() {
    local api_url="https://api.github.com/repos/${GITHUB_REPO}/releases/latest"
    local version
    
    log "Fetching latest release information..."
    
    if command_exists "jq"; then
        version=$(curl -sfL "$api_url" | jq -r '.tag_name')
    else
        # Fallback without jq
        version=$(curl -sfL "$api_url" | grep '"tag_name":' | sed -E 's/.*"tag_name": "([^"]+)".*/\1/')
    fi
    
    if [[ -z "$version" || "$version" == "null" ]]; then
        fatal "Failed to get latest version from GitHub API"
    fi
    
    echo "$version"
}

# Download file with verification
download_file() {
    local url="$1"
    local output="$2"
    local description="$3"
    
    log "Downloading $description..."
    log "URL: $url"
    
    if ! curl -sfL -o "$output" "$url"; then
        fatal "Failed to download $description from $url"
    fi
}

# Verify SHA256 checksum
verify_checksum() {
    local file="$1"
    local expected_checksum="$2"
    local actual_checksum
    
    log "Verifying checksum for $(basename "$file")..."
    
    if command_exists "sha256sum"; then
        actual_checksum=$(sha256sum "$file" | cut -d' ' -f1)
    elif command_exists "shasum"; then
        actual_checksum=$(shasum -a 256 "$file" | cut -d' ' -f1)
    else
        fatal "No checksum command available"
    fi
    
    if [[ "$actual_checksum" != "$expected_checksum" ]]; then
        fatal "Checksum verification failed!\nExpected: $expected_checksum\nActual: $actual_checksum"
    fi
    
    success "Checksum verified successfully"
}

# Create installation directory
create_install_dir() {
    if [[ ! -d "$INSTALL_DIR" ]]; then
        log "Creating installation directory: $INSTALL_DIR"
        if ! mkdir -p "$INSTALL_DIR"; then
            fatal "Failed to create installation directory: $INSTALL_DIR"
        fi
    fi
}

# Check if installation directory is in PATH
check_path() {
    if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
        warn "Installation directory $INSTALL_DIR is not in your PATH"
        warn "Add it to your PATH by adding this line to your shell profile:"
        warn "  export PATH=\"$INSTALL_DIR:\$PATH\""
    fi
}

# Main installation function
install_catctrl() {
    local os arch version binary_url checksum_url
    local temp_dir binary_file checksum_file
    local expected_checksum
    
    # Detect system
    os=$(detect_os)
    arch=$(detect_arch)
    
    log "Detected system: $os/$arch"
    
    # Get version
    if [[ "$VERSION" == "latest" ]]; then
        version=$(get_latest_version)
    else
        version="$VERSION"
    fi
    
    log "Installing catctrl version: $version"
    
    # Construct download URLs (GoReleaser format)
    local base_url="https://github.com/${GITHUB_REPO}/releases/download/${version}"
    local archive_name="${BINARY_NAME}_${version#v}_${os}_${arch}.tar.gz"
    local checksum_name="checksums.txt"
    
    binary_url="${base_url}/${archive_name}"
    checksum_url="${base_url}/${checksum_name}"
    
    # Create temporary directory
    temp_dir=$(mktemp -d)
    trap "rm -rf '$temp_dir'" EXIT
    
    binary_file="$temp_dir/$archive_name"
    checksum_file="$temp_dir/$checksum_name"
    
    # Download files
    download_file "$binary_url" "$binary_file" "binary archive"
    download_file "$checksum_url" "$checksum_file" "checksums"
    
    # Extract expected checksum
    expected_checksum=$(grep "$archive_name" "$checksum_file" | cut -d' ' -f1)
    if [[ -z "$expected_checksum" ]]; then
        fatal "Could not find checksum for $archive_name in checksums file"
    fi
    
    # Verify checksum
    verify_checksum "$binary_file" "$expected_checksum"
    
    # Extract and install
    log "Extracting and installing catctrl..."
    create_install_dir
    
    # Extract tar.gz archive
    if ! tar -xzf "$binary_file" -C "$temp_dir"; then
        fatal "Failed to extract tar.gz archive"
    fi
    
    local extracted_binary="$temp_dir/$BINARY_NAME"
    if [[ ! -f "$extracted_binary" ]]; then
        fatal "Binary not found in archive: $BINARY_NAME"
    fi
    
    # Install binary
    local install_path="$INSTALL_DIR/$BINARY_NAME"
    if ! cp "$extracted_binary" "$install_path"; then
        fatal "Failed to install binary to $install_path"
    fi
    
    # Make executable
    if ! chmod +x "$install_path"; then
        fatal "Failed to make binary executable"
    fi
    
    success "catctrl installed successfully to $install_path"
    
    # Verify installation
    if command_exists "$BINARY_NAME"; then
        local installed_version
        installed_version=$("$BINARY_NAME" --version 2>/dev/null || "$BINARY_NAME" version 2>/dev/null || echo "unknown")
        success "Installation verified: $installed_version"
    else
        check_path
    fi
    
    log ""
    log "Installation complete! You can now use catctrl:"
    log "  $BINARY_NAME --help"
    log ""
    log "If the command is not found, make sure $INSTALL_DIR is in your PATH."
}

# Parse command line arguments
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            --version)
                VERSION="$2"
                shift 2
                ;;
            --install-dir)
                INSTALL_DIR="$2"
                shift 2
                ;;
            --help|-h)
                cat << 'EOF'
Catnip CLI (catctrl) Installation Script

USAGE:
    install.sh [OPTIONS]

OPTIONS:
    --version VERSION       Install specific version (default: latest)
    --install-dir DIR       Installation directory (default: ~/.local/bin)
    --help, -h              Show this help message

ENVIRONMENT VARIABLES:
    INSTALL_DIR            Installation directory (same as --install-dir)

EXAMPLES:
    # Install latest version
    curl -sfL https://raw.githubusercontent.com/wandb/catnip/main/public/install.sh | sh

    # Install specific version
    curl -sfL https://raw.githubusercontent.com/wandb/catnip/main/public/install.sh | sh -s -- --version v1.0.0

    # Install to custom directory
    curl -sfL https://raw.githubusercontent.com/wandb/catnip/main/public/install.sh | INSTALL_DIR=/usr/local/bin sh

EOF
                exit 0
                ;;
            *)
                fatal "Unknown option: $1"
                ;;
        esac
    done
}

# Main execution
main() {
    log "Catnip CLI (catctrl) Installation Script"
    log "========================================"
    
    parse_args "$@"
    verify_dependencies
    install_catctrl
    
    success "Installation completed successfully!"
}

# Run main function with all arguments
main "$@"