#!/usr/bin/env bash
set -euo pipefail

# renovate: datasource=github-releases depName=gruntwork-io/terragrunt
readonly TERRAGRUNT_VERSION="${TERRAGRUNT_VERSION:-1.0.5}"
# renovate: datasource=github-releases depName=openbao/openbao
readonly OPENBAO_VERSION="${OPENBAO_VERSION:-2.5.4}"
# renovate: datasource=github-releases depName=opentofu/opentofu
readonly OPENTOFU_VERSION="${OPENTOFU_VERSION:-1.12.0}"
# renovate: datasource=github-releases depName=helm/helm
readonly HELM_VERSION="${HELM_VERSION:-4.2.0}"
# renovate: datasource=github-releases depName=derailed/k9s
readonly K9S_VERSION="${K9S_VERSION:-0.50.18}"
# renovate: datasource=github-releases depName=sbstp/kubie
readonly KUBIE_VERSION="${KUBIE_VERSION:-0.27.0}"
# renovate: datasource=github-releases depName=FiloSottile/age
readonly AGE_VERSION="${AGE_VERSION:-1.3.1}"
# renovate: datasource=github-releases depName=getsops/sops
readonly SOPS_VERSION="${SOPS_VERSION:-3.13.1}"
# renovate: datasource=github-releases depName=helmfile/helmfile
readonly HELMFILE_VERSION="${HELMFILE_VERSION:-1.5.1}"
# renovate: datasource=github-releases depName=cilium/cilium-cli
readonly CILIUM_VERSION="${CILIUM_VERSION:-0.19.4}"
# renovate: datasource=github-releases depName=kubernetes/kubernetes
readonly KUBECTL_VERSION="${KUBECTL_VERSION:-1.35}"

readonly BIN_DIR="$HOME/.local/bin"
readonly VERSION_CACHE_DIR="$HOME/.local/share/tool-versions"
readonly BIN_ARCH="$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')"

# Detect OS
. /etc/os-release
readonly OS_ID="${ID}"

log_info() { echo "[INFO] $*"; }
log_error() { echo "[ERROR] $*" >&2; }

mkdir -p "$BIN_DIR" "$VERSION_CACHE_DIR"
export PATH="${BIN_DIR}:${PATH}"

# Wait for cloud-init to finish before running apt
if command -v cloud-init &>/dev/null; then
  log_info "Waiting for cloud-init to complete..."
  cloud-init status --wait || true
fi

# ============================================================================
# Helpers
# ============================================================================

install_binary() {
    local name="$1"
    local url="$2"
    local output_file="$3"

    log_info "Installing ${name}..."
    local tmp_dir
    tmp_dir=$(mktemp -d)
    trap "rm -rf '${tmp_dir}'" RETURN

    if [[ "$url" == *.tar.gz ]]; then
        curl -fsSL "$url" | tar xz -C "$tmp_dir"
        install -m 0755 "$tmp_dir/${name}" "$output_file" 2>/dev/null || \
            install -m 0755 "$tmp_dir/${name##*/}" "$output_file"
    else
        curl -fsSL "$url" -o "$output_file"
        chmod 0755 "$output_file"
    fi
}

install_if_needed() {
    local cmd="$1"
    local version="$2"
    local install_func="$3"
    local cache_file="${VERSION_CACHE_DIR}/${cmd}"

    if ! command -v "$cmd" &>/dev/null || [[ "$(cat "$cache_file" 2>/dev/null)" != "$version" ]]; then
        "$install_func"
        echo "$version" > "$cache_file"
    else
        log_info "${cmd} ${version} is already up to date, skipping"
    fi
}

ensure_installed() {
    local cmd="$1"
    local install_func="$2"
    if ! command -v "$cmd" &>/dev/null; then
        "$install_func"
    fi
}

update_package_cache() {
    case "$OS_ID" in
        ubuntu) sudo apt-get update -qq ;;
        rocky)  sudo dnf makecache --refresh -q ;;
    esac
}

install_packages() {
    case "$OS_ID" in
        ubuntu) sudo apt-get install -y "$@" ;;
        rocky)  sudo dnf install -y "$@" ;;
    esac
}

add_apt_repository() {
    local repo_name="$1"
    local gpg_url="$2"
    local repo_line="$3"
    local keyring_path="/usr/share/keyrings/${repo_name}-keyring.gpg"

    curl -fsSL "$gpg_url" | gpg --dearmor | sudo tee "$keyring_path" > /dev/null
    sudo chmod 644 "$keyring_path"
    echo "$repo_line" | sudo tee "/etc/apt/sources.list.d/${repo_name}.list" > /dev/null
    sudo chmod 644 "/etc/apt/sources.list.d/${repo_name}.list"
}

