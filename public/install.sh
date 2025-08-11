#!/usr/bin/env bash

# Catnip CLI Installation Script
# 
# This script downloads and installs the latest version of catnip
# from GitHub releases with SHA256 verification.
#
# Usage:
#   curl -sSfL install.catnip.sh | sh
#   curl -sSfL install.catnip.sh | sh -s -- --version v1.0.0
#   curl -sSfL install.catnip.sh | INSTALL_DIR=/usr/local/bin sh

set -euo pipefail

# Default configuration
GITHUB_REPO="wandb/catnip"
BINARY_NAME="catnip"
INSTALL_DIR="${INSTALL_DIR:-${HOME}/.local/bin}"
VERSION="${1:-latest}"
CATNIP_PROXY_URL="${CATNIP_PROXY_URL:-https://install.catnip.sh}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log() {
    printf "${BLUE}[INFO]${NC} %s\n" "$*" >&2
}

warn() {
    printf "${YELLOW}[WARN]${NC} %s\n" "$*" >&2
}

error() {
    printf "${RED}[ERROR]${NC} %s\n" "$*" >&2
}

success() {
    printf "${GREEN}[SUCCESS]${NC} %s\n" "$*" >&2
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

# Get latest release version from Catnip proxy
get_latest_version() {
    local api_url="${CATNIP_PROXY_URL}/v1/github/releases/latest"
    local version
    
    log "Fetching latest release information..."
    
    if command_exists "jq"; then
        version=$(curl -sfL "$api_url" | jq -r '.tag_name')
    else
        # Fallback without jq
        version=$(curl -sfL "$api_url" | grep '"tag_name":' | sed -E 's/.*"tag_name": "([^"]+)".*/\1/')
    fi
    
    if [[ -z "$version" || "$version" == "null" ]]; then
        fatal "Failed to get latest version from Catnip proxy"
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
install_catnip() {
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
    
    log "Installing catnip version: $version"
    
    # Construct download URLs (GoReleaser format) - using proxy
    local base_url="${CATNIP_PROXY_URL}/v1/github/releases/download/${version}"
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
    log "Extracting and installing catnip..."
    create_install_dir
    
    # Extract tar.gz archive
    if ! tar -xzf "$binary_file" -C "$temp_dir"; then
        fatal "Failed to extract tar.gz archive"
    fi
    
    # Handle macOS app bundle vs standard binary
    if [[ "$os" == "darwin" ]]; then
        # macOS: Look for app bundle structure
        if [[ -d "$temp_dir/Catnip.app" ]]; then
            log "Installing macOS app bundle..."
            
            # Use ~/Library/Application Support for the app bundle (standard location for CLI support files)
            local app_support_dir="${HOME}/Library/Application Support/catnip"
            local app_bundle_path="${app_support_dir}/Catnip.app"
            
            # Clean up old app bundle from incorrect location (if it exists)
            local old_app_location="$INSTALL_DIR/Catnip.app"
            if [[ -d "$old_app_location" ]]; then
                log "Cleaning up old app bundle from $INSTALL_DIR..."
                rm -rf "$old_app_location"
            fi
            
            # Create Application Support directory if needed
            if [[ ! -d "$app_support_dir" ]]; then
                log "Creating Application Support directory..."
                if ! mkdir -p "$app_support_dir"; then
                    fatal "Failed to create Application Support directory: $app_support_dir"
                fi
            fi
            
            # Remove existing app bundle if present
            if [[ -d "$app_bundle_path" ]]; then
                log "Removing existing app bundle..."
                rm -rf "$app_bundle_path"
            fi
            
            # Copy app bundle to Application Support
            if ! cp -R "$temp_dir/Catnip.app" "$app_bundle_path"; then
                fatal "Failed to install app bundle to $app_bundle_path"
            fi
            
            # Create CLI wrapper in the install directory that calls the app bundle
            local install_path="$INSTALL_DIR/$BINARY_NAME"
            cat > "$install_path" << EOF
#!/bin/bash
# Catnip CLI wrapper - calls the binary inside the app bundle
exec "${app_bundle_path}/Contents/MacOS/catnip" "\$@"
EOF
            
            # Make wrapper executable
            if ! chmod +x "$install_path"; then
                fatal "Failed to make CLI wrapper executable"
            fi
            
            success "macOS app bundle installed to Application Support with CLI wrapper in $INSTALL_DIR"
        else
            # Fallback: Look for standalone binary (older releases)
            local extracted_binary="$temp_dir/$BINARY_NAME"
            if [[ ! -f "$extracted_binary" ]]; then
                fatal "Neither app bundle nor standalone binary found in archive"
            fi
            
            # Install standalone binary
            local install_path="$INSTALL_DIR/$BINARY_NAME"
            if ! cp "$extracted_binary" "$install_path"; then
                fatal "Failed to install binary to $install_path"
            fi
            
            # Make executable
            if ! chmod +x "$install_path"; then
                fatal "Failed to make binary executable"
            fi
            
            warn "Installed standalone binary (older release without native notifications)"
        fi
    else
        # Linux/FreeBSD: Standard binary installation
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
    fi
    
    success "catnip installed successfully to $INSTALL_DIR"
    
    # Verify installation
    if command_exists "$BINARY_NAME"; then
        local installed_version
        installed_version=$("$BINARY_NAME" --version 2>/dev/null || "$BINARY_NAME" version 2>/dev/null || echo "unknown")
        success "Installation verified: $installed_version"
    else
        check_path
    fi
    
    log ""
    log "Installation complete! You can now use catnip:"
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
Catnip CLI Installation Script

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
    curl -sSfL install.catnip.sh | sh

    # Install specific version
    curl -sSfL install.catnip.sh | sh -s -- --version v1.0.0

    # Install to custom directory
    curl -sSfL install.catnip.sh | INSTALL_DIR=/usr/local/bin sh

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
    log "Catnip CLI Installation Script"
    log "========================================"
    
    parse_args "$@"
    verify_dependencies
    install_catnip
    
    success "Installation completed successfully!"
}

# Run main function with all arguments
main "$@"