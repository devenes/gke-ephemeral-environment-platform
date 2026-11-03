# Safe Teardown Procedure & Execution Record — GKE Ephemeral Environment Platform

## 1. Executed Teardown Summary (`2026-10-24`)

Following successful completion of all live GKE integration and GitHub PR lifecycle tests (`T00`–`T14`), the demo GCP infrastructure in project `your-gcp-project-id` (`us-west4` / `us-west4-a`) was safely torn down on **`2026-10-24` between `09:16:12Z` and `09:32:17Z` (`12:16:12+03:00` – `12:32:17+03:00`)**.

- **Target GCP Project**: `your-gcp-project-id` (Project Number `123456789012`)
- **Terraform Working Directory & Workspace**: `/path/to/gke-ephemeral-environment-platform/terraform` (workspace `default`)
- **Total Terraform Resources Destroyed**: **`31` of `31`** (`0` remaining in `terraform state list`)
- **Continuing Billable Resource Exposure**: **`$0.00/hr`** (independently verified via `gcloud`)

---

## 2. Step-by-Step Teardown Execution Record

### Step 1 — Pre-Destroy Kubernetes Load Balancer & Preview Environment Release (`09:16:12Z` – `09:18:25Z`)

Before running `terraform destroy`, active Kubernetes resources (`PreviewEnvironment/pr-101`, `Gateway/preview-shared-gateway`, `Deployment/preview-controller`, and the `PreviewEnvironment` CRD) were deleted so GKE's Gateway controller (`sc-gateway-controller`) cleanly released the Regional External Application Load Balancer (`gkegw1-tot4-preview-platform-sy-preview-shared-gat-k1fyo43l5rgh`, IP `203.0.113.10`), target HTTP proxy, URL map, backend service, and health check from the `REGIONAL_MANAGED_PROXY` subnet:

```bash
export PATH="/opt/homebrew/share/google-cloud-sdk/bin:$PATH"
./bin/previewctl delete pr-101 --wait --timeout 3m
kubectl delete -f deploy/controller/controller.yaml --ignore-not-found --wait=true
kubectl delete -f deploy/crd/previewenvironment-crd.yaml --ignore-not-found
gcloud compute forwarding-rules list --project=your-gcp-project-id
# Result: Listed 0 items.
```

### Step 2 — Pre-Destroy Safety Gate & Destroy Plan Review (`09:18:31Z` – `09:18:48Z`)

In `/path/to/gke-ephemeral-environment-platform/terraform`, we verified the active GCP project (`your-gcp-project-id`), inspected `terraform state list` (`31` project-scoped resources), validated formatting and schema, and generated a reviewed destroy plan:

```bash
terraform fmt -check
terraform validate
terraform plan -destroy -out=tf-destroy.plan
terraform show tf-destroy.plan
# Result: Plan: 0 to add, 0 to change, 31 to destroy.
```

All `31` resources in `tf-destroy.plan` were confirmed to belong exclusively to `gke-ephemeral-environment-platform` (with `disable_on_destroy = false` on all 10 `google_project_service.required[*]` entries so project-wide APIs are not disabled).

### Step 3 — Terraform Destroy Execution & Orphaned GCE NEG Resolution (`09:20:30Z` – `09:32:17Z`)

1. **First Apply (`terraform apply tf-destroy.plan`, `09:20:30Z` – `09:29:48Z`)**:
   - Destroyed **20 of 31** resources, including:
     - `google_container_node_pool.primary` (`gke-ephemeral-env-pool`)
     - `google_container_cluster.primary` (`gke-ephemeral-env-cluster`, destroyed in `4m48s`)
     - `google_compute_router_nat.nat` (`gke-ephemeral-env-nat`) & `google_compute_router.router` (`gke-ephemeral-env-router`)
     - `google_compute_subnetwork.proxy_only` (`gke-ephemeral-env-proxy`) & `google_compute_subnetwork.nodes` (`gke-ephemeral-env-nodes`)
     - `google_artifact_registry_repository.repo` (`gke-ephemeral-env`)
     - `google_iam_workload_identity_pool_provider.github` (`github-oidc`) & `google_iam_workload_identity_pool.github` (`gke-ephemeral-gh-pool`)
     - `google_service_account.github_actions`, `google_service_account.gke_nodes`, `google_service_account_iam_member.github_wif_binding`, and all 8 `google_project_iam_member` bindings.
   - Stopped on `google_compute_network.vpc` (`gke-ephemeral-env-vpc`) with:
     `Error: The network resource 'projects/your-gcp-project-id/global/networks/gke-ephemeral-env-vpc' is already being used by 'projects/your-gcp-project-id/zones/us-west4-a/networkEndpointGroups/k8s1-b25dba4a-preview-scale-4-preview-svc-80-e20dd2b5'`
