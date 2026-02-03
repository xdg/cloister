#!/bin/sh
# Cloister installer
# Usage: curl -fsSL https://raw.githubusercontent.com/xdg/cloister/main/install.sh | sh
#
# Environment variables:
#   VERSION  - Specific version to install (e.g., v1.0.0). Defaults to latest.
#   INSTALL_DIR - Installation directory. Defaults to ~/.local/bin.

set -e

# Configuration
REPO="xdg/cloister"
BINARY_NAME="cloister"
DEFAULT_INSTALL_DIR="$HOME/.local/bin"

# Colors (disabled if not a terminal)
if [ -t 1 ]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[0;33m'
    BLUE='\033[0;34m'
    NC='\033[0m' # No Color
else
    RED=''
    GREEN=''
    YELLOW=''
    BLUE=''
    NC=''
fi

info() {
    printf "${BLUE}==>${NC} %s\n" "$1"
}

success() {
    printf "${GREEN}==>${NC} %s\n" "$1"
}

warn() {
    printf "${YELLOW}Warning:${NC} %s\n" "$1"
}

error() {
    printf "${RED}Error:${NC} %s\n" "$1" >&2
    exit 1
}

# Detect OS
detect_os() {
    case "$(uname -s)" in
        Linux*)  echo "linux" ;;
        Darwin*) echo "darwin" ;;
        *)       error "Unsupported operating system: $(uname -s)" ;;
    esac
}

# Detect architecture
detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64)  echo "amd64" ;;
        aarch64|arm64) echo "arm64" ;;
        *)             error "Unsupported architecture: $(uname -m)" ;;
    esac
}

# Get latest version from GitHub API
get_latest_version() {
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/'
    elif command -v wget >/dev/null 2>&1; then
        wget -qO- "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/'
    else
        error "Neither curl nor wget found. Please install one of them."
    fi
}

# Download file
download() {
    url="$1"
    output="$2"

    if command -v curl >/dev/null 2>&1; then
        curl -fsSL "$url" -o "$output"
    elif command -v wget >/dev/null 2>&1; then
        wget -q "$url" -O "$output"
    else
        error "Neither curl nor wget found. Please install one of them."
    fi
}

# Check if directory is in PATH
in_path() {
    case ":$PATH:" in
        *":$1:"*) return 0 ;;
        *)        return 1 ;;
    esac
}

# Get shell config file
get_shell_config() {
    shell_name="$(basename "$SHELL")"
    case "$shell_name" in
        bash)
            if [ -f "$HOME/.bashrc" ]; then
                echo "$HOME/.bashrc"
            elif [ -f "$HOME/.bash_profile" ]; then
                echo "$HOME/.bash_profile"
            else
                echo "$HOME/.bashrc"
            fi
            ;;
        zsh)
            echo "$HOME/.zshrc"
            ;;
        fish)
            echo "$HOME/.config/fish/config.fish"
            ;;
        *)
            echo ""
            ;;
    esac
}

# Add directory to PATH in shell config
add_to_path() {
    dir="$1"
    config_file="$2"
    shell_name="$(basename "$SHELL")"

    case "$shell_name" in
        fish)
            echo "" >> "$config_file"
            echo "# Added by cloister installer" >> "$config_file"
            echo "fish_add_path $dir" >> "$config_file"
            ;;
        *)
            echo "" >> "$config_file"
            echo "# Added by cloister installer" >> "$config_file"
            echo "export PATH=\"$dir:\$PATH\"" >> "$config_file"
            ;;
    esac
}

# Main installation
main() {
    info "Installing cloister..."

    # Detect platform
    OS="$(detect_os)"
    ARCH="$(detect_arch)"
    info "Detected platform: ${OS}/${ARCH}"

    # Determine version
    if [ -n "$VERSION" ]; then
        # Ensure version starts with 'v'
        case "$VERSION" in
            v*) ;;
            *)  VERSION="v$VERSION" ;;
        esac
    else
        info "Fetching latest version..."
        VERSION="$(get_latest_version)"
        if [ -z "$VERSION" ]; then
            error "Failed to determine latest version"
        fi
    fi
    info "Installing version: ${VERSION}"

    # Determine install directory
    INSTALL_DIR="${INSTALL_DIR:-$DEFAULT_INSTALL_DIR}"

    # Create install directory if needed
    if [ ! -d "$INSTALL_DIR" ]; then
        info "Creating directory: ${INSTALL_DIR}"
        mkdir -p "$INSTALL_DIR"
    fi

    # Build download URL
    # Version in filename excludes 'v' prefix for GoReleaser default template
    VERSION_NUM="${VERSION#v}"
    ARCHIVE_NAME="${BINARY_NAME}_${VERSION_NUM}_${OS}_${ARCH}.tar.gz"
    DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${ARCHIVE_NAME}"

    # Create temp directory
    TMP_DIR="$(mktemp -d)"
    trap 'rm -rf "$TMP_DIR"' EXIT

    # Download archive
    info "Downloading ${DOWNLOAD_URL}..."
    download "$DOWNLOAD_URL" "$TMP_DIR/$ARCHIVE_NAME"

    # Extract binary
    info "Extracting..."
    tar -xzf "$TMP_DIR/$ARCHIVE_NAME" -C "$TMP_DIR"

    # Install binary
    if [ -f "$TMP_DIR/$BINARY_NAME" ]; then
        mv "$TMP_DIR/$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"
    else
        error "Binary not found in archive"
    fi
    chmod +x "$INSTALL_DIR/$BINARY_NAME"

    success "Installed ${BINARY_NAME} to ${INSTALL_DIR}/${BINARY_NAME}"

    # Check PATH
    if ! in_path "$INSTALL_DIR"; then
        echo ""
        warn "${INSTALL_DIR} is not in your PATH"

        config_file="$(get_shell_config)"
        shell_name="$(basename "$SHELL")"

        if [ -n "$config_file" ]; then
            printf "Add it to your %s now? [y/N] " "$config_file"
            read -r response
            case "$response" in
                [yY]|[yY][eE][sS])
                    add_to_path "$INSTALL_DIR" "$config_file"
                    success "Added to ${config_file}"
                    echo ""
                    echo "Run this to update your current shell:"
                    case "$shell_name" in
                        fish)
                            echo "  source $config_file"
                            ;;
                        *)
                            echo "  export PATH=\"$INSTALL_DIR:\$PATH\""
                            ;;
                    esac
                    ;;
                *)
                    echo ""
                    echo "To add manually, add this to your shell config:"
                    case "$shell_name" in
                        fish)
                            echo "  fish_add_path $INSTALL_DIR"
                            ;;
                        *)
                            echo "  export PATH=\"$INSTALL_DIR:\$PATH\""
                            ;;
                    esac
                    ;;
            esac
        else
            echo ""
            echo "Add this to your shell configuration file:"
            echo "  export PATH=\"$INSTALL_DIR:\$PATH\""
        fi
    fi

    echo ""
    success "Installation complete!"
    echo ""
    echo "Next steps:"
    echo "  1. Run 'cloister setup claude' to configure credentials"
    echo "  2. Navigate to a git repository and run 'cloister start'"
    echo ""
    echo "Documentation: https://github.com/${REPO}#readme"
}

main "$@"
