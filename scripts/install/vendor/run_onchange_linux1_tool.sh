#!/usr/bin/env bash
set -euo pipefail

[[ "$(uname)" == "Linux" ]] || exit 0

. /etc/os-release
readonly OS_ID="${ID}"

# renovate: datasource=github-releases depName=junegunn/fzf
readonly FZF_VERSION="${FZF_VERSION:-0.73.1}"
# renovate: datasource=github-releases depName=zellij-org/zellij
readonly ZELLIJ_VERSION="${ZELLIJ_VERSION:-0.44.3}"
# renovate: datasource=github-releases depName=sbstp/kubie
readonly KUBIE_VERSION="${KUBIE_VERSION:-0.28.0}"
# renovate: datasource=github-releases depName=derailed/k9s
readonly K9S_VERSION="${K9S_VERSION:-0.51.0}"
# renovate: datasource=github-releases depName=helmfile/helmfile
readonly HELMFILE_VERSION="${HELMFILE_VERSION:-1.5.2}"
# renovate: datasource=github-releases depName=k0sproject/k0sctl
readonly K0SCTL_VERSION="${K0SCTL_VERSION:-0.30.1}"
# renovate: datasource=github-releases depName=getsops/sops
readonly SOPS_VERSION="${SOPS_VERSION:-3.13.1}"
# renovate: datasource=github-releases depName=gruntwork-io/terragrunt
readonly TERRAGRUNT_VERSION="${TERRAGRUNT_VERSION:-1.0.7}"
# renovate: datasource=github-releases depName=opentofu/opentofu
readonly OPENTOFU_VERSION="${OPENTOFU_VERSION:-1.12.1}"
# renovate: datasource=github-releases depName=openbao/openbao
readonly OPENBAO_VERSION="${OPENBAO_VERSION:-2.5.4}"
# renovate: datasource=github-releases depName=helm/helm
readonly HELM_VERSION="${HELM_VERSION:-4.2.0}"
# renovate: datasource=github-releases depName=FiloSottile/age
readonly AGE_VERSION="${AGE_VERSION:-1.3.1}"
# renovate: datasource=github-releases depName=cilium/cilium-cli
readonly CILIUM_VERSION="${CILIUM_VERSION:-0.19.4}"
# renovate: datasource=github-releases depName=kubernetes/kubernetes
readonly KUBECTL_VERSION="${KUBECTL_VERSION:-1.36}"
# renovate: datasource=github-releases depName=eza-community/eza
readonly EZA_VERSION="${EZA_VERSION:-0.23.4}"
# renovate: datasource=github-releases depName=starship/starship
readonly STARSHIP_VERSION="${STARSHIP_VERSION:-1.25.1}"
# renovate: datasource=github-releases depName=rossmacarthur/sheldon
readonly SHELDON_VERSION="${SHELDON_VERSION:-0.8.5}"
# renovate: datasource=github-releases depName=direnv/direnv
readonly DIRENV_VERSION="${DIRENV_VERSION:-2.37.1}"
# renovate: datasource=github-releases depName=kubernetes-sigs/krew
readonly KREW_VERSION="${KREW_VERSION:-0.5.0}"
# renovate: datasource=github-releases depName=DNSControl/dnscontrol
readonly DNSCONTROL_VERSION="${DNSCONTROL_VERSION:-4.41.0}"
# renovate: datasource=pypi depName=ansible-core
readonly ANSIBLE_CORE_VERSION="${ANSIBLE_CORE_VERSION:-2.20.5}"
# renovate: datasource=pypi depName=ansible-lint
readonly ANSIBLE_LINT_VERSION="${ANSIBLE_LINT_VERSION:-26.4.0}"

