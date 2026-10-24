# Operations Runbook — GKE Ephemeral Environment Platform

> [!NOTE]
> **Current Infrastructure State**: The demo GCP infrastructure in `enes-turan-demo` (`us-west4` / `us-west4-a`) was torn down via Terraform on `2026-10-24T09:32:17Z` after live GKE validation (`T00`–`T14`) completed.
> - **Section 1** documents **offline local verification and build commands** that run immediately without any cloud resources.
> - **Sections 2–6** document the **verified cloud provisioning, deployment, CLI operation, and observability procedures** that require re-applying the Terraform configuration first.

---

## 1. Local Offline Validation & Build (No Cloud Infrastructure Required)

You can run Go formatting checks, static analysis (`go vet`), unit tests with the race detector (`controller-runtime` fake client), binary compilation, and Terraform static validation locally without an active GKE cluster:

```bash
# Format check & static analysis
gofmt -l .
go vet ./...

# Unit tests across controller, CLI, sample-app, gateway, and validation packages
go test -v -race ./...

# Compile CLI, controller, and sample-app binaries
mkdir -p bin
go build -o bin/previewctl ./cmd/previewctl
go build -o bin/controller ./cmd/controller
go build -o bin/sample-app ./cmd/sample-app

# Validate Terraform configuration
cd terraform
terraform fmt -check
terraform init -backend=false
terraform validate
```

---

## 2. Re-Provisioning Cloud Infrastructure via Terraform (Requires GCP Access)

To recreate the GKE cluster, dedicated VPC (`gke-ephemeral-env-vpc`), proxy-only subnet, Cloud Router/NAT, Artifact Registry (`gke-ephemeral-env`), least-privilege Service Accounts, and GitHub OIDC Workload Identity Pool in `enes-turan-demo`:

```bash
cd terraform
cp terraform.tfvars.example terraform.tfvars
# Edit terraform.tfvars to set master_authorized_cidr to your current <PUBLIC_IPV4>/32:
# master_authorized_cidr = "<YOUR_PUBLIC_IPV4>/32"

terraform init
terraform plan -out=tf.plan
terraform apply tf.plan
```

Configure `kubectl` to connect to the newly created GKE cluster in `enes-turan-demo` (`us-west4-a`):

```bash
export PATH="/opt/homebrew/share/google-cloud-sdk/bin:$PATH"
gcloud container clusters get-credentials gke-ephemeral-env-cluster \
  --zone us-west4-a \
  --project enes-turan-demo
```

For IAM-gated DNS endpoint access (used by GitHub Actions CI/CD or when connecting from dynamic IPs with appropriate IAM roles):

```bash
gcloud container clusters get-credentials gke-ephemeral-env-cluster \
  --zone us-west4-a \
  --project enes-turan-demo \
  --dns-endpoint
```

---

## 3. Building & Publishing Container Images (Requires Artifact Registry)

Authenticate Docker to Artifact Registry in `us-west4`:

```bash
gcloud auth configure-docker us-west4-docker.pkg.dev --quiet
```

Build and push `linux/amd64` images for the sample application and controller:

```bash
export REGISTRY="us-west4-docker.pkg.dev/enes-turan-demo/gke-ephemeral-env"
export TAG="v0.1.0"

# Sample HTTP Application
docker buildx build --platform linux/amd64 \
  -f Dockerfile.sample-app \
  --build-arg VERSION="${TAG}" \
  --build-arg GIT_COMMIT="$(git rev-parse --short HEAD)" \
  --build-arg BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -t "${REGISTRY}/sample-app:${TAG}" \
  --push .

# Preview Environment Controller
docker buildx build --platform linux/amd64 \
  -f Dockerfile.controller \
  -t "${REGISTRY}/preview-controller:${TAG}" \
  --push .
```

---

## 4. Deploying the CRD, Shared Gateway, and Controller (Requires Active GKE Cluster)

```bash
kubectl apply -f deploy/crd/previewenvironment-crd.yaml
kubectl apply -f deploy/controller/controller.yaml
kubectl -n preview-platform-system rollout status deployment/preview-controller --timeout=120s
kubectl -n preview-platform-system get gateway preview-shared-gateway
```

Wait until `preview-shared-gateway` reports `PROGRAMMED = True` and an external `ADDRESS`.

---

## 5. Operating Preview Environments with `previewctl` (Requires Active GKE Cluster)

### Create a Preview Environment (Flags or Manifest)

```bash
# Via CLI flags
./bin/previewctl create \
  --name pr-101 \
  --image us-west4-docker.pkg.dev/enes-turan-demo/gke-ephemeral-env/sample-app:v0.1.0 \
  --pr 101 \
  --ttl 1800 \
  --env FEATURE_BANNER=enabled \
  --wait

# Or via declarative YAML manifest
./bin/previewctl apply -f deploy/samples/preview-demo.yaml --wait
```

### Inspect Status

```bash
./bin/previewctl get pr-101
./bin/previewctl get pr-101 -o json
```

### List All Environments

```bash
./bin/previewctl list
```

### Delete an Environment

```bash
./bin/previewctl delete pr-101 --wait
```

---

## 6. Observability, Metrics & Troubleshooting (Requires Active GKE Cluster)

### Controller Logs

```bash
kubectl -n preview-platform-system logs deployment/preview-controller --tail=100 -f
```

### Prometheus Metrics Inspection

Port-forward and query the controller metrics endpoint on `:8080`:

```bash
kubectl -n preview-platform-system port-forward svc/preview-controller-metrics 18080:8080 &
curl -s http://127.0.0.1:18080/metrics | grep -E "^preview_"
```

Key exported metrics:
- `preview_reconcile_total{result="success|error|requeue"}`
- `preview_environments_by_phase{phase="Pending|Provisioning|Ready|Failed|Deleting|Expired"}`
- `preview_provisioning_duration_seconds_bucket` / `_sum` / `_count`
- `preview_ttl_cleanup_delay_seconds_bucket` / `_sum` / `_count`
- `preview_cleanup_failures_total`

### Orphan Resource Detection

To audit whether any tenant namespaces or GCE Network Endpoint Groups exist without a corresponding `PreviewEnvironment` custom resource:

```bash
kubectl get ns -l platform.gke.io/managed-by=preview-controller
kubectl get previewenvironments
gcloud compute network-endpoint-groups list --project=enes-turan-demo --zones=us-west4-a
```
