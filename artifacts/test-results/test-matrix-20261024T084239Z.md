# Live GKE Integration & E2E Test Matrix Evidence

- **Run Timestamp (UTC)**: `2026-10-24T08:42:39Z`
- **GCP Project**: `your-gcp-project-id`
- **GKE Cluster**: `gke-ephemeral-env-cluster` (`us-west4-a`)
- **Sample Image**: `us-west4-docker.pkg.dev/your-gcp-project-id/gke-ephemeral-env/sample-app:v0.1.0`

---

## T00 — Preflight and Code Quality
- **Timestamp (UTC)**: `2026-10-24T08:42:39Z`
- **Status**: **PASS**
- **Active Kube Context**: `gke_your-gcp-project-id_us-west4-a_gke-ephemeral-env-cluster`
- **Kind / Minikube Check**: kind=`/opt/homebrew/bin/kind`, minikube=`not-installed` (neither used)
- **Terraform Validation**: `Success! The configuration is valid.`

```text
?   	github.com/devenes/gke-ephemeral-environment-platform/cmd/controller	[no test files]
ok  	github.com/devenes/gke-ephemeral-environment-platform/cmd/previewctl	2.223s
ok  	github.com/devenes/gke-ephemeral-environment-platform/cmd/sample-app	(cached)
?   	github.com/devenes/gke-ephemeral-environment-platform/pkg/apis/platform/v1alpha1	[no test files]
ok  	github.com/devenes/gke-ephemeral-environment-platform/pkg/controller	(cached)
ok  	github.com/devenes/gke-ephemeral-environment-platform/pkg/gateway	(cached)
?   	github.com/devenes/gke-ephemeral-environment-platform/pkg/metrics	[no test files]
ok  	github.com/devenes/gke-ephemeral-environment-platform/pkg/validation	(cached)
```

---

## T01 — GKE Deployment & Shared Gateway Readiness
- **Timestamp (UTC)**: `2026-10-24T08:42:48Z`
- **Status**: **PASS**
- **Shared Gateway IP**: `203.0.113.10`

```text
=== NODES ===
NAME                                                  STATUS   ROLES    AGE   VERSION               INTERNAL-IP   EXTERNAL-IP   OS-IMAGE                             KERNEL-VERSION      CONTAINER-RUNTIME
gke-gke-ephemeral-en-gke-ephemeral-en-ef45ad22-9z4b   Ready    <none>   40m   v1.36.4-gke.1495002   10.42.0.3     <none>        Container-Optimized OS from Google   6.12.105+ (amd64)   containerd://2.2.7
gke-gke-ephemeral-en-gke-ephemeral-en-ef45ad22-v1xv   Ready    <none>   39m   v1.36.4-gke.1495002   10.42.0.4     <none>        Container-Optimized OS from Google   6.12.105+ (amd64)   containerd://2.2.7
=== CRD ===
NAME                                  CREATED AT
previewenvironments.platform.gke.io   2026-10-24T08:06:03Z
=== CONTROLLER PODS ===
NAME                                  READY   STATUS    RESTARTS   AGE    IP             NODE                                                  NOMINATED NODE   READINESS GATES
preview-controller-6d8f8cb5d9-m4zmf   1/1     Running   0          8m6s   10.42.129.17   gke-gke-ephemeral-en-gke-ephemeral-en-ef45ad22-v1xv   <none>           <none>
=== GATEWAYCLASS & SHARED GATEWAY ===
NAME                               CONTROLLER                  ACCEPTED   AGE
gke-l7-regional-external-managed   networking.gke.io/gateway   True       41m
NAME                     CLASS                              ADDRESS         PROGRAMMED   AGE
preview-shared-gateway   gke-l7-regional-external-managed   203.0.113.10   True         36m
```

---

## T02 — Create One Environment (`pr-101`)
- **Timestamp (UTC)**: `2026-10-24T08:42:56Z`
- **Status**: **PASS**
- **Provisioning Duration**: `26s`

