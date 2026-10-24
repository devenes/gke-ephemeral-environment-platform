# Live GKE Integration & End-to-End Testing Guide

## 1. Testing Principles

All Kubernetes integration, controller lifecycle, Gateway routing, multi-tenant network isolation, resource policy, TTL expiration, failure recovery, scale, and GitHub PR lifecycle tests (`T00`–`T14`) are executed **directly against the real GKE cluster (`gke-ephemeral-env-cluster` in `us-west4-a`)**.

- **No local clusters**: Neither Kind nor Minikube is installed or used.
- **Evidence-first verification**: Every test records ISO-8601 UTC timestamps, exact commands executed, expected outcomes, observed outputs, and structured JSON/Markdown artifacts under `artifacts/test-results/` and `artifacts/benchmarks/`.
- **Real external HTTP validation**: Preview endpoints are verified by sending live HTTP requests from outside the GKE cluster to the shared Regional External Application Load Balancer IP (`http://<GATEWAY_IP>/preview/<env-name>`).

---

## 2. Test Matrix (`T00`–`T14`)

| Test ID | Scenario | Layer Tested | Key Verification Criteria |
|---|---|---|---|
| **T00** | Preflight & Code Quality | Local Go / Terraform / Git | `gofmt`, `go vet`, `go test -race ./...`, `terraform fmt/validate`, `.gitignore` hygiene, zero Kind/Minikube clusters |
| **T01** | GKE Deployment & Gateway Readiness | GKE Control Plane & Gateway API | Nodes `Ready`, CRD established, `preview-controller` pod `1/1 Ready`, `gke-l7-regional-external-managed` `Accepted=True`, `preview-shared-gateway` assigned external IP |
| **T02** | Single Environment Provisioning | Controller Reconciliation | `PreviewEnvironment` `pr-101` reaches `Phase=Ready`, `Namespace`, `ResourceQuota`, `LimitRange`, `NetworkPolicy`, `Deployment`, `Service`, `HTTPRoute` created |
| **T03** | External HTTP Access | GKE Regional External ALB + NEG | Live `curl` to `http://<GATEWAY_IP>/preview/pr-101/`, `/healthz`, `/readyz`, `/version` returns HTTP `200` and matches `environmentName: "pr-101"` |
| **T04** | Concurrent Multi-Environment Routing | Shared Gateway + Multi-Tenant Routing | `env-alpha` and `env-beta` run concurrently in `preview-env-alpha` and `preview-env-beta`; both route through the **single** shared Gateway IP to their distinct pods |
| **T05** | Resource Policies (`LimitRange` & `ResourceQuota`) | K8s API Admission vs. Controller | (1) Default `LimitRange` injects `50m`/`64Mi` requests and `200m`/`128Mi` limits into unannotated pod; (2) API admission rejects pod exceeding `500m` container max; (3) API admission rejects pod exceeding `1000m`/`1Gi` namespace `ResourceQuota` |
| **T06** | eBPF Network Isolation (`Dataplane V2`) | Cilium eBPF `NetworkPolicy` | Direct Pod-to-Pod HTTP request from `preview-env-alpha` to `preview-env-beta` Pod IP times out / is dropped by eBPF policy, while external Gateway traffic to both environments succeeds |
| **T07** | Invalid Request Rejection | CRD OpenAPI/CEL Admission & Controller Policy | (1) Disallowed image registry (`docker.io/library/nginx:1.27`) -> Controller `Phase=Failed`, `Reason=DisallowedImage`; (2) `:latest` tag -> `Phase=Failed`; (3) Excessive CPU/memory (`2000m`/`4Gi`) -> `Phase=Failed`, `Reason=ExcessiveResources`; (4) Invalid name (>40 chars), invalid TTL (`5s`), invalid replicas (`10`), privileged port (`80`) -> Rejected by `kube-apiserver` OpenAPI/CEL admission |
| **T08** | Automatic TTL Expiration & Cleanup | Controller TTL Loop & Finalizer | Environment created with `ttlSeconds: 35`; after expiration, controller automatically deletes `Namespace`, `HTTPRoute`, `Deployment`, `Service`, and `PreviewEnvironment`; measures cleanup delay |
| **T09** | Manual Deletion & Finalizer Verification | Finalizer & Cascade Cleanup | `previewctl delete` removes `PreviewEnvironment`; finalizer blocks until `Namespace` is confirmed `NotFound`; zero orphan resources remain |
| **T10** | Controller Restart Recovery | Stateless Reconciliation | `kubectl rollout restart deployment/preview-controller` while environment is active; new controller pod reconciles existing state without duplicate resources or downtime |
| **T11** | Idempotency & Boundary Conditions | Reconcile Loop Stability | Repeated `kubectl apply` and label annotations do not bump child `generation` or create duplicates; rapid create/delete boundary handled cleanly |
| **T12** | Failure Recovery & Self-Healing | Child Watch & Drift Recovery | Manually deleting child `Service` and `Deployment` inside an active preview namespace triggers immediate controller watch reconciliation (< 2s) that restores the deleted resources and returns the environment to `Ready` |
| **T13** | Controlled Scale Benchmark (5 Concurrent Environments) | Cluster Capacity & Latency | Provisions 5 concurrent environments (`scale-1`..`scale-5`), measures creation-to-Ready latency, external HTTP reachability, deletion latency, controller error count (`0`), and Prometheus metrics |
| **T14** | GitHub Pull Request Lifecycle & OIDC WIF | GitHub Actions + GCP OIDC WIF | Opens real GitHub PR -> triggers `PR Preview Environment Lifecycle` workflow via OIDC WIF -> provisions `pr-<N>` on GKE -> verifies external URL -> closes PR -> workflow deletes `pr-<N>` and cleans up namespace |

