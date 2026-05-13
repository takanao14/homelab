#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly NC='\033[0m'

pass() { echo -e "${GREEN}✓ PASS${NC} $*"; }
fail() { echo -e "${RED}✗ FAIL${NC} $*"; FAILURES=$((FAILURES + 1)); }
info() { echo -e "${YELLOW}→${NC} $*" >&2; }

FAILURES=0

cleanup() {
    info "Cleaning up smoke-test namespace..."
    kubectl delete namespace smoke-test --ignore-not-found --timeout=60s || true
}
trap cleanup EXIT

# ── helpers ───────────────────────────────────────────────────────────────────

wait_for_external_ip() {
    local svc="$1" ns="$2" timeout="$3"
    local elapsed=0 interval=5 ip=""
    info "Waiting for EXTERNAL-IP on service ${ns}/${svc} (timeout: ${timeout}s)..."
    while [[ "$elapsed" -lt "$timeout" ]]; do
        ip=$(kubectl get svc "$svc" -n "$ns" \
            -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || true)
        [[ -n "$ip" ]] && echo "$ip" && return 0
        sleep "$interval"
        elapsed=$((elapsed + interval))
    done
    return 1
}

wait_job_complete() {
    local job="$1" ns="$2" timeout="$3"
    kubectl wait job/"$job" -n "$ns" --for=condition=complete --timeout="${timeout}s"
}

wait_job_failed() {
    local job="$1" ns="$2" timeout="$3"
    kubectl wait job/"$job" -n "$ns" --for=condition=failed --timeout="${timeout}s"
}

get_job_log() {
    local job="$1" ns="$2"
    local pod
    pod=$(kubectl get pods -n "$ns" -l "batch.kubernetes.io/job-name=${job}" \
        -o jsonpath='{.items[0].metadata.name}')
    kubectl logs -n "$ns" "$pod"
}

# ── setup ─────────────────────────────────────────────────────────────────────

info "=== Smoke Test ==="

# Apply base resources upfront; netpol-jobs and pvc jobs are applied in each test
# after their dependencies are confirmed ready.
kubectl apply -f "$SCRIPT_DIR/l2lb.yaml"
kubectl apply -f "$SCRIPT_DIR/pvc.yaml"
kubectl apply -f "$SCRIPT_DIR/netpol.yaml"

# ── test: L2 LoadBalancer ─────────────────────────────────────────────────────

info "--- Test: L2 LoadBalancer ---"

EXTERNAL_IP=""
if ! EXTERNAL_IP=$(wait_for_external_ip "smoke-nginx" "smoke-test" 120); then
    fail "L2LB: timed out waiting for EXTERNAL-IP"
else
    info "EXTERNAL-IP assigned: ${EXTERNAL_IP}"
    kubectl rollout status deployment/smoke-nginx -n smoke-test --timeout=60s

    # L2 announcement may take a few seconds to propagate; retry up to ~30s.
    HTTP_CODE="000"
    for i in $(seq 1 6); do
        HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout 5 --max-time 10 \
            "http://${EXTERNAL_IP}" || true)
        [[ "$HTTP_CODE" == "200" ]] && break
        info "Attempt ${i}/6: HTTP ${HTTP_CODE} — retrying in 5s..."
        sleep 5
    done
    if [[ "$HTTP_CODE" == "200" ]]; then
        pass "L2LB: HTTP 200 from ${EXTERNAL_IP}"
    else
        fail "L2LB: expected HTTP 200, got ${HTTP_CODE} from ${EXTERNAL_IP}"
    fi
fi

# ── test: ClusterIP (kube-proxy replacement) ──────────────────────────────────

info "--- Test: ClusterIP (kube-proxy replacement) ---"

kubectl apply -f "$SCRIPT_DIR/clusterip.yaml"

if wait_job_complete "smoke-clusterip-check" "smoke-test" 60; then
    pass "ClusterIP: pod reached smoke-nginx via ClusterIP service"
else
    fail "ClusterIP: pod could not reach smoke-nginx via ClusterIP service"
fi

# ── test: NetworkPolicy ───────────────────────────────────────────────────────

info "--- Test: NetworkPolicy ---"

# Wait for the dedicated server to be ready before launching client jobs,
# so that a "connection refused" from an unready server doesn't cause a false negative.
kubectl rollout status deployment/netpol-server -n smoke-test --timeout=60s
kubectl apply -f "$SCRIPT_DIR/netpol-jobs.yaml"

if wait_job_complete "smoke-netpol-allowed" "smoke-test" 60; then
    pass "NetworkPolicy: allowed client (role=client-allowed) reached server"
else
    fail "NetworkPolicy: allowed client could not reach server"
fi

# backoffLimit: 0 ensures the Job fails immediately when the pod times out.
if wait_job_failed "smoke-netpol-denied" "smoke-test" 30; then
    pass "NetworkPolicy: denied client (role=client-denied) was blocked"
else
    fail "NetworkPolicy: denied client was NOT blocked (or timed out unexpectedly)"
fi

# ── test: PVC ─────────────────────────────────────────────────────────────────

info "--- Test: PVC ---"

info "Waiting for PVC to be Bound..."
kubectl wait pvc/smoke-pvc -n smoke-test \
    --for=jsonpath='{.status.phase}'=Bound \
    --timeout=120s

info "Running write job..."
if wait_job_complete "smoke-pvc-write" "smoke-test" 60; then
    kubectl apply -f "$SCRIPT_DIR/pvc-read-job.yaml"
    if wait_job_complete "smoke-pvc-read" "smoke-test" 60; then
        ACTUAL=$(get_job_log "smoke-pvc-read" "smoke-test" | tr -d '[:space:]')
        if [[ "$ACTUAL" == "smoke-test-ok" ]]; then
            pass "PVC: data written and read back correctly"
        else
            fail "PVC: expected 'smoke-test-ok', got '${ACTUAL}'"
        fi
    else
        fail "PVC: read job did not complete"
    fi
else
    fail "PVC: write job did not complete"
fi

# ── test: PVC Persistence ─────────────────────────────────────────────────────

info "--- Test: PVC Persistence (data survives pod lifecycle) ---"

# A third, independent pod mounts the same PVC and verifies the data written by
# the first pod is still intact after that pod has been terminated.
kubectl apply -f "$SCRIPT_DIR/pvc-persist-job.yaml"
if wait_job_complete "smoke-pvc-persist" "smoke-test" 60; then
    ACTUAL=$(get_job_log "smoke-pvc-persist" "smoke-test" | tr -d '[:space:]')
    if [[ "$ACTUAL" == "smoke-test-ok" ]]; then
        pass "PVC Persistence: data intact after original pod terminated"
    else
        fail "PVC Persistence: expected 'smoke-test-ok', got '${ACTUAL}'"
    fi
else
    fail "PVC Persistence: verification job did not complete"
fi

# ── summary ───────────────────────────────────────────────────────────────────

echo ""
if [[ "$FAILURES" -eq 0 ]]; then
    echo -e "${GREEN}All smoke tests passed.${NC}"
    exit 0
else
    echo -e "${RED}${FAILURES} smoke test(s) failed.${NC}"
    exit 1
fi
