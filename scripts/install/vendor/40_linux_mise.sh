#!/usr/bin/env bash
set -euo pipefail

# mise-config-sha256: bfe87fdcd24d5438ccb1ad2c9c3437a0ba9f45a9d7c169a7950313cd1238723f
# mise-lock-sha256: 0ee0895c41ffc090ce049f62346888b369052c6837a843daa32b9211a21eeb65

[[ "$(uname -s)" == "Linux" ]] || exit 0

# renovate: datasource=github-releases depName=jdx/mise
readonly MISE_VERSION="${MISE_VERSION:-2026.9.17}"
# renovate: datasource=github-releases depName=databus23/helm-diff
readonly HELM_DIFF_VERSION="${HELM_DIFF_VERSION:-3.15.15}"
readonly MISE_CONFIG_FILE="${MISE_CONFIG_FILE:-$HOME/.config/mise/config.toml}"
readonly MISE_INSTALL_SCOPE="${MISE_INSTALL_SCOPE:-user}"

case "$(uname -m)" in
    x86_64)
        readonly MISE_ARCH="x64"
        readonly MISE_SHA256="63049bc35fb9065e8dc35ac8b25fdae53e9bd6f1885a843aedeba398e046a1ee"
        ;;
    aarch64 | arm64)
        readonly MISE_ARCH="arm64"
        readonly MISE_SHA256="ab125726a93a2967e30290ade713a960b9fe8eed0ae0e2fc0976e327b08af70d"
        ;;
    *)
        echo "Unsupported architecture: $(uname -m)" >&2
        exit 1
        ;;
esac

if [[ ! -f "$MISE_CONFIG_FILE" ]]; then
    echo "mise config not found: $MISE_CONFIG_FILE" >&2
    exit 1
fi

case "$MISE_INSTALL_SCOPE" in
    user)
        readonly MISE_BIN_DIR="${MISE_BIN_DIR:-$HOME/.local/bin}"
        readonly MISE_DATA_DIR="${MISE_DATA_DIR:-$HOME/.local/share/mise}"
        readonly MISE_SYSTEM_FLAG=""
        readonly HELM_PLUGINS_DIR="${HELM_PLUGINS:-$HOME/.local/share/helm/plugins}"
        ;;
    system)
        [[ "$(id -u)" -eq 0 ]] || {
            echo "MISE_INSTALL_SCOPE=system must run as root" >&2
            exit 1
        }
        readonly MISE_BIN_DIR="${MISE_BIN_DIR:-/usr/local/bin}"
        readonly MISE_SYSTEM_DATA_DIR="${MISE_SYSTEM_DATA_DIR:-/usr/local/share/mise}"
        readonly MISE_DATA_DIR="${MISE_DATA_DIR:-$MISE_SYSTEM_DATA_DIR}"
        readonly MISE_SYSTEM_FLAG="--system"
        readonly HELM_PLUGINS_DIR="${HELM_PLUGINS:-/usr/local/share/helm/plugins}"
        ;;
    *)
        echo "Unsupported MISE_INSTALL_SCOPE: $MISE_INSTALL_SCOPE" >&2
        exit 1
        ;;
esac

readonly MISE_BIN="$MISE_BIN_DIR/mise"
readonly MISE_URL="https://github.com/jdx/mise/releases/download/v${MISE_VERSION}/mise-v${MISE_VERSION}-linux-${MISE_ARCH}"

TMP_PATHS=()

cleanup_tmp_paths() {
    local path

    for path in "${TMP_PATHS[@]}"; do
        rm -f "$path"
    done
}

trap cleanup_tmp_paths EXIT

mise_version_matches() {
    [[ -x "$MISE_BIN" ]] && "$MISE_BIN" --version 2>/dev/null | grep -q "^${MISE_VERSION} "
}

install_mise() {
    local tmp
    local actual

    tmp="$(mktemp)"
    TMP_PATHS+=("$tmp")
    curl -fsSL "$MISE_URL" -o "$tmp"
    actual="$(sha256sum "$tmp" | awk '{print $1}')"
    if [[ "$actual" != "$MISE_SHA256" ]]; then
        echo "mise checksum mismatch: expected $MISE_SHA256, got $actual" >&2
        exit 1
    fi
    install -d "$MISE_BIN_DIR"
    install -m 0755 "$tmp" "$MISE_BIN"
}

mise_version_matches || install_mise

# mise's minimum_release_age does not reach `uv tool install` for pinned pypi
# tools, so their transitive dependencies would resolve to the newest releases.
# Apply the same two-day cutoff here, scoped to this install only.
export UV_EXCLUDE_NEWER="${UV_EXCLUDE_NEWER:-2 days}"

export MISE_CONFIG_FILE MISE_DATA_DIR
if [[ "$MISE_INSTALL_SCOPE" == "system" ]]; then
    export MISE_SYSTEM_DATA_DIR
fi
if [[ -n "$MISE_SYSTEM_FLAG" ]]; then
    "$MISE_BIN" install --yes --locked "$MISE_SYSTEM_FLAG"
    "$MISE_BIN" reshim "$MISE_SYSTEM_FLAG"
else
    "$MISE_BIN" install --yes --locked
    "$MISE_BIN" reshim
fi

if [[ "$MISE_INSTALL_SCOPE" == "user" && ! -x "${KREW_ROOT:-$HOME/.krew}/bin/kubectl-krew" ]]; then
    "$MISE_BIN" exec -- krew install krew
fi

export HELM_PLUGINS="$HELM_PLUGINS_DIR"
if ! "$MISE_BIN" exec -- helm plugin list 2>/dev/null | awk -v version="$HELM_DIFF_VERSION" '
    $1 == "diff" && $2 == version { found = 1 }
    END { exit found ? 0 : 1 }
'; then
    if "$MISE_BIN" exec -- helm plugin list 2>/dev/null | awk '$1 == "diff" { found = 1 } END { exit found ? 0 : 1 }'; then
        "$MISE_BIN" exec -- helm plugin uninstall diff
    fi
    "$MISE_BIN" exec -- helm plugin install --verify=false \
        --version "v${HELM_DIFF_VERSION}" https://github.com/databus23/helm-diff
fi