add_dnf_repository() {
    local repo_name="$1"
    local repo_url="$2"
    local gpgkey_url="$3"

    sudo tee "/etc/yum.repos.d/${repo_name}.repo" > /dev/null <<EOF
[${repo_name}]
name=${repo_name}
baseurl=${repo_url}
enabled=1
gpgcheck=1
gpgkey=${gpgkey_url}
EOF
}

# ============================================================================
# Tool installers
# ============================================================================

install_hashicorp_tools() {
    log_info "Installing HashiCorp tools (Terraform, Packer, Vault)..."
    update_package_cache
    case "$OS_ID" in
        ubuntu)
            install_packages gnupg software-properties-common
            local codename="${UBUNTU_CODENAME:-$(lsb_release -cs)}"
            add_apt_repository "hashicorp" \
                "https://apt.releases.hashicorp.com/gpg" \
                "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/hashicorp-keyring.gpg] https://apt.releases.hashicorp.com ${codename} main"
            ;;
        rocky)
            install_packages yum-utils
            sudo yum-config-manager --add-repo https://rpm.releases.hashicorp.com/RHEL/hashicorp.repo
            ;;
    esac
    update_package_cache
    install_packages terraform packer vault
}

install_kubectl() {
    log_info "Installing kubectl..."
    case "$OS_ID" in
        ubuntu)
            update_package_cache
            install_packages ca-certificates curl gnupg apt-transport-https
            sudo mkdir -p -m 755 /etc/apt/keyrings
            add_apt_repository "kubernetes" \
                "https://pkgs.k8s.io/core:/stable:/v1.35/deb/Release.key" \
                "deb [signed-by=/usr/share/keyrings/kubernetes-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v1.35/deb/ /"
            update_package_cache
            install_packages kubectl
            ;;
        rocky)
            install_packages ca-certificates curl gnupg
            add_dnf_repository "kubernetes" \
                "https://pkgs.k8s.io/core:/stable:/v1.35/rpm/" \
                "https://pkgs.k8s.io/core:/stable:/v1.35/rpm/repodata/repomd.xml.key"
            update_package_cache
            install_packages kubectl
            ;;
    esac
}

install_terragrunt() {
    install_binary "terragrunt" \
        "https://github.com/gruntwork-io/terragrunt/releases/download/v${TERRAGRUNT_VERSION}/terragrunt_linux_${BIN_ARCH}" \
        "$BIN_DIR/terragrunt"
}

install_opentofu() {
    log_info "Installing opentofu..."
    install_packages unzip
    local tmp_dir
    tmp_dir="$(mktemp -d)"
    trap "rm -rf '${tmp_dir}'" RETURN
    curl -fsSL "https://github.com/opentofu/opentofu/releases/download/v${OPENTOFU_VERSION}/tofu_${OPENTOFU_VERSION}_linux_${BIN_ARCH}.zip" \
        -o "${tmp_dir}/tofu.zip"
    unzip -q "${tmp_dir}/tofu.zip" tofu -d "${tmp_dir}"
    install -m 0755 "${tmp_dir}/tofu" "$BIN_DIR/tofu"
}

install_openbao() {
    log_info "Installing openbao..."
    local tmp_dir
    tmp_dir="$(mktemp -d)"
    trap "rm -rf '${tmp_dir}'" RETURN
    case "$OS_ID" in
        ubuntu)
            local deb_file="${tmp_dir}/openbao.deb"
            curl -fsSL "https://github.com/openbao/openbao/releases/download/v${OPENBAO_VERSION}/openbao_${OPENBAO_VERSION}_linux_${BIN_ARCH}.deb" \
                -o "${deb_file}"
            sudo dpkg -i "${deb_file}"
            ;;
        rocky)
            local rpm_file="${tmp_dir}/openbao.rpm"
            curl -fsSL "https://github.com/openbao/openbao/releases/download/v${OPENBAO_VERSION}/openbao_${OPENBAO_VERSION}_linux_${BIN_ARCH}.rpm" \
                -o "${rpm_file}"
            sudo rpm -i "${rpm_file}"
            ;;
    esac
}

