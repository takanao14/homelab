#!/usr/bin/env bash
set -euo pipefail

[[ "$(uname)" == "Linux" ]] || exit 0

. /etc/os-release
readonly OS_ID="${ID}"

# renovate: datasource=github-releases depName=yuru7/udev-gothic
readonly UDEV_GOTHIC_VERSION="${UDEV_GOTHIC_VERSION:-2.2.0}"
# Install location. Defaults to a per-user prefix. Set TOOL_FONT_DIR to a
# system-wide path such as /usr/local/share/fonts to make the font available to
# every user (e.g. for a shared / golden-image VM); that requires running as
# root. fontconfig scans /usr/local/share/fonts by default. Also set
# TOOL_VERSION_CACHE_DIR=/usr/local/share/tool-versions so the baseline marker is
# recorded where per-user installs look for it; otherwise the deferral below
# silently no-ops (the marker lands in $HOME instead of SYSTEM_CACHE_DIR).
readonly VERSION_CACHE_DIR="${TOOL_VERSION_CACHE_DIR:-$HOME/.local/share/tool-versions}"
# A per-user install defers to a current system-wide baseline (golden image),
# so it does not shadow it with a duplicate in $HOME/.local.
readonly SYSTEM_CACHE_DIR="/usr/local/share/tool-versions"
readonly FONTS_DIR="${TOOL_FONT_DIR:-$HOME/.local/share/fonts}/udev-gothic"
readonly DOWNLOAD_URL="https://github.com/yuru7/udev-gothic/releases/download/v${UDEV_GOTHIC_VERSION}/UDEVGothic_NF_v${UDEV_GOTHIC_VERSION}.zip"

# ============================================================================
# Logging
# ============================================================================

readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly NC='\033[0m'

log_info()  { echo -e "${GREEN}[INFO]${NC} $*"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*" >&2; }

TMP_PATHS=()

cleanup_tmp_paths() {
    local path
    for path in "${TMP_PATHS[@]}"; do
        rm -rf "$path"
    done
}

trap cleanup_tmp_paths EXIT

# ============================================================================
# Helpers
# ============================================================================

make_tmp_dir() {
    local __var_name="$1" path
    path="$(mktemp -d)"
    TMP_PATHS+=("$path")
    printf -v "$__var_name" '%s' "$path"
}

check_gui() {
    local skip_msg="${1:-}"
    log_info "Checking system requirements..."
    # Golden-image builds (e.g. Packer) install the font before the xrdp
    # service runs; TOOL_FORCE_GUI_INSTALL=1 bypasses the live-GUI check.
    if [[ "${TOOL_FORCE_GUI_INSTALL:-}" == "1" ]]; then
        log_info "TOOL_FORCE_GUI_INSTALL=1; installing regardless of GUI session"
        return 0
    fi
    local has_gui=false
    if systemctl is-active --quiet xrdp 2>/dev/null; then
        log_info "xrdp service detected"
        has_gui=true
    fi
    if pgrep -E "^(weston|sway|wayfire|labwc|river|hyprland)$" >/dev/null 2>&1; then
        log_info "Wayland compositor detected"
        has_gui=true
    fi
    if [[ "$has_gui" == "false" ]]; then
        log_warn "No GUI session detected (xrdp or Wayland required)"
        [[ -n "$skip_msg" ]] && log_warn "$skip_msg"
        exit 0
    fi
}

check_dependencies() {
    local missing_deps=()
    for cmd in curl unzip fc-cache fc-list; do
        if ! command -v "$cmd" &>/dev/null; then
            missing_deps+=("$cmd")
        fi
    done
    if [[ ${#missing_deps[@]} -gt 0 ]]; then
        log_info "Installing missing dependencies: ${missing_deps[*]}"
        case "$OS_ID" in
            ubuntu|debian) sudo apt-get update -qq && sudo apt-get install -y fontconfig unzip curl ;;
            rocky)  sudo dnf install -y fontconfig unzip curl ;;
            *) log_error "Unsupported OS: ${OS_ID}"; return 1 ;;
        esac
    fi
}

is_font_installed() {
    local cache_file="${VERSION_CACHE_DIR}/udev-gothic"
    [[ "$(cat "$cache_file" 2>/dev/null)" == "$UDEV_GOTHIC_VERSION" ]] && \
        fc-list : family | grep -q "UDEV Gothic NF"
}

download_and_extract_font() {
    local tmp_dir="$1" zip_file="$1/udev-gothic.zip"
    log_info "Downloading UDEV Gothic NF ${UDEV_GOTHIC_VERSION}..."
    curl -fsSL --retry 3 --retry-delay 2 "$DOWNLOAD_URL" -o "$zip_file"
    log_info "Extracting fonts..."
    unzip -q "$zip_file" -d "$tmp_dir"
}

install_font_files() {
    local tmp_dir="$1"
    log_info "Installing font files to ${FONTS_DIR}..."
    mkdir -p "$FONTS_DIR"
    find "$tmp_dir" -type f \( -name "*.ttf" -o -name "*.otf" \) -exec cp {} "$FONTS_DIR/" \;
}

rebuild_font_cache() {
    log_info "Rebuilding font cache..."
    fc-cache -f "$FONTS_DIR"
}

# True when a system-wide baseline already provides KEY at VERSION. Only
# meaningful for a per-user install (our cache dir is not the system one).
baseline_satisfies() {
    local key="$1" version="$2"
    [[ "$VERSION_CACHE_DIR" != "$SYSTEM_CACHE_DIR" ]] || return 1
    [[ "$(cat "${SYSTEM_CACHE_DIR}/${key}" 2>/dev/null)" == "$version" ]]
}

install_udev_gothic() {
    log_info "Installing UDEV Gothic NF font..."
    if baseline_satisfies "udev-gothic" "$UDEV_GOTHIC_VERSION" && \
       fc-list : family | grep -q "UDEV Gothic NF"; then
        log_info "UDEV Gothic NF ${UDEV_GOTHIC_VERSION} provided system-wide, skipping per-user install"
        return 0
    fi
    if is_font_installed; then
        log_info "UDEV Gothic NF ${UDEV_GOTHIC_VERSION} is already installed"
        return 0
    fi
    check_dependencies
    local tmp_dir
    make_tmp_dir tmp_dir
    download_and_extract_font "$tmp_dir"
    install_font_files "$tmp_dir"
    rebuild_font_cache
    mkdir -p "$VERSION_CACHE_DIR"
    echo "$UDEV_GOTHIC_VERSION" > "${VERSION_CACHE_DIR}/udev-gothic"
    log_info "UDEV Gothic NF installed successfully"
}

main() {
    log_info "=== Font Installation Script ==="
    check_gui "Skipping font installation"
    install_udev_gothic
    log_info "=== Font installation completed ==="
}

main "$@"
