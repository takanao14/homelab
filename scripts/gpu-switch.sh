#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "Usage: $0 [ollama|comfyui|off]"
  exit 1
}

[[ $# -ne 1 ]] && usage

case "$1" in
  ollama)
    kubectl scale deployment comfyui -n comfyui --replicas=0
    kubectl scale deployment ollama -n ollama --replicas=1
    echo "Ollama started, ComfyUI stopped."
    ;;
  comfyui)
    kubectl scale deployment ollama -n ollama --replicas=0
    kubectl scale deployment comfyui -n comfyui --replicas=1
    echo "ComfyUI started, Ollama stopped."
    ;;
  off)
    kubectl scale deployment ollama -n ollama --replicas=0
    kubectl scale deployment comfyui -n comfyui --replicas=0
    echo "All GPU workloads stopped."
    ;;
  *)
    usage
    ;;
esac
