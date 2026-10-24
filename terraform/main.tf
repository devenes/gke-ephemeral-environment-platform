locals {
  common_labels = {
    project     = "gke-ephemeral-environment-platform"
    managed_by  = "terraform"
    environment = "demo"
  }

  required_apis = toset([
    "compute.googleapis.com",
    "container.googleapis.com",
    "artifactregistry.googleapis.com",
    "iam.googleapis.com",
    "iamcredentials.googleapis.com",
    "sts.googleapis.com",
    "networkservices.googleapis.com",
    "networksecurity.googleapis.com",
    "logging.googleapis.com",
    "monitoring.googleapis.com",
  ])
}

resource "google_project_service" "required" {
  for_each           = local.required_apis
  project            = var.project_id
  service            = each.value
  disable_on_destroy = false
}

# ---------------------------------------------------------------------------
# Dedicated VPC, Subnets, Cloud Router & NAT
# ---------------------------------------------------------------------------

resource "google_compute_network" "vpc" {
  name                    = var.vpc_name
  project                 = var.project_id
  auto_create_subnetworks = false
  routing_mode            = "REGIONAL"
  description             = "Dedicated VPC for GKE Ephemeral Environment Platform"

  depends_on = [google_project_service.required]
}

resource "google_compute_subnetwork" "nodes" {
  name                     = "gke-ephemeral-env-nodes"
  project                  = var.project_id
  region                   = var.region
  network                  = google_compute_network.vpc.id
  ip_cidr_range            = var.nodes_subnet_cidr
  private_ip_google_access = true

  secondary_ip_range {
    range_name    = "gke-ephemeral-env-pods"
    ip_cidr_range = var.pods_secondary_cidr
  }

  secondary_ip_range {
    range_name    = "gke-ephemeral-env-services"
    ip_cidr_range = var.services_secondary_cidr
  }
}

resource "google_compute_subnetwork" "proxy_only" {
  name          = "gke-ephemeral-env-proxy"
  project       = var.project_id
  region        = var.region
  network       = google_compute_network.vpc.id
  ip_cidr_range = var.proxy_subnet_cidr
  purpose       = "REGIONAL_MANAGED_PROXY"
  role          = "ACTIVE"
}

resource "google_compute_router" "router" {
  name    = "gke-ephemeral-env-router"
  project = var.project_id
  region  = var.region
  network = google_compute_network.vpc.id
}

resource "google_compute_router_nat" "nat" {
  name                               = "gke-ephemeral-env-nat"
  project                            = var.project_id
  region                             = var.region
  router                             = google_compute_router.router.name
  nat_ip_allocate_option             = "AUTO_ONLY"
  source_subnetwork_ip_ranges_to_nat = "LIST_OF_SUBNETWORKS"

  subnetwork {
    name                    = google_compute_subnetwork.nodes.id
    source_ip_ranges_to_nat = ["ALL_IP_RANGES"]
  }
}

# ---------------------------------------------------------------------------
# Artifact Registry
# ---------------------------------------------------------------------------

resource "google_artifact_registry_repository" "repo" {
  project       = var.project_id
  location      = var.region
  repository_id = var.artifact_registry_repo_id
  description   = "Container images for GKE Ephemeral Environment Platform controller and sample apps"
  format        = "DOCKER"
  labels        = local.common_labels

  depends_on = [google_project_service.required]
}

# ---------------------------------------------------------------------------
# Least-Privilege IAM Service Accounts
# ---------------------------------------------------------------------------

resource "google_service_account" "gke_nodes" {
  project      = var.project_id
  account_id   = "gke-ephemeral-nodes"
  display_name = "GKE Ephemeral Env Node Pool Service Account"
  description  = "Least-privilege service account for GKE worker nodes"
}

locals {
  node_sa_roles = toset([
    "roles/logging.logWriter",
    "roles/monitoring.metricWriter",
    "roles/monitoring.viewer",
    "roles/stackdriver.resourceMetadata.writer",
    "roles/artifactregistry.reader",
  ])
}

resource "google_project_iam_member" "gke_nodes_roles" {
  for_each = local.node_sa_roles
  project  = var.project_id
  role     = each.value
  member   = "serviceAccount:${google_service_account.gke_nodes.email}"
}

resource "google_service_account" "github_actions" {
  project      = var.project_id
  account_id   = "gke-ephemeral-gh-actions"
  display_name = "GitHub Actions Preview Automation SA"
  description  = "Service account federated via GitHub OIDC for preview environment CI/CD"
}

