# Security Architecture & Limitations — GKE Ephemeral Environment Platform

## 1. Defense-in-Depth Security Controls

### 1.1 Control Plane & Cloud IAM Security
- **Restricted GKE API Server Access**:
  - The cluster's public IP endpoint (`master_authorized_networks_config`) is locked down to the verified developer workstation `/32` CIDR (`78.189.189.246/32` during live validation; `0.0.0.0/0` is explicitly prohibited via a Terraform variable validation rule in `terraform/variables.tf`).
  - External automation (GitHub Actions) connects through GKE's IAM-gated DNS endpoint (`control_plane_endpoints_config.dns_endpoint_config`), which requires valid Google Cloud IAM credentials (`container.clusters.connect`).
- **Private Worker Nodes**:
  - GKE worker nodes are provisioned with `enable_private_nodes = true` (no public IPv4 addresses on nodes). Outbound connectivity is handled via Cloud NAT and Private Google Access.
- **Least-Privilege Service Accounts**:
  - **GKE Node Pool SA** (`gke-ephemeral-nodes@enes-turan-demo.iam.gserviceaccount.com`): Granted only `roles/logging.logWriter`, `roles/monitoring.metricWriter`, `roles/monitoring.viewer`, `roles/stackdriver.resourceMetadata.writer`, and `roles/artifactregistry.reader` (never the default Compute Engine service account with Editor permissions).
  - **GitHub Actions SA** (`gke-ephemeral-gh-actions@enes-turan-demo.iam.gserviceaccount.com`): Authenticated exclusively via **Workload Identity Federation (OIDC)** (`gke-ephemeral-gh-pool` / `github-oidc`) with an `attribute_condition` restricting token exchange to `assertion.repository == 'devenes/gke-ephemeral-environment-platform'`. No long-lived JSON service-account keys exist.
- **Kubernetes RBAC (`deploy/controller/controller.yaml`)**:
  - **Controller RBAC (`preview-controller-role`)**: `preview-controller` runs under a dedicated Kubernetes `ServiceAccount` (`preview-controller` in `preview-platform-system`) bound only to the resource types it manages (`previewenvironments`, `namespaces`, `resourcequotas`, `limitranges`, `networkpolicies`, `deployments`, `services`, `httproutes`, `servicenetworkendpointgroups`, and read-only `gateways`). `secrets` and `clusterroles` are excluded from the controller's RBAC permissions.
  - **GitHub Actions Operator RBAC (`preview-github-actions-role`)**: Binds `User: gke-ephemeral-gh-actions@enes-turan-demo.iam.gserviceaccount.com` exclusively to `previewenvironments` (`get`, `list`, `watch`, `create`, `update`, `patch`, `delete`), `previewenvironments/status` (`get`), and read-only `namespaces` (`get`, `list` to verify teardown). Verified via `kubectl auth can-i`: the GitHub Actions SA cannot delete namespaces directly or access secrets/workloads.

### 1.2 Container Image Allowlist Policy
- Every `PreviewEnvironment` must specify an image originating from the project's Terraform-managed Artifact Registry repository (`us-west4-docker.pkg.dev/enes-turan-demo/gke-ephemeral-env/`).
- Mutable `:latest` tags and untagged images are rejected by `pkg/validation`; images must specify an explicit version tag (e.g., `:v0.1.0`, `:pr-1-815e888`) or an immutable `@sha256:` digest.
- Requests referencing external public registries (e.g., `docker.io`, `ghcr.io`, `quay.io`) are immediately marked `Phase=Failed` with `Condition[Validated]=False` (`Reason=DisallowedImage`) and never scheduled (verified live in Test `T07`).

