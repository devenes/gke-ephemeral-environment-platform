# Cost & Quotas Analysis — GKE Ephemeral Environment Platform

## 1. Pre-Provisioning Cost Estimate & Historical Runtime Cost (`us-west4` / `us-west4-a`)

Before applying the Terraform plan, we estimated the hourly and daily run-rate of every cloud resource provisioned by this platform in `your-gcp-project-id` (`us-west4`):

| Resource | Configuration | Pricing Basis (`us-west4`) | Estimated Hourly Cost | Estimated Daily Cost |
|---|---|---|---|---|
| **GKE Zonal Control Plane** | 1 zonal cluster (`gke-ephemeral-env-cluster` in `us-west4-a`) | Covered by GKE Free Tier ($74.40/mo credit for 1 zonal/Autopilot cluster per billing account; `$0.10/hr` otherwise) | **$0.00/hr** (Free Tier) | **$0.00/day** |
| **GKE Worker Nodes** | 2 × `e2-standard-2` (2 vCPU, 8 GB RAM each = 4 vCPU, 16 GB RAM total) | ~$0.06701/hr per `e2-standard-2` VM in `us-west4` | **~$0.134/hr** | **~$3.22/day** |
| **Worker Boot Disks** | 2 × 50 GB `pd-balanced` (100 GB total) | $0.10 / GB-month (~$10.00/mo total) | **~$0.014/hr** | **~$0.33/day** |
| **Shared GKE Gateway (Regional External ALB)** | 1 × `gke-l7-regional-external-managed` forwarding rule | ~$0.025/hr for 1st forwarding rule + minimal data processing | **~$0.025/hr** | **~$0.60/day** |
| **Cloud NAT** | 1 × Regional Cloud NAT gateway (`gke-ephemeral-env-nat`) | ~$0.044/hr base gateway + 1 NAT IP | **~$0.049/hr** | **~$1.18/day** |
| **Artifact Registry** | `gke-ephemeral-env` Docker repository (`42.015 MB` peak storage) | First 0.5 GB free; $0.10 / GB-month thereafter | **~$0.00/hr** | **~$0.00/day** |
| **Dedicated VPC, Subnets, WIF, IAM** | `gke-ephemeral-env-vpc`, subnets, OIDC pool, SAs | No charge | **$0.00/hr** | **$0.00/day** |
| **Total Estimated Run-Rate While Active** | | | **~$0.222/hr** | **~$5.33/day** |

### Historical Session Cost Note (Estimates vs. Measured Billing)

- **Active Runtime Window**: The Terraform infrastructure was provisioned between `2026-10-24T07:53Z` and `2026-10-24T08:03Z` and destroyed between `2026-10-24T09:16Z` and `2026-10-24T09:32Z` (~1.6 hours of active GKE cluster/node/NAT runtime and ~1.2 hours of active Regional External ALB forwarding rule runtime).
- **Estimated Session Cost**: At `~$0.222/hr`, the historical session incurred an **estimated ~$0.35–$0.50** in Compute Engine VM, persistent disk, Cloud NAT, and Load Balancing charges before teardown.
- **Important Distinction**: Destroying the cloud resources at the conclusion of testing reduces **future ongoing cost exposure to $0.00/hr**, but it does **not** mean the historical deployment had zero cost while it was running. Exact billed amounts are determined asynchronously by Google Cloud Billing invoice export (no BigQuery billing export dataset was configured in `your-gcp-project-id`).

### Architectural Cost Optimization Decisions

1. **Shared Gateway Architecture (`1` Load Balancer vs. `N` Load Balancers)**:
   Using one shared `Gateway` (`preview-shared-gateway`) with per-environment `HTTPRoute` rules avoided provisioning a separate Google Cloud Load Balancer per preview environment (~$0.025/hr = ~$18/month saved *per environment*).
2. **Zonal GKE Standard (`us-west4-a`) over Regional Multi-Zone**:
   Using a single-zone cluster (`us-west4-a`) qualified for the GKE Free Tier control-plane credit and avoided multiplying node counts across 3 zones (saving 66% on compute and disk costs).
3. **2 × `e2-standard-2` Nodes**:
   Provided 4 vCPUs and 16 GB RAM total, accommodating GKE Dataplane V2 system pods, the `preview-controller`, and up to 5–10 concurrent preview environments (`50m` CPU / `64Mi` memory request each) without triggering additional VM scale-out charges.
