# Live GKE Integration & End-to-End Testing Guide

> [!IMPORTANT]
> **Pre-Teardown Live GKE Verification Record**:
> All Kubernetes integration, controller lifecycle, Gateway routing, multi-tenant network isolation, resource policy, TTL expiration, failure recovery, scale, and GitHub PR lifecycle tests (`T00`–`T14`) were executed **directly against the real GKE cluster (`gke-ephemeral-env-cluster`, Kubernetes `v1.36.4-gke.1495002` in `us-west4-a`) between `2026-10-24T08:09:33Z` and `2026-10-24T09:06:52Z`, prior to the Terraform teardown executed at `2026-10-24T09:32:17Z`**.
> - After teardown, local unit and static validation checks (`T00`: `gofmt`, `go vet`, `go test -race ./...`, `terraform fmt -check`, `terraform validate`) continue to run locally without cloud infrastructure.
> - Live cluster tests (`T01`–`T14`) require an active GKE cluster provisioned via `terraform apply` (see [docs/OPERATIONS.md](OPERATIONS.md)).

---

## 1. Testing Principles

- **No local clusters**: Neither Kind nor Minikube was installed or used at any point.
- **Evidence-first verification**: Every test recorded ISO-8601 UTC timestamps, exact commands executed, expected outcomes, observed outputs, and structured JSON/Markdown artifacts under `artifacts/test-results/` and `artifacts/benchmarks/`.
- **Real external HTTP validation**: Preview endpoints were verified by sending live HTTP requests from outside the GKE cluster to the shared Regional External Application Load Balancer IP (`http://203.0.113.10/preview/<env-name>`).
- **Retained defect evidence**: Earlier test runs (`test-matrix-20261024T080933Z.md`, `test-matrix-20261024T081915Z.md`, `test-matrix-20261024T083506Z.md`) that surfaced real control-plane timing and CLI flag-ordering defects are preserved in `artifacts/test-results/` alongside the final passing run (`test-matrix-20261024T084239Z.md` / `latest-test-matrix.md`).

---

## 2. Final Pre-Teardown Test Matrix Results (`T00`–`T14`)

Full raw outputs and timestamps are preserved in [artifacts/test-results/latest-test-matrix.md](../artifacts/test-results/latest-test-matrix.md) and [artifacts/benchmarks/latest-scale-benchmark.json](../artifacts/benchmarks/latest-scale-benchmark.json).

