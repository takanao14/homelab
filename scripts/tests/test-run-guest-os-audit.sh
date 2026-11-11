#!/usr/bin/env bash
set -euo pipefail
script_dir=$(cd "$(dirname "$0")/.." && pwd)
umask 077
workdir=$(mktemp -d)
trap 'rm -rf "$workdir"' EXIT
mkdir -p "$workdir/repo/scripts" "$workdir/bin" "$workdir/tmp" \
    "$workdir/repo/tf/vm/node1/service" "$workdir/repo/tf/vm/node1/test"
cp "$script_dir"/audit-guest-os-recipes.{sh,jq} "$script_dir/run-guest-os-audit.sh" "$workdir/repo/scripts/"
touch "$workdir/repo/tf/vm/node1/service/terragrunt.hcl" "$workdir/repo/tf/vm/node1/test/terragrunt.hcl"
cat > "$workdir/bin/direnv" <<'STUB'
#!/usr/bin/env bash
[ "$1" = exec ] && [ "$2" = . ] || exit 90
shift 2
exec "$@"
STUB
cat > "$workdir/bin/terragrunt" <<'STUB'
#!/usr/bin/env bash
[ "$*" = 'run --no-auto-init --log-disable --tf-forward-stdout -- output -json guest_os_recipes' ] || exit 90
if [ "${STUB_MODE:-}" = output-failure ]; then
    echo redaction-test-sentinel >&2; exit 1
fi
case "${STUB_MODE:-}" in
    init-required) echo 'Initialization required redaction-test-sentinel' >&2; exit 1 ;;
    output-missing) echo 'Output "guest_os_recipes" not found redaction-test-sentinel' >&2; exit 1 ;;
    no-outputs) echo 'Warning: No outputs found redaction-test-sentinel'; exit 0 ;;
    mixed-missing) if [ "${PWD##*/}" = test ]; then echo 'Output "guest_os_recipes" not found redaction-test-sentinel' >&2; exit 1; fi ;;
    mixed-auth) echo 'AccessDenied Output "guest_os_recipes" not found redaction-test-sentinel' >&2; exit 1 ;;
    direnv-blocked) echo '.envrc is blocked redaction-test-sentinel' >&2; exit 1 ;;
    backend-denied) echo 'AccessDenied redaction-test-sentinel' >&2; exit 1 ;;
esac
if [ "${STUB_MODE:-}" = invalid-output ]; then
    echo '{"password":"redaction-test-sentinel"}'; exit 0
fi
if [ "${PWD##*/}" = test ] || [ "${STUB_MODE:-}" = empty ]; then
    echo '{}'
else
    echo '{"service":{"kind":"vm","node":"node1","image":"local:iso/debian-13-base.img"}}'
fi
STUB
cat > "$workdir/bin/curl" <<'STUB'
#!/usr/bin/env bash
touch "$STUB_ROOT/queried"
if [ "${STUB_MODE:-}" = query-failure ]; then echo redaction-test-sentinel >&2; exit 1; fi
if [ "${STUB_MODE:-}" = invalid-observation ]; then echo redaction-test-sentinel; exit 0; fi
version=13
[ "${STUB_MODE:-}" != mismatch ] || version=12
printf '{"status":"success","data":{"resultType":"vector","result":[{"metric":{"nodename":"service","id":"debian","version_id":"%s","extra":"redaction-test-sentinel"},"value":[0,"1"]}]}}\n' "$version"
STUB
cat > "$workdir/bin/kubectl" <<'STUB'
#!/usr/bin/env bash
if [ "${STUB_MODE:-}" = forward-failure ]; then echo redaction-test-sentinel >&2; exit 1; fi
echo "$$" > "$STUB_ROOT/forward-pid"
echo 'Forwarding from 127.0.0.1:48765 -> 9090'
exec sleep 60
STUB
chmod +x "$workdir/bin/"*
count=0
check() {
    local mode=$1 expected=$2 actual=0
    shift 2
    rm -f "$workdir/queried"
    STUB_MODE=$mode STUB_ROOT=$workdir TMPDIR="$workdir/tmp" \
      PATH="$workdir/bin:$PATH" PROMETHEUS_URL="${TEST_PROMETHEUS_URL-http://localhost:9090}" \
      bash "$workdir/repo/scripts/run-guest-os-audit.sh" "$@" \
      > "$workdir/stdout" 2> "$workdir/stderr" || actual=$?
    [ "$actual" -eq "$expected" ] || { echo "Unexpected exit for $mode: $actual"; exit 1; }
    if grep -q redaction-test-sentinel "$workdir/stdout" "$workdir/stderr"; then
        echo 'Input was exposed.' >&2; exit 1
    fi
    [ -z "$(ls -A "$workdir/tmp")" ] || { echo 'Temporary files remain.'; exit 1; }
    if [ -f "$workdir/forward-pid" ]; then
        if kill -0 "$(cat "$workdir/forward-pid")" 2>/dev/null; then
            echo 'Port-forward remains running.'; exit 1
        fi
        rm "$workdir/forward-pid"
    fi
    count=$((count + 1))
}
check match 0
grep -q 'Audited=1 matched=1' "$workdir/stdout"
check match 0 tf/vm/node1/test tf/vm/node1/service
check empty 3
grep -q 'No audit targets' "$workdir/stdout"
[ ! -f "$workdir/queried" ]
check mismatch 1
check output-failure 2
check invalid-output 2
check init-required 2
grep -q 'needs initialization' "$workdir/stderr"
check output-missing 3
grep -q 'SKIP .*guest_os_recipes absent' "$workdir/stderr"
[ ! -f "$workdir/queried" ]
check no-outputs 3
check mixed-missing 0
grep -q 'Audited=1 matched=1' "$workdir/stdout"
check mixed-auth 2
check direnv-blocked 2
grep -q 'direnv is blocked' "$workdir/stderr"
check backend-denied 2
grep -q 'backend authentication' "$workdir/stderr"
check query-failure 2
check invalid-observation 2
check match 2 ../outside
TEST_PROMETHEUS_URL=
check match 0
check mismatch 1
check query-failure 2
check forward-failure 2
echo "$count audit runner cases passed."
