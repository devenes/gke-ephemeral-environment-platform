# Operations Runbook — GKE Ephemeral Environment Platform

## 1. Environment & Cluster Access

Configure `kubectl` to connect to the GKE cluster in `enes-turan-demo` (`us-west4-a`):

```bash
export PATH="/opt/homebrew/share/google-cloud-sdk/bin:$PATH"
gcloud container clusters get-credentials gke-ephemeral-env-cluster \
  --zone us-west4-a \
  --project enes-turan-demo
```

For IAM-gated DNS endpoint access (used by CI/CD or when connecting from dynamic IPs with appropriate IAM roles):

```bash
gcloud container clusters get-credentials gke-ephemeral-env-cluster \
  --zone us-west4-a \
  --project enes-turan-demo \
  --dns-endpoint
```

---

## 2. Building & Publishing Container Images

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

## 3. Deploying the CRD, Shared Gateway, and Controller

```bash
kubectl apply -f deploy/crd/previewenvironment-crd.yaml
kubectl apply -f deploy/controller/controller.yaml
kubectl -n preview-platform-system rollout status deployment/preview-controller --timeout=120s
kubectl -n preview-platform-system get gateway preview-shared-gateway
```

---

## 4. Using `previewctl`

Build the CLI locally:

```bash
go build -o bin/previewctl ./cmd/previewctl
```

### Create a Preview Environment

```bash
./bin/previewctl create \
  --name pr-101 \
  --image us-west4-docker.pkg.dev/enes-turan-demo/gke-ephemeral-env/sample-app:v0.1.0 \
  --pr 101 \
  --ttl 1800 \
  --env FEATURE_BANNER=enabled \
  --wait
```

### Inspect Status

```bash
./bin/previewctl get pr-101
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

## 5. Observability, Metrics & Troubleshooting

### Controller Logs

```bash
kubectl -n preview-platform-system logs deployment/preview-controller --tail=100 -f
```

### Prometheus Metrics Inspection

Port-forward or query the controller metrics endpoint on `:8080`:

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

To audit whether any tenant namespaces exist without a corresponding `PreviewEnvironment` custom resource:

```bash
kubectl get ns -l platform.gke.io/managed-by=preview-controller
kubectl get previewenvironments
```