| Test ID | Scenario | Layer Tested | Observed Pre-Teardown Result & Measurements | Status |
|---|---|---|---|---|
| **T00** | Preflight & Code Quality | Local Go / Terraform / Git | `gofmt`, `go vet`, `go test -race ./...` (5 test packages pass), `terraform fmt/validate` valid, `.gitignore` hygiene verified, zero Kind/Minikube clusters | **PASS** |
| **T01** | GKE Deployment & Gateway Readiness | GKE Control Plane & Gateway API | 2 Ready `e2-standard-2` nodes (`v1.36.4-gke.1495002`), CRD established, `preview-controller` `1/1 Running`, `gke-l7-regional-external-managed` `Accepted=True`, `preview-shared-gateway` `Programmed=True` at `203.0.113.10` | **PASS** |
| **T02** | Single Environment Provisioning (`pr-101`) | Controller Reconciliation | `PreviewEnvironment/pr-101` reached `Phase=Ready` in **`27s`** (`08:42:55Z` -> `08:43:22Z`); `Namespace/preview-pr-101`, `ResourceQuota`, `LimitRange`, `NetworkPolicy`, `Deployment`, `Service`, `HTTPRoute` created | **PASS** |
| **T03** | External HTTP Access | GKE Regional External ALB + NEG | Live `curl` to `http://203.0.113.10/preview/pr-101/`, `/healthz`, `/readyz`, `/version` returned HTTP `200` (`4s` ALB convergence after `Ready`) with `"environmentName": "pr-101"` and custom env vars | **PASS** |
| **T04** | Concurrent Multi-Environment Routing | Shared Gateway + Multi-Tenant Routing | `env-alpha` (`preview-env-alpha`) and `env-beta` (`preview-env-beta`) ran concurrently alongside `pr-101`; all three routed through **1** shared GCP forwarding rule (`gkegw1-tot4-preview-platform-sy-preview-shared-gat-k1fyo43l5rgh`, IP `203.0.113.10`) | **PASS** |
| **T05** | Resource Policies (`LimitRange` & `ResourceQuota`) | K8s API Admission vs. Controller | (1) `LimitRange` injected `requests={"cpu":"50m","memory":"64Mi"} limits={"cpu":"200m","memory":"128Mi"}`; (2) API admission rejected `limitrange-exceed-pod` (`limit 800m > max 500m`); (3) API admission rejected `quota-filler-2` (`exceeded quota: preview-quota, requested: requests.cpu=350m, used: requests.cpu=400m, limited: requests.cpu=600m`) | **PASS** |
| **T06** | eBPF Network Isolation (`Dataplane V2`) | Cilium eBPF `NetworkPolicy` | Intra-namespace probe (`preview-env-alpha` -> `10.42.129.27:8080`) succeeded (`HTTP 200`); cross-namespace Pod-to-Pod probe (`preview-env-alpha` -> `10.42.129.28:8080` in `preview-env-beta`) blocked by eBPF policy (`context deadline exceeded`), while external Gateway routing to both returned `HTTP 200` | **PASS** |
| **T07** | Invalid Request Rejection | CRD OpenAPI/CEL Admission & Controller Policy | (1) `docker.io/library/nginx:1.27` -> Controller `Phase=Failed`, `Reason=DisallowedImage` (zero namespace created); (2) `cpuLimit: 2000m` -> Controller `Phase=Failed`, `Reason=ExcessiveResources`; (3) >40 char name, `ttlSeconds: 5`, `replicas: 10` -> Rejected by `kube-apiserver` CEL/OpenAPI schema admission | **PASS** |
| **T08** | Automatic TTL Expiration & Cleanup | Controller TTL Loop & Finalizer | `ttl-test` created with `ttlSeconds: 35` (`expiresAt: 2026-10-24T08:47:45Z`); automatically expired and completed full staged teardown (`preview-ttl-test NotFound`) at `08:48:36Z` | **PASS** |
| **T09** | Manual Deletion & Finalizer Verification | Finalizer & Cascade Cleanup | `./bin/previewctl delete pr-101 --wait` executed staged teardown and confirmed `Namespace/preview-pr-101` removal in **`64s`** | **PASS** |
| **T10** | Controller Restart Recovery | Stateless Reconciliation | `kubectl rollout restart deployment/preview-controller` while `resilient-env` was active preserved `Deployment` UID (`23391ae2-8f27-4277-ba33-96614eb9f055`) and `Service` UID (`7982b050-fd42-4ca9-9504-5175fce913c3`) | **PASS** |
| **T11** | Idempotency & Boundary Conditions | Reconcile Loop Stability | 3x repeated `previewctl create` applications on `resilient-env` preserved resource UIDs and `1` pod (`0` duplicates) | **PASS** |
| **T12** | Failure Recovery & Self-Healing | Child Watch & Drift Recovery | Deleting child `Service/preview-svc` and `Deployment/preview-app` inside `preview-resilient-env` triggered immediate controller watch self-healing that recreated both and restored `1/1 Ready` in `~10s` | **PASS** |
| **T13** | Controlled Scale Benchmark (5 Concurrent Environments) | Cluster Capacity & Latency | Provisioned 5 concurrent environments (`scale-1`..`scale-5`): batch creation-to-Ready **`45s`** (histogram sum `170.71s` over 5 envs = `34.14s` avg), batch deletion **`31s`**, `0` orphan namespaces, `preview_cleanup_failures_total = 0`, `preview_reconcile_total{result="success"} = 132` | **PASS** |
| **T14** | GitHub Pull Request Lifecycle & OIDC WIF | GitHub Actions + GCP OIDC WIF | Opened real [PR #1](https://github.com/devenes/gke-ephemeral-environment-platform/pull/1) (`815e888`) -> workflow run `40080038798` (`5m35s`) authenticated via OIDC WIF, built/pushed `sample-app:pr-1-815e888`, deployed `pr-1` via GKE `--dns-endpoint`, verified `http://203.0.113.10/preview/pr-1/version` -> closed PR #1 -> workflow run `40080468687` (`3m01s`) deleted `pr-1` and confirmed `preview-pr-1` removal | **PASS** |

---

## 3. Running Tests

### 3.1 Local Unit & Static Tests (Works Post-Teardown Without Cluster)

```bash
gofmt -l .
go vet ./...
go test -v -race ./...
cd terraform && terraform fmt -check && terraform validate
```

### 3.2 Full Live GKE Integration Matrix (Requires Active GKE Cluster)

When an active GKE cluster is provisioned (see [docs/OPERATIONS.md](OPERATIONS.md)):

```bash
./scripts/run-test-matrix.sh
```

---

## 4. Defect Diagnosis & Resolution Log During Live GKE Validation

### Defect #1 — `T08` Short-TTL Premature Expiration During Regional Gateway Sync & Service Annotation Conflict

- **Initial Evidence Retained**: [artifacts/test-results/test-matrix-20261024T080933Z.md](../artifacts/test-results/test-matrix-20261024T080933Z.md) (`T00`–`T07` passed; `T08` failed waiting for `ttl-test` readiness).
- **Root Cause Diagnosis**:
  1. Immediately following `T06`, two environments (`env-alpha` and `env-beta`) were deleted. While GKE's regional Gateway controller (`networking.gke.io/gateway`) was detaching their NEGs and updating the regional URL map (~38s batch window), `T08` created `ttl-test` with `ttlSeconds: 35`. Because `expiresAt` was initially anchored strictly to `creationTimestamp + 35s`, the 35-second TTL elapsed (`delaySeconds: 0.20s`) before GKE's Gateway controller finished setting `Accepted=True` on the new `HTTPRoute`, causing the controller to tear down `ttl-test` before `previewctl --wait` observed `Phase=Ready`.
  2. Controller logs also showed a transient `409 Conflict` on `Service/preview-svc` when GKE's `neg-controller` concurrently wrote the `cloud.google.com/neg-status` annotation.
- **Fix Implemented**:
  1. Anchored active TTL (`status.expiresAt`) to `status.readyAt + spec.ttlSeconds` once the environment reaches `Ready` (with a `180s` provisioning deadline for unready environments), ensuring cloud load-balancer control-plane sync time never consumes the developer's active preview TTL.
  2. Wrapped all child resource `CreateOrUpdate` reconciliations (`Namespace`, `ResourceQuota`, `LimitRange`, `NetworkPolicy`, `Deployment`, `Service`, `HTTPRoute`) in `retry.RetryOnConflict` and preserved existing GKE controller labels/annotations (`mergeLabels`).

### Defect #2 — Staged `HTTPRoute`-Before-`Service` Teardown & GKE `ServiceNetworkEndpointGroup` Finalizer Race

- **Initial Evidence Retained**: [artifacts/test-results/test-matrix-20261024T081915Z.md](../artifacts/test-results/test-matrix-20261024T081915Z.md).
- **Root Cause Diagnosis**:
  - When `reconcileDelete` deleted `HTTPRoute/preview-route`, `Service/preview-svc`, and `Namespace/preview-pr-101` in the same millisecond, `Service/preview-svc` was removed from `etcd` immediately while GKE's Gateway controller (`sc-gateway-controller`) still had a regional `BackendService` referencing the standalone NEG (`k8s1-...-preview-svc-80-...`).
  - `gke-neg-controller` attempted to delete the GCE NEG upon `Service` deletion, received `resourceInUseByAnotherResource` from the Compute Engine API, and entered its 20-minute periodic GC backoff, leaving `networking.gke.io/neg-finalizer` on `ServiceNetworkEndpointGroup` and holding `Namespace/preview-pr-101` in `Terminating`.
- **Fix Implemented & Post-Teardown Observation**:
  - Updated `reconcileDelete` in `pkg/controller/reconciler.go` to execute a **4-stage teardown**:
    1. Delete `HTTPRoute/preview-route` and `Deployment/preview-app` first while keeping `Service/preview-svc` and `Namespace/preview-<name>` intact.
    2. Inspect `Gateway/preview-shared-gateway` annotation `networking.gke.io/backend-services` and wait until `sc-gateway-controller` detaches the tenant's backend service before deleting `Service/preview-svc` and `Namespace/preview-<name>`.
    3. Provide a self-healing fallback (`clearStuckSVCNEGFinalizers`) if a `Terminating` namespace remains blocked by an orphaned `ServiceNetworkEndpointGroup` finalizer after the Gateway has detached the backend service.
    4. Remove `preview.platform.gke.io/finalizer` once the tenant `Namespace` is confirmed `NotFound`.
  - **Known Limitation Observed During Terraform Teardown**: When `clearStuckSVCNEGFinalizers` strips `networking.gke.io/neg-finalizer` from a Kubernetes `ServiceNetworkEndpointGroup` CR before `gke-neg-controller`'s 20-minute retry loop deletes the underlying GCE `networkEndpointGroup`, the empty (`SIZE=0`) GCE `networkEndpointGroup` (`k8s1-b25dba4a-preview-*`) can remain in Compute Engine after the GKE cluster is deleted. During final infrastructure teardown (`docs/TEARDOWN.md`), 11 such empty NEGs were identified and explicitly deleted via `gcloud compute network-endpoint-groups delete` so `gke-ephemeral-env-vpc` could be cleanly destroyed.

### Defect #3 — `previewctl` Positional Argument Flag Parsing Order in `T09`

- **Initial Evidence Retained**: [artifacts/test-results/test-matrix-20261024T083506Z.md](../artifacts/test-results/test-matrix-20261024T083506Z.md) (`T00`–`T08` all passed; `T09` failed immediately on `./bin/previewctl delete pr-101 --wait`).
- **Root Cause Diagnosis**:
  - Go's standard library `flag.FlagSet.Parse` stops parsing flags at the first non-flag positional argument (`pr-101`). Consequently, in `previewctl delete pr-101 --wait`, `--wait` was treated as a positional argument (`fs.Arg(1)`) rather than setting `waitGone = true`, causing `previewctl delete` to return immediately before the staged teardown completed (which finished cleanly ~35s later at `08:41:43Z`).
- **Fix Implemented**:
  - Added `reorderPositionalArgsLast` in `cmd/previewctl/main.go` so flags (`--wait`, `--timeout`, `-o`, `-f`) are parsed regardless of whether they appear before or after the positional environment name.
