#!/usr/bin/env bash
set -euo pipefail

[[ "$(uname)" == "Linux" ]] || exit 0

# shellcheck source=/dev/null
. /etc/os-release
readonly OS_ID="${ID}"

# kubectl pins a repository minor; openbao pins its package asset.
# renovate: datasource=github-releases depName=kubernetes/kubernetes
readonly KUBECTL_VERSION="${KUBECTL_VERSION:-1.36}"
# renovate: datasource=github-releases depName=openbao/openbao
readonly OPENBAO_VERSION="${OPENBAO_VERSION:-2.6.2}"
# renovate: datasource=github-releases depName=freelensapp/freelens
readonly FREELENS_VERSION="${FREELENS_VERSION:-1.10.3}"

BIN_ARCH="$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')"
readonly BIN_ARCH

# This script owns all privileged operations. By default it installs packages;
# TOOL_SKIP_SYSTEM_PACKAGES=1 only verifies preinstalled dependencies.
readonly SKIP_PACKAGES="${TOOL_SKIP_SYSTEM_PACKAGES:-0}"

# Share linux1's cache layout for consistent baseline deferral.
readonly VERSION_CACHE_DIR="${TOOL_VERSION_CACHE_DIR:-$HOME/.local/share/tool-versions}"
readonly SYSTEM_CACHE_DIR="/usr/local/share/tool-versions"

# Logging

readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly NC='\033[0m'

log_info()  { echo -e "${GREEN}[INFO]${NC} $*"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*" >&2; }

is_desktop_machine() {
    local profile="${TOOL_MACHINE_PROFILE:-auto}"

    case "$profile" in
        desktop)
            log_info "Desktop machine profile selected"
            return 0
            ;;
        server)
            log_info "Server machine profile selected"
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
                return 1
                ;;
            *)
                log_error "Invalid machine profile in ${profile_file}: ${profile}"
                exit 1
                ;;
        esac
    fi

    log_warn "No explicit machine profile or image marker; defaulting to server"
    return 1
}

glibc_supports_freelens() {
    local version major minor
    version="$(getconf GNU_LIBC_VERSION 2>/dev/null | awk '{print $2}')"
    IFS=. read -r major minor _ <<<"$version"
    [[ "$major" =~ ^[0-9]+$ && "$minor" =~ ^[0-9]+$ ]] || return 1
    (( major > 2 || (major == 2 && minor >= 34) ))
}

TMP_PATHS=()

cleanup_tmp_paths() {
    local path
    for path in "${TMP_PATHS[@]}"; do
        rm -rf "$path"
    done
}

trap cleanup_tmp_paths EXIT

make_tmp_dir() {
    local __var_name="$1" path
    path="$(mktemp -d)"
    TMP_PATHS+=("$path")
    printf -v "$__var_name" '%s' "$path"
}

# Package-manager helpers (privileged)

update_package_cache() {
    case "$OS_ID" in
        ubuntu|debian) sudo apt-get update -qq ;;
        rocky)  sudo dnf makecache --refresh -q ;;
        *) log_error "Unsupported OS: ${OS_ID}"; exit 1 ;;
    esac
}

install_packages() {
    case "$OS_ID" in
        ubuntu|debian) sudo apt-get install -y "$@" ;;
        rocky)  sudo dnf install -y "$@" ;;
        *) log_error "Unsupported OS: ${OS_ID}"; exit 1 ;;
    esac
}

add_apt_repository() {
    local repo_name="$1" gpg_url="$2" repo_line="$3"
    # Keep an overridden keyring path in sync with repo_line's signed-by value.
    local keyring_path="${4:-/usr/share/keyrings/${repo_name}-keyring.gpg}"
    log_info "Adding ${repo_name} repository..."
    curl -fsSL "$gpg_url" | gpg --dearmor | sudo tee "$keyring_path" > /dev/null
    sudo chmod 644 "$keyring_path"
    echo "$repo_line" | sudo tee "/etc/apt/sources.list.d/${repo_name}.list" > /dev/null
    sudo chmod 644 "/etc/apt/sources.list.d/${repo_name}.list"
}