---

## 3. Running the Automated Test Suite

Execute the test runner against the active GKE cluster:

```bash
./scripts/run-test-matrix.sh
```

---

## 4. Defect Diagnosis & Resolution Log During Live GKE Validation

### Defect #1 — `T08` Short-TTL Premature Expiration During Regional Gateway Sync & Service Annotation Conflict

- **Initial Evidence Retained**: `artifacts/test-results/test-matrix-20261024T080933Z.md` (`T00`–`T07` passed; `T08` failed waiting for `ttl-test` readiness).
- **Root Cause Diagnosis**:
  1. Immediately following `T06`, two environments (`env-alpha` and `env-beta`) were deleted. While GKE's regional Gateway controller (`networking.gke.io/gateway`) was detaching their NEGs and updating the regional URL map (~38s batch window), `T08` created `ttl-test` with `ttlSeconds: 35`. Because `expiresAt` was initially anchored strictly to `creationTimestamp + 35s`, the 35-second TTL elapsed (`delaySeconds: 0.20s`) before GKE's Gateway controller finished setting `Accepted=True` on the new `HTTPRoute`, causing the controller to tear down `ttl-test` before `previewctl --wait` observed `Phase=Ready`.
  2. Controller logs also showed a transient `409 Conflict` on `Service/preview-svc` when GKE's `neg-controller` concurrently wrote the `cloud.google.com/neg-status` annotation.
- **Fix Implemented**:
  1. Anchored active TTL (`status.expiresAt`) to `status.readyAt + spec.ttlSeconds` once the environment reaches `Ready` (with a `180s` provisioning deadline for unready environments), ensuring cloud load-balancer control-plane sync time never consumes the developer's active preview TTL.
  2. Wrapped all child resource `CreateOrUpdate` reconciliations (`Namespace`, `ResourceQuota`, `LimitRange`, `NetworkPolicy`, `Deployment`, `Service`, `HTTPRoute`) in `retry.RetryOnConflict` and preserved existing GKE controller labels/annotations (`mergeLabels`).

### Defect #2 — Staged `HTTPRoute`-Before-`Service` Teardown to Prevent GKE `ServiceNetworkEndpointGroup` Finalizer Race

- **Initial Evidence Retained**: `artifacts/test-results/test-matrix-20261024T081915Z.md`.
- **Root Cause Diagnosis**:
  - When `reconcileDelete` deleted `HTTPRoute/preview-route`, `Service/preview-svc`, and `Namespace/preview-pr-101` in the same millisecond, `Service/preview-svc` was removed from `etcd` immediately while GKE's Gateway controller (`sc-gateway-controller`) still had a regional `BackendService` referencing the standalone NEG (`k8s1-...-preview-svc-80-...`).
  - `gke-neg-controller` attempted to delete the GCE NEG upon `Service` deletion, received `resourceInUseByAnotherResource` from the Compute Engine API, and entered its 20-minute periodic GC backoff, leaving `networking.gke.io/neg-finalizer` on `ServiceNetworkEndpointGroup` and holding `Namespace/preview-pr-101` in `Terminating`.
- **Fix Implemented**:
  - Updated `reconcileDelete` in `pkg/controller/reconciler.go` to execute a **4-stage teardown**:
    1. Delete `HTTPRoute/preview-route` and `Deployment/preview-app` first while keeping `Service/preview-svc` and `Namespace/preview-<name>` intact.
    2. Inspect `Gateway/preview-shared-gateway` annotation `networking.gke.io/backend-services` and wait until `sc-gateway-controller` detaches the tenant's backend service before deleting `Service/preview-svc` and `Namespace/preview-<name>`.
    3. Provide a self-healing fallback (`clearStuckSVCNEGFinalizers`) if a `Terminating` namespace ever remains blocked by an orphaned `ServiceNetworkEndpointGroup` finalizer after the Gateway has detached the backend service.
    4. Remove `preview.platform.gke.io/finalizer` once the tenant `Namespace` is confirmed `NotFound`.

### Defect #3 — `previewctl` Positional Argument Flag Parsing Order in `T09`

- **Initial Evidence Retained**: `artifacts/test-results/test-matrix-20261024T083506Z.md` (`T00`–`T08` all passed; `T09` failed immediately on `./bin/previewctl delete pr-101 --wait`).
- **Root Cause Diagnosis**:
  - Go's standard library `flag.FlagSet.Parse` stops parsing flags at the first non-flag positional argument (`pr-101`). Consequently, in `previewctl delete pr-101 --wait`, `--wait` was treated as a positional argument (`fs.Arg(1)`) rather than setting `waitGone = true`, causing `previewctl delete` to return immediately before the staged teardown completed (which finished cleanly ~35s later at `08:41:43Z`).
- **Fix Implemented**:
  - Added `reorderPositionalArgsLast` in `cmd/previewctl/main.go` so flags (`--wait`, `--timeout`, `-o`) are parsed regardless of whether they appear before or after the positional environment name.



