#!/usr/bin/env bash
set -euo pipefail

fail() { echo "$1" >&2; exit 2; }
script_dir=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$script_dir/.." && pwd)
for command in direnv terragrunt jq curl; do
    command -v "$command" >/dev/null || fail "Required command unavailable: $command"
done
if [ "$#" -eq 0 ]; then
    stacks=()
    for config in "$repo"/tf/{vm,lxc,k8s}/*/*/terragrunt.hcl; do
        [ -f "$config" ] || continue
        stack=${config#"$repo/"}
        stacks+=("${stack%/terragrunt.hcl}")
    done
else
    stacks=("$@")
fi
[ "${#stacks[@]}" -gt 0 ] || fail 'No guest stacks found.'
for stack in "${stacks[@]}"; do
    [[ "$stack" =~ ^tf/(vm|lxc|k8s)/[A-Za-z0-9_-]+/[A-Za-z0-9_-]+$ ]] &&
        [ -f "$repo/$stack/terragrunt.hcl" ] || fail 'Invalid guest stack path.'
done

umask 077
workdir=$(mktemp -d)
forward_pid=
cleanup() {
    if [ -n "$forward_pid" ]; then
        kill "$forward_pid" 2>/dev/null || true
        wait "$forward_pid" 2>/dev/null || true
    fi
    rm -rf "$workdir"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
output_failure() {
    case "$raw" in
        *'is blocked'*) reason='direnv is blocked; review .envrc and run direnv allow locally.' ;;
        *'Initialization required'*|*'initialization required'*|*'Auto-Init is disabled'*|*'auto-init is disabled'*)
            reason='the local Terraform cache needs initialization; run init separately.' ;;
        *'AccessDenied'*|*'ExpiredToken'*|*'InvalidAccessKeyId'*|*'No valid credential sources'*)
            reason='state backend authentication or access failed; check local credentials.' ;;
        *'No outputs found'*|*'Output "guest_os_recipes" not found'*)
            echo "SKIP $stack: guest_os_recipes absent" >&2
            unset raw
            return 0 ;;
        *) reason='output retrieval failed; inspect the named-output command locally.' ;;
    esac
    fail "Cannot read guest_os_recipes: $stack ($reason)"
}
recipes=()
for stack in "${stacks[@]}"; do
    # Capture diagnostics in memory; emit only fixed, classified failure reasons.
    if ! raw=$(cd "$repo/$stack" && DIRENV_LOG_FORMAT='' direnv exec . \
        terragrunt run --no-auto-init --log-disable --tf-forward-stdout -- \
        output -json guest_os_recipes 2>&1); then
        output_failure
        continue
    fi
    # Only validated, allowlisted scalars may leave memory or enter artifacts.
    if ! safe=$(printf '%s\n' "$raw" | jq -ces '
      if length == 1 then .[0] else error("invalid") end |
      def host: type == "string" and test("^[A-Za-z0-9][A-Za-z0-9.-]{0,252}$");
      if type == "object" and all(to_entries[];
        (.key | host) and (.value | type == "object" and
          keys == ["image", "kind", "node"] and
          (.kind == "vm" or .kind == "lxc") and (.node | host) and
          (.image == null or (.image | type == "string" and
            test("^[A-Za-z0-9_.-]+:[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$")))))
      then with_entries(.value |= {kind, node, image}) else error("invalid") end
    ' 2>/dev/null); then
        output_failure
        continue
    fi
    unset raw
    if [ "$safe" = '{}' ]; then
        echo "SKIP $stack: guest_os_recipes empty" >&2
        continue
    fi
    file="$workdir/${stack//\//-}-recipes.json"
    printf '%s\n' "$safe" > "$file"
    recipes+=("$file")
done

if [ "${#recipes[@]}" -eq 0 ]; then
    echo 'No audit targets: all selected stacks were skipped.'
    echo 'Audited=0 matched=0 mismatched=0 incomplete=0'
    exit 3
fi
prometheus_url=${PROMETHEUS_URL:-}
if [ -z "$prometheus_url" ]; then
    command -v kubectl >/dev/null || fail 'kubectl is required without PROMETHEUS_URL.'
    # The OS chooses a free port; read readiness through an in-memory pipe.
    mkfifo "$workdir/forward"
    kubectl --context "${AUDIT_KUBE_CONTEXT:-prd-homelab}" \
        --kubeconfig "${AUDIT_KUBECONFIG:-$HOME/.kube/prd.yaml}" \
        -n monitoring port-forward --address 127.0.0.1 \
        svc/prometheus-kube-prometheus-prometheus :9090 \
        > "$workdir/forward" 2>/dev/null &
    forward_pid=$!
    exec 3< "$workdir/forward"
    port=
    for ((attempt=0; attempt<10; attempt++)); do
        kill -0 "$forward_pid" 2>/dev/null || fail 'Prometheus port-forward failed.'
        if IFS= read -r -t 1 line <&3; then
            if [[ "$line" =~ ^Forwarding\ from\ 127\.0\.0\.1:([0-9]+)\ -\>\ 9090$ ]]; then
                port=${BASH_REMATCH[1]}
                break
            fi
        fi
    done
    [ -n "$port" ] || fail 'Prometheus port-forward timed out.'
    prometheus_url="http://127.0.0.1:$port"
fi
# PromQL needs a literal capture-group reference.
# shellcheck disable=SC2016
query='max by (nodename,id,version_id) (label_replace((node_os_info * on(instance,job) group_left(nodename) node_uname_info) and on(instance,job) (up == 1), "nodename", "$1", "nodename", "([^.]+).*"))'
if ! raw=$(curl --fail --silent --show-error --connect-timeout 5 --max-time 30 \
    --get --data-urlencode "query=$query" "${prometheus_url%/}/api/v1/query" 2>/dev/null); then
    fail 'Cannot query observed OS data.'
fi
if ! safe=$(printf '%s\n' "$raw" | jq -ces '
      if length == 1 then .[0] else error("invalid") end |
  def text($pattern): type == "string" and test($pattern);
  if .status == "success" and .data.resultType == "vector" and
    (.data.result | type == "array" and all(.[];
      (.metric.nodename | text("^[A-Za-z0-9][A-Za-z0-9.-]{0,252}$")) and
      (.metric.id | text("^[a-z][a-z0-9_]*$")) and
      (.metric.version_id | text("^[A-Za-z0-9.+_-]{1,64}$")) and
      (.value | type == "array" and length == 2 and (.[1] | tonumber) == 1)))
  then {status:"success",data:{resultType:"vector",result:[.data.result[] |
    {metric:(.metric | {nodename,id,version_id}),value:[0,"1"]}]}}
  else error("invalid") end
' 2>/dev/null); then
    fail 'Invalid observed OS data.'
fi
unset raw
printf '%s\n' "$safe" > "$workdir/observed-os.json"
bash "$script_dir/audit-guest-os-recipes.sh" \
    --recipes "${recipes[@]}" --observations "$workdir/observed-os.json"
