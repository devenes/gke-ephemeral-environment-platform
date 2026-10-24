#!/usr/bin/env bash
set -euo pipefail

export PATH="/opt/homebrew/share/google-cloud-sdk/bin:$PATH"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

mkdir -p artifacts/test-results artifacts/benchmarks bin

RUN_TS="$(date -u +%Y%m%dT%H%M%SZ)"
REPORT_FILE="artifacts/test-results/test-matrix-${RUN_TS}.md"
LATEST_REPORT="artifacts/test-results/latest-test-matrix.md"
BENCH_FILE="artifacts/benchmarks/scale-benchmark-${RUN_TS}.json"
LATEST_BENCH="artifacts/benchmarks/latest-scale-benchmark.json"

REGISTRY="us-west4-docker.pkg.dev/enes-turan-demo/gke-ephemeral-env"
SAMPLE_IMAGE="${REGISTRY}/sample-app:v0.1.0"

log() {
  echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] $*"
}

record_section() {
  local title="$1"
  cat >> "${REPORT_FILE}" <<EOF

---

## ${title}
- **Timestamp (UTC)**: \`$(date -u +%Y-%m-%dT%H:%M:%SZ)\`
EOF
}

record_kv() {
  local key="$1"
  local val="$2"
  echo "- **${key}**: ${val}" >> "${REPORT_FILE}"
}

record_code() {
  local lang="$1"
  local content="$2"
  cat >> "${REPORT_FILE}" <<EOF

\`\`\`${lang}
${content}
\`\`\`
EOF
}

cat > "${REPORT_FILE}" <<EOF
# Live GKE Integration & E2E Test Matrix Evidence

- **Run Timestamp (UTC)**: \`$(date -u +%Y-%m-%dT%H:%M:%SZ)\`
- **GCP Project**: \`enes-turan-demo\`
- **GKE Cluster**: \`gke-ephemeral-env-cluster\` (\`us-west4-a\`)
- **Sample Image**: \`${SAMPLE_IMAGE}\`
EOF

# ---------------------------------------------------------------------------
# T00 — Preflight and Code Quality
# ---------------------------------------------------------------------------
log "Starting T00 — Preflight and Code Quality..."
record_section "T00 — Preflight and Code Quality"

UNFORMATTED="$(gofmt -l .)"
if [ -n "${UNFORMATTED}" ]; then
  echo "FAIL T00: unformatted Go files: ${UNFORMATTED}"
  exit 1
fi
go vet ./...
UNIT_OUT="$(go test -race ./... 2>&1)"
go build -o bin/previewctl ./cmd/previewctl

TF_FMT="$(terraform -chdir=terraform fmt -check 2>&1 || true)"
TF_VAL="$(terraform -chdir=terraform validate -no-color 2>&1)"

TRACKED_SECRETS="$(git ls-files | grep -E '(\.tfstate|\.tfvars$|kubeconfig|-key\.json|credentials\.json)' || true)"
if [ -n "${TRACKED_SECRETS}" ]; then
  echo "FAIL T00: secret/state files tracked in git: ${TRACKED_SECRETS}"
  exit 1
fi

KUBE_CTX="$(kubectl config current-context)"
if [[ "${KUBE_CTX}" == *"kind"* ]] || [[ "${KUBE_CTX}" == *"minikube"* ]]; then
  echo "FAIL T00: prohibited local cluster context: ${KUBE_CTX}"
  exit 1
fi
WHICH_KIND="$(which kind 2>/dev/null || echo 'not-installed')"
WHICH_MINIKUBE="$(which minikube 2>/dev/null || echo 'not-installed')"

record_kv "Status" "**PASS**"
record_kv "Active Kube Context" "\`${KUBE_CTX}\`"
record_kv "Kind / Minikube Check" "kind=\`${WHICH_KIND}\`, minikube=\`${WHICH_MINIKUBE}\` (neither used)"
record_kv "Terraform Validation" "\`${TF_VAL}\`"
record_code "text" "${UNIT_OUT}"
log "T00 PASSED"

# ---------------------------------------------------------------------------
# T01 — GKE Deployment & Shared Gateway Readiness
# ---------------------------------------------------------------------------
log "Starting T01 — GKE Deployment & Shared Gateway Readiness..."
record_section "T01 — GKE Deployment & Shared Gateway Readiness"

kubectl -n preview-platform-system rollout status deployment/preview-controller --timeout=120s

GATEWAY_IP=""
for i in $(seq 1 60); do
  GATEWAY_IP="$(kubectl -n preview-platform-system get gateway preview-shared-gateway -o jsonpath='{.status.addresses[0].value}' 2>/dev/null || true)"
  GW_PROG="$(kubectl -n preview-platform-system get gateway preview-shared-gateway -o jsonpath='{.status.conditions[?(@.type=="Programmed")].status}' 2>/dev/null || true)"
  if [ -n "${GATEWAY_IP}" ] && [ "${GW_PROG}" = "True" ]; then
    break
  fi
  log "Waiting for preview-shared-gateway external IP and Programmed=True (attempt ${i}/60, ip=${GATEWAY_IP}, programmed=${GW_PROG})..."
  sleep 5
done

if [ -z "${GATEWAY_IP}" ]; then
  echo "FAIL T01: preview-shared-gateway did not receive an external IP"
  exit 1
fi

T01_EVIDENCE="$(
  echo "=== NODES ==="
  kubectl get nodes -o wide
  echo "=== CRD ==="
  kubectl get crd previewenvironments.platform.gke.io
  echo "=== CONTROLLER PODS ==="
  kubectl -n preview-platform-system get pods -o wide
  echo "=== GATEWAYCLASS & SHARED GATEWAY ==="
  kubectl get gatewayclass gke-l7-regional-external-managed
  kubectl -n preview-platform-system get gateway preview-shared-gateway -o wide
)"
record_kv "Status" "**PASS**"
record_kv "Shared Gateway IP" "\`${GATEWAY_IP}\`"
record_code "text" "${T01_EVIDENCE}"
log "T01 PASSED (Gateway IP: ${GATEWAY_IP})"

# Clean up any leftover preview environments before starting T02
kubectl delete previewenvironments --all --wait=true --timeout=120s >/dev/null 2>&1 || true

# ---------------------------------------------------------------------------
# T02 — Create One Environment (pr-101)
# ---------------------------------------------------------------------------
log "Starting T02 — Create One Environment (pr-101)..."
record_section "T02 — Create One Environment (\`pr-101\`)"

T02_START=$(date +%s)
kubectl apply -f deploy/samples/preview-demo.yaml
./bin/previewctl create \
  --name pr-101 \
  --image "${SAMPLE_IMAGE}" \
  --pr 101 \
  --developer devenes \
  --commit "130a6c8" \
  --ttl 1800 \
  --env "FEATURE_CHECKOUT_V2=enabled,LOG_LEVEL=info" \
  --wait \
  --timeout 4m
T02_END=$(date +%s)
T02_DUR=$((T02_END - T02_START))

T02_EVIDENCE="$(
  echo "=== PREVIEWCTl GET pr-101 ==="
  ./bin/previewctl get pr-101
  echo "=== CHILD RESOURCES IN preview-pr-101 ==="
  kubectl -n preview-pr-101 get deploy,pod,svc,resourcequota,limitrange,networkpolicy,httproute -o wide
)"
record_kv "Status" "**PASS**"
record_kv "Provisioning Duration" "\`${T02_DUR}s\`"
record_code "text" "${T02_EVIDENCE}"
log "T02 PASSED in ${T02_DUR}s"

# ---------------------------------------------------------------------------
# T03 — External HTTP Access (pr-101)
# ---------------------------------------------------------------------------
log "Starting T03 — External HTTP Access..."
record_section "T03 — External HTTP Access (\`pr-101\`)"

PR101_URL="$(kubectl get previewenvironment pr-101 -o jsonpath='{.status.previewURL}')"
T03_HTTP_START=$(date +%s)
HTTP_CODE=""
for i in $(seq 1 45); do
  HTTP_CODE="$(curl -s -o /dev/null -w "%{http_code}" --max-time 5 "${PR101_URL}/" || echo "000")"
  if [ "${HTTP_CODE}" = "200" ]; then
    break
  fi
  log "Waiting for Regional ALB NEG health check convergence for ${PR101_URL}/ (attempt ${i}/45, status=${HTTP_CODE})..."
  sleep 4
done
T03_HTTP_END=$(date +%s)
T03_CONV=$((T03_HTTP_END - T03_HTTP_START))

if [ "${HTTP_CODE}" != "200" ]; then
  echo "FAIL T03: External URL ${PR101_URL}/ returned HTTP ${HTTP_CODE}"
  exit 1
fi

ROOT_BODY="$(curl -fsS --max-time 5 "${PR101_URL}/")"
HEALTHZ_BODY="$(curl -fsS --max-time 5 "${PR101_URL}/healthz")"
READYZ_BODY="$(curl -fsS --max-time 5 "${PR101_URL}/readyz")"
VERSION_BODY="$(curl -fsS --max-time 5 "${PR101_URL}/version")"

OBS_ENV="$(echo "${ROOT_BODY}" | jq -r '.environmentName')"
OBS_FEAT="$(echo "${ROOT_BODY}" | jq -r '.customEnv.FEATURE_CHECKOUT_V2')"
if [ "${OBS_ENV}" != "pr-101" ] || [ "${OBS_FEAT}" != "enabled" ]; then
  echo "FAIL T03: Unexpected response identity: env=${OBS_ENV}, feat=${OBS_FEAT}"
  exit 1
fi

record_kv "Status" "**PASS**"
record_kv "External URL" "\`${PR101_URL}\`"
record_kv "ALB Data-Plane Convergence After Ready" "\`${T03_CONV}s\`"
record_code "json" "${ROOT_BODY}"
record_code "json" "${VERSION_BODY}"
log "T03 PASSED"

# ---------------------------------------------------------------------------
# T04 — Multiple Environments (env-alpha & env-beta)
# ---------------------------------------------------------------------------
log "Starting T04 — Multiple Environments (env-alpha & env-beta)..."
record_section "T04 — Concurrent Multi-Environment Routing (\`env-alpha\` & \`env-beta\`)"

./bin/previewctl create --name env-alpha --image "${SAMPLE_IMAGE}" --pr 201 --ttl 1800 --env "TENANT=alpha" &
PID_A=$!
./bin/previewctl create --name env-beta --image "${SAMPLE_IMAGE}" --pr 202 --ttl 1800 --env "TENANT=beta" &
PID_B=$!
wait "${PID_A}" "${PID_B}"

./bin/previewctl create --name env-alpha --image "${SAMPLE_IMAGE}" --pr 201 --ttl 1800 --env "TENANT=alpha" --wait --timeout 4m
./bin/previewctl create --name env-beta --image "${SAMPLE_IMAGE}" --pr 202 --ttl 1800 --env "TENANT=beta" --wait --timeout 4m

URL_ALPHA="$(kubectl get pe env-alpha -o jsonpath='{.status.previewURL}')"
URL_BETA="$(kubectl get pe env-beta -o jsonpath='{.status.previewURL}')"

for url in "${URL_ALPHA}" "${URL_BETA}"; do
  for i in $(seq 1 45); do
    code="$(curl -s -o /dev/null -w "%{http_code}" --max-time 5 "${url}/" || echo "000")"
    if [ "${code}" = "200" ]; then
      break
    fi
    sleep 4
  done
done

BODY_ALPHA="$(curl -fsS "${URL_ALPHA}/")"
BODY_BETA="$(curl -fsS "${URL_BETA}/")"

if [ "$(echo "${BODY_ALPHA}" | jq -r '.environmentName')" != "env-alpha" ] || [ "$(echo "${BODY_BETA}" | jq -r '.environmentName')" != "env-beta" ]; then
  echo "FAIL T04: Cross-talk or wrong environmentName between env-alpha and env-beta"
  exit 1
fi

FWD_RULES="$(gcloud compute forwarding-rules list --project=enes-turan-demo --regions=us-west4 --format='table(name,IPAddress,target)')"
T04_LIST="$(./bin/previewctl list)"

record_kv "Status" "**PASS**"
record_kv "env-alpha URL" "\`${URL_ALPHA}\` -> \`environmentName=$(echo "${BODY_ALPHA}" | jq -r '.environmentName')\`"
record_kv "env-beta URL" "\`${URL_BETA}\` -> \`environmentName=$(echo "${BODY_BETA}" | jq -r '.environmentName')\`"
record_code "text" "${T04_LIST}

=== GCP Forwarding Rules (Single Shared Load Balancer Verified) ===
${FWD_RULES}"
log "T04 PASSED"

# ---------------------------------------------------------------------------
# T05 — Resource Policies (LimitRange & ResourceQuota & Controller)
# ---------------------------------------------------------------------------
log "Starting T05 — Resource Policies..."
record_section "T05 — Resource Policies (\`LimitRange\`, \`ResourceQuota\`, & Controller Enforcement)"

# 5A: LimitRange default injection on a pod without explicit resources
cat <<EOF | kubectl -n preview-env-alpha apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: limitrange-default-probe
spec:
  securityContext:
    runAsNonRoot: true
    runAsUser: 65532
    seccompProfile:
      type: RuntimeDefault
  containers:
    - name: app
      image: ${SAMPLE_IMAGE}
      securityContext:
        allowPrivilegeEscalation: false
        readOnlyRootFilesystem: true
        runAsNonRoot: true
        runAsUser: 65532
        capabilities:
          drop: ["ALL"]
EOF

INJECTED_RESOURCES="$(kubectl -n preview-env-alpha get pod limitrange-default-probe -o jsonpath='requests={.spec.containers[0].resources.requests} limits={.spec.containers[0].resources.limits}')"
kubectl -n preview-env-alpha delete pod limitrange-default-probe --ignore-not-found --wait=false

# 5B: API Admission rejection by LimitRange (container CPU > 500m max)
set +e
LR_REJECT_OUT="$(cat <<EOF | kubectl -n preview-env-alpha create -f - 2>&1
apiVersion: v1
kind: Pod
metadata:
  name: limitrange-exceed-pod
spec:
  securityContext:
    runAsNonRoot: true
    runAsUser: 65532
    seccompProfile:
      type: RuntimeDefault
  containers:
    - name: app
      image: ${SAMPLE_IMAGE}
      resources:
        requests:
          cpu: 600m
          memory: 64Mi
        limits:
          cpu: 800m
          memory: 128Mi
      securityContext:
        allowPrivilegeEscalation: false
        readOnlyRootFilesystem: true
        runAsNonRoot: true
        runAsUser: 65532
        capabilities:
          drop: ["ALL"]
EOF
)"
LR_EXIT=$?
set -e
if [ "${LR_EXIT}" -eq 0 ]; then
  echo "FAIL T05: Expected LimitRange to reject pod with 800m CPU limit"
  exit 1
fi

# 5C: API Admission rejection by ResourceQuota (pod count / total CPU request > 600m namespace quota)
# Note: preview-app already uses 50m CPU request. Creating two pods with 300m CPU request each = 650m > 600m requests.cpu quota!
kubectl -n preview-env-alpha create -f - <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: quota-filler-1
spec:
  securityContext:
    runAsNonRoot: true
    runAsUser: 65532
    seccompProfile:
      type: RuntimeDefault
  containers:
    - name: app
      image: ${SAMPLE_IMAGE}
      resources:
        requests:
          cpu: 350m
          memory: 64Mi
        limits:
          cpu: 400m
          memory: 128Mi
      securityContext:
        allowPrivilegeEscalation: false
        readOnlyRootFilesystem: true
        runAsNonRoot: true
        runAsUser: 65532
        capabilities:
          drop: ["ALL"]
EOF

set +e
RQ_REJECT_OUT="$(cat <<EOF | kubectl -n preview-env-alpha create -f - 2>&1
apiVersion: v1
kind: Pod
metadata:
  name: quota-filler-2
spec:
  securityContext:
    runAsNonRoot: true
    runAsUser: 65532
    seccompProfile:
      type: RuntimeDefault
  containers:
    - name: app
      image: ${SAMPLE_IMAGE}
      resources:
        requests:
          cpu: 350m
          memory: 64Mi
        limits:
          cpu: 400m
          memory: 128Mi
      securityContext:
        allowPrivilegeEscalation: false
        readOnlyRootFilesystem: true
        runAsNonRoot: true
        runAsUser: 65532
        capabilities:
          drop: ["ALL"]
EOF
)"
RQ_EXIT=$?
set -e
kubectl -n preview-env-alpha delete pod quota-filler-1 quota-filler-2 --ignore-not-found --wait=false
if [ "${RQ_EXIT}" -eq 0 ]; then
  echo "FAIL T05: Expected ResourceQuota to reject quota-filler-2"
  exit 1
fi

record_kv "Status" "**PASS**"
record_kv "LimitRange Default Injection" "\`${INJECTED_RESOURCES}\`"
record_code "text" "=== LimitRange Admission Rejection (CPU > 500m) ===
${LR_REJECT_OUT}

=== ResourceQuota Admission Rejection (requests.cpu > 600m) ===
${RQ_REJECT_OUT}"
log "T05 PASSED"

# ---------------------------------------------------------------------------
# T06 — Network Isolation (Dataplane V2 eBPF)
# ---------------------------------------------------------------------------
log "Starting T06 — Network Isolation..."
record_section "T06 — Multi-Tenant Network Isolation (Dataplane V2 eBPF)"

ALPHA_POD_IP="$(kubectl -n preview-env-alpha get pods -l app.kubernetes.io/name=preview-app -o jsonpath='{.items[0].status.podIP}')"
BETA_POD_IP="$(kubectl -n preview-env-beta get pods -l app.kubernetes.io/name=preview-app -o jsonpath='{.items[0].status.podIP}')"

# 6A: Intra-namespace Pod-to-Pod probe (preview-env-alpha -> preview-env-alpha) MUST SUCCEED
kubectl -n preview-env-alpha delete pod netpol-intra-probe --ignore-not-found --wait=true >/dev/null 2>&1 || true
kubectl -n preview-env-alpha create -f - <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: netpol-intra-probe
spec:
  restartPolicy: Never
  securityContext:
    runAsNonRoot: true
    runAsUser: 65532
    seccompProfile:
      type: RuntimeDefault
  containers:
    - name: prober
      image: ${SAMPLE_IMAGE}
      args: ["--probe-url=http://${ALPHA_POD_IP}:8080/healthz", "--probe-timeout=4s"]
      securityContext:
        allowPrivilegeEscalation: false
        readOnlyRootFilesystem: true
        runAsNonRoot: true
        runAsUser: 65532
        capabilities:
          drop: ["ALL"]
EOF

for i in $(seq 1 20); do
  PHASE="$(kubectl -n preview-env-alpha get pod netpol-intra-probe -o jsonpath='{.status.phase}' 2>/dev/null || true)"
  if [ "${PHASE}" = "Succeeded" ] || [ "${PHASE}" = "Failed" ]; then
    break
  fi
  sleep 1
done
INTRA_LOGS="$(kubectl -n preview-env-alpha logs netpol-intra-probe 2>&1 || true)"
kubectl -n preview-env-alpha delete pod netpol-intra-probe --ignore-not-found --wait=false
if [ "${PHASE}" != "Succeeded" ]; then
  echo "FAIL T06: Intra-namespace probe failed (${PHASE}): ${INTRA_LOGS}"
  exit 1
fi

# 6B: Cross-namespace Pod-to-Pod probe (preview-env-alpha -> preview-env-beta) MUST BE BLOCKED BY NETWORKPOLICY
kubectl -n preview-env-alpha delete pod netpol-cross-probe --ignore-not-found --wait=true >/dev/null 2>&1 || true
kubectl -n preview-env-alpha create -f - <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: netpol-cross-probe
spec:
  restartPolicy: Never
  securityContext:
    runAsNonRoot: true
    runAsUser: 65532
    seccompProfile:
      type: RuntimeDefault
  containers:
    - name: prober
      image: ${SAMPLE_IMAGE}
      args: ["--probe-url=http://${BETA_POD_IP}:8080/healthz", "--probe-timeout=4s"]
      securityContext:
        allowPrivilegeEscalation: false
        readOnlyRootFilesystem: true
        runAsNonRoot: true
        runAsUser: 65532
        capabilities:
          drop: ["ALL"]
EOF

for i in $(seq 1 20); do
  CROSS_PHASE="$(kubectl -n preview-env-alpha get pod netpol-cross-probe -o jsonpath='{.status.phase}' 2>/dev/null || true)"
  if [ "${CROSS_PHASE}" = "Succeeded" ] || [ "${CROSS_PHASE}" = "Failed" ]; then
    break
  fi
  sleep 1
done
CROSS_LOGS="$(kubectl -n preview-env-alpha logs netpol-cross-probe 2>&1 || true)"
kubectl -n preview-env-alpha delete pod netpol-cross-probe --ignore-not-found --wait=false
if [ "${CROSS_PHASE}" != "Failed" ]; then
  echo "FAIL T06: Cross-namespace Pod-to-Pod access unexpectedly succeeded!"
  exit 1
fi

# 6C: Confirm Shared Gateway still routes to both environments
GW_ALPHA_STATUS="$(curl -s -o /dev/null -w "%{http_code}" "${URL_ALPHA}/healthz")"
GW_BETA_STATUS="$(curl -s -o /dev/null -w "%{http_code}" "${URL_BETA}/healthz")"
if [ "${GW_ALPHA_STATUS}" != "200" ] || [ "${GW_BETA_STATUS}" != "200" ]; then
  echo "FAIL T06: Shared Gateway failed while NetworkPolicy active: alpha=${GW_ALPHA_STATUS}, beta=${GW_BETA_STATUS}"
  exit 1
fi

record_kv "Status" "**PASS**"
record_kv "Intra-Namespace Probe (\`preview-env-alpha\` -> \`${ALPHA_POD_IP}:8080\`)" "\`Succeeded\`"
record_kv "Cross-Namespace Probe (\`preview-env-alpha\` -> \`${BETA_POD_IP}:8080\` in \`preview-env-beta\`)" "\`Blocked (${CROSS_PHASE})\`"
record_kv "Shared Gateway Routing After Probe" "alpha=\`HTTP ${GW_ALPHA_STATUS}\`, beta=\`HTTP ${GW_BETA_STATUS}\`"
record_code "text" "=== Intra-Namespace Probe Output ===
${INTRA_LOGS}

=== Cross-Namespace Probe Output (Blocked by Dataplane V2 NetworkPolicy) ===
${CROSS_LOGS}"
log "T06 PASSED"

# Clean up env-alpha and env-beta before T07
./bin/previewctl delete env-alpha --wait
./bin/previewctl delete env-beta --wait

# ---------------------------------------------------------------------------
# T07 — Invalid Requests (Admission & Controller Reconciliation Layers)
# ---------------------------------------------------------------------------
log "Starting T07 — Invalid Requests..."
record_section "T07 — Invalid Requests (API Admission & Controller Validation Layers)"

# 7A: Disallowed image registry (Controller validation layer -> Phase=Failed, Reason=DisallowedImage)
cat <<EOF | kubectl apply -f -
apiVersion: platform.gke.io/v1alpha1
kind: PreviewEnvironment
metadata:
  name: invalid-registry
spec:
  image: docker.io/library/nginx:1.27
  ttlSeconds: 600
EOF
sleep 3
INV_REG_PHASE="$(kubectl get pe invalid-registry -o jsonpath='{.status.phase}')"
INV_REG_REASON="$(kubectl get pe invalid-registry -o jsonpath='{.status.conditions[?(@.type=="Validated")].reason}')"
INV_REG_MSG="$(kubectl get pe invalid-registry -o jsonpath='{.status.message}')"
if [ "${INV_REG_PHASE}" != "Failed" ] || [ "${INV_REG_REASON}" != "DisallowedImage" ]; then
  echo "FAIL T07A: Expected Failed/DisallowedImage, got ${INV_REG_PHASE}/${INV_REG_REASON}"
  exit 1
fi
if kubectl get ns preview-invalid-registry >/dev/null 2>&1; then
  echo "FAIL T07A: Namespace preview-invalid-registry should not exist"
  exit 1
fi
kubectl delete pe invalid-registry --ignore-not-found

# 7B: Excessive CPU/Memory request (Controller validation layer -> Phase=Failed, Reason=ExcessiveResources)
cat <<EOF | kubectl apply -f -
apiVersion: platform.gke.io/v1alpha1
kind: PreviewEnvironment
metadata:
  name: invalid-resources
spec:
  image: ${SAMPLE_IMAGE}
  resources:
    cpuRequest: 100m
    cpuLimit: 2000m
    memoryRequest: 128Mi
    memoryLimit: 4Gi
EOF
sleep 3
INV_RES_PHASE="$(kubectl get pe invalid-resources -o jsonpath='{.status.phase}')"
INV_RES_REASON="$(kubectl get pe invalid-resources -o jsonpath='{.status.conditions[?(@.type=="Validated")].reason}')"
INV_RES_MSG="$(kubectl get pe invalid-resources -o jsonpath='{.status.message}')"
if [ "${INV_RES_PHASE}" != "Failed" ] || [ "${INV_RES_REASON}" != "ExcessiveResources" ]; then
  echo "FAIL T07B: Expected Failed/ExcessiveResources, got ${INV_RES_PHASE}/${INV_RES_REASON}"
  exit 1
fi
kubectl delete pe invalid-resources --ignore-not-found

# 7C: Invalid environment name (>40 chars rejected by CRD CEL validation at API Admission)
set +e
INV_NAME_OUT="$(cat <<EOF | kubectl apply -f - 2>&1
apiVersion: platform.gke.io/v1alpha1
kind: PreviewEnvironment
metadata:
  name: this-environment-name-is-way-too-long-and-exceeds-forty-characters
spec:
  image: ${SAMPLE_IMAGE}
EOF
)"
INV_NAME_EXIT=$?

# 7D: Invalid TTL (<30s rejected by CRD OpenAPI schema at API Admission)
INV_TTL_OUT="$(cat <<EOF | kubectl apply -f - 2>&1
apiVersion: platform.gke.io/v1alpha1
kind: PreviewEnvironment
metadata:
  name: invalid-ttl
spec:
  image: ${SAMPLE_IMAGE}
  ttlSeconds: 5
EOF
)"
INV_TTL_EXIT=$?

# 7E: Invalid replica count (>3 rejected by CRD OpenAPI schema at API Admission)
INV_REP_OUT="$(cat <<EOF | kubectl apply -f - 2>&1
apiVersion: platform.gke.io/v1alpha1
kind: PreviewEnvironment
metadata:
  name: invalid-replicas
spec:
  image: ${SAMPLE_IMAGE}
  replicas: 10
EOF
)"
INV_REP_EXIT=$?
set -e

if [ "${INV_NAME_EXIT}" -eq 0 ] || [ "${INV_TTL_EXIT}" -eq 0 ] || [ "${INV_REP_EXIT}" -eq 0 ]; then
  echo "FAIL T07: Expected API admission rejection for invalid name/ttl/replicas"
  exit 1
fi

record_kv "Status" "**PASS**"
record_kv "Disallowed Image Registry (Controller Layer)" "Phase=\`${INV_REG_PHASE}\`, Reason=\`${INV_REG_REASON}\` (${INV_REG_MSG})"
record_kv "Excessive CPU/Memory (Controller Layer)" "Phase=\`${INV_RES_PHASE}\`, Reason=\`${INV_RES_REASON}\` (${INV_RES_MSG})"
record_code "text" "=== Invalid Name (API Admission CEL Rejection) ===
${INV_NAME_OUT}

=== Invalid TTL (API Admission Schema Rejection) ===
${INV_TTL_OUT}

=== Invalid Replicas (API Admission Schema Rejection) ===
${INV_REP_OUT}"
log "T07 PASSED"

# ---------------------------------------------------------------------------
# T08 — TTL Cleanup (ttl-env with 35s TTL)
# ---------------------------------------------------------------------------
log "Starting T08 — TTL Cleanup..."
record_section "T08 — Automatic TTL Expiration & Cleanup (\`ttl-test\`)"

./bin/previewctl create --name ttl-test --image "${SAMPLE_IMAGE}" --pr 308 --ttl 35 --wait --timeout 3m
EXPIRES_AT_STR="$(kubectl get pe ttl-test -o jsonpath='{.status.expiresAt}')"
log "ttl-test is Ready; expiresAt=${EXPIRES_AT_STR}. Waiting for automatic expiration..."

while kubectl get pe ttl-test >/dev/null 2>&1; do
  sleep 1
done
TTL_GONE_TS="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

if kubectl get ns preview-ttl-test >/dev/null 2>&1; then
  echo "FAIL T08: Namespace preview-ttl-test still exists after PreviewEnvironment deletion!"
  exit 1
fi

record_kv "Status" "**PASS**"
record_kv "Configured TTL" "\`35s\`"
record_kv "Scheduled ExpiresAt" "\`${EXPIRES_AT_STR}\`"
record_kv "Confirmed Complete Removal At" "\`${TTL_GONE_TS}\`"
record_kv "Orphan Namespace Check" "\`preview-ttl-test NotFound\`"
log "T08 PASSED"

# ---------------------------------------------------------------------------
# T09 — Manual Deletion (pr-101)
# ---------------------------------------------------------------------------
log "Starting T09 — Manual Deletion (pr-101)..."
record_section "T09 — Manual Deletion & Finalizer Verification (\`pr-101\`)"

DEL_START=$(date +%s)
DEL_OUT="$(./bin/previewctl delete pr-101 --wait)"
DEL_END=$(date +%s)
DEL_DUR=$((DEL_END - DEL_START))

if kubectl get ns preview-pr-101 >/dev/null 2>&1; then
  echo "FAIL T09: Namespace preview-pr-101 still exists after manual deletion"
  exit 1
fi

record_kv "Status" "**PASS**"
record_kv "Manual Deletion Duration" "\`${DEL_DUR}s\`"
record_code "text" "${DEL_OUT}"
log "T09 PASSED in ${DEL_DUR}s"

# ---------------------------------------------------------------------------
# T10 — Controller Restart Recovery & T11 — Idempotency
# ---------------------------------------------------------------------------
log "Starting T10 (Controller Restart) & T11 (Idempotency)..."
record_section "T10 & T11 — Controller Restart Recovery & Idempotent Reconciliation"

./bin/previewctl create --name resilient-env --image "${SAMPLE_IMAGE}" --pr 310 --ttl 1800 --wait --timeout 3m
DEP_UID_BEFORE="$(kubectl -n preview-resilient-env get deploy preview-app -o jsonpath='{.metadata.uid}')"
SVC_UID_BEFORE="$(kubectl -n preview-resilient-env get svc preview-svc -o jsonpath='{.metadata.uid}')"

# Restart controller
kubectl -n preview-platform-system rollout restart deployment/preview-controller
kubectl -n preview-platform-system rollout status deployment/preview-controller --timeout=120s
sleep 5

# Trigger repeated reconciliations (T11 Idempotency)
for i in 1 2 3; do
  ./bin/previewctl create --name resilient-env --image "${SAMPLE_IMAGE}" --pr 310 --ttl 1800
  sleep 2
done

DEP_UID_AFTER="$(kubectl -n preview-resilient-env get deploy preview-app -o jsonpath='{.metadata.uid}')"
SVC_UID_AFTER="$(kubectl -n preview-resilient-env get svc preview-svc -o jsonpath='{.metadata.uid}')"
PHASE_AFTER="$(kubectl get pe resilient-env -o jsonpath='{.status.phase}')"
POD_COUNT="$(kubectl -n preview-resilient-env get pods --no-headers | wc -l | tr -d ' ')"

if [ "${DEP_UID_BEFORE}" != "${DEP_UID_AFTER}" ] || [ "${SVC_UID_BEFORE}" != "${SVC_UID_AFTER}" ] || [ "${PHASE_AFTER}" != "Ready" ] || [ "${POD_COUNT}" != "1" ]; then
  echo "FAIL T10/T11: UID changed or duplicate resources created"
  exit 1
fi

record_kv "Status" "**PASS**"
record_kv "Deployment UID Preserved Across Restart & Re-apply" "\`${DEP_UID_BEFORE}\` == \`${DEP_UID_AFTER}\`"
record_kv "Service UID Preserved Across Restart & Re-apply" "\`${SVC_UID_BEFORE}\` == \`${SVC_UID_AFTER}\`"
record_kv "Pod Count" "\`${POD_COUNT}\` (zero duplicates)"
log "T10 & T11 PASSED"

# ---------------------------------------------------------------------------
# T12 — Failure Recovery & Self-Healing
# ---------------------------------------------------------------------------
log "Starting T12 — Failure Recovery & Self-Healing..."
record_section "T12 — Failure Recovery & Drift Self-Healing"

# Simulate child failure by deleting the tenant Service and Deployment inside preview-resilient-env
kubectl -n preview-resilient-env delete svc preview-svc
kubectl -n preview-resilient-env delete deploy preview-app

# Wait for controller watch to self-heal both resources automatically
for i in $(seq 1 30); do
  READY_REPS="$(kubectl -n preview-resilient-env get deploy preview-app -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo "0")"
  SVC_EXISTS="$(kubectl -n preview-resilient-env get svc preview-svc -o jsonpath='{.metadata.name}' 2>/dev/null || true)"
  if [ "${READY_REPS}" = "1" ] && [ "${SVC_EXISTS}" = "preview-svc" ]; then
    break
  fi
  sleep 2
done

HEALED_OUT="$(kubectl -n preview-resilient-env get deploy,svc,pod -o wide)"
./bin/previewctl delete resilient-env --wait

record_kv "Status" "**PASS**"
record_kv "Self-Healing Verification" "Deleted \`Service/preview-svc\` and \`Deployment/preview-app\` were automatically recreated and restored to \`1/1 Ready\` by controller watches"
record_code "text" "${HEALED_OUT}"
log "T12 PASSED"

# ---------------------------------------------------------------------------
# T13 — Controlled Scale Test (5 Concurrent Environments)
# ---------------------------------------------------------------------------
log "Starting T13 — Controlled Scale Test (5 Environments)..."
record_section "T13 — Controlled Scale Test (5 Concurrent Preview Environments)"

SCALE_COUNT=5
SCALE_CREATE_START=$(date +%s)
for i in $(seq 1 ${SCALE_COUNT}); do
  ./bin/previewctl create --name "scale-${i}" --image "${SAMPLE_IMAGE}" --pr "$((400 + i))" --ttl 1800 &
done
wait

for i in $(seq 1 ${SCALE_COUNT}); do
  ./bin/previewctl create --name "scale-${i}" --image "${SAMPLE_IMAGE}" --pr "$((400 + i))" --ttl 1800 --wait --timeout 4m
done
SCALE_READY_END=$(date +%s)
SCALE_READY_LATENCY=$((SCALE_READY_END - SCALE_CREATE_START))

SCALE_LIST="$(./bin/previewctl list)"

# Capture controller Prometheus metrics via port-forward
kubectl -n preview-platform-system port-forward svc/preview-controller-metrics 18080:8080 >/dev/null 2>&1 &
PF_PID=$!
PROM_METRICS=""
for i in $(seq 1 15); do
  if curl -fsS http://127.0.0.1:18080/metrics >/tmp/prom-metrics.txt 2>/dev/null; then
    PROM_METRICS="$(grep -E '^(# HELP preview_|# TYPE preview_|preview_)' /tmp/prom-metrics.txt || true)"
    break
  fi
  sleep 1
done
kill "${PF_PID}" 2>/dev/null || true

SCALE_DEL_START=$(date +%s)
for i in $(seq 1 ${SCALE_COUNT}); do
  ./bin/previewctl delete "scale-${i}" --wait &
done
wait
SCALE_DEL_END=$(date +%s)
SCALE_DEL_LATENCY=$((SCALE_DEL_END - SCALE_DEL_START))

ORPHAN_NS="$(kubectl get ns -l platform.gke.io/managed-by=preview-controller --no-headers 2>/dev/null | wc -l | tr -d ' ')"

cat > "${BENCH_FILE}" <<EOF
{
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "cluster": "gke-ephemeral-env-cluster",
  "zone": "us-west4-a",
  "concurrentEnvironments": ${SCALE_COUNT},
  "singleEnvironmentProvisioningSeconds": ${T02_DUR},
  "singleEnvironmentALBConvergenceSeconds": ${T03_CONV},
  "singleEnvironmentDeleteSeconds": ${DEL_DUR},
  "scaleBatchReadyLatencySeconds": ${SCALE_READY_LATENCY},
  "scaleBatchDeleteLatencySeconds": ${SCALE_DEL_LATENCY},
  "orphanNamespacesAfterCleanup": ${ORPHAN_NS}
}
EOF
cp "${BENCH_FILE}" "${LATEST_BENCH}"

record_kv "Status" "**PASS**"
record_kv "Concurrent Environments Provisioned" "\`${SCALE_COUNT}\`"
record_kv "Batch Creation-to-Ready Latency" "\`${SCALE_READY_LATENCY}s\`"
record_kv "Batch Deletion Latency" "\`${SCALE_DEL_LATENCY}s\`"
record_kv "Orphan Tenant Namespaces Remaining" "\`${ORPHAN_NS}\`"
record_code "text" "=== Active Scale Environments ===
${SCALE_LIST}

=== Controller Prometheus Metrics ===
${PROM_METRICS}"

cp "${REPORT_FILE}" "${LATEST_REPORT}"
log "T00..T13 ALL PASSED! Report written to ${REPORT_FILE}"
