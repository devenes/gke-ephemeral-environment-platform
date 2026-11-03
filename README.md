# GKE Ephemeral Environment Platform

[![CI](https://github.com/devenes/gke-ephemeral-environment-platform/actions/workflows/ci.yaml/badge.svg)](https://github.com/devenes/gke-ephemeral-environment-platform/actions/workflows/ci.yaml)
![Go 1.27](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)
![GKE Standard + Dataplane V2](https://img.shields.io/badge/GKE-Standard%20%2B%20Dataplane%20V2-4285F4?logo=googlekubernetesengine&logoColor=white)
![Gateway API](https://img.shields.io/badge/Kubernetes-Gateway%20API-326CE5?logo=kubernetes&logoColor=white)
![controller-runtime](https://img.shields.io/badge/Operator-controller--runtime-326CE5?logo=kubernetes&logoColor=white)
![Terraform](https://img.shields.io/badge/Terraform-%3E%3D1.5-7B42BC?logo=terraform&logoColor=white)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

An Internal Developer Platform (IDP) on **Google Kubernetes Engine (GKE)** that provisions isolated, TTL-governed preview environments on demand for pull requests, feature demonstrations, and debugging workflows.

---

## 1. Key Capabilities

- **Declarative Kubernetes API (`PreviewEnvironment`)**:
  A cluster-scoped CustomResourceDefinition (`platform.gke.io/v1alpha1`, shortNames `pe`, `preview`) with OpenAPI v3 + CEL admission validation and rich lifecycle status conditions (`Validated`, `NamespaceReady`, `WorkloadReady`, `RouteAccepted`, `Ready`).
- **Custom Go Controller (`preview-controller`)**:
  Built with `controller-runtime`, providing deterministic resource naming, conflict-resilient idempotent reconciliation, `readyAt`-anchored TTL expiration, 4-stage finalizer-enforced Gateway/NEG/Namespace teardown, drift self-healing, and Prometheus lifecycle metrics.
- **Multi-Tenant Isolation**:
  - Dedicated tenant `Namespace` (`preview-<name>`) per environment with `pod-security.kubernetes.io/enforce: restricted`.
  - Per-namespace `ResourceQuota` (`600m` CPU req / `1000m` CPU limit / `512Mi` mem req / `1Gi` mem limit) and `LimitRange` (`50m`/`64Mi` default request, `200m`/`128Mi` default limit, `500m`/`512Mi` per-container ceiling).
  - Native eBPF `NetworkPolicy` enforced by **GKE Dataplane V2** (Cilium), blocking cross-environment Pod-to-Pod traffic while permitting intra-namespace traffic, Google Cloud health check probers (`35.191.0.0/16`, `130.211.0.0/22`), and the Regional External Application Load Balancer proxy-only subnet (`10.42.16.0/23`).
- **Cost-Efficient Shared GKE Gateway**:
  A single shared `Gateway` (`preview-shared-gateway`, class `gke-l7-regional-external-managed`) in `preview-platform-system` routes traffic to all tenant environments via per-namespace `HTTPRoute` resources with `PathPrefix` matching (`/preview/<env-name>`) and `URLRewrite` (`ReplacePrefixMatch: /`), eliminating per-environment load balancer sprawl.
- **Developer CLI (`previewctl`)**:
  Supports `create` / `apply` (via flags or `-f <manifest.yaml>`), `get` / `status`, `list` / `ls`, `delete` / `rm`, and a local-only read-only HTML status `dashboard` (`127.0.0.1:9090`).
- **Keyless GitHub Actions PR Automation**:
  Federated via **GitHub OIDC and GCP Workload Identity Federation** (`gke-ephemeral-gh-pool` / `github-oidc`) and GKE's IAM-gated DNS control-plane endpoint (`--dns-endpoint`), automatically provisioning/updating preview environments when trusted PRs open/synchronize and deleting them when PRs close.

---

## 2. Verified Test Coverage (`T00`–`T14`)

All 15 verification suites (`T00`–`T14`) were executed directly against a live GKE cluster (`gke-ephemeral-env-cluster` in `us-west4-a`):

| Suite | Scope | Result |
|---|---|---|
| **Local Unit & Static Checks (`T00`)** | `gofmt`, `go vet`, `go test -race ./...` (`cmd/previewctl`, `cmd/sample-app`, `pkg/controller`, `pkg/gateway`, `pkg/validation`), `terraform fmt -check`, `terraform validate` | **PASS** (runs locally without cluster) |
| **GKE & Gateway Lifecycle (`T01`–`T04`)** | Cluster readiness, CRD/Controller deployment, single & multi-environment routing (`pr-101`, `env-alpha`, `env-beta`) over 1 shared Gateway IP (`<GATEWAY_IP>`) | **PASS** (`27s` provisioning, `4s` ALB convergence) |
| **Multi-Tenant Isolation (`T05`–`T07`)** | `LimitRange` default injection & max ceiling, `ResourceQuota` admission rejection, Dataplane V2 Cilium eBPF `NetworkPolicy` cross-namespace drop, invalid registry/resource/schema rejection | **PASS** |
| **TTL, Deletion, Resilience & Scale (`T08`–`T13`)** | Automatic TTL expiration (`35s`), manual staged deletion (`64s`), controller restart UID preservation, idempotency, child deletion self-healing (`~10s`), 5-environment concurrent scale benchmark (`45s` ready, `31s` batch delete, `0` orphans) | **PASS** |
| **GitHub Actions PR Lifecycle (`T14`)** | Real GitHub PR #1 opened -> OIDC WIF auth -> image build/push -> GKE DNS endpoint deploy -> live HTTP verification -> PR close automatic cleanup | **PASS** |

See [docs/TESTING.md](docs/TESTING.md) and [artifacts/test-results/latest-test-matrix.md](artifacts/test-results/latest-test-matrix.md) for the complete verification evidence and defect resolution log.

---

## 3. `previewctl` CLI Usage

Build `previewctl` locally (works offline without an active cluster):

```bash
go build -o bin/previewctl ./cmd/previewctl
./bin/previewctl --help
```

When connected to a GKE cluster where the platform is deployed (see [docs/OPERATIONS.md](docs/OPERATIONS.md) to provision infrastructure):

```bash
# Request a preview environment via flags and wait for Ready + external URL
./bin/previewctl create \
  --name demo-pr-42 \
  --image us-west4-docker.pkg.dev/your-gcp-project-id/gke-ephemeral-env/sample-app:v0.1.0 \
  --pr 42 \
  --ttl 1800 \
  --env FEATURE_FLAG=true \
  --wait

# Or apply a declarative YAML manifest
./bin/previewctl apply -f deploy/samples/preview-demo.yaml --wait

# Inspect environment details, URL, remaining TTL, and conditions
./bin/previewctl get demo-pr-42

# List all preview environments in tabular or JSON format
./bin/previewctl list
./bin/previewctl list -o json

# Start a local-only (127.0.0.1:9090) read-only HTML dashboard
./bin/previewctl dashboard --addr 127.0.0.1:9090

# Delete environment and wait for staged Gateway/NEG/Namespace cleanup
./bin/previewctl delete demo-pr-42 --wait
```

---

## 4. Documentation Index

- [Architecture & Design Decisions](docs/ARCHITECTURE.md)
- [Operations Runbook (Local Checks vs. Full Cloud Recreate)](docs/OPERATIONS.md)
- [Testing & Verification Guide (`T00`–`T14`)](docs/TESTING.md)
- [Security Controls & Limitations](docs/SECURITY.md)
- [Cost & Quotas Analysis (Pre-Teardown & Post-Teardown)](docs/COST_AND_QUOTAS.md)
- [Executed Teardown Procedure & Verification](docs/TEARDOWN.md)
- [Phase 1 Preflight Report](artifacts/preflight/preflight-2026-10-24.md)
- [Historical Live GKE Test Matrix Results](artifacts/test-results/latest-test-matrix.md)
- [Historical Scale Benchmark Results](artifacts/benchmarks/latest-scale-benchmark.json)