```text
=== PREVIEWCTl GET pr-101 ===
Name:               pr-101
Phase:              Ready
Namespace:          preview-pr-101
Preview URL:        http://203.0.113.10/preview/pr-101
Image:              us-west4-docker.pkg.dev/your-gcp-project-id/gke-ephemeral-env/sample-app:v0.1.0
Developer / PR:     devenes (PR #101)
Ready Replicas:     1/1
Remaining TTL:      29m58s
Created At:         2026-10-24T11:42:57+03:00
Ready At:           2026-10-24T11:43:22+03:00
Expires At:         2026-10-24T12:13:22+03:00
Message:            Environment ready at http://203.0.113.10/preview/pr-101
Conditions:
  - Validated        Status=True  Reason=SpecValid              Message=Spec passed platform security and resource validation
  - NamespaceReady   Status=True  Reason=NamespaceProvisioned   Message=Namespace preview-pr-101, ResourceQuota, LimitRange, and NetworkPolicy reconciled
  - WorkloadReady    Status=True  Reason=DeploymentAvailable    Message=Deployment has 1/1 ready replicas
  - RouteAccepted    Status=True  Reason=Accepted               Message=HTTPRoute accepted by shared Gateway
  - Ready            Status=True  Reason=EnvironmentReady       Message=Preview environment healthy and routed at http://203.0.113.10/preview/pr-101
=== CHILD RESOURCES IN preview-pr-101 ===
NAME                          READY   UP-TO-DATE   AVAILABLE   AGE   CONTAINERS   IMAGES                                                                        SELECTOR
deployment.apps/preview-app   1/1     1            1           27s   app          us-west4-docker.pkg.dev/your-gcp-project-id/gke-ephemeral-env/sample-app:v0.1.0   app.kubernetes.io/instance=pr-101,app.kubernetes.io/name=preview-app

NAME                               READY   STATUS    RESTARTS   AGE   IP             NODE                                                  NOMINATED NODE   READINESS GATES
pod/preview-app-675bf986bf-lc552   1/1     Running   0          28s   10.42.129.26   gke-gke-ephemeral-en-gke-ephemeral-en-ef45ad22-v1xv   <none>           <none>

NAME                  TYPE        CLUSTER-IP    EXTERNAL-IP   PORT(S)   AGE   SELECTOR
service/preview-svc   ClusterIP   10.43.6.170   <none>        80/TCP    28s   app.kubernetes.io/instance=pr-101,app.kubernetes.io/name=preview-app

NAME                          REQUEST                                                                         LIMIT                                          AGE
resourcequota/preview-quota   pods: 1/6, requests.cpu: 50m/600m, requests.memory: 64Mi/512Mi, services: 1/4   limits.cpu: 200m/1, limits.memory: 128Mi/1Gi   28s

NAME                        CREATED AT
limitrange/preview-limits   2026-10-24T08:42:57Z

NAME                                                POD-SELECTOR   AGE
networkpolicy.networking.k8s.io/preview-isolation   <none>         28s

NAME                                                HOSTNAMES   AGE
httproute.gateway.networking.k8s.io/preview-route               29s
```

---

## T03 — External HTTP Access (`pr-101`)
- **Timestamp (UTC)**: `2026-10-24T08:43:26Z`
- **Status**: **PASS**
- **External URL**: `http://203.0.113.10/preview/pr-101`
- **ALB Data-Plane Convergence After Ready**: `0s`

```json
{
  "service": "preview-sample-app",
  "status": "running",
  "environmentName": "pr-101",
  "namespace": "preview-pr-101",
  "prNumber": "101",
  "repository": "devenes/gke-ephemeral-environment-platform",
  "commitSHA": "130a6c8",
  "developer": "devenes",
  "hostname": "preview-app-675bf986bf-lc552",
  "version": "v0.1.0",
  "gitCommit": "130a6c8",
  "requestPath": "/",
  "timestamp": "2026-10-24T08:43:27Z",
  "customEnv": {
    "FEATURE_CHECKOUT_V2": "enabled",
    "LOG_LEVEL": "info"
  }
}
```