add_dnf_repository() {
    local repo_name="$1" repo_url="$2" gpgkey_url="$3"
    log_info "Adding ${repo_name} repository..."
    sudo tee "/etc/yum.repos.d/${repo_name}.repo" > /dev/null <<EOF
[${repo_name}]
name=${repo_name}
baseurl=${repo_url}
enabled=1
gpgcheck=1
gpgkey=${gpgkey_url}
EOF
}

# Idempotency helpers (mirrors run_onchange_linux1_tool.sh)

# True when a per-user install can defer to the system baseline.
baseline_satisfies() {
    local key="$1" version="$2"
    [[ "$VERSION_CACHE_DIR" != "$SYSTEM_CACHE_DIR" ]] || return 1
    [[ "$(cat "${SYSTEM_CACHE_DIR}/${key}" 2>/dev/null)" == "$version" ]]
}

install_if_needed() {
    local cmd="$1" version="$2" install_func="$3"
    if baseline_satisfies "$cmd" "$version" && command -v "$cmd" &>/dev/null; then
        log_info "${cmd} ${version} provided system-wide, skipping reinstall"
        return
    fi
    local cache_file="${VERSION_CACHE_DIR}/${cmd}"
    if ! command -v "$cmd" &>/dev/null || [[ "$(cat "$cache_file" 2>/dev/null)" != "$version" ]]; then
        "$install_func"
        mkdir -p "$VERSION_CACHE_DIR"
        echo "$version" > "$cache_file"
    else
        log_info "${cmd} ${version} is already up to date, skipping"
    fi
}

# Baseline OS dependencies

install_base_dependencies() {
    log_info "Installing baseline dependencies..."
    update_package_cache
    local packages=()
    case "$OS_ID" in
        ubuntu|debian)
            packages=(ca-certificates curl coreutils file findutils git gnupg gzip make tar unzip xz-utils mosh tmux podman)
            ;;
        rocky)
            packages=(ca-certificates curl coreutils file findutils git gnupg2 gzip make tar unzip xz mosh tmux podman)
            ;;
        *)
            log_error "Unsupported OS: ${OS_ID}"
            exit 1
            ;;
    esac
    # fontconfig provides fc-cache/fc-list consumed by the desktop font script.
    is_desktop_machine && packages+=(fontconfig)
    install_packages "${packages[@]}"
}

# HashiCorp tools (terraform / packer / vault) via official repository

install_hashicorp_tools() {
    log_info "Installing HashiCorp tools (Terraform, Packer, Vault)..."
    update_package_cache
    case "$OS_ID" in
        ubuntu|debian)
            install_packages gnupg software-properties-common
            local codename="${VERSION_CODENAME:-${UBUNTU_CODENAME:-$(lsb_release -cs)}}"
            add_apt_repository "hashicorp" \
                "https://apt.releases.hashicorp.com/gpg" \
                "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com ${codename} main" \
                "/usr/share/keyrings/hashicorp-archive-keyring.gpg"
            ;;
        rocky)
            install_packages yum-utils
            sudo yum-config-manager --add-repo https://rpm.releases.hashicorp.com/RHEL/hashicorp.repo
            ;;
    esac
    update_package_cache
    install_packages terraform packer vault
}

# kubectl via official Kubernetes repository

