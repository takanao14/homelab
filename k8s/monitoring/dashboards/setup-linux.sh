#!/usr/bin/env bash
set -euo pipefail

# Install the toolchain `make dev` needs on Ubuntu 24.04 and Rocky 9/10.
#
#   podman          distro package (both ship a release new enough for compose)
#   podman-compose  the project's recommended pip install, run through pipx.
#                   Ubuntu 24.04 marks the system Python as externally managed
#                   (PEP 668), so a bare `pip3 install` is refused; pipx keeps
#                   both distros on one code path. Rocky carries pipx in EPEL.
#                   https://github.com/containers/podman-compose#installation
#   go              the official linux tarball unpacked into /usr/local/go.
#                   https://go.dev/doc/install
#
# Go is not taken from the distro: Ubuntu's golang-go is far older than this
# module's go directive and its packaging pins GOTOOLCHAIN=local, so it cannot
# download the toolchain go.mod asks for either. Rocky has no current Go at all
# in its base repositories.
#
# Idempotent: every step is skipped when it is already satisfied.
#
# Usage: ./setup-linux.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GO_MOD="${SCRIPT_DIR}/go.mod"
COMPOSE_FILE="${SCRIPT_DIR}/docker-compose.yml"
GO_INSTALL_DIR="/usr/local/go"
GO_PROFILE="/etc/profile.d/go.sh"

WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT

# Package-manager verbs, filled in by check_host.
PKG_UPDATE=()
PKG_INSTALL=()
OS_ID=""

log() { printf '\033[0;32m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[0;33mwarn:\033[0m %s\n' "$*" >&2; }
die() {
  printf '\033[0;31merror:\033[0m %s\n' "$*" >&2
  exit 1
}

check_host() {
  # pipx installs into the invoking user's home, so running the whole script
  # under sudo would put podman-compose in root's home instead.
  [[ $EUID -ne 0 ]] || die "run as a regular user; the script calls sudo only where it needs to"
  command -v sudo >/dev/null || die "sudo is required"

  [[ -r /etc/os-release ]] || die "/etc/os-release not found; cannot identify the distribution"
  # shellcheck source=/dev/null
  . /etc/os-release
  OS_ID="${ID:-unknown}"

  case "$OS_ID" in
    ubuntu | debian)
      PKG_UPDATE=(sudo apt-get update -qq)
      # `sudo env VAR=val` survives sudo's env reset and does not depend on the
      # sudoers `setenv` option (a bare `sudo VAR=val` can be rejected).
      PKG_INSTALL=(sudo env DEBIAN_FRONTEND=noninteractive apt-get install -y)
      [[ "$OS_ID" != "ubuntu" || "${VERSION_ID:-}" == "24.04" ]] ||
        warn "tested on Ubuntu 24.04, found ${VERSION_ID:-unknown}; continuing"
      ;;
    rocky)
      PKG_UPDATE=(sudo dnf makecache -q)
      PKG_INSTALL=(sudo dnf install -y)
      ;;
    *)
      die "unsupported distribution: ${OS_ID} (expected ubuntu, debian or rocky)"
      ;;
  esac
}

