# Safe Teardown Procedure — GKE Ephemeral Environment Platform

> [!IMPORTANT]
> The demo environment is intentionally kept running at the conclusion of validation per project requirements. Execute this procedure **only** when you explicitly want to tear down the platform.

---

## 1. Pre-Teardown Ownership Verification

Before deleting any resources, verify that you are targeting `enes-turan-demo` and only resources created by `gke-ephemeral-environment-platform`:

```bash
gcloud config get-value project
# Expected: enes-turan-demo

gcloud container clusters list --project=enes-turan-demo --filter="name=gke-ephemeral-env-cluster"
gcloud compute networks list --project=enes-turan-demo --filter="name=gke-ephemeral-env-vpc"
gcloud artifacts repositories list --project=enes-turan-demo --location=us-west4 --filter="name~gke-ephemeral-env"
```

---

## 2. Step 1 — Delete Active Preview Environments & Shared Gateway

Delete all `PreviewEnvironment` custom resources and the shared `Gateway` first so GKE's Gateway controller cleanly releases the Regional External Application Load Balancer, forwarding rules, backend services, NEGs, and health checks before Terraform deletes the VPC:

```bash
export PATH="/opt/homebrew/share/google-cloud-sdk/bin:$PATH"
kubectl delete previewenvironments --all --wait=true --timeout=180s
kubectl delete -f deploy/controller/controller.yaml --ignore-not-found --wait=true
kubectl delete -f deploy/crd/previewenvironment-crd.yaml --ignore-not-found
```

Wait until the regional forwarding rule created by the GKE Gateway controller is removed:

```bash
gcloud compute forwarding-rules list --project=enes-turan-demo --regions=us-west4
```

---

## 3. Step 2 — Destroy Terraform-Managed Infrastructure

Once Kubernetes load-balancer resources are released, run `terraform destroy` inside `terraform/`:

```bash
cd /Users/enes/GitHub/gke-ephemeral-environment-platform/terraform
terraform plan -destroy
terraform destroy
```

Note that `google_project_service.required` sets `disable_on_destroy = false`, ensuring project-wide APIs (`compute.googleapis.com`, `container.googleapis.com`, etc.) and pre-existing service accounts are **never** disabled or deleted during teardown.

---

## 4. Step 3 — Post-Teardown Verification

Verify that all project-created resources have been removed:

```bash
gcloud container clusters list --project=enes-turan-demo
gcloud compute networks list --project=enes-turan-demo
gcloud compute addresses list --project=enes-turan-demo
gcloud compute forwarding-rules list --project=enes-turan-demo
gcloud artifacts repositories list --project=enes-turan-demo
```