### 1.3 Multi-Tenant Workload & Pod Hardening
- **Pod Security Standards (`restricted`)**:
  - Every tenant namespace (`preview-<name>`) and `preview-platform-system` is labeled with `pod-security.kubernetes.io/enforce: restricted`.
  - Every managed container runs as non-root (`UID/GID 65532`), with `readOnlyRootFilesystem: true`, `allowPrivilegeEscalation: false`, `capabilities.drop: ["ALL"]`, `seccompProfile: RuntimeDefault`, and `automountServiceAccountToken: false`.
- **No Arbitrary Host Paths or Privileged Specs**:
  - The `PreviewEnvironment` CRD exposes only high-level application parameters (`image`, `port`, `replicas`, `ttlSeconds`, bounded `resources`, and validated `APP_CUSTOM_*` environment variables). Tenants cannot specify volume mounts, `hostPath`, `hostNetwork`, `securityContext` overrides, or service account bindings.

### 1.4 Network Isolation (GKE Dataplane V2 eBPF)
- Each preview namespace receives a dedicated `NetworkPolicy` (`preview-isolation`) enforced by Cilium eBPF in GKE Dataplane V2:
  - Default-denies all ingress traffic to pods in `preview-<name>`.
  - Allows TCP traffic to the container port (`8080`) **only** from:
    1. Pods within the same namespace (`podSelector: {}`).
    2. Google Cloud Load Balancer Health Check probers (`35.191.0.0/16`, `130.211.0.0/22`).
    3. The Regional External Application Load Balancer's managed Envoy proxy subnet (`10.42.16.0/23`).
  - Direct Pod-to-Pod traffic across different preview namespaces (`10.42.128.0/17`) is dropped at the eBPF dataplane layer (verified live in Test `T06`).

---

## 2. GitHub Actions & Fork PR Security Model

- **Workflow Triggers & Untrusted Code**:
  - `pull_request_target` is **never** used to check out or execute untrusted code from fork PRs.
  - Our PR preview workflow (`.github/workflows/pr-preview.yaml`) triggers on standard `pull_request` events (`opened`, `synchronize`, `reopened`, `closed`) and explicitly checks `github.event.pull_request.head.repo.full_name == github.repository` in the `security-gate` job.
  - Fork PRs do not receive OIDC `id-token: write` tokens or GCP credentials and cannot push images to Artifact Registry or deploy to GKE.
  - Workflow files enforce least-privilege `permissions` blocks (`contents: read`, `id-token: write`, `pull-requests: write` only in the trusted deployment job).

---

## 3. Documented Security Limitations

1. **Public HTTP (Port 80) Without TLS**:
   - Because no registered custom DNS domain is assumed in the demo project, the shared GKE Gateway (`preview-shared-gateway`) serves traffic over **HTTP on port 80** using path-based routing (`http://<GATEWAY_IP>/preview/<env-name>`).
   - **Mitigation for Production**: Attach a verified Cloud DNS domain and Google-managed TLS certificate (`CertificateMap` / `ManagedCertificate`) on an HTTPS (`port: 443`) Gateway listener, and configure an HTTP-to-HTTPS redirect filter.
2. **Public Reachability of Preview URLs While Cluster Is Active**:
   - When the platform is deployed, preview endpoints on the shared Regional External Application Load Balancer are reachable on the Gateway's external IP so developers and reviewers can test them from outside the cluster without VPN/bastion setup.
   - **Mitigation for Production**: Attach a Google Cloud Armor security policy (IP allowlisting / rate limiting) or Identity-Aware Proxy (IAP) via `GCPBackendPolicy` to restrict preview access to authenticated organization members.
3. **Egress NetworkPolicy Scope**:
   - The per-namespace `NetworkPolicy` (`preview-isolation`) enforces strict **Ingress** isolation (`policyTypes: ["Ingress"]`), preventing lateral movement into any preview pod from other namespaces while allowing preview pods to make standard outbound DNS/HTTPS requests if required by the application. Environments requiring strict egress lock-down should add an `Egress` policy restricting traffic to `kube-dns` (`UDP/TCP 53`) and approved external CIDRs.