# ensure_commands <command>:<package>...
# Keyed on the command, not the package: Rocky satisfies curl with
# curl-minimal, and asking dnf for `curl` there raises a package conflict.
ensure_commands() {
  local spec cmd missing=()
  for spec in "$@"; do
    cmd="${spec%%:*}"
    command -v "$cmd" >/dev/null 2>&1 || missing+=("${spec#*:}")
  done
  if ((${#missing[@]} == 0)); then
    log "already installed: ${*%%:*}"
    return
  fi
  log "Installing packages: ${missing[*]}"
  "${PKG_UPDATE[@]}"
  "${PKG_INSTALL[@]}" "${missing[@]}"
}

ensure_pipx() {
  if command -v pipx >/dev/null 2>&1; then
    log "pipx already installed, skipping"
    return
  fi
  if [[ "$OS_ID" == "rocky" ]]; then
    # Rocky carries pipx in EPEL, not in the base repositories.
    if ! rpm -q epel-release >/dev/null 2>&1; then
      log "Enabling EPEL (provides pipx)"
      "${PKG_UPDATE[@]}"
      "${PKG_INSTALL[@]}" epel-release
    fi
  fi
  log "Installing pipx"
  "${PKG_UPDATE[@]}"
  "${PKG_INSTALL[@]}" python3 python3-pip pipx
}

# version_ge <a> <b> -> true when a >= b
version_ge() { [[ "$(printf '%s\n%s\n' "$2" "$1" | sort -V | head -n1)" == "$2" ]]; }

detect_go() {
  if command -v go >/dev/null 2>&1; then
    command -v go
  elif [[ -x "${GO_INSTALL_DIR}/bin/go" ]]; then
    echo "${GO_INSTALL_DIR}/bin/go"
  fi
}

ensure_go() {
  local required current go_bin latest arch tarball expected actual

  # go.mod is the single source of truth for the minimum version.
  required="$(awk '$1 == "go" { print $2; exit }' "$GO_MOD")"
  [[ -n "$required" ]] || die "could not read the go directive from ${GO_MOD}"

  go_bin="$(detect_go)"
  if [[ -n "$go_bin" ]]; then
    current="$("$go_bin" version | awk '{print $3}')"
    current="${current#go}"
    if version_ge "$current" "$required"; then
      log "Go ${current} already satisfies go.mod (>= ${required}), skipping"
      return
    fi
    log "Go ${current} is older than go.mod's ${required}, installing the current release"
  else
    log "Go not found, installing the current release (go.mod needs >= ${required})"
  fi

  # go.dev/VERSION?m=text is the endpoint the Go toolchain itself uses to
  # resolve "the current stable release".
  latest="$(curl -fsSL 'https://go.dev/VERSION?m=text' | head -n1)"
  [[ "$latest" == go* ]] || die "unexpected reply from go.dev/VERSION: ${latest}"
  version_ge "${latest#go}" "$required" ||
    die "current Go release ${latest} is older than go.mod's ${required}"

  arch="$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')"
  [[ "$arch" == "amd64" || "$arch" == "arm64" ]] ||
    die "unsupported architecture: $(uname -m)"
  tarball="${latest}.linux-${arch}.tar.gz"

  log "Downloading ${tarball}"
  curl -fsSL -o "${WORKDIR}/${tarball}" "https://go.dev/dl/${tarball}"
  # go.dev/dl/ redirects downloads to dl.google.com, which is also where the
  # per-file .sha256 sidecars live; go.dev itself serves HTML for that path.
  expected="$(curl -fsSL "https://dl.google.com/go/${tarball}.sha256")"
  actual="$(sha256sum "${WORKDIR}/${tarball}" | awk '{print $1}')"
  [[ "$expected" == "$actual" ]] ||
    die "checksum mismatch for ${tarball}: expected ${expected}, got ${actual}"

  # Per the official instructions: remove any previous install, then extract.
  log "Installing Go to ${GO_INSTALL_DIR}"
  sudo rm -rf "$GO_INSTALL_DIR"
  sudo tar -C /usr/local -xzf "${WORKDIR}/${tarball}"

  # System-wide PATH entry, as documented for a /usr/local/go install.
  # shellcheck disable=SC2016  # $PATH must stay literal in the profile snippet
  printf 'export PATH=$PATH:%s/bin\n' "$GO_INSTALL_DIR" |
    sudo tee "$GO_PROFILE" >/dev/null
  sudo chmod 0644 "$GO_PROFILE"
  export PATH="${PATH}:${GO_INSTALL_DIR}/bin"
  log "Installed $(go version)"
}

ensure_podman_compose() {
  if command -v podman-compose >/dev/null 2>&1; then
    log "podman-compose already installed ($(podman-compose --version 2>/dev/null | head -n1)), skipping"
    return
  fi
  log "Installing podman-compose with pipx"
  pipx install podman-compose
  pipx ensurepath
  export PATH="${HOME}/.local/bin:${PATH}"
}

check_rootless_prerequisites() {
  local mode

  # The compose file bind-mounts provisioning/ and the generated dashboards.
  # Grafana runs as UID 472, which rootless podman maps to a subuid that is not
  # the owner of those files, so an unreadable $HOME yields an empty Grafana
  # with no error in the logs.
  mode="$(stat -c '%A' "$HOME")"
  if [[ "${mode:9:1}" != "x" ]]; then
    warn "${HOME} is ${mode}: rootless podman cannot traverse it as UID 472."
    warn "Grafana will start with no datasources and no dashboards."
    warn "Fix with 'chmod o+x ${HOME}', or add 'user: \"0\"' to the grafana service."
  fi

  # Rootless podman needs a subuid/subgid range for the invoking user. Cloud
  # images usually provide one, but accounts created without useradd defaults
  # (LDAP, some Rocky kickstarts) do not.
  if ! grep -q "^${USER}:" /etc/subuid 2>/dev/null; then
    warn "no /etc/subuid range for ${USER}: rootless podman will refuse to start."
    warn "Fix with: sudo usermod --add-subuids 100000-165535 --add-subgids 100000-165535 ${USER}"
  fi

  # SELinux (enforcing by default on Rocky) blocks a container from reading an
  # unlabelled bind mount. The compose file carries :z for this; flag it if the
  # labels were dropped.
  if command -v getenforce >/dev/null 2>&1 && [[ "$(getenforce)" == "Enforcing" ]]; then
    if ! grep -q ':z$' "$COMPOSE_FILE"; then
      warn "SELinux is enforcing but the volumes in docker-compose.yml carry no :z"
      warn "label; Grafana will not be able to read the mounted dashboards."
    fi
  fi

  # Podman has no implicit default registry, and Ubuntu ships registries.conf
  # with no unqualified-search-registries, so a short image name simply fails
  # to resolve. The compose file spells the registry out; flag it if undone.
  if ! grep -qE '^[[:space:]]*image:[[:space:]]*[^[:space:]/]+\.[^[:space:]/]+/' "$COMPOSE_FILE"; then
    warn "the image in docker-compose.yml is not registry-qualified; podman"
    warn "cannot resolve a short name without unqualified-search-registries."
  fi
}

main() {
  check_host
  log "Detected ${OS_ID} ${VERSION_ID:-unknown}"
  ensure_commands curl:curl make:make tar:tar podman:podman
  ensure_pipx
  ensure_go
  ensure_podman_compose
  check_rootless_prerequisites

  cat <<EOF

$(log "Setup complete")

Open a new shell (or run: export PATH=\$PATH:${GO_INSTALL_DIR}/bin) so go and
podman-compose are on PATH, then:

  cd ${SCRIPT_DIR}
  cp .env.example .env    # set PROMETHEUS_URL / LOKI_URL
  make dev

Grafana: http://localhost:3000
EOF
}

main "$@"