```json
{
  "service": "preview-sample-app",
  "environmentName": "pr-101",
  "version": "v0.1.0",
  "gitCommit": "130a6c8",
  "buildTime": "2026-10-24T08:07:48Z",
  "commitSHA": "130a6c8"
}
```

---

## T04 — Concurrent Multi-Environment Routing (`env-alpha` & `env-beta`)
- **Timestamp (UTC)**: `2026-10-24T08:43:29Z`
- **Status**: **PASS**
- **env-alpha URL**: `http://203.0.113.10/preview/env-alpha` -> `environmentName=env-alpha`
- **env-beta URL**: `http://203.0.113.10/preview/env-beta` -> `environmentName=env-beta`

```text
NAME       PHASE  NAMESPACE          READY  REMAINING_TTL  PREVIEW_URL                             MESSAGE
env-alpha  Ready  preview-env-alpha  1/1    29m50s         http://203.0.113.10/preview/env-alpha  Environment ready at http://203.0.113.10/preview/env-alpha
env-beta   Ready  preview-env-beta   1/1    29m50s         http://203.0.113.10/preview/env-beta   Environment ready at http://203.0.113.10/preview/env-beta
pr-101     Ready  preview-pr-101     1/1    29m10s         http://203.0.113.10/preview/pr-101     Environment ready at http://203.0.113.10/preview/pr-101

=== GCP Forwarding Rules (Single Shared Load Balancer Verified) ===
NAME                                                             IP_ADDRESS     TARGET
gkegw1-tot4-preview-platform-sy-preview-shared-gat-k1fyo43l5rgh  203.0.113.10  https://www.googleapis.com/compute/v1/projects/your-gcp-project-id/regions/us-west4/targetHttpProxies/gkegw1-tot4-preview-platform-sy-preview-shared-gat-6mtow4usstbr
```

---

## T05 — Resource Policies (`LimitRange`, `ResourceQuota`, & Controller Enforcement)
- **Timestamp (UTC)**: `2026-10-24T08:44:12Z`
- **Status**: **PASS**
- **LimitRange Default Injection**: `requests={"cpu":"50m","memory":"64Mi"} limits={"cpu":"200m","memory":"128Mi"}`

```text
=== LimitRange Admission Rejection (CPU > 500m) ===
Error from server (Forbidden): error when creating "STDIN": pods "limitrange-exceed-pod" is forbidden: maximum cpu usage per Container is 500m, but limit is 800m

=== ResourceQuota Admission Rejection (requests.cpu > 600m) ===
Error from server (Forbidden): error when creating "STDIN": pods "quota-filler-2" is forbidden: exceeded quota: preview-quota, requested: requests.cpu=350m, used: requests.cpu=400m, limited: requests.cpu=600m
```

---

## T06 — Multi-Tenant Network Isolation (Dataplane V2 eBPF)
- **Timestamp (UTC)**: `2026-10-24T08:44:21Z`
- **Status**: **PASS**
- **Intra-Namespace Probe (`preview-env-alpha` -> `10.42.129.27:8080`)**: `Succeeded`
- **Cross-Namespace Probe (`preview-env-alpha` -> `10.42.129.28:8080` in `preview-env-beta`)**: `Blocked (Failed)`
- **Shared Gateway Routing After Probe**: alpha=`HTTP 200`, beta=`HTTP 200`

```text
=== Intra-Namespace Probe Output ===
probe status=200 url=http://10.42.129.27:8080/healthz body={
  "status": "ok",
  "check": "liveness",
  "environmentName": "env-alpha",
  "timestamp": "2026-10-24T08:44:27Z"
}

=== Cross-Namespace Probe Output (Blocked by Dataplane V2 NetworkPolicy) ===
probe failed for http://10.42.129.28:8080/healthz: Get "http://10.42.129.28:8080/healthz": context deadline exceeded (Client.Timeout exceeded while awaiting headers)
```

---

