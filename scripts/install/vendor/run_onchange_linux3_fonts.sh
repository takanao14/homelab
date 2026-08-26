#!/usr/bin/env bash
set -euo pipefail

[[ "$(uname)" == "Linux" ]] || exit 0

# renovate: datasource=github-releases depName=yuru7/udev-gothic
readonly UDEV_GOTHIC_VERSION="${UDEV_GOTHIC_VERSION:-2.2.0}"
# Defaults to per-user paths; a system-wide TOOL_FONT_DIR requires root.
# Use /usr/local/share/tool-versions so per-user installs see the baseline.
readonly VERSION_CACHE_DIR="${TOOL_VERSION_CACHE_DIR:-$HOME/.local/share/tool-versions}"
# A per-user install defers to a current system-wide baseline (golden image),
# so it does not shadow it with a duplicate in $HOME/.local.
readonly SYSTEM_CACHE_DIR="/usr/local/share/tool-versions"
readonly FONTS_DIR="${TOOL_FONT_DIR:-$HOME/.local/share/fonts}/udev-gothic"
readonly DOWNLOAD_URL="https://github.com/yuru7/udev-gothic/releases/download/v${UDEV_GOTHIC_VERSION}/UDEVGothic_NF_v${UDEV_GOTHIC_VERSION}.zip"

# Logging

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

# Helpers

make_tmp_dir() {
    local __var_name="$1" path
    path="$(mktemp -d)"
    TMP_PATHS+=("$path")
    printf -v "$__var_name" '%s' "$path"
}

is_desktop_machine() {
    local skip_msg="${1:-}"
    local profile="${TOOL_MACHINE_PROFILE:-auto}"

    case "$profile" in
        desktop)
            log_info "Desktop machine profile selected"
            return 0
            ;;
        server)
            log_info "Server machine profile selected"
            [[ -n "$skip_msg" ]] && log_info "$skip_msg"
            return 1
            ;;
        auto) ;;
        *)
            log_error "TOOL_MACHINE_PROFILE must be desktop, server, auto, or unset (got: ${profile})"
            exit 1
            ;;
    esac

    # Backward-compatible image-build override. New callers should pass the
    # explicit desktop profile instead.
    if [[ "${TOOL_FORCE_GUI_INSTALL:-}" == "1" ]]; then
        log_warn "TOOL_FORCE_GUI_INSTALL is deprecated; use TOOL_MACHINE_PROFILE=desktop"
        return 0
    fi

    local profile_file="/etc/homelab/machine-profile"
    if [[ -r "$profile_file" ]]; then
        profile="$(<"$profile_file")"
        case "$profile" in
            desktop)
                log_info "Desktop machine profile read from ${profile_file}"
                return 0
                ;;
            server)
                log_info "Server machine profile read from ${profile_file}"
                [[ -n "$skip_msg" ]] && log_info "$skip_msg"
                return 1
                ;;
            *)
                log_error "Invalid machine profile in ${profile_file}: ${profile}"
                exit 1
                ;;
        esac
    fi

    log_warn "No explicit machine profile or image marker; defaulting to server"
    [[ -n "$skip_msg" ]] && log_warn "$skip_msg"
    return 1
}

# This script never calls sudo; linux0 must provide its OS-level dependencies.
check_dependencies() {
    local missing_deps=()
    for cmd in curl unzip fc-cache fc-list; do
        if ! command -v "$cmd" &>/dev/null; then
            missing_deps+=("$cmd")
        fi
    done
    if [[ ${#missing_deps[@]} -gt 0 ]]; then
        log_error "Missing dependencies: ${missing_deps[*]}"
        log_error "Run run_onchange_linux0_package.sh first (it installs curl, unzip and fontconfig)."
        exit 1
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
    is_desktop_machine "Skipping font installation" || return 0
    install_udev_gothic
    log_info "=== Font installation completed ==="
}

main "$@"
