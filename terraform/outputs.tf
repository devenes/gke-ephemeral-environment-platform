output "project_id" {
  description = "GCP Project ID."
  value       = var.project_id
}

output "region" {
  description = "GCP Region."
  value       = var.region
}

output "zone" {
  description = "GCP Zone for GKE cluster."
  value       = var.zone
}

output "cluster_name" {
  description = "GKE cluster name."
  value       = google_container_cluster.primary.name
}

output "cluster_endpoint" {
  description = "GKE cluster IP endpoint."
  value       = google_container_cluster.primary.endpoint
  sensitive   = true
}

output "cluster_dns_endpoint" {
  description = "GKE cluster IAM-gated DNS endpoint."
  value       = google_container_cluster.primary.control_plane_endpoints_config[0].dns_endpoint_config[0].endpoint
}

output "vpc_name" {
  description = "Dedicated VPC name."
  value       = google_compute_network.vpc.name
}

output "nodes_subnet_name" {
  description = "GKE nodes subnet name."
  value       = google_compute_subnetwork.nodes.name
}

output "proxy_subnet_name" {
  description = "Regional Managed Proxy subnet name."
  value       = google_compute_subnetwork.proxy_only.name
}

output "artifact_registry_url" {
  description = "Artifact Registry base URL for Docker images."
  value       = "${var.region}-docker.pkg.dev/${var.project_id}/${google_artifact_registry_repository.repo.repository_id}"
}

output "node_service_account_email" {
  description = "Least-privilege GKE node pool service account email."
  value       = google_service_account.gke_nodes.email
}

output "github_actions_service_account_email" {
  description = "GitHub Actions service account email."
  value       = google_service_account.github_actions.email
}

output "workload_identity_provider" {
  description = "Full resource name of the GitHub OIDC Workload Identity Pool Provider."
  value       = google_iam_workload_identity_pool_provider.github.name
}