install_kubectl() {
    log_info "Installing kubectl..."
    update_package_cache
    install_packages ca-certificates curl gnupg
    case "$OS_ID" in
        ubuntu|debian)
            sudo apt-get install -y apt-transport-https
            sudo mkdir -p -m 755 /etc/apt/keyrings
            add_apt_repository "kubernetes" \
                "https://pkgs.k8s.io/core:/stable:/v${KUBECTL_VERSION}/deb/Release.key" \
                "deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v${KUBECTL_VERSION}/deb/ /" \
                "/etc/apt/keyrings/kubernetes-apt-keyring.gpg"
            ;;
        rocky)
            add_dnf_repository "kubernetes" \
                "https://pkgs.k8s.io/core:/stable:/v${KUBECTL_VERSION}/rpm/" \
                "https://pkgs.k8s.io/core:/stable:/v${KUBECTL_VERSION}/rpm/repodata/repomd.xml.key"
            ;;
    esac
    update_package_cache
    install_packages kubectl
}

# openbao via release .deb / .rpm

install_openbao() {
    log_info "Installing openbao..."
    local tmp_dir
    make_tmp_dir tmp_dir
    case "$OS_ID" in
        ubuntu|debian)
            local pkg_name="openbao_${OPENBAO_VERSION}_linux_${BIN_ARCH}.deb"
            curl -fsSL "https://github.com/openbao/openbao/releases/download/v${OPENBAO_VERSION}/${pkg_name}" \
                -o "${tmp_dir}/${pkg_name}"
            sudo apt-get install -y "${tmp_dir}/${pkg_name}"
            ;;
        rocky)
            local pkg_name="openbao_${OPENBAO_VERSION}_linux_${BIN_ARCH}.rpm"
            curl -fsSL "https://github.com/openbao/openbao/releases/download/v${OPENBAO_VERSION}/${pkg_name}" \
                -o "${tmp_dir}/${pkg_name}"
            sudo dnf install -y "${tmp_dir}/${pkg_name}"
            ;;
    esac
}

# Freelens desktop application via signed release checksums and native packages

freelens_installed_version() {
    case "$OS_ID" in
        ubuntu|debian)
            dpkg-query -W -f='${Version}' freelens 2>/dev/null || true
            ;;
        rocky)
            rpm -q --qf '%{VERSION}' freelens 2>/dev/null || true
            ;;
    esac
}

install_freelens() {
    local current_version
    current_version="$(freelens_installed_version)"
    if [[ "$current_version" == "$FREELENS_VERSION" ]]; then
        log_info "Freelens ${FREELENS_VERSION} is already up to date, skipping"
        return
    fi
    if ! glibc_supports_freelens; then
        log_error "Freelens ${FREELENS_VERSION} requires glibc 2.34 or later."
        exit 1
    fi

    log_info "Installing Freelens ${FREELENS_VERSION}..."
    local extension package_name tmp_dir checksum_file expected actual
    case "$OS_ID" in
        ubuntu|debian) extension="deb" ;;
        rocky)         extension="rpm" ;;
    esac
    package_name="Freelens-${FREELENS_VERSION}-linux-${BIN_ARCH}.${extension}"
    make_tmp_dir tmp_dir
    curl -fsSL \
        "https://github.com/freelensapp/freelens/releases/download/v${FREELENS_VERSION}/${package_name}" \
        -o "${tmp_dir}/${package_name}"
    checksum_file="${tmp_dir}/${package_name}.sha256"
    curl -fsSL \
        "https://github.com/freelensapp/freelens/releases/download/v${FREELENS_VERSION}/${package_name}.sha256" \
        -o "$checksum_file"
    expected="$(awk 'NF > 0 {print $1; exit}' "$checksum_file")"
    actual="$(sha256sum "${tmp_dir}/${package_name}" | awk '{print $1}')"
    if [[ ! "$expected" =~ ^[0-9a-fA-F]{64}$ || "$expected" != "$actual" ]]; then
        log_error "Checksum mismatch for ${package_name}"
        exit 1
    fi

    case "$OS_ID" in
        ubuntu|debian) sudo apt-get install -y "${tmp_dir}/${package_name}" ;;
        rocky)         sudo dnf install -y "${tmp_dir}/${package_name}" ;;
    esac
}

# pipx toolchain bootstrap (consumed by the ansible installs in linux1)