# Install location. Defaults to a per-user prefix. Set TOOL_BIN_DIR (and
# TOOL_VERSION_CACHE_DIR) to a system-wide path such as /usr/local/bin to make
# the tools available to every user (e.g. for a shared / golden-image VM). A
# system-wide target requires running this script as root.
readonly BIN_DIR="${TOOL_BIN_DIR:-$HOME/.local/bin}"
readonly VERSION_CACHE_DIR="${TOOL_VERSION_CACHE_DIR:-$HOME/.local/share/tool-versions}"
# A system-wide baseline (golden-image VM) records installed versions here. A
# per-user install defers to it when it already provides the desired version, so
# running this via chezmoi / provision.sh on a baked image does not shadow a
# current baseline with a duplicate in $HOME/.local.
readonly SYSTEM_CACHE_DIR="/usr/local/share/tool-versions"
# pipx-managed Python tools (ansible, ansible-lint). Venvs live next to the
# version cache (per-user $HOME/.local/share, or /usr/local/share when this runs
# as a system-wide baseline); app symlinks go into BIN_DIR like every other tool.
readonly PIPX_HOME_DIR="$(dirname "$VERSION_CACHE_DIR")/pipx"
readonly ARCH="$(uname -m)"
readonly BIN_ARCH="$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')"

mkdir -p "$BIN_DIR" "$VERSION_CACHE_DIR"
export PATH="${BIN_DIR}:${PATH}"

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

install_base_dependencies() {
    log_info "Installing baseline dependencies..."
    update_package_cache
    case "$OS_ID" in
        ubuntu|debian)
            install_packages ca-certificates curl coreutils file findutils git gnupg gzip make tar unzip xz-utils
            ;;
        rocky)
            install_packages ca-certificates curl coreutils file findutils git gnupg2 gzip make tar unzip xz
            ;;
        *)
            log_error "Unsupported OS: ${OS_ID}"
            exit 1
            ;;
    esac
}

make_tmp_dir() {
    local __var_name="$1" path
    path="$(mktemp -d)"
    TMP_PATHS+=("$path")
    printf -v "$__var_name" '%s' "$path"
}

make_tmp_file() {
    local __var_name="$1" suffix="${2:-}" path
    if [[ -n "$suffix" ]]; then
        path="$(mktemp --suffix="$suffix")"
    else
        path="$(mktemp)"
    fi
    TMP_PATHS+=("$path")
    printf -v "$__var_name" '%s' "$path"
}

