#!/bin/bash
set -euo pipefail

CRDS=(
  "ciliuml2announcementpolicies.cilium.io"
  "ciliumloadbalancerippools.cilium.io"
)
TIMEOUT=300
INTERVAL=5
elapsed=0

echo "Waiting for Cilium CRDs to be registered..."

for crd in "${CRDS[@]}"; do
  while ! kubectl get crd "$crd" &>/dev/null; do
    if [ "$elapsed" -ge "$TIMEOUT" ]; then
      echo "Timeout waiting for CRD: $crd"
      exit 1
    fi
    echo "  $crd not found, retrying in ${INTERVAL}s..."
    sleep "$INTERVAL"
    elapsed=$((elapsed + INTERVAL))
  done
  kubectl wait --for=condition=established "crd/$crd" --timeout=60s
  echo "  $crd is ready"
done

echo "All Cilium CRDs are ready"