# Install Python 3.12 when the distro default cannot run ansible-core.
have_python312() {
    command -v python3.12 &>/dev/null && return 0
    command -v python3 &>/dev/null && \
        python3 -c 'import sys; raise SystemExit(0 if sys.version_info[:2] >= (3, 12) else 1)' 2>/dev/null
}

ensure_pipx_toolchain() {
    if ! command -v pipx &>/dev/null; then
        log_info "Installing pipx..."
        update_package_cache
        case "$OS_ID" in
            ubuntu|debian)
                install_packages python3 python3-pip pipx
                ;;
            rocky)
                install_packages epel-release
                update_package_cache
                install_packages python3 python3-pip pipx
            ;;
        esac
    fi
    if ! have_python312; then
        log_info "Installing python3.12 (ansible controller interpreter)..."
        local python_packages=()
        case "$OS_ID" in
            ubuntu|debian) python_packages=(python3.12 python3.12-venv) ;;
            rocky)         python_packages=(python3.12 python3.12-pip) ;;
        esac
        if ! install_packages "${python_packages[@]}"; then
            log_error "Python >=3.12 is required but is not available from the configured ${OS_ID} repositories."
            log_error "Use a supported release/repository that provides python3.12, then rerun this script."
            exit 1
        fi
    fi
    if ! have_python312; then
        log_error "Python >=3.12 is required, but python3.12 is still unavailable after package installation."
        exit 1
    fi
}

# Preflight (no-sudo mode): verify prerequisites instead of installing

print_install_hint() {
    log_error "  Run this script once without TOOL_SKIP_SYSTEM_PACKAGES as a user with sudo access."
    log_error "  It configures the required third-party repositories as well as installing OS packages."
}

preflight_packages() {
    log_info "TOOL_SKIP_SYSTEM_PACKAGES set: verifying pre-provided packages (no sudo)..."
    local missing=() cmd
    # Check base and package-managed tool dependencies.
    for cmd in curl tar gzip unzip xz gpg git file find make sha256sum install \
               terraform packer vault kubectl bao pipx mosh tmux podman; do
        command -v "$cmd" &>/dev/null || missing+=("$cmd")
    done
    if is_desktop_machine; then
        for cmd in fc-cache fc-list; do
            command -v "$cmd" &>/dev/null || missing+=("$cmd")
        done
        [[ "$(freelens_installed_version)" == "$FREELENS_VERSION" ]] || \
            missing+=("freelens=${FREELENS_VERSION}")
        glibc_supports_freelens || missing+=("glibc>=2.34")
    fi
    have_python312 || missing+=("python>=3.12")
    if [[ ${#missing[@]} -gt 0 ]]; then
        log_error "Missing pre-provided prerequisites: ${missing[*]}"
        log_error "A privileged user must install these once (no-sudo runs assume they exist), e.g.:"
        print_install_hint
        exit 1
    fi
    log_info "All required system packages are present."
}

# Main

main() {
    log_info "=== Linux System Package Installation ==="

    if [[ "$SKIP_PACKAGES" == "1" ]]; then
        preflight_packages
        log_info "=== Preflight passed ==="
        return
    fi
    if [[ "$SKIP_PACKAGES" != "0" ]]; then
        log_error "TOOL_SKIP_SYSTEM_PACKAGES must be 0, 1, or unset (got: ${SKIP_PACKAGES})."
        exit 1
    fi

    install_base_dependencies

    if ! command -v terraform &>/dev/null || ! command -v packer &>/dev/null || ! command -v vault &>/dev/null; then
        install_hashicorp_tools
    fi

    install_if_needed "kubectl" "$KUBECTL_VERSION" install_kubectl
    install_if_needed "bao"     "$OPENBAO_VERSION" install_openbao

    if is_desktop_machine; then
        install_freelens
    else
        log_info "Skipping Freelens installation on server profile"
    fi

    ensure_pipx_toolchain

    log_info "=== Package installation completed ==="
}

main