## T07 — Invalid Requests (API Admission & Controller Validation Layers)
- **Timestamp (UTC)**: `2026-10-24T08:46:36Z`
- **Status**: **PASS**
- **Disallowed Image Registry (Controller Layer)**: Phase=`Failed`, Reason=`DisallowedImage` (image "docker.io/library/nginx:1.27" is not from an allowed Artifact Registry prefix ([us-west4-docker.pkg.dev/your-gcp-project-id/gke-ephemeral-env/]))
- **Excessive CPU/Memory (Controller Layer)**: Phase=`Failed`, Reason=`ExcessiveResources` (per-pod cpuLimit (2) exceeds platform maximum (500m))

```text
=== Invalid Name (API Admission CEL Rejection) ===
The PreviewEnvironment "this-environment-name-is-way-too-long-and-exceeds-forty-characters" is invalid: <nil>: Invalid value: metadata.name must be 40 characters or fewer so derived preview-<name> identifiers stay within DNS-1035 limits

=== Invalid TTL (API Admission Schema Rejection) ===
The PreviewEnvironment "invalid-ttl" is invalid: spec.ttlSeconds: Invalid value: 5: spec.ttlSeconds in body should be greater than or equal to 30

=== Invalid Replicas (API Admission Schema Rejection) ===
The PreviewEnvironment "invalid-replicas" is invalid: spec.replicas: Invalid value: 10: spec.replicas in body should be less than or equal to 3
```

---

## T08 — Automatic TTL Expiration & Cleanup (`ttl-test`)
- **Timestamp (UTC)**: `2026-10-24T08:46:59Z`
- **Status**: **PASS**
- **Configured TTL**: `35s`
- **Scheduled ExpiresAt**: `2026-10-24T08:47:45Z`
- **Confirmed Complete Removal At**: `2026-10-24T08:48:36Z`
- **Orphan Namespace Check**: `preview-ttl-test NotFound`

---

## T09 — Manual Deletion & Finalizer Verification (`pr-101`)
- **Timestamp (UTC)**: `2026-10-24T08:48:37Z`
- **Status**: **PASS**
- **Manual Deletion Duration**: `64s`

```text
Deleted PreviewEnvironment "pr-101"
Confirmed PreviewEnvironment "pr-101" and its owned namespace are completely removed
```

---

## T10 & T11 — Controller Restart Recovery & Idempotent Reconciliation
- **Timestamp (UTC)**: `2026-10-24T08:49:42Z`
- **Status**: **PASS**
- **Deployment UID Preserved Across Restart & Re-apply**: `23391ae2-8f27-4277-ba33-96614eb9f055` == `23391ae2-8f27-4277-ba33-96614eb9f055`
- **Service UID Preserved Across Restart & Re-apply**: `7982b050-fd42-4ca9-9504-5175fce913c3` == `7982b050-fd42-4ca9-9504-5175fce913c3`
- **Pod Count**: `1` (zero duplicates)

---

## T12 — Failure Recovery & Drift Self-Healing
- **Timestamp (UTC)**: `2026-10-24T08:50:45Z`
- **Status**: **PASS**
- **Self-Healing Verification**: Deleted `Service/preview-svc` and `Deployment/preview-app` were automatically recreated and restored to `1/1 Ready` by controller watches

```text
NAME                          READY   UP-TO-DATE   AVAILABLE   AGE   CONTAINERS   IMAGES                                                                        SELECTOR
deployment.apps/preview-app   1/1     1            1           10s   app          us-west4-docker.pkg.dev/your-gcp-project-id/gke-ephemeral-env/sample-app:v0.1.0   app.kubernetes.io/instance=resilient-env,app.kubernetes.io/name=preview-app

NAME                  TYPE        CLUSTER-IP     EXTERNAL-IP   PORT(S)   AGE   SELECTOR
service/preview-svc   ClusterIP   10.43.15.123   <none>        80/TCP    11s   app.kubernetes.io/instance=resilient-env,app.kubernetes.io/name=preview-app

NAME                             READY   STATUS    RESTARTS   AGE   IP             NODE                                                  NOMINATED NODE   READINESS GATES
pod/preview-app-fc799f8f-pq67b   1/1     Running   0          10s   10.42.129.36   gke-gke-ephemeral-en-gke-ephemeral-en-ef45ad22-v1xv   <none>           1/1
```

