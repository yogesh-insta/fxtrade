variable "project_id" {
  description = "GCP project ID"
  type        = string
  default     = "fxtrade-prod-12345"
}

variable "region" {
  description = "GCP region"
  type        = string
  default     = "us-east1"
}

variable "zone" {
  description = "GCP zone for the VM (free-tier eligible: us-east1-b)"
  type        = string
  default     = "us-east1-b"
}

variable "vm_name" {
  description = "Compute Engine instance name"
  type        = string
  default     = "fxtrade-vm"
}

variable "machine_type" {
  description = "VM machine type"
  type        = string
  default     = "e2-micro"
}

variable "boot_disk_size_gb" {
  description = "Boot disk size in GB"
  type        = number
  default     = 10
}

variable "network_tag" {
  description = "Network tag applied to the VM for firewall rules"
  type        = string
  default     = "fxtrade"
}

variable "admin_cidr" {
  description = "CIDR allowed for SSH (restrict to your IP/32 in production; 0.0.0.0/0 is open to the world)"
  type        = string
  default     = "0.0.0.0/0"
}

variable "enable_health_firewall" {
  description = "Create a firewall rule for the health check port"
  type        = bool
  default     = false
}

variable "health_port" {
  description = "TCP port for optional external health checks"
  type        = number
  default     = 8080
}

variable "health_source_cidr" {
  description = "CIDR allowed for health port when enable_health_firewall is true"
  type        = string
  default     = "0.0.0.0/0"
}

variable "ssh_public_keys" {
  description = "SSH public keys for instance metadata (user:keys format). Leave empty to use OS Login or add keys later."
  type        = string
  default     = ""
}

variable "github_repo_url" {
  description = "Git repository cloned on first boot"
  type        = string
  default     = "https://github.com/yogesh-insta/fxtrade.git"
}

variable "credentials_secret_id" {
  description = "Secret Manager secret ID for fxtrade .credentials JSON"
  type        = string
  default     = "fxtrade-credentials"
}

variable "credentials_secret_data" {
  description = "Optional .credentials JSON contents. Prefer `gcloud secrets versions add` instead of committing this value."
  type        = string
  sensitive   = true
  default     = null
}
