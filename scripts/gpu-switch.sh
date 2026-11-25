#!/usr/bin/env bash
set -euo pipefail

# Switch the single active prd GPU workload discovered by label (ADR-0027).
# Compatible with macOS' Bash 3.

LABEL_SELECTOR="homelab/gpu-switchable=true"
EXPECTED_CONTEXT="prd-homelab"

usage() {
  echo "Usage: $0 [--wait] <workload|off|status>"
  echo "  workload  a deployment labelled ${LABEL_SELECTOR}"
  echo "  off       stop every GPU workload"
  echo "  status    show which workload currently holds the GPU"
  echo "  --wait    wait for stopped workloads to terminate and the target to become ready"
  exit 1
}

WAIT=false
target=""
for arg in "$@"; do
  case "$arg" in
    --wait) WAIT=true ;;
    -*) usage ;;
    *)
      if [[ -n "$target" ]]; then
        usage
      fi
      target="$arg"
      ;;
  esac
done

[[ -z "$target" ]] && usage

current_context=$(kubectl config current-context)
if [[ "$current_context" != "$EXPECTED_CONTEXT" ]]; then
  echo "Error: current context is '$current_context', expected '$EXPECTED_CONTEXT'"
  exit 1
fi

# Snapshot tab-separated namespace/name, desired, and ready replica counts.
gpu_workloads=$(kubectl get deployment -A -l "$LABEL_SELECTOR" \
  -o jsonpath='{range .items[*]}{.metadata.namespace}/{.metadata.name}{"\t"}{.spec.replicas}{"\t"}{.status.readyReplicas}{"\n"}{end}')

if [[ -z "$gpu_workloads" ]]; then
  echo "Error: no deployment labelled ${LABEL_SELECTOR} exists on ${EXPECTED_CONTEXT}"
  exit 1
fi

scale_all_down() {
  local workload pods deadline
  while IFS=$'\t' read -r workload _ _; do
    kubectl scale deployment "${workload##*/}" -n "${workload%%/*}" --replicas=0
  done <<<"$gpu_workloads"

  if [[ "$WAIT" == true ]]; then
    deadline=$((SECONDS + 120))
    while true; do
      # Terminating pods can still hold the GPU after Deployment replica counts drop.
      pods=$(kubectl --request-timeout=5s get pods -A -l "$LABEL_SELECTOR" -o name)
      [[ -z "$pods" ]] && break
      if (( SECONDS >= deadline )); then
        echo "Error: timed out waiting for GPU workload pods to terminate." >&2
        return 1
      fi
      sleep 1
    done
  fi
}

list_targets() {
  while IFS=$'\t' read -r workload _ _; do
    echo "  ${workload##*/}"
  done <<<"$gpu_workloads"
  echo "  off"
}

show_status() {
  local workload desired ready active marker state
  active=""
  while IFS=$'\t' read -r workload desired ready; do
    if [[ "${desired:-0}" -gt 0 ]]; then
      active="yes"
      # Report readiness because scaled-up pods may still be Pending.
      if [[ "${ready:-0}" -gt 0 ]]; then
        marker="*"
        state="running (${ready}/${desired})"
      else
        marker="!"
        state="starting (0/${desired})"
      fi
    else
      marker=" "
      state="stopped"
    fi
    printf '%s %-18s %s\n' "$marker" "${workload##*/}" "$state"
  done <<<"$gpu_workloads"

  [[ -z "$active" ]] && echo "No GPU workload is running."
  return 0
}

if [[ "$target" == "status" ]]; then
  show_status
  exit 0
fi

if [[ "$target" == "off" ]]; then
  scale_all_down
  echo "All GPU workloads stopped."
  exit 0
fi

# Require deployment names to resolve to one namespace.
match=""
while IFS=$'\t' read -r workload _ _; do
  if [[ "${workload##*/}" == "$target" ]]; then
    if [[ -n "$match" ]]; then
      echo "Error: '$target' matches both $match and $workload"
      exit 1
    fi
    match="$workload"
  fi
done <<<"$gpu_workloads"

if [[ -z "$match" ]]; then
  echo "Error: '$target' is not a switchable GPU workload. Available:"
  list_targets
  exit 1
fi

# Release the GPU before scaling up the target.
scale_all_down
kubectl scale deployment "${match##*/}" -n "${match%%/*}" --replicas=1
if [[ "$WAIT" == true ]]; then
  kubectl rollout status deployment "${match##*/}" -n "${match%%/*}" --timeout=300s
fi
echo "$target started, others stopped."