4. **Distroless / Static Go Binaries**:
   Both the `preview-controller` and `sample-app` container images were multi-stage static Go builds (`42.015 MB` total across all tags in Artifact Registry), staying well within the 0.5 GB Artifact Registry free tier.

---

## 2. Pre-Provisioning Regional & Global Quota Verification (`your-gcp-project-id`)

Verified via `gcloud compute regions describe us-west4` and `gcloud compute project-info describe` prior to provisioning:

### Regional Quotas (`us-west4`)

| Quota Metric | Pre-Deployment Usage | Peak Platform Usage | Quota Limit | Status |
|---|---|---|---|---|
| `CPUS` | `0` | `4` (`2 × e2-standard-2`) | `24` (`CPUS` in `us-west4`) | PASS (16.7% utilized) |
| `DISKS_TOTAL_GB` | `0` | `100` GB (`2 × 50 GB`) | `4,096` GB | PASS (2.4% utilized) |
| `IN_USE_ADDRESSES` | `0` | `2` (1 Gateway IP + 1 Cloud NAT IP) | `8` | PASS (25% utilized) |
| `STATIC_ADDRESSES` | `0` | `0` | `8` | PASS |
| `INSTANCE_GROUPS` | `0` | `1` | `100` | PASS |

### Global Project Quotas

| Quota Metric | Pre-Deployment Usage | Peak Platform Usage | Quota Limit | Status |
|---|---|---|---|---|
| `NETWORKS` | `1` (`default`) | `2` (`default` + `gke-ephemeral-env-vpc`) | `5` | PASS (`2 / 5`) |
| `SUBNETWORKS` | `0` (custom) | `+2` (`nodes` + `proxy`) | `100` | PASS (`2 / 100`) |
| `FIREWALLS` | `4` (`default`) | `+4` (GKE auto-created rules) | `100` | PASS |
| `FORWARDING_RULES` | `0` | `1` (Shared Gateway) | `15` | PASS (`1 / 15`) |
| `TARGET_HTTP_PROXIES` | `0` | `1` (Shared Gateway) | `10` | PASS (`1 / 10`) |
| `URL_MAPS` | `0` | `1` (Shared Gateway) | `10` | PASS (`1 / 10`) |
| `BACKEND_SERVICES` | `0` | `1` per active preview env (peak `5` in T13) | `50` | PASS (`5 / 50`) |
| `HEALTH_CHECKS` | `0` | `1` per active preview env (peak `5` in T13) | `75` | PASS (`5 / 75`) |

---

## 3. Post-Teardown Cloud Resource & Cost Status (`2026-10-24T09:33:00Z`)

Independently verified via `gcloud` CLI across `your-gcp-project-id` after executing `terraform apply tf-destroy.plan` (see [docs/TEARDOWN.md](TEARDOWN.md)):

| Resource Category | Post-Teardown Count in `your-gcp-project-id` | Continuing Hourly Cost |
|---|---|---|
| **GKE Clusters** (`gcloud container clusters list`) | `0` | `$0.00/hr` |
| **Compute Engine VMs, Disks, Instance Groups** | `0` VMs, `0` Disks, `0` Instance Groups | `$0.00/hr` |
| **Project VPC & Subnets** (`gke-ephemeral-env-*`) | `0` (only pre-existing `default` VPC remains) | `$0.00/hr` |
| **Cloud Routers & Cloud NAT** | `0` | `$0.00/hr` |
| **Forwarding Rules, External IPs, Target Proxies, URL Maps, Backend Services, Health Checks** | `0` | `$0.00/hr` |
| **Network Endpoint Groups (NEGs)** | `0` (all 11 `k8s1-b25dba4a-preview-*` NEGs deleted) | `$0.00/hr` |
| **Artifact Registry Repositories** (`us-west4`) | `0` | `$0.00/hr` |
| **Project Service Accounts & WIF Pool** | `gke-ephemeral-nodes` & `gke-ephemeral-gh-actions` deleted; `gke-ephemeral-gh-pool` in `DELETED` soft-delete state (expires `2026-11-23`) | `$0.00/hr` |
| **Total Continuing Platform Cost** | | **$0.00/hr** |