---

## T13 — Controlled Scale Test (5 Concurrent Preview Environments)
- **Timestamp (UTC)**: `2026-10-24T08:51:41Z`
- **Status**: **PASS**
- **Concurrent Environments Provisioned**: `5`
- **Batch Creation-to-Ready Latency**: `45s`
- **Batch Deletion Latency**: `31s`
- **Orphan Tenant Namespaces Remaining**: `0`

```text
=== Active Scale Environments ===
NAME     PHASE  NAMESPACE        READY  REMAINING_TTL  PREVIEW_URL                           MESSAGE
scale-1  Ready  preview-scale-1  1/1    29m49s         http://203.0.113.10/preview/scale-1  Environment ready at http://203.0.113.10/preview/scale-1
scale-2  Ready  preview-scale-2  1/1    29m49s         http://203.0.113.10/preview/scale-2  Environment ready at http://203.0.113.10/preview/scale-2
scale-3  Ready  preview-scale-3  1/1    29m50s         http://203.0.113.10/preview/scale-3  Environment ready at http://203.0.113.10/preview/scale-3
scale-4  Ready  preview-scale-4  1/1    29m49s         http://203.0.113.10/preview/scale-4  Environment ready at http://203.0.113.10/preview/scale-4
scale-5  Ready  preview-scale-5  1/1    29m49s         http://203.0.113.10/preview/scale-5  Environment ready at http://203.0.113.10/preview/scale-5

=== Controller Prometheus Metrics ===
# HELP preview_cleanup_failures_total Total number of errors encountered during PreviewEnvironment teardown or finalizer cleanup.
# TYPE preview_cleanup_failures_total counter
preview_cleanup_failures_total 0
# HELP preview_environments_by_phase Current number of PreviewEnvironment resources partitioned by lifecycle phase.
# TYPE preview_environments_by_phase gauge
preview_environments_by_phase{phase="Deleting"} 0
preview_environments_by_phase{phase="Expired"} 0
preview_environments_by_phase{phase="Failed"} 0
preview_environments_by_phase{phase="Pending"} 0
preview_environments_by_phase{phase="Provisioning"} 0
preview_environments_by_phase{phase="Ready"} 0
# HELP preview_provisioning_duration_seconds Duration in seconds from PreviewEnvironment creation to Ready phase.
# TYPE preview_provisioning_duration_seconds histogram
preview_provisioning_duration_seconds_bucket{le="1"} 0
preview_provisioning_duration_seconds_bucket{le="2"} 0
preview_provisioning_duration_seconds_bucket{le="5"} 0
preview_provisioning_duration_seconds_bucket{le="10"} 0
preview_provisioning_duration_seconds_bucket{le="20"} 0
preview_provisioning_duration_seconds_bucket{le="30"} 0
preview_provisioning_duration_seconds_bucket{le="45"} 5
preview_provisioning_duration_seconds_bucket{le="60"} 5
preview_provisioning_duration_seconds_bucket{le="90"} 5
preview_provisioning_duration_seconds_bucket{le="120"} 5
preview_provisioning_duration_seconds_bucket{le="180"} 5
preview_provisioning_duration_seconds_bucket{le="300"} 5
preview_provisioning_duration_seconds_bucket{le="+Inf"} 5
preview_provisioning_duration_seconds_sum 170.710876095
preview_provisioning_duration_seconds_count 5
# HELP preview_reconcile_total Total number of PreviewEnvironment reconciliations partitioned by result.
# TYPE preview_reconcile_total counter
preview_reconcile_total{result="requeue"} 68
preview_reconcile_total{result="success"} 132
# HELP preview_ttl_cleanup_delay_seconds Delay in seconds between TTL expiration timestamp and controller cleanup initiation.
# TYPE preview_ttl_cleanup_delay_seconds histogram
preview_ttl_cleanup_delay_seconds_bucket{le="0.1"} 0
preview_ttl_cleanup_delay_seconds_bucket{le="0.5"} 0
preview_ttl_cleanup_delay_seconds_bucket{le="1"} 0
preview_ttl_cleanup_delay_seconds_bucket{le="2"} 0
preview_ttl_cleanup_delay_seconds_bucket{le="5"} 0
preview_ttl_cleanup_delay_seconds_bucket{le="10"} 0
preview_ttl_cleanup_delay_seconds_bucket{le="30"} 0
preview_ttl_cleanup_delay_seconds_bucket{le="60"} 0
preview_ttl_cleanup_delay_seconds_bucket{le="+Inf"} 0
preview_ttl_cleanup_delay_seconds_sum 0
preview_ttl_cleanup_delay_seconds_count 0
```