add_apt_repository() {
    local repo_name="$1" gpg_url="$2" repo_line="$3"
    # Optional 4th arg overrides the keyring path so we can match the upstream
    # project's official name (keep it in sync with the signed-by= in repo_line).
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

verify_sha256() {
    local file="$1" checksum_url="$2" checksum_name="${3:-$(basename "$1")}"
    local sum_file
    make_tmp_file sum_file
    curl -fsSL "$checksum_url" -o "$sum_file"
    local expected actual
    expected="$(awk -v name="$checksum_name" '
        {
            candidate = $NF
            sub(/^\*/, "", candidate)
            sub(/^.*\//, "", candidate)
            if (candidate == name) {
                print $1
                exit
            }
        }
    ' "$sum_file")"
    if [[ -z "$expected" ]]; then
        expected="$(awk 'NF > 0 {print $1; exit}' "$sum_file")"
    fi
    actual="$(sha256sum "$file" | awk '{print $1}')"
    if [[ "$expected" != "$actual" ]]; then
        log_error "Checksum mismatch for ${checksum_name}"
        log_error "  expected: ${expected}"
        log_error "  actual:   ${actual}"
        exit 1
    fi
    log_info "Checksum OK: ${checksum_name}"
}

install_binary() {
    local name="$1" url="$2" output_file="$3" checksum_url="${4:-}"
    log_info "Installing ${name}..."
    local tmp_dir
    make_tmp_dir tmp_dir
    local archive_name
    archive_name="$(basename "$url")"
    if [[ "$url" == *.tar.gz ]]; then
        local archive="${tmp_dir}/${archive_name}"
        curl -fsSL "$url" -o "$archive"
        [[ -n "$checksum_url" ]] && verify_sha256 "$archive" "$checksum_url" "$archive_name"
        tar xz -C "$tmp_dir" -f "$archive"
        install -m 0755 "$tmp_dir/${name}" "$output_file"
    else
        local bin="${tmp_dir}/${archive_name}"
        curl -fsSL "$url" -o "$bin"
        [[ -n "$checksum_url" ]] && verify_sha256 "$bin" "$checksum_url" "$archive_name"
        install -m 0755 "$bin" "$output_file"
    fi
}

# True when a system-wide baseline already provides KEY at VERSION. Only
# meaningful for a per-user install (our cache dir is not the system one).
baseline_satisfies() {
    local key="$1" version="$2"
    [[ "$VERSION_CACHE_DIR" != "$SYSTEM_CACHE_DIR" ]] || return 1
    [[ "$(cat "${SYSTEM_CACHE_DIR}/${key}" 2>/dev/null)" == "$version" ]]
}

is_system_wide_install() {
    [[ "$VERSION_CACHE_DIR" == "$SYSTEM_CACHE_DIR" || "$BIN_DIR" == "/usr/local/bin" ]]
}

install_if_needed() {
    local cmd="$1" version="$2" install_func="$3"
    if baseline_satisfies "$cmd" "$version" && command -v "$cmd" &>/dev/null; then
        log_info "${cmd} ${version} provided system-wide, skipping per-user install"
        return
    fi
    local cache_file="${VERSION_CACHE_DIR}/${cmd}"
    if ! command -v "$cmd" &>/dev/null || [[ "$(cat "$cache_file" 2>/dev/null)" != "$version" ]]; then
        "$install_func"
        echo "$version" > "$cache_file"
    else
        log_info "${cmd} ${version} is already up to date, skipping"
    fi
}

# ============================================================================
# Shell Enhancement Tools
# ============================================================================

install_sheldon() {
    log_info "Installing sheldon ${SHELDON_VERSION}..."
    # sheldon release tags have no 'v' prefix.
    curl --proto '=https' -fsSL https://rossmacarthur.github.io/install/crate.sh \
        | bash -s -- --repo rossmacarthur/sheldon --tag "${SHELDON_VERSION}" --to "$BIN_DIR"
}

install_starship() {
    log_info "Installing starship ${STARSHIP_VERSION}..."
    curl -fsSL https://starship.rs/install.sh \
        | sh -s -- --version "v${STARSHIP_VERSION}" --bin-dir "$BIN_DIR" -y
}

install_direnv() {
    install_binary "direnv" \
        "https://github.com/direnv/direnv/releases/download/v${DIRENV_VERSION}/direnv.linux-${BIN_ARCH}" \
        "$BIN_DIR/direnv"
}

install_eza() {
    install_binary "eza" \
        "https://github.com/eza-community/eza/releases/download/v${EZA_VERSION}/eza_${ARCH}-unknown-linux-gnu.tar.gz" \
        "$BIN_DIR/eza"
}

install_fzf() {
    install_binary "fzf" \
        "https://github.com/junegunn/fzf/releases/download/v${FZF_VERSION}/fzf-${FZF_VERSION}-linux_${BIN_ARCH}.tar.gz" \
        "$BIN_DIR/fzf"
}

install_zellij() {
    local archive_name="zellij-${ARCH}-unknown-linux-musl.tar.gz"
    install_binary "zellij" \
        "https://github.com/zellij-org/zellij/releases/download/v${ZELLIJ_VERSION}/${archive_name}" \
        "$BIN_DIR/zellij" \
        "https://github.com/zellij-org/zellij/releases/download/v${ZELLIJ_VERSION}/${archive_name%.tar.gz}.sha256sum"
}

# ============================================================================
# HashiCorp Tools
# ============================================================================

install_hashicorp_tools() {
    log_info "Installing HashiCorp tools (Terraform, Packer, Vault)..."
    update_package_cache
    case "$OS_ID" in
        ubuntu|debian)
            install_packages gnupg software-properties-common
            . /etc/os-release
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

install_terragrunt() {
    local archive_name="terragrunt_linux_${BIN_ARCH}"
    install_binary "terragrunt" \
        "https://github.com/gruntwork-io/terragrunt/releases/download/v${TERRAGRUNT_VERSION}/${archive_name}" \
        "$BIN_DIR/terragrunt" \
        "https://github.com/gruntwork-io/terragrunt/releases/download/v${TERRAGRUNT_VERSION}/SHA256SUMS"
}

install_opentofu() {
    log_info "Installing opentofu..."
    install_packages unzip
    local tmp_dir zip_name
    make_tmp_dir tmp_dir
    zip_name="tofu_${OPENTOFU_VERSION}_linux_${BIN_ARCH}.zip"
    curl -fsSL "https://github.com/opentofu/opentofu/releases/download/v${OPENTOFU_VERSION}/${zip_name}" \
        -o "${tmp_dir}/tofu.zip"
    verify_sha256 "${tmp_dir}/tofu.zip" \
        "https://github.com/opentofu/opentofu/releases/download/v${OPENTOFU_VERSION}/tofu_${OPENTOFU_VERSION}_SHA256SUMS" \
        "$zip_name"
    unzip -q "${tmp_dir}/tofu.zip" tofu -d "${tmp_dir}"
    install -m 0755 "${tmp_dir}/tofu" "$BIN_DIR/tofu"
}

install_openbao() {
    log_info "Installing openbao..."
    local tmp_dir
    make_tmp_dir tmp_dir
    case "$OS_ID" in
        ubuntu|debian)
            local pkg_name="openbao_${OPENBAO_VERSION}_linux_${BIN_ARCH}.deb"
            curl -fsSL "https://github.com/openbao/openbao/releases/download/v${OPENBAO_VERSION}/${pkg_name}" \
                -o "${tmp_dir}/${pkg_name}"
            sudo dpkg -i "${tmp_dir}/${pkg_name}"
            ;;
        rocky)
            local pkg_name="openbao_${OPENBAO_VERSION}_linux_${BIN_ARCH}.rpm"
            curl -fsSL "https://github.com/openbao/openbao/releases/download/v${OPENBAO_VERSION}/${pkg_name}" \
                -o "${tmp_dir}/${pkg_name}"
            sudo dnf install -y "${tmp_dir}/${pkg_name}"
            ;;
    esac
}

