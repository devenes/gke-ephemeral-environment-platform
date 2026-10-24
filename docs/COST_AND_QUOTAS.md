# Cost & Quotas Analysis — GKE Ephemeral Environment Platform

## 1. Pre-Provisioning Cost Estimate (`us-west4` / `us-west4-a`)

Before applying the Terraform plan, we estimated the ongoing hourly and daily costs of every cloud resource provisioned by this platform in `enes-turan-demo` (`us-west4`):

| Resource | Configuration | Pricing Basis (`us-west4`) | Estimated Hourly Cost | Estimated Daily Cost |
|---|---|---|---|---|
| **GKE Zonal Control Plane** | 1 zonal cluster (`gke-ephemeral-env-cluster` in `us-west4-a`) | Covered by GKE Free Tier ($74.40/mo credit for 1 zonal/Autopilot cluster per billing account; `$0.10/hr` otherwise) | **$0.00/hr** (Free Tier) | **$0.00/day** |
| **GKE Worker Nodes** | 2 × `e2-standard-2` (2 vCPU, 8 GB RAM each = 4 vCPU, 16 GB RAM total) | ~$0.06701/hr per `e2-standard-2` VM in `us-west4` | **~$0.134/hr** | **~$3.22/day** |
| **Worker Boot Disks** | 2 × 50 GB `pd-balanced` (100 GB total) | $0.10 / GB-month (~$10.00/mo total) | **~$0.014/hr** | **~$0.33/day** |
| **Shared GKE Gateway (Regional External ALB)** | 1 × `gke-l7-regional-external-managed` forwarding rule | ~$0.025/hr for 1st forwarding rule + minimal data processing | **~$0.025/hr** | **~$0.60/day** |
| **Cloud NAT** | 1 × Regional Cloud NAT gateway (`gke-ephemeral-env-nat`) | ~$0.044/hr base gateway + 1 NAT IP | **~$0.049/hr** | **~$1.18/day** |
| **Artifact Registry** | `gke-ephemeral-env` Docker repository (< 0.5 GB total images) | First 0.5 GB free; $0.10 / GB-month thereafter | **~$0.00/hr** | **~$0.00/day** |
| **Dedicated VPC, Subnets, WIF, IAM** | `gke-ephemeral-env-vpc`, subnets, OIDC pool, SAs | No charge | **$0.00/hr** | **$0.00/day** |
| **Total Estimated Baseline** | | | **~$0.222/hr** | **~$5.33/day** |

### Cost Optimization Decisions

1. **Shared Gateway Architecture (`1` Load Balancer vs. `N` Load Balancers)**:
   Using one shared `Gateway` (`preview-shared-gateway`) with per-environment `HTTPRoute` rules avoids provisioning a separate Google Cloud Load Balancer per preview environment (~$0.025/hr = ~$18/month saved *per environment*).
2. **Zonal GKE Standard (`us-west4-a`) over Regional Multi-Zone**:
   Using a single-zone cluster (`us-west4-a`) qualifies for the GKE Free Tier control-plane credit and avoids multiplying node counts across 3 zones (saving 66% on compute and disk costs).
3. **2 × `e2-standard-2` Nodes**:
   Provides 4 vCPUs and 16 GB RAM total, which accommodates GKE Dataplane V2 system pods, the `preview-controller`, and up to 10 concurrent preview environments (`50m` CPU / `64Mi` memory request each) without triggering additional VM scale-out charges.
4. **Distroless / Scratch Static Go Binaries**:
   Both the `preview-controller` and `sample-app` container images are multi-stage static Go builds (< 25 MB each), keeping Artifact Registry storage well within the 0.5 GB free tier.

---

## 2. Regional & Global Quota Verification (`enes-turan-demo`)

Verified via `gcloud compute regions describe us-west4` and `gcloud compute project-info describe` before provisioning:

### Regional Quotas (`us-west4`)

| Quota Metric | Pre-Deployment Usage | Platform Requirement | Quota Limit | Status |
|---|---|---|---|---|
| `CPUS` | `0` | `4` (`2 × e2-standard-2`) | `200` | PASS (2% utilized) |
| `DISKS_TOTAL_GB` | `0` | `100` GB (`2 × 50 GB`) | `4,096` GB | PASS (2.4% utilized) |
| `IN_USE_ADDRESSES` | `0` | `2` (1 Gateway IP + 1 Cloud NAT IP) | `8` | PASS (25% utilized) |
| `STATIC_ADDRESSES` | `0` | `0` | `8` | PASS |
| `INSTANCE_GROUPS` | `0` | `1` | `100` | PASS |

### Global Project Quotas

| Quota Metric | Pre-Deployment Usage | Platform Requirement | Quota Limit | Status |
|---|---|---|---|---|
| `NETWORKS` | `1` (`default`) | `+1` (`gke-ephemeral-env-vpc`) | `5` | PASS (`2 / 5`) |
| `SUBNETWORKS` | `0` (custom) | `+2` (`nodes` + `proxy`) | `100` | PASS (`2 / 100`) |
| `FIREWALLS` | `4` | `+3` (GKE auto-created rules) | `100` | PASS |
| `FORWARDING_RULES` | `0` | `+1` (Shared Gateway) | `15` | PASS (`1 / 15`) |
| `TARGET_HTTP_PROXIES` | `0` | `+1` (Shared Gateway) | `10` | PASS (`1 / 10`) |
| `URL_MAPS` | `0` | `+1` (Shared Gateway) | `10` | PASS (`1 / 10`) |
| `BACKEND_SERVICES` | `0` | `1` per active preview env (up to 10 in T13) | `50` | PASS (`<= 10 / 50`) |
| `HEALTH_CHECKS` | `0` | `1` per active preview env (up to 10 in T13) | `75` | PASS (`<= 10 / 75`) |
