#!/usr/bin/env bash
set -euo pipefail

[[ "$(uname)" == "Linux" ]] || exit 0

# renovate: datasource=github-releases depName=junegunn/fzf
readonly FZF_VERSION="${FZF_VERSION:-0.73.1}"
# renovate: datasource=github-releases depName=zellij-org/zellij
readonly ZELLIJ_VERSION="${ZELLIJ_VERSION:-0.44.3}"
# renovate: datasource=github-releases depName=sbstp/kubie
readonly KUBIE_VERSION="${KUBIE_VERSION:-0.28.0}"
# renovate: datasource=github-releases depName=derailed/k9s
readonly K9S_VERSION="${K9S_VERSION:-0.51.0}"
# renovate: datasource=github-releases depName=helmfile/helmfile
readonly HELMFILE_VERSION="${HELMFILE_VERSION:-1.5.3}"
# renovate: datasource=github-releases depName=k0sproject/k0sctl
readonly K0SCTL_VERSION="${K0SCTL_VERSION:-0.30.1}"
# renovate: datasource=github-releases depName=getsops/sops
readonly SOPS_VERSION="${SOPS_VERSION:-3.13.1}"
# renovate: datasource=github-releases depName=gruntwork-io/terragrunt
readonly TERRAGRUNT_VERSION="${TERRAGRUNT_VERSION:-1.0.7}"
# renovate: datasource=github-releases depName=opentofu/opentofu
readonly OPENTOFU_VERSION="${OPENTOFU_VERSION:-1.12.1}"
# renovate: datasource=github-releases depName=helm/helm
readonly HELM_VERSION="${HELM_VERSION:-4.2.0}"
# renovate: datasource=github-releases depName=FiloSottile/age
readonly AGE_VERSION="${AGE_VERSION:-1.3.1}"
# renovate: datasource=github-releases depName=cilium/cilium-cli
readonly CILIUM_VERSION="${CILIUM_VERSION:-0.19.4}"
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
readonly ANSIBLE_CORE_VERSION="${ANSIBLE_CORE_VERSION:-2.21.0}"
# renovate: datasource=pypi depName=ansible-lint
readonly ANSIBLE_LINT_VERSION="${ANSIBLE_LINT_VERSION:-26.4.0}"
# renovate: datasource=github-tags depName=aws/aws-cli
readonly AWS_CLI_VERSION="${AWS_CLI_VERSION:-2.35.2}"

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
PIPX_HOME_DIR="$(dirname "$VERSION_CACHE_DIR")/pipx"
readonly PIPX_HOME_DIR
# AWS CLI v2 installs its own self-contained tree here and symlinks `aws` into
# BIN_DIR. Deriving from BIN_DIR's parent matches AWS's own default of
# /usr/local/aws-cli for a system-wide (BIN_DIR=/usr/local/bin) install, and
# mirrors it under $HOME (~/.local/aws-cli) for a per-user install.
AWS_CLI_INSTALL_DIR="$(dirname "$BIN_DIR")/aws-cli"
readonly AWS_CLI_INSTALL_DIR
ARCH="$(uname -m)"
readonly ARCH
BIN_ARCH="$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')"
readonly BIN_ARCH

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