# ============================================================================
# Kubernetes Tools
# ============================================================================

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

install_helm() {
    log_info "Installing helm ${HELM_VERSION}..."
    local tmp_dir archive_name
    make_tmp_dir tmp_dir
    archive_name="helm-v${HELM_VERSION}-linux-${BIN_ARCH}.tar.gz"
    curl -fsSL "https://get.helm.sh/${archive_name}" -o "${tmp_dir}/${archive_name}"
    verify_sha256 "${tmp_dir}/${archive_name}" \
        "https://get.helm.sh/${archive_name}.sha256sum" \
        "$archive_name"
    tar xz -C "$tmp_dir" -f "${tmp_dir}/${archive_name}"
    install -m 0755 "$tmp_dir/linux-${BIN_ARCH}/helm" "$BIN_DIR/helm"
}

install_kubie() {
    install_binary "kubie" \
        "https://github.com/sbstp/kubie/releases/download/v${KUBIE_VERSION}/kubie-linux-${BIN_ARCH}" \
        "$BIN_DIR/kubie"
}

install_k9s() {
    install_binary "k9s" \
        "https://github.com/derailed/k9s/releases/download/v${K9S_VERSION}/k9s_Linux_${BIN_ARCH}.tar.gz" \
        "$BIN_DIR/k9s" \
        "https://github.com/derailed/k9s/releases/download/v${K9S_VERSION}/checksums.sha256"
}

install_helmfile() {
    install_binary "helmfile" \
        "https://github.com/helmfile/helmfile/releases/download/v${HELMFILE_VERSION}/helmfile_${HELMFILE_VERSION}_linux_${BIN_ARCH}.tar.gz" \
        "$BIN_DIR/helmfile" \
        "https://github.com/helmfile/helmfile/releases/download/v${HELMFILE_VERSION}/helmfile_${HELMFILE_VERSION}_checksums.txt"
}

