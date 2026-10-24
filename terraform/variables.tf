variable "project_id" {
  description = "Target Google Cloud project ID."
  type        = string
  default     = "enes-turan-demo"
}

variable "region" {
  description = "Primary GCP region for VPC subnets, Artifact Registry, and Regional Gateway."
  type        = string
  default     = "us-west4"
}

variable "zone" {
  description = "GCP zone for the zonal GKE Standard cluster."
  type        = string
  default     = "us-west4-a"
}

variable "cluster_name" {
  description = "Name of the GKE Standard cluster."
  type        = string
  default     = "gke-ephemeral-env-cluster"
}

variable "vpc_name" {
  description = "Name of the dedicated custom-mode VPC network."
  type        = string
  default     = "gke-ephemeral-env-vpc"
}

variable "nodes_subnet_cidr" {
  description = "Primary IPv4 CIDR block for GKE nodes."
  type        = string
  default     = "10.42.0.0/20"
}

variable "pods_secondary_cidr" {
  description = "Secondary IPv4 CIDR block for GKE VPC-native alias IP Pods."
  type        = string
  default     = "10.42.128.0/17"
}

variable "services_secondary_cidr" {
  description = "Secondary IPv4 CIDR block for GKE Services."
  type        = string
  default     = "10.43.0.0/20"
}

variable "proxy_subnet_cidr" {
  description = "IPv4 CIDR block for the Regional External Application Load Balancer proxy-only subnet."
  type        = string
  default     = "10.42.16.0/23"
}

variable "master_ipv4_cidr_block" {
  description = "Private /28 CIDR block for the GKE control plane."
  type        = string
  default     = "172.16.0.32/28"
}

variable "authorized_ipv4_cidr" {
  description = "Authorized public IPv4 CIDR (/32) allowed to reach the GKE IP endpoint."
  type        = string

  validation {
    condition     = var.authorized_ipv4_cidr != "0.0.0.0/0"
    error_message = "authorized_ipv4_cidr must not be 0.0.0.0/0; restrict access to a specific CIDR."
  }
}

variable "node_machine_type" {
  description = "Compute Engine machine type for GKE worker nodes."
  type        = string
  default     = "e2-standard-2"
}

variable "node_count" {
  description = "Number of worker nodes in the primary node pool."
  type        = number
  default     = 2
}

variable "node_disk_size_gb" {
  description = "Boot disk size in GB for each GKE worker node."
  type        = number
  default     = 50
}

variable "artifact_registry_repo_id" {
  description = "Artifact Registry Docker repository ID."
  type        = string
  default     = "gke-ephemeral-env"
}

variable "github_repository" {
  description = "GitHub owner/repository allowed to authenticate via Workload Identity Federation."
  type        = string
  default     = "devenes/gke-ephemeral-environment-platform"
}
