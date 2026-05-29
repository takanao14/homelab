#!/usr/bin/env bash
set -euo pipefail

# namespace/deployment pairs for all GPU workloads
GPU_WORKLOADS=(
  "ollama/ollama"
  "comfyui/comfyui"
  "lemonade-server/lemonade-server"
)

usage() {
  echo "Usage: $0 [ollama|comfyui|lemonade-server|off]"
  exit 1
}

[[ $# -ne 1 ]] && usage

current_context=$(kubectl config current-context)
if [[ "$current_context" != "dev-homelab" ]]; then
  echo "Error: current context is '$current_context', expected 'dev-homelab'"
  exit 1
fi

scale_all_down() {
  for workload in "${GPU_WORKLOADS[@]}"; do
    kubectl scale deployment "${workload##*/}" -n "${workload%%/*}" --replicas=0
  done
}

case "$1" in
  off)
    scale_all_down
    echo "All GPU workloads stopped."
    ;;
  ollama|comfyui|lemonade-server)
    scale_all_down
    kubectl scale deployment "$1" -n "$1" --replicas=1
    echo "$1 started, others stopped."
    ;;
  *)
    usage
    ;;
esac