---

## T14 — GitHub Pull Request Lifecycle (`PR #1` / `pr-1`)
- **Timestamp (UTC)**: `2026-10-24T09:06:52Z`
- **Status**: **PASS**
- **Pull Request**: `https://github.com/devenes/gke-ephemeral-environment-platform/pull/1` (`test/pr-preview-e2e` -> `main`, commit `815e888`)
- **CI Workflow Run (`pull_request` opened)**: `40080038836` (`CI — Build, Test & Static Validation` — `completed / success` in `5m20s`)
- **PR Provisioning Workflow Run (`pull_request` opened)**: `40080038798` (`PR Preview Environment Lifecycle` — `completed / success` in `5m35s`)
  - Authenticated keyless via GitHub OIDC Workload Identity Provider `projects/123456789012/locations/global/workloadIdentityPools/gke-ephemeral-gh-pool/providers/github-oidc` as `gke-ephemeral-gh-actions@your-gcp-project-id.iam.gserviceaccount.com`
  - Built and pushed `us-west4-docker.pkg.dev/your-gcp-project-id/gke-ephemeral-env/sample-app:pr-1-815e888`
  - Connected to GKE control plane via IAM-gated DNS endpoint (`--dns-endpoint`) without modifying `master_authorized_networks_config`
  - Provisioned `PreviewEnvironment/pr-1` (`Namespace: preview-pr-1`) and verified live external HTTP traffic at `http://203.0.113.10/preview/pr-1/version`
- **PR Close Cleanup Workflow Run (`pull_request` closed)**: `40080468687` (`PR Preview Environment Lifecycle` — `completed / success` in `3m01s`)
  - Executed `./bin/previewctl delete pr-1 --wait --timeout 3m` and confirmed `Namespace/preview-pr-1` was completely removed (`NotFound`)

```text
=== Live PR #1 Environment Status (Before PR Close) ===
Name:               pr-1
Phase:              Ready
Namespace:          preview-pr-1
Preview URL:        http://203.0.113.10/preview/pr-1
Image:              us-west4-docker.pkg.dev/your-gcp-project-id/gke-ephemeral-env/sample-app:pr-1-815e888
Developer / PR:     devenes (PR #1)
Ready Replicas:     1/1
Remaining TTL:      58m30s
Created At:         2026-10-24T12:01:40+03:00
Ready At:           2026-10-24T12:02:08+03:00
Expires At:         2026-10-24T13:02:08+03:00
Message:            Environment ready at http://203.0.113.10/preview/pr-1

=== External HTTP Verification Response (http://203.0.113.10/preview/pr-1/) ===
{
  "service": "preview-sample-app",
  "status": "running",
  "environmentName": "pr-1",
  "namespace": "preview-pr-1",
  "prNumber": "1",
  "repository": "devenes/gke-ephemeral-environment-platform",
  "commitSHA": "815e888",
  "developer": "devenes",
  "hostname": "preview-app-68794d8888-7djlq",
  "version": "pr-1-815e888",
  "gitCommit": "815e888",
  "requestPath": "/",
  "timestamp": "2026-10-24T09:03:38Z"
}

=== Post-Close Verification ===
Error from server (NotFound): namespaces "preview-pr-1" not found
```

