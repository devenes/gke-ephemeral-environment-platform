# Phase 1 — Read-Only Preflight Report

- **Timestamp**: `2026-10-24T10:44:49+03:00` (`2026-10-24T07:44:49Z`)
- **Workspace Path**: `/Users/enes/GitHub/gke-ephemeral-environment-platform`
- **Target GCP Project**: `enes-turan-demo` (Project Number: `697289276442`)
- **Preferred Region / Zone**: `us-west4` / `us-west4-a`

---

## 1. Local Directory & Git / GitHub Verification

| Check | Observed Value | Status |
|---|---|---|
| Working directory exists | `/Users/enes/GitHub/gke-ephemeral-environment-platform` | PASS |
| Existing contents | `.git/`, `.gitattributes` (66B) | PASS |
| Git branch & status | `main...origin/main` (clean working tree) | PASS |
| HEAD commit | `130a6c8 Initial commit` (`2026-10-24 10:40:49 +0300`) | PASS |
| Git remote `origin` | `https://github.com/devenes/gke-ephemeral-environment-platform.git` | PASS |
| GitHub CLI auth | Authenticated as `devenes` (`repo`, `read:org`, `gist`, `admin:public_key`) | PASS |
| Remote repository | `devenes/gke-ephemeral-environment-platform` (`PRIVATE`, default branch `main`) | PASS |

---

## 2. Local Toolchain Versions

| Tool | Version / Path | Status |
|---|---|---|
| `git` | `git version 2.54.0 (Apple Git-157)` | PASS |
| `gh` | `gh version 2.102.0 (2026-09-30)` | PASS |
| `gcloud` | `Google Cloud SDK 590.0.0` (`/opt/homebrew/bin/gcloud`) | PASS |
| `gke-gcloud-auth-plugin` | `/opt/homebrew/share/google-cloud-sdk/bin/gke-gcloud-auth-plugin` | PASS (add `/opt/homebrew/share/google-cloud-sdk/bin` to `PATH`) |
| `terraform` | `Terraform v1.16.5` | PASS |
| `kubectl` | `Client Version: v1.37.1`, `Kustomize Version: v5.8.1` | PASS |
| `go` | `go version go1.27.2 darwin/arm64` | PASS |
| `docker` | Client `29.9.0`, Daemon `29.9.2` (`linux/aarch64`) | PASS |

---

## 3. Google Cloud Identity, Project & Billing

| Check | Observed Value | Status |
|---|---|---|
| Active Account | `ahmedenesturan@gmail.com` | PASS |
| Application Default Credentials (ADC) | Valid (`gcloud auth application-default print-access-token` succeeded) | PASS |
| Active Project ID | `enes-turan-demo` | PASS |
| Project Number | `697289276442` | PASS |
| Project Lifecycle State | `ACTIVE` | PASS |
| Billing Enabled | `true` (`billingAccounts/016EBC-28B3CB-A7470E`) | PASS |
| Developer Egress IP | Verified `/32` public IPv4 egress for master authorized networks | PASS |

---

## 4. Existing GCP Infrastructure Inventory (Read-Only)

- **GKE Clusters**: None (`0` active clusters across the project).
- **VPC Networks**: Only `default` (`auto` subnet mode; `10.182.0.0/20` in `us-west4`). No custom VPCs exist.
- **Addresses & Forwarding Rules**: `0` static/ephemeral addresses, `0` forwarding rules.
- **Artifact Registry Repositories**: `0` repositories across all locations.
- **Cloud Storage Buckets**: `0` buckets.
- **Service Accounts**:
  - `697289276442-compute@developer.gserviceaccount.com` (Compute Engine default service account)
  - `ais-gemini-key-c62308ae7c414d2@697289276442.iam.gserviceaccount.com` (Unrelated — preserved untouched)
- **Workload Identity Pools**:
  - `projects/697289276442/locations/global/workloadIdentityPools/enes-turan-demo.svc.id.goog` (`ACTIVE`)
  - No soft-deleted custom Workload Identity Pools.
- **Monitoring Alert Policies**: `0` policies.
- **Key Enabled APIs**:
  - `compute.googleapis.com`
  - `container.googleapis.com`
  - `artifactregistry.googleapis.com`
  - `iam.googleapis.com`
  - `iamcredentials.googleapis.com`
  - `sts.googleapis.com`
  - `networkservices.googleapis.com`
  - `networksecurity.googleapis.com`
  - `logging.googleapis.com`
  - `monitoring.googleapis.com`

---

## 5. Quotas & Regional Capacity (`us-west4` & Global)

### Regional Quotas (`us-west4`)

| Metric | Usage | Limit | Headroom |
|---|---|---|---|
| `CPUS` | `0.0` | `200.0` | `200` vCPUs |
| `DISKS_TOTAL_GB` | `0.0` | `4096.0` | `4096` GB |
| `SSD_TOTAL_GB` | `0.0` | `500.0` | `500` GB |
| `STATIC_ADDRESSES` | `0.0` | `8.0` | `8` IPv4 addresses |
| `IN_USE_ADDRESSES` | `0.0` | `8.0` | `8` IPv4 addresses |
| `INSTANCE_GROUPS` | `0.0` | `100.0` | `100` groups |

### Global Project Quotas

| Metric | Usage | Limit | Headroom |
|---|---|---|---|
| `NETWORKS` | `1.0` | `5.0` | `4` VPC networks |
| `SUBNETWORKS` | `0.0` | `100.0` | `100` subnets |
| `FIREWALLS` | `4.0` | `100.0` | `96` firewall rules |
| `BACKEND_SERVICES` | `0.0` | `50.0` | `50` backend services |
| `URL_MAPS` | `0.0` | `10.0` | `10` URL maps |
| `TARGET_HTTP_PROXIES` | `0.0` | `10.0` | `10` target HTTP proxies |
| `FORWARDING_RULES` | `0.0` | `15.0` | `15` forwarding rules |
| `HEALTH_CHECKS` | `0.0` | `75.0` | `75` health checks |

---

## 6. Preflight Conclusion

All tools, authentications, permissions, APIs, and regional/global quotas in `enes-turan-demo` (`us-west4`) are verified and healthy. No conflicting infrastructure exists. Safe to proceed to Phase 2 (Research & Architecture Decisions) and Phase 3 (Terraform Infrastructure).