2. **Root Cause & Targeted Cleanup of 11 Project-Owned Empty GCE NEGs (`09:30:20Z` – `09:31:40Z`)**:
   - Inspection via `gcloud compute network-endpoint-groups list --project=your-gcp-project-id` showed 11 empty (`SIZE=0`) standalone GCE Network Endpoint Groups (`k8s1-b25dba4a-preview-*-preview-svc-80-*` in `us-west4-a`) left behind on `gke-ephemeral-env-vpc` from the earlier preview environment test runs (`env-alpha`, `env-beta`, `pr-1`, `pr-101`, `resilient-env`, `scale-1`..`scale-5`, `ttl-test`).
   - All 11 `k8s1-b25dba4a-preview-*` NEGs were deleted via `gcloud compute network-endpoint-groups delete <neg> --project=your-gcp-project-id --zone=us-west4-a --quiet`.
3. **Second Apply (`terraform plan -destroy -out=tf-destroy.plan` & `terraform apply tf-destroy.plan`, `09:31:40Z` – `09:32:17Z`)**:
   - Regenerated and reviewed `tf-destroy.plan` (`Plan: 0 to add, 0 to change, 11 to destroy` — `google_compute_network.vpc` + the 10 `google_project_service.required[*]` state entries).
   - Applied `tf-destroy.plan`: `google_compute_network.vpc` (`gke-ephemeral-env-vpc`) was destroyed in `23s` and all 10 `google_project_service.required[*]` state entries were removed (`Apply complete! Resources: 0 added, 0 changed, 11 destroyed`).
   - Removed the temporary `terraform/tf-destroy.plan` file and confirmed `terraform state list` is completely empty (`0` resources).

---

## 3. Independent Post-Teardown GCP Verification (`2026-10-24T09:33:00Z`)

Independently verified via read-only `gcloud` CLI commands against `your-gcp-project-id`:

### 3.1 Resources Destroyed / Removed

| Resource Type | Verification Command | Observed State |
|---|---|---|
| **GKE Cluster** | `gcloud container clusters list --project=your-gcp-project-id` | `Listed 0 items.` (`gke-ephemeral-env-cluster` removed) |
| **Compute VMs / Instance Groups / Disks** | `gcloud compute instances/instance-groups/disks list --project=your-gcp-project-id` | `Listed 0 items.` |
| **Project VPC & Subnets** | `gcloud compute networks list` / `subnets list --regions=us-west4` | `gke-ephemeral-env-vpc`, `gke-ephemeral-env-nodes`, and `gke-ephemeral-env-proxy` removed |
| **Cloud Router & Cloud NAT** | `gcloud compute routers list --project=your-gcp-project-id` | `Listed 0 items.` (`gke-ephemeral-env-router` & `gke-ephemeral-env-nat` removed) |
| **Forwarding Rules, External IPs, Target Proxies, URL Maps, Backend Services, Health Checks** | `gcloud compute forwarding-rules / addresses / target-http-proxies / url-maps / backend-services / health-checks list` | `Listed 0 items.` across all 6 resource types (historical Gateway IP `203.0.113.10` released) |
| **Network Endpoint Groups** | `gcloud compute network-endpoint-groups list --project=your-gcp-project-id` | `Listed 0 items.` |
| **Artifact Registry** | `gcloud artifacts repositories list --project=your-gcp-project-id --location=us-west4` | `Listed 0 items.` (`gke-ephemeral-env` removed) |
| **Project Service Accounts** | `gcloud iam service-accounts list --project=your-gcp-project-id` | `gke-ephemeral-nodes` and `gke-ephemeral-gh-actions` removed |

### 3.2 Resources That Intentionally Remain (Pre-Existing or Non-Billable GCP Metadata)

| Remaining Resource | Ownership / Origin | State & Cost Implication |
|---|---|---|
| **`default` VPC Network & 4 Default Firewall Rules** (`default-allow-icmp`, `default-allow-internal`, `default-allow-rdp`, `default-allow-ssh`) | Pre-existing GCP project default network (verified in Phase 1 preflight) | Intentionally untouched; **$0.00/hr** (non-billable) |
| **2 Pre-Existing Service Accounts** (`123456789012-compute@developer.gserviceaccount.com`, `preexisting-workload-sa@123456789012.iam.gserviceaccount.com`) | Pre-existing project service accounts (verified in Phase 1 preflight) | Intentionally untouched; **$0.00/hr** (non-billable) |
| **System Workload Identity Pool (`your-gcp-project-id.svc.id.goog`)** | Google-managed `SYSTEM_TRUST_DOMAIN` pool created automatically by GCP Container API | Managed by GCP (`ACTIVE`), cannot be user-deleted; **$0.00/hr** (non-billable) |
| **Soft-Deleted WIF Pool Tombstone (`gke-ephemeral-gh-pool`)** | Project-created Workload Identity Pool destroyed by Terraform | `state: DELETED` (`expireTime: 2026-11-23T09:20:46Z`). GCP IAM retains deleted WIF pools in a soft-deleted state for 30 days before permanent purge; **$0.00/hr** (non-billable) |
| **Enabled Google Cloud APIs** (`compute`, `container`, `artifactregistry`, `iam`, `iamcredentials`, `sts`, `logging`, `monitoring`, `networkservices`, `networksecurity`) | Enabled/verified with `disable_on_destroy = false` | Intentionally preserved per safety requirements; **$0.00/hr** (non-billable) |