install_krew_if_needed() {
    if is_system_wide_install; then
        log_info "Skipping krew during system-wide install; krew is user-scoped"
        return
    fi

    local krew_root krew_bin cache_file tmp_dir archive_name
    krew_root="${KREW_ROOT:-$HOME/.krew}"
    krew_bin="${krew_root}/bin/kubectl-krew"
    cache_file="${VERSION_CACHE_DIR}/krew"

    if [[ -x "$krew_bin" && "$(cat "$cache_file" 2>/dev/null)" == "$KREW_VERSION" ]]; then
        log_info "krew ${KREW_VERSION} is already up to date, skipping"
        return
    fi

    log_info "Installing krew ${KREW_VERSION}..."
    make_tmp_dir tmp_dir
    archive_name="krew-linux_${BIN_ARCH}.tar.gz"
    curl -fsSL "https://github.com/kubernetes-sigs/krew/releases/download/v${KREW_VERSION}/${archive_name}" \
        -o "${tmp_dir}/${archive_name}"
    tar xz -C "$tmp_dir" -f "${tmp_dir}/${archive_name}"
    "${tmp_dir}/krew-linux_${BIN_ARCH}" install krew
    echo "$KREW_VERSION" > "$cache_file"
}

install_k0sctl() {
    install_binary "k0sctl" \
        "https://github.com/k0sproject/k0sctl/releases/download/v${K0SCTL_VERSION}/k0sctl-linux-${BIN_ARCH}" \
        "$BIN_DIR/k0sctl"
}

install_age() {
    log_info "Installing age ${AGE_VERSION}..."
    local tmp_dir archive_name
    make_tmp_dir tmp_dir
    archive_name="age-v${AGE_VERSION}-linux-${BIN_ARCH}.tar.gz"
    curl -fsSL "https://github.com/FiloSottile/age/releases/download/v${AGE_VERSION}/${archive_name}" \
        -o "${tmp_dir}/${archive_name}"
    tar xz -C "$tmp_dir" -f "${tmp_dir}/${archive_name}"
    install -m 0755 "$tmp_dir/age/age"        "$BIN_DIR/age"
    install -m 0755 "$tmp_dir/age/age-keygen" "$BIN_DIR/age-keygen"
}

install_cilium() {
    local archive_name="cilium-linux-${BIN_ARCH}.tar.gz"
    install_binary "cilium" \
        "https://github.com/cilium/cilium-cli/releases/download/v${CILIUM_VERSION}/${archive_name}" \
        "$BIN_DIR/cilium" \
        "https://github.com/cilium/cilium-cli/releases/download/v${CILIUM_VERSION}/${archive_name}.sha256sum"
}

install_sops() {
    log_info "Installing sops..."
    case "$OS_ID" in
        ubuntu|debian)
            install_packages age
            local deb_file pkg_name
            make_tmp_file deb_file .deb
            pkg_name="sops_${SOPS_VERSION}_${BIN_ARCH}.deb"
            curl -fsSL "https://github.com/getsops/sops/releases/download/v${SOPS_VERSION}/${pkg_name}" -o "$deb_file"
            verify_sha256 "$deb_file" \
                "https://github.com/getsops/sops/releases/download/v${SOPS_VERSION}/sops-v${SOPS_VERSION}.checksums.txt" \
                "$pkg_name"
            sudo dpkg -i "$deb_file"
            ;;
        rocky)
            install_if_needed "age" "$AGE_VERSION" install_age
            install_binary "sops" \
                "https://github.com/getsops/sops/releases/download/v${SOPS_VERSION}/sops-v${SOPS_VERSION}.linux.${BIN_ARCH}" \
                "$BIN_DIR/sops" \
                "https://github.com/getsops/sops/releases/download/v${SOPS_VERSION}/sops-v${SOPS_VERSION}.checksums.txt"
            ;;
    esac
}

# ============================================================================
# DNS Tools
# ============================================================================

install_dnscontrol() {
    local archive_name="dnscontrol_${DNSCONTROL_VERSION}_linux_${BIN_ARCH}.tar.gz"
    install_binary "dnscontrol" \
        "https://github.com/DNSControl/dnscontrol/releases/download/v${DNSCONTROL_VERSION}/${archive_name}" \
        "$BIN_DIR/dnscontrol" \
        "https://github.com/DNSControl/dnscontrol/releases/download/v${DNSCONTROL_VERSION}/checksums.txt"
}