install_helm() {
    log_info "Installing helm ${HELM_VERSION}..."
    local tmp_dir
    tmp_dir="$(mktemp -d)"
    trap "rm -rf '${tmp_dir}'" RETURN
    curl -fsSL "https://get.helm.sh/helm-v${HELM_VERSION}-linux-${BIN_ARCH}.tar.gz" \
        | tar xz -C "$tmp_dir"
    install -m 0755 "$tmp_dir/linux-${BIN_ARCH}/helm" "$BIN_DIR/helm"
}

install_k9s() {
    install_binary "k9s" \
        "https://github.com/derailed/k9s/releases/download/v${K9S_VERSION}/k9s_Linux_${BIN_ARCH}.tar.gz" \
        "$BIN_DIR/k9s"
}

install_kubie() {
    install_binary "kubie" \
        "https://github.com/sbstp/kubie/releases/download/v${KUBIE_VERSION}/kubie-linux-${BIN_ARCH}" \
        "$BIN_DIR/kubie"
}

install_age() {
    log_info "Installing age ${AGE_VERSION}..."
    local tmp_dir
    tmp_dir="$(mktemp -d)"
    trap "rm -rf '${tmp_dir}'" RETURN
    curl -fsSL "https://github.com/FiloSottile/age/releases/download/v${AGE_VERSION}/age-v${AGE_VERSION}-linux-${BIN_ARCH}.tar.gz" \
        | tar xz -C "$tmp_dir"
    install -m 0755 "$tmp_dir/age/age"        "$BIN_DIR/age"
    install -m 0755 "$tmp_dir/age/age-keygen" "$BIN_DIR/age-keygen"
}

install_sops() {
    log_info "Installing sops ${SOPS_VERSION}..."
    ensure_installed "age" install_age
    local tmp_dir
    tmp_dir="$(mktemp -d)"
    trap "rm -rf '${tmp_dir}'" RETURN
    case "$OS_ID" in
        ubuntu)
            local deb_file="${tmp_dir}/sops.deb"
            curl -fsSL "https://github.com/getsops/sops/releases/download/v${SOPS_VERSION}/sops_${SOPS_VERSION}_${BIN_ARCH}.deb" \
                -o "${deb_file}"
            sudo dpkg -i "${deb_file}"
            ;;
        rocky)
            install_binary "sops" \
                "https://github.com/getsops/sops/releases/download/v${SOPS_VERSION}/sops-v${SOPS_VERSION}.linux.${BIN_ARCH}" \
                "$BIN_DIR/sops"
            ;;
    esac
}

install_helmfile() {
    install_binary "helmfile" \
        "https://github.com/helmfile/helmfile/releases/download/v${HELMFILE_VERSION}/helmfile_${HELMFILE_VERSION}_linux_${BIN_ARCH}.tar.gz" \
        "$BIN_DIR/helmfile"
}

install_cilium() {
    install_binary "cilium" \
        "https://github.com/cilium/cilium-cli/releases/download/v${CILIUM_VERSION}/cilium-linux-${BIN_ARCH}.tar.gz" \
        "$BIN_DIR/cilium"
}

install_helm_diff_plugin() {
    if helm plugin list 2>/dev/null | grep -q "^diff"; then
        log_info "helm-diff plugin is already installed, skipping"
        return
    fi
    log_info "Installing helm-diff plugin..."
    helm plugin install --verify=false https://github.com/databus23/helm-diff
}

# ============================================================================
# Main
# ============================================================================

main() {
    log_info "=== Tool Installation (${OS_ID}) ==="

    if ! command -v terraform &>/dev/null || ! command -v packer &>/dev/null || ! command -v vault &>/dev/null; then
        install_hashicorp_tools
    fi

    ensure_installed "kubectl"    install_kubectl
    install_if_needed "terragrunt" "$TERRAGRUNT_VERSION" install_terragrunt
    install_if_needed "tofu"       "$OPENTOFU_VERSION"   install_opentofu
    install_if_needed "bao"        "$OPENBAO_VERSION"    install_openbao
    install_if_needed "helm"       "$HELM_VERSION"       install_helm
    install_helm_diff_plugin
    install_if_needed "k9s"        "$K9S_VERSION"        install_k9s
    install_if_needed "kubie"      "$KUBIE_VERSION"      install_kubie
    install_if_needed "age"        "$AGE_VERSION"        install_age
    install_if_needed "sops"       "$SOPS_VERSION"       install_sops
    install_if_needed "helmfile"   "$HELMFILE_VERSION"   install_helmfile
    install_if_needed "cilium"     "$CILIUM_VERSION"     install_cilium

    log_info "=== Installation complete ==="
}

main