# This script is unprivileged by design: it never calls sudo. The OS packages it
# depends on (curl, unzip, gnupg, pipx, a >=3.12 python, ...) are provided by
# run_onchange_linux0_package.sh, which chezmoi runs first. Verify they exist and
# fail fast with a clear pointer rather than failing deep inside an install.
local_preflight() {
    local missing=() cmd
    for cmd in curl tar gzip unzip gpg git sha256sum awk install mktemp pipx; do
        command -v "$cmd" &>/dev/null || missing+=("$cmd")
    done
    if ! command -v python3.12 &>/dev/null && \
       ! { command -v python3 &>/dev/null && \
           python3 -c 'import sys; raise SystemExit(0 if sys.version_info[:2] >= (3, 12) else 1)' 2>/dev/null; }; then
        missing+=("python>=3.12")
    fi
    if [[ ${#missing[@]} -gt 0 ]]; then
        log_error "Missing prerequisites: ${missing[*]}"
        log_error "Run run_onchange_linux0_package.sh first (it installs the OS packages these tools need)."
        exit 1
    fi
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
    log_info "Installing zellij ${ZELLIJ_VERSION}..."
    # zellij publishes the checksum of the *extracted* binary, not the tarball,
    # so the generic install_binary (which verifies the archive) can't be used.
    # Extract first, then verify the binary against the .sha256sum.
    local tmp_dir base url
    make_tmp_dir tmp_dir
    base="zellij-${ARCH}-unknown-linux-musl"
    url="https://github.com/zellij-org/zellij/releases/download/v${ZELLIJ_VERSION}"
    curl -fsSL "${url}/${base}.tar.gz" -o "${tmp_dir}/${base}.tar.gz"
    tar xz -C "$tmp_dir" -f "${tmp_dir}/${base}.tar.gz"
    verify_sha256 "${tmp_dir}/zellij" "${url}/${base}.sha256sum" "zellij"
    install -m 0755 "${tmp_dir}/zellij" "$BIN_DIR/zellij"
}

# ============================================================================
# HashiCorp Tools
# ============================================================================

install_terragrunt() {
    local archive_name="terragrunt_linux_${BIN_ARCH}"
    install_binary "terragrunt" \
        "https://github.com/gruntwork-io/terragrunt/releases/download/v${TERRAGRUNT_VERSION}/${archive_name}" \
        "$BIN_DIR/terragrunt" \
        "https://github.com/gruntwork-io/terragrunt/releases/download/v${TERRAGRUNT_VERSION}/SHA256SUMS"
}

install_opentofu() {
    log_info "Installing opentofu..."
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

# ============================================================================
# Kubernetes Tools
# ============================================================================

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
    # sops only publishes checksums for the raw binaries (the .deb/.rpm packages
    # are absent from checksums.txt), so install the verified linux binary into
    # BIN_DIR on every distro. The age dependency is installed as a no-sudo
    # binary on every distro too (no apt path), keeping this script sudo-free.
    install_if_needed "age" "$AGE_VERSION" install_age
    install_binary "sops" \
        "https://github.com/getsops/sops/releases/download/v${SOPS_VERSION}/sops-v${SOPS_VERSION}.linux.${BIN_ARCH}" \
        "$BIN_DIR/sops" \
        "https://github.com/getsops/sops/releases/download/v${SOPS_VERSION}/sops-v${SOPS_VERSION}.checksums.txt"
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
# AWS Tools
# ============================================================================

# AWS CLI v2 installer zips are signed only with the AWS CLI Team PGP key (AWS
# publishes no sha256 file), so verify the .sig against this embedded key. The
# key is imported into a throwaway keyring holding only it, so a good signature
# already proves the zip came from AWS. The key currently expires 2026-07-07 --
# if verification later fails with an expired-key error, refresh the block from
# https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html
install_aws_cli() {
    log_info "Installing aws-cli ${AWS_CLI_VERSION}..."
    local tmp_dir gnupg_home url zip
    make_tmp_dir tmp_dir
    make_tmp_dir gnupg_home
    chmod 700 "$gnupg_home"
    url="https://awscli.amazonaws.com/awscli-exe-linux-${ARCH}-${AWS_CLI_VERSION}.zip"
    zip="${tmp_dir}/awscliv2.zip"
    curl -fsSL "$url"       -o "$zip"
    curl -fsSL "${url}.sig" -o "${zip}.sig"
    gpg --homedir "$gnupg_home" --batch --quiet --import <<'AWS_CLI_PGP_KEY'
-----BEGIN PGP PUBLIC KEY BLOCK-----

mQINBF2Cr7UBEADJZHcgusOJl7ENSyumXh85z0TRV0xJorM2B/JL0kHOyigQluUG
ZMLhENaG0bYatdrKP+3H91lvK050pXwnO/R7fB/FSTouki4ciIx5OuLlnJZIxSzx
PqGl0mkxImLNbGWoi6Lto0LYxqHN2iQtzlwTVmq9733zd3XfcXrZ3+LblHAgEt5G
TfNxEKJ8soPLyWmwDH6HWCnjZ/aIQRBTIQ05uVeEoYxSh6wOai7ss/KveoSNBbYz
gbdzoqI2Y8cgH2nbfgp3DSasaLZEdCSsIsK1u05CinE7k2qZ7KgKAUIcT/cR/grk
C6VwsnDU0OUCideXcQ8WeHutqvgZH1JgKDbznoIzeQHJD238GEu+eKhRHcz8/jeG
94zkcgJOz3KbZGYMiTh277Fvj9zzvZsbMBCedV1BTg3TqgvdX4bdkhf5cH+7NtWO
lrFj6UwAsGukBTAOxC0l/dnSmZhJ7Z1KmEWilro/gOrjtOxqRQutlIqG22TaqoPG
fYVN+en3Zwbt97kcgZDwqbuykNt64oZWc4XKCa3mprEGC3IbJTBFqglXmZ7l9ywG
EEUJYOlb2XrSuPWml39beWdKM8kzr1OjnlOm6+lpTRCBfo0wa9F8YZRhHPAkwKkX
XDeOGpWRj4ohOx0d2GWkyV5xyN14p2tQOCdOODmz80yUTgRpPVQUtOEhXQARAQAB
tCFBV1MgQ0xJIFRlYW0gPGF3cy1jbGlAYW1hem9uLmNvbT6JAlQEEwEIAD4CGwMF
CwkIBwIGFQoJCAsCBBYCAwECHgECF4AWIQT7Xbd/1cEYuAURraimMQrMRnJHXAUC
aGveYQUJDMpiLAAKCRCmMQrMRnJHXKBYD/9Ab0qQdGiO5hObchG8xh8Rpb4Mjyf6
0JrVo6m8GNjNj6BHkSc8fuTQJ/FaEhaQxj3pjZ3GXPrXjIIVChmICLlFuRXYzrXc
Pw0lniybypsZEVai5kO0tCNBCCFuMN9RsmmRG8mf7lC4FSTbUDmxG/QlYK+0IV/l
uJkzxWa+rySkdpm0JdqumjegNRgObdXHAQDWlubWQHWyZyIQ2B4U7AxqSpcdJp6I
S4Zds4wVLd1WE5pquYQ8vS2cNlDm4QNg8wTj58e3lKN47hXHMIb6CHxRnb947oJa
pg189LLPR5koh+EorNkA1wu5mAJtJvy5YMsppy2y/kIjp3lyY6AmPT1posgGk70Z
CmToEZ5rbd7ARExtlh76A0cabMDFlEHDIK8RNUOSRr7L64+KxOUegKBfQHb9dADY
qqiKqpCbKgvtWlds909Ms74JBgr2KwZCSY1HaOxnIr4CY43QRqAq5YHOay/mU+6w
hhmdF18vpyK0vfkvvGresWtSXbag7Hkt3XjaEw76BzxQH21EBDqU8WJVjHgU6ru+
DJTs+SxgJbaT3hb/vyjlw0lK+hFfhWKRwgOXH8vqducF95NRSUxtS4fpqxWVaw3Q
V2OWSjbne99A5EPEySzryFTKbMGwaTlAwMCwYevt4YT6eb7NmFhTx0Fis4TalUs+
j+c7Kg92pDx2uQ==
=OBAt
-----END PGP PUBLIC KEY BLOCK-----
AWS_CLI_PGP_KEY
    if ! gpg --homedir "$gnupg_home" --batch --verify "${zip}.sig" "$zip" 2>&1; then
        log_error "AWS CLI signature verification failed"
        exit 1
    fi
    unzip -q "$zip" -d "$tmp_dir"
    # --update makes a re-run (version bump) overwrite the existing install tree.
    "${tmp_dir}/aws/install" --bin-dir "$BIN_DIR" --install-dir "$AWS_CLI_INSTALL_DIR" --update
}

# ============================================================================
# Python Tools (pipx)
# ============================================================================

# ansible / ansible-lint install into isolated pipx venvs (upstream Ansible
# recommends pip/pipx, and ansible-lint is not reliably packaged by the distros).
# pipx itself is bootstrapped by run_onchange_linux0_package.sh; here we only
# verify it is present (local_preflight already guards this) and never apt/dnf.
ensure_pipx() {
    command -v pipx &>/dev/null && return
    log_error "pipx not found. Run run_onchange_linux0_package.sh first."
    exit 1
}

# ansible-core needs a controller Python >= 3.12 (Rocky 9 ships 3.9; linux0
# installs python3.12 there). Pick a >=3.12 interpreter to hand pipx via
# --python; do not install one here (that is linux0's job).
PIPX_PYTHON=""
resolve_pipx_python() {
    [[ -n "$PIPX_PYTHON" ]] && return
    if command -v python3.12 &>/dev/null; then
        PIPX_PYTHON="python3.12"
    elif command -v python3 &>/dev/null && \
         python3 -c 'import sys; raise SystemExit(0 if sys.version_info[:2] >= (3, 12) else 1)' 2>/dev/null; then
        PIPX_PYTHON="python3"
    else
        log_error "No python>=3.12 found. Run run_onchange_linux0_package.sh first."
        exit 1
    fi
}

# Route pipx so app symlinks land in BIN_DIR (already on PATH) and venvs live in
# PIPX_HOME_DIR. --force makes the install idempotent and lets a version bump
# reinstall over an existing venv; --python pins the venv to a 3.12 interpreter.
pipx_install() {
    ensure_pipx
    resolve_pipx_python
    PIPX_HOME="$PIPX_HOME_DIR" PIPX_BIN_DIR="$BIN_DIR" \
        pipx install --force --python "$PIPX_PYTHON" "$1"
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

    local_preflight

    install_if_needed "sheldon"  "$SHELDON_VERSION"  install_sheldon
    install_if_needed "starship" "$STARSHIP_VERSION" install_starship
    install_if_needed "direnv"   "$DIRENV_VERSION"   install_direnv
    install_if_needed "eza"      "$EZA_VERSION"      install_eza

    install_if_needed "fzf"    "$FZF_VERSION"    install_fzf
    install_if_needed "zellij" "$ZELLIJ_VERSION" install_zellij

    # terraform/packer/vault, kubectl and openbao (bao) are installed by
    # run_onchange_linux0_package.sh (they require apt/dnf/dpkg).
    install_if_needed "terragrunt" "$TERRAGRUNT_VERSION" install_terragrunt
    install_if_needed "tofu"       "$OPENTOFU_VERSION"   install_opentofu

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

    install_if_needed "aws" "$AWS_CLI_VERSION" install_aws_cli

    install_if_needed "ansible"      "$ANSIBLE_CORE_VERSION" install_ansible
    install_if_needed "ansible-lint" "$ANSIBLE_LINT_VERSION" install_ansible_lint

    log_info "=== Installation completed ==="
}

main