locals {
  github_sa_roles = toset([
    "roles/artifactregistry.writer",
    "roles/container.developer",
    "roles/container.clusterViewer",
  ])
}

resource "google_project_iam_member" "github_actions_roles" {
  for_each = local.github_sa_roles
  project  = var.project_id
  role     = each.value
  member   = "serviceAccount:${google_service_account.github_actions.email}"
}

# ---------------------------------------------------------------------------
# GitHub OIDC Workload Identity Federation
# ---------------------------------------------------------------------------

resource "google_iam_workload_identity_pool" "github" {
  project                   = var.project_id
  workload_identity_pool_id = "gke-ephemeral-gh-pool"
  display_name              = "GitHub Actions OIDC Pool"
  description               = "OIDC pool for GitHub Actions preview environment automation"

  depends_on = [google_project_service.required]
}

resource "google_iam_workload_identity_pool_provider" "github" {
  project                            = var.project_id
  workload_identity_pool_id          = google_iam_workload_identity_pool.github.workload_identity_pool_id
  workload_identity_pool_provider_id = "github-oidc"
  display_name                       = "GitHub OIDC Provider"
  description                        = "Restricts OIDC token exchange to the target GitHub repository"

  attribute_mapping = {
    "google.subject"             = "assertion.sub"
    "attribute.actor"            = "assertion.actor"
    "attribute.repository"       = "assertion.repository"
    "attribute.repository_owner" = "assertion.repository_owner"
  }

  attribute_condition = "assertion.repository == '${var.github_repository}'"

  oidc {
    issuer_uri = "https://token.actions.githubusercontent.com"
  }
}

resource "google_service_account_iam_member" "github_wif_binding" {
  service_account_id = google_service_account.github_actions.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "principalSet://iam.googleapis.com/${google_iam_workload_identity_pool.github.name}/attribute.repository/${var.github_repository}"
}

# ---------------------------------------------------------------------------
# GKE Standard Cluster (Dataplane V2 + Gateway API + Restricted Endpoint)
# ---------------------------------------------------------------------------

resource "google_container_cluster" "primary" {
  name     = var.cluster_name
  project  = var.project_id
  location = var.zone

  network         = google_compute_network.vpc.id
  subnetwork      = google_compute_subnetwork.nodes.id
  networking_mode = "VPC_NATIVE"

  remove_default_node_pool = true
  initial_node_count       = 1
  deletion_protection      = false

  # Enable Dataplane V2 (Cilium eBPF) for native NetworkPolicy enforcement
  datapath_provider = "ADVANCED_DATAPATH"

  # Enable GKE Gateway API controller (Standard channel)
  gateway_api_config {
    channel = "CHANNEL_STANDARD"
  }

  ip_allocation_policy {
    cluster_secondary_range_name  = "gke-ephemeral-env-pods"
    services_secondary_range_name = "gke-ephemeral-env-services"
  }

  private_cluster_config {
    enable_private_nodes    = true
    enable_private_endpoint = false
    master_ipv4_cidr_block  = var.master_ipv4_cidr_block
  }

  control_plane_endpoints_config {
    dns_endpoint_config {
      allow_external_traffic = true
    }
    ip_endpoints_config {
      enabled = true
    }
  }

  master_authorized_networks_config {
    cidr_blocks {
      cidr_block   = var.authorized_ipv4_cidr
      display_name = "developer-workstation"
    }
  }

  workload_identity_config {
    workload_pool = "${var.project_id}.svc.id.goog"
  }

  release_channel {
    channel = "REGULAR"
  }

  resource_labels = local.common_labels

  depends_on = [
    google_project_service.required,
    google_compute_subnetwork.nodes,
    google_compute_subnetwork.proxy_only,
  ]
}

resource "google_container_node_pool" "primary" {
  name       = "gke-ephemeral-env-pool"
  project    = var.project_id
  location   = var.zone
  cluster    = google_container_cluster.primary.name
  node_count = var.node_count

  node_config {
    machine_type    = var.node_machine_type
    disk_size_gb    = var.node_disk_size_gb
    disk_type       = "pd-balanced"
    service_account = google_service_account.gke_nodes.email

    oauth_scopes = [
      "https://www.googleapis.com/auth/cloud-platform",
    ]

    workload_metadata_config {
      mode = "GKE_METADATA"
    }

    shielded_instance_config {
      enable_secure_boot          = true
      enable_integrity_monitoring = true
    }

    labels = local.common_labels
  }

  management {
    auto_repair  = true
    auto_upgrade = true
  }
}
