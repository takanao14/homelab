#!/usr/bin/env bash
set -euo pipefail

[[ "$(uname)" == "Linux" ]] || exit 0

# renovate: datasource=github-releases depName=kovidgoyal/kitty
readonly KITTY_VERSION="${KITTY_VERSION:-0.47.1}"

# Install location. Defaults to a per-user prefix. Override TOOL_BIN_DIR /
# TOOL_KITTY_PREFIX / TOOL_APPS_DIR / TOOL_VERSION_CACHE_DIR with system-wide
# paths (e.g. /usr/local) to make kitty available to every user (golden-image
# VM); requires running as root. Point TOOL_VERSION_CACHE_DIR at
# /usr/local/share/tool-versions so the baseline marker is recorded where
# per-user installs look for it; otherwise the deferral below silently no-ops.
readonly BIN_DIR="${TOOL_BIN_DIR:-$HOME/.local/bin}"
readonly VERSION_CACHE_DIR="${TOOL_VERSION_CACHE_DIR:-$HOME/.local/share/tool-versions}"
# A per-user install defers to a current system-wide baseline (golden image),
# so it does not shadow it with a duplicate in $HOME/.local.
readonly SYSTEM_CACHE_DIR="/usr/local/share/tool-versions"
readonly KITTY_PREFIX="${TOOL_KITTY_PREFIX:-$HOME/.local}"
readonly APPS_DIR="${TOOL_APPS_DIR:-$HOME/.local/share/applications}"
readonly KITTY_APP="${KITTY_PREFIX}/kitty.app"

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
    # Golden-image builds (e.g. Packer) install before the xrdp service runs;
    # TOOL_FORCE_GUI_INSTALL=1 bypasses the live-GUI check.
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

# ============================================================================
# Kitty
# ============================================================================

install_kitty() {
    log_info "Installing kitty ${KITTY_VERSION}..."

    local arch
    case "$(uname -m)" in
        x86_64)  arch="x86_64" ;;
        aarch64) arch="arm64" ;;
        *) log_error "Unsupported architecture: $(uname -m)"; exit 1 ;;
    esac

    local tmp_dir
    make_tmp_dir tmp_dir

    curl -fsSL "https://github.com/kovidgoyal/kitty/releases/download/v${KITTY_VERSION}/kitty-${KITTY_VERSION}-${arch}.txz" \
        -o "${tmp_dir}/kitty.txz"

    rm -rf "$KITTY_APP"
    mkdir -p "$KITTY_APP"
    tar xJf "${tmp_dir}/kitty.txz" -C "$KITTY_APP"

    mkdir -p "$BIN_DIR"
    ln -sf "$KITTY_APP/bin/kitty"  "$BIN_DIR/kitty"
    ln -sf "$KITTY_APP/bin/kitten" "$BIN_DIR/kitten"

    mkdir -p "$APPS_DIR"
    cp "$KITTY_APP/share/applications/kitty.desktop" \
        "$APPS_DIR/kitty.desktop"
    cp "$KITTY_APP/share/applications/kitty-open.desktop" \
        "$APPS_DIR/kitty-open.desktop"
    sed -i "s|Icon=kitty|Icon=$KITTY_APP/share/icons/hicolor/256x256/apps/kitty.png|g" \
        "$APPS_DIR/kitty.desktop" \
        "$APPS_DIR/kitty-open.desktop"
    sed -i "s|Exec=kitty|Exec=$KITTY_APP/bin/kitty|g" \
        "$APPS_DIR/kitty.desktop" \
        "$APPS_DIR/kitty-open.desktop"

    mkdir -p "$VERSION_CACHE_DIR"
    echo "$KITTY_VERSION" > "$VERSION_CACHE_DIR/kitty"

    log_info "kitty ${KITTY_VERSION} installed"
}

# True when a system-wide baseline already provides KEY at VERSION. Only
# meaningful for a per-user install (our cache dir is not the system one).
baseline_satisfies() {
    local key="$1" version="$2"
    [[ "$VERSION_CACHE_DIR" != "$SYSTEM_CACHE_DIR" ]] || return 1
    [[ "$(cat "${SYSTEM_CACHE_DIR}/${key}" 2>/dev/null)" == "$version" ]]
}

main() {
    log_info "=== Terminal Installation Script ==="

    check_gui "Skipping kitty installation"

    if baseline_satisfies "kitty" "$KITTY_VERSION" && command -v kitty &>/dev/null; then
        log_info "kitty ${KITTY_VERSION} provided system-wide, skipping per-user install"
        exit 0
    fi

    local cache_file="$VERSION_CACHE_DIR/kitty"
    if command -v kitty &>/dev/null && \
       [[ "$(cat "$cache_file" 2>/dev/null)" == "$KITTY_VERSION" ]]; then
        log_info "kitty ${KITTY_VERSION} is already up to date, skipping"
        exit 0
    fi

    install_kitty

    log_info "=== Installation completed successfully ==="
}

main "$@"