install_helm_diff_plugin() {
    # helm plugins live under HELM_DATA_HOME (per-user by default). For a
    # shared install, set TOOL_HELM_DATA_HOME to a system-wide path; users must
    # then export HELM_DATA_HOME to the same path (e.g. via /etc/profile.d).
    if [[ -n "${TOOL_HELM_DATA_HOME:-}" ]]; then
        export HELM_DATA_HOME="$TOOL_HELM_DATA_HOME"
        mkdir -p "$HELM_DATA_HOME"
    fi
    if helm plugin list 2>/dev/null | grep -q "^diff"; then
        log_info "helm-diff plugin is already installed, skipping"
        return
    fi
    log_info "Installing helm-diff plugin..."
    # helm v4 verifies plugin provenance by default; the git source does not
    # support verification, so verification must be skipped explicitly.
    helm plugin install --verify=false https://github.com/databus23/helm-diff
}

# ============================================================================
# Python Tools (pipx)
# ============================================================================

# Bootstrap pipx once; ansible / ansible-lint are then installed into isolated
# venvs. ansible-lint in particular is not reliably packaged by the distros, and
# upstream Ansible recommends pip/pipx, so we avoid apt/dnf for these.
ensure_pipx() {
    command -v pipx &>/dev/null && return
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
}

# Route pipx so app symlinks land in BIN_DIR (already on PATH) and venvs live in
# PIPX_HOME_DIR. --force makes the install idempotent and lets a version bump
# reinstall over an existing venv.
pipx_install() {
    ensure_pipx
    PIPX_HOME="$PIPX_HOME_DIR" PIPX_BIN_DIR="$BIN_DIR" pipx install --force "$1"
}

install_ansible() {
    log_info "Installing ansible-core ${ANSIBLE_CORE_VERSION}..."
    pipx_install "ansible-core==${ANSIBLE_CORE_VERSION}"
}

install_ansible_lint() {
    log_info "Installing ansible-lint ${ANSIBLE_LINT_VERSION}..."
    pipx_install "ansible-lint==${ANSIBLE_LINT_VERSION}"
}

# ============================================================================
# Main
# ============================================================================

main() {
    log_info "=== Linux Development Tools Installation ==="

    install_base_dependencies

    install_if_needed "sheldon"  "$SHELDON_VERSION"  install_sheldon
    install_if_needed "starship" "$STARSHIP_VERSION" install_starship
    install_if_needed "direnv"   "$DIRENV_VERSION"   install_direnv
    install_if_needed "eza"      "$EZA_VERSION"      install_eza

    install_if_needed "fzf"    "$FZF_VERSION"    install_fzf
    install_if_needed "zellij" "$ZELLIJ_VERSION" install_zellij

    if ! command -v terraform &>/dev/null || ! command -v packer &>/dev/null || ! command -v vault &>/dev/null; then
        install_hashicorp_tools
    fi

    install_if_needed "terragrunt" "$TERRAGRUNT_VERSION" install_terragrunt
    install_if_needed "tofu"       "$OPENTOFU_VERSION"   install_opentofu
    install_if_needed "bao"        "$OPENBAO_VERSION"    install_openbao

    install_if_needed "kubectl" "$KUBECTL_VERSION" install_kubectl

    install_if_needed "helm"     "$HELM_VERSION"     install_helm
    install_helm_diff_plugin
    install_krew_if_needed
    install_if_needed "kubie"    "$KUBIE_VERSION"    install_kubie
    install_if_needed "k9s"      "$K9S_VERSION"      install_k9s
    install_if_needed "helmfile" "$HELMFILE_VERSION" install_helmfile
    install_if_needed "k0sctl"   "$K0SCTL_VERSION"   install_k0sctl
    install_if_needed "sops"     "$SOPS_VERSION"     install_sops
    install_if_needed "cilium"   "$CILIUM_VERSION"   install_cilium
    install_if_needed "dnscontrol" "$DNSCONTROL_VERSION" install_dnscontrol

    install_if_needed "ansible"      "$ANSIBLE_CORE_VERSION" install_ansible
    install_if_needed "ansible-lint" "$ANSIBLE_LINT_VERSION" install_ansible_lint

    log_info "=== Installation completed ==="
}

main
