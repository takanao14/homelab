#!/usr/bin/env bash
set -euo pipefail

# Switch which single GPU workload runs on the prd-homelab cluster by scaling
# deployments. Only runs against the prd-homelab kube context.
#
# The switchable workloads are discovered from the cluster by the
# homelab/gpu-switchable label, not listed here. That label is already on every
# GPU workload's Deployment and is load-bearing for the Argo CD health checks in
# k8s/argocd/values-common.yaml, so a list in this script would be a second
# place to update whenever a GPU workload is added or removed. Adding one is
# now just adding the label. See ADR-0027.
#
# Avoids bash-4-only features (mapfile, associative arrays) so it also runs on
# macOS' stock /bin/bash.

LABEL_SELECTOR="homelab/gpu-switchable=true"
EXPECTED_CONTEXT="prd-homelab"

usage() {
  echo "Usage: $0 <workload|off|status>"
  echo "  workload  a deployment labelled ${LABEL_SELECTOR}"
  echo "  off       stop every GPU workload"
  echo "  status    show which workload currently holds the GPU"
  exit 1
}

[[ $# -ne 1 ]] && usage

current_context=$(kubectl config current-context)
if [[ "$current_context" != "$EXPECTED_CONTEXT" ]]; then
  echo "Error: current context is '$current_context', expected '$EXPECTED_CONTEXT'"
  exit 1
fi

# One tab-separated "namespace/name  desired  ready" record per line. The
# replica counts are only read by status, but come from the same query so the
# script never shows a state from a different moment than the one it acts on.
# readyReplicas is absent rather than 0 on a scaled-down Deployment, so that
# field arrives empty.
gpu_workloads=$(kubectl get deployment -A -l "$LABEL_SELECTOR" \
  -o jsonpath='{range .items[*]}{.metadata.namespace}/{.metadata.name}{"\t"}{.spec.replicas}{"\t"}{.status.readyReplicas}{"\n"}{end}')

if [[ -z "$gpu_workloads" ]]; then
  echo "Error: no deployment labelled ${LABEL_SELECTOR} exists on ${EXPECTED_CONTEXT}"
  exit 1
fi

scale_all_down() {
  while IFS=$'\t' read -r workload _ _; do
    kubectl scale deployment "${workload##*/}" -n "${workload%%/*}" --replicas=0
  done <<<"$gpu_workloads"
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
      # Scaling up is not the same as holding the GPU: the pod stays Pending
      # until the device is free, so report readiness rather than intent.
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

target="$1"

if [[ "$target" == "status" ]]; then
  show_status
  exit 0
fi

if [[ "$target" == "off" ]]; then
  scale_all_down
  echo "All GPU workloads stopped."
  exit 0
fi

# Workloads are addressed by deployment name alone, so resolve it to exactly one
# namespace. Names are unique today; refuse rather than guess if that changes.
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

# Scale everything down first, including the target: the single GPU must be
# released before the incoming pod can claim it.
scale_all_down
kubectl scale deployment "${match##*/}" -n "${match%%/*}" --replicas=1
echo "$target started, others stopped."
