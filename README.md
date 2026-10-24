# GKE Ephemeral Environment Platform

An Internal Developer Platform (IDP) on **Google Kubernetes Engine (GKE)** that provisions isolated, TTL-governed preview environments on demand for pull requests, feature demonstrations, and debugging workflows.

---

## 1. Key Capabilities

- **Declarative Kubernetes API (`PreviewEnvironment`)**:
  A cluster-scoped CustomResourceDefinition (`platform.gke.io/v1alpha1`, shortNames `pe`, `preview`) with OpenAPI v3 + CEL admission validation and rich lifecycle status conditions (`Validated`, `NamespaceReady`, `WorkloadReady`, `RouteAccepted`, `Ready`).
- **Custom Go Controller (`preview-controller`)**:
  Built with `controller-runtime`, providing deterministic resource naming, idempotent reconciliation, automatic TTL expiration, finalizer-enforced namespace teardown, drift self-healing, and Prometheus lifecycle metrics.
- **Multi-Tenant Isolation**:
  - Dedicated tenant `Namespace` (`preview-<name>`) per environment with `pod-security.kubernetes.io/enforce: restricted`.
  - Per-namespace `ResourceQuota` (`600m` CPU req / `1000m` CPU limit / `512Mi` mem req / `1Gi` mem limit) and `LimitRange` (`50m`/`64Mi` default request, `200m`/`128Mi` default limit, `500m`/`512Mi` per-container ceiling).
  - Native eBPF `NetworkPolicy` enforced by **GKE Dataplane V2** (Cilium), blocking cross-environment Pod-to-Pod traffic while permitting intra-namespace traffic, Google Cloud health check probers (`35.191.0.0/16`, `130.211.0.0/22`), and the Regional External Application Load Balancer proxy-only subnet (`10.42.16.0/23`).
- **Cost-Efficient Shared GKE Gateway**:
  A single shared `Gateway` (`preview-shared-gateway`, class `gke-l7-regional-external-managed`) in `preview-platform-system` routes traffic to all tenant environments via per-namespace `HTTPRoute` resources with `PathPrefix` matching (`/preview/<env-name>`) and `URLRewrite` (`ReplacePrefixMatch: /`), eliminating per-environment load balancer sprawl.
- **Developer CLI (`previewctl`)**:
  Supports `create`, `get`, `list`, `delete`, and a local-only read-only HTML status `dashboard` (`127.0.0.1:9090`).
- **Keyless GitHub Actions PR Automation**:
  Federated via **GitHub OIDC and GCP Workload Identity Federation** (`gke-ephemeral-gh-pool` / `github-oidc`) and GKE's IAM-gated DNS control-plane endpoint (`--dns-endpoint`), automatically provisioning/updating preview environments when trusted PRs open/synchronize and deleting them when PRs close.

---

## 2. Repository Structure

```text
.
├── .github/workflows/
│   ├── ci.yaml                    # Go unit tests, vet, Secret check, Terraform validation
│   └── pr-preview.yaml            # PR preview lifecycle (open/sync -> deploy, close -> cleanup)
├── artifacts/
│   ├── benchmarks/                # Live GKE scale & latency benchmark JSON outputs
│   ├── preflight/                 # Read-only GCP, GitHub, and local toolchain preflight report
│   └── test-results/              # Timestamped live GKE test matrix evidence (T00–T14)
├── cmd/
│   ├── controller/                # Kubernetes controller manager entrypoint
│   ├── previewctl/                # Developer CLI (create, get, list, delete, dashboard)
│   └── sample-app/                # Sample Go HTTP preview service (/, /healthz, /readyz, /version)
├── deploy/
│   ├── controller/controller.yaml # Namespace, RBAC, Shared Gateway, Metrics Service, Deployment
│   ├── crd/                       # PreviewEnvironment CustomResourceDefinition
│   └── samples/preview-demo.yaml  # Sample PreviewEnvironment manifest
├── docs/
│   ├── ARCHITECTURE.md            # Architecture, Autopilot vs Standard, Gateway & NetworkPolicy design
│   ├── COST_AND_QUOTAS.md         # Pre-deployment cost estimate, quota headroom, optimization choices
│   ├── OPERATIONS.md              # Build, deploy, CLI usage, metrics, and troubleshooting runbook
│   ├── SECURITY.md                # RBAC, allowlists, eBPF isolation, OIDC WIF, and HTTP TLS limitation
│   ├── TEARDOWN.md                # Safe, project-scoped teardown instructions
│   └── TESTING.md                 # T00–T14 test matrix specification and execution guide
├── pkg/
│   ├── apis/platform/v1alpha1/    # PreviewEnvironment CRD Go types & scheme registration
│   ├── controller/                # Reconciler implementation & unit tests
│   ├── gateway/                   # Gateway API HTTPRoute builder & status inspector
│   ├── metrics/                   # Prometheus metrics definitions
│   └── validation/                # Image allowlist, resource bounds, TTL, and env validator
├── scripts/
│   └── run-test-matrix.sh         # Reproducible live GKE integration & E2E test runner
├── terraform/                     # Terraform configuration for VPC, subnets, NAT, AR, WIF, and GKE
├── Dockerfile.controller          # Multi-stage distroless non-root image for preview-controller
└── Dockerfile.sample-app          # Multi-stage distroless non-root image for sample-app
```

---

## 3. Quick Start (`previewctl`)

```bash
# Build previewctl
go build -o bin/previewctl ./cmd/previewctl

# Request a preview environment and wait for Ready + external URL
./bin/previewctl create \
  --name demo-pr-42 \
  --image us-west4-docker.pkg.dev/enes-turan-demo/gke-ephemeral-env/sample-app:v0.1.0 \
  --pr 42 \
  --ttl 1800 \
  --env FEATURE_FLAG=true \
  --wait

# Inspect environment details and conditions
./bin/previewctl get demo-pr-42

# List all preview environments
./bin/previewctl list

# Delete environment and wait for namespace cleanup
./bin/previewctl delete demo-pr-42 --wait
```

---

## 4. Documentation Index

- [Architecture & Design Decisions](docs/ARCHITECTURE.md)
- [Operations Runbook](docs/OPERATIONS.md)
- [Testing & Verification Guide (`T00`–`T14`)](docs/TESTING.md)
- [Security Controls & Limitations](docs/SECURITY.md)
- [Cost & Quotas Analysis](docs/COST_AND_QUOTAS.md)
- [Safe Teardown Procedure](docs/TEARDOWN.md)
- [Phase 1 Preflight Report](artifacts/preflight/preflight-2026-10-24.md)
- [Latest Live GKE Test Matrix Results](artifacts/test-results/latest-test-matrix.md)
- [Latest Scale Benchmark Results](artifacts/benchmarks/latest-scale-benchmark.json)
