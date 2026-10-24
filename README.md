# GKE Ephemeral Environment Platform

An Internal Developer Platform (IDP) on **Google Kubernetes Engine (GKE)** that provisions isolated, TTL-governed preview environments on demand for pull requests, feature demonstrations, and debugging workflows.

> [!NOTE]
> **Cloud Infrastructure Status (Torn Down After Live Validation)**:
> This platform was provisioned via Terraform in GCP project `enes-turan-demo` (`us-west4` / `us-west4-a`), verified end-to-end across the full `T00`–`T14` live GKE test matrix (including GitHub Actions OIDC Workload Identity Federation PR lifecycle), and subsequently **torn down via Terraform on `2026-10-24T09:32:17Z`** with independent GCP verification confirming zero remaining active cluster, load balancer, VPC, or Artifact Registry resources. The historical Gateway endpoint (`http://35.219.134.88/preview/<name>`) recorded in the test evidence is no longer active. To recreate the environment, follow [docs/OPERATIONS.md](docs/OPERATIONS.md).

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

## 2. Repository Structure

```text
.
├── .github/workflows/
│   ├── ci.yaml                    # Go unit tests, vet, secret check, Terraform validation
│   └── pr-preview.yaml            # PR preview lifecycle (open/sync -> deploy, close -> cleanup)
├── artifacts/
│   ├── benchmarks/                # Historical live GKE scale & latency benchmark JSON outputs
│   ├── preflight/                 # Read-only GCP, GitHub, and local toolchain preflight report
│   └── test-results/              # Timestamped live GKE test matrix evidence (T00–T14 + defect runs)
├── cmd/
│   ├── controller/                # Kubernetes controller manager entrypoint
│   ├── previewctl/                # Developer CLI (create/apply, get, list, delete, dashboard)
│   └── sample-app/                # Sample Go HTTP preview service (/, /healthz, /readyz, /version)
├── deploy/
│   ├── controller/controller.yaml # Namespace, RBAC, Shared Gateway, Metrics Service, Deployment
│   ├── crd/                       # PreviewEnvironment CustomResourceDefinition
│   └── samples/preview-demo.yaml  # Sample PreviewEnvironment manifest
├── docs/
│   ├── ARCHITECTURE.md            # Architecture, Autopilot vs Standard, Gateway & NetworkPolicy design
│   ├── COST_AND_QUOTAS.md         # Pre-deployment cost estimate, quota headroom, post-teardown status
│   ├── OPERATIONS.md              # Build, deploy, CLI usage, metrics, and troubleshooting runbook
│   ├── SECURITY.md                # RBAC, allowlists, eBPF isolation, OIDC WIF, and HTTP TLS limitation
│   ├── TEARDOWN.md                # Executed Terraform teardown procedure & verification record
│   └── TESTING.md                 # Pre-teardown T00–T14 test matrix results & defect resolution log
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

## 3. Verified Test Coverage (`T00`–`T14`)

Before the demo infrastructure was torn down, all 15 verification suites (`T00`–`T14`) were executed directly against the real GKE cluster (`gke-ephemeral-env-cluster` in `us-west4-a`):

| Suite | Scope | Pre-Teardown Result |
|---|---|---|
| **Local Unit & Static Checks (`T00`)** | `gofmt`, `go vet`, `go test -race ./...` (`cmd/previewctl`, `cmd/sample-app`, `pkg/controller`, `pkg/gateway`, `pkg/validation`), `terraform fmt -check`, `terraform validate` | **PASS** (runs locally without cluster) |
| **GKE & Gateway Lifecycle (`T01`–`T04`)** | Cluster readiness, CRD/Controller deployment, single & multi-environment routing (`pr-101`, `env-alpha`, `env-beta`) over 1 shared Gateway IP | **PASS** (`27s` provisioning, `4s` ALB convergence) |
| **Multi-Tenant Isolation (`T05`–`T07`)** | `LimitRange` default injection & max ceiling, `ResourceQuota` admission rejection, Dataplane V2 Cilium eBPF `NetworkPolicy` cross-namespace drop, invalid registry/resource/schema rejection | **PASS** |
| **TTL, Deletion, Resilience & Scale (`T08`–`T13`)** | Automatic TTL expiration (`35s`), manual staged deletion (`64s`), controller restart UID preservation, idempotency, child deletion self-healing (`~10s`), 5-environment concurrent scale benchmark (`45s` ready, `31s` batch delete, `0` orphans) | **PASS** |
| **GitHub Actions PR Lifecycle (`T14`)** | Real GitHub PR #1 opened -> OIDC WIF auth -> image build/push -> GKE DNS endpoint deploy -> live HTTP verification -> PR close automatic cleanup | **PASS** |

See [docs/TESTING.md](docs/TESTING.md) and [artifacts/test-results/latest-test-matrix.md](artifacts/test-results/latest-test-matrix.md) for the complete pre-teardown evidence and defect resolution log.

---

## 4. `previewctl` CLI Usage

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
  --image us-west4-docker.pkg.dev/enes-turan-demo/gke-ephemeral-env/sample-app:v0.1.0 \
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

## 5. Documentation Index

- [Architecture & Design Decisions](docs/ARCHITECTURE.md)
- [Operations Runbook (Local Checks vs. Full Cloud Recreate)](docs/OPERATIONS.md)
- [Testing & Verification Guide (`T00`–`T14`)](docs/TESTING.md)
- [Security Controls & Limitations](docs/SECURITY.md)
- [Cost & Quotas Analysis (Pre-Teardown & Post-Teardown)](docs/COST_AND_QUOTAS.md)
- [Executed Teardown Procedure & Verification](docs/TEARDOWN.md)
- [Phase 1 Preflight Report](artifacts/preflight/preflight-2026-10-24.md)
- [Historical Live GKE Test Matrix Results](artifacts/test-results/latest-test-matrix.md)
- [Historical Scale Benchmark Results](artifacts/benchmarks/latest-scale-benchmark.json)
