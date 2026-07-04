locals {
  apis = [
    "compute.googleapis.com",
    "secretmanager.googleapis.com",
    "iam.googleapis.com",
  ]

  startup_script = templatefile("${path.module}/startup.sh.tpl", {
    project_id  = var.project_id
    secret_name = var.credentials_secret_id
    github_repo = var.github_repo_url
  })
}

resource "google_project_service" "apis" {
  for_each = toset(local.apis)

  project            = var.project_id
  service            = each.value
  disable_on_destroy = false
}

resource "google_service_account" "vm" {
  account_id   = "fxtrade-vm"
  display_name = "fxtrade VM runtime"
  project      = var.project_id

  depends_on = [google_project_service.apis]
}

resource "google_secret_manager_secret" "credentials" {
  secret_id = var.credentials_secret_id
  project   = var.project_id

  replication {
    auto {}
  }

  depends_on = [google_project_service.apis]
}

resource "google_secret_manager_secret_version" "credentials" {
  count = var.credentials_secret_data != null ? 1 : 0

  secret      = google_secret_manager_secret.credentials.id
  secret_data = var.credentials_secret_data
}

resource "google_secret_manager_secret_iam_member" "vm_accessor" {
  project   = var.project_id
  secret_id = google_secret_manager_secret.credentials.secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.vm.email}"
}

resource "google_compute_firewall" "ssh" {
  name    = "fxtrade-allow-ssh"
  project = var.project_id
  network = "default"

  description = "SSH to fxtrade VM (tag: ${var.network_tag})"

  allow {
    protocol = "tcp"
    ports    = ["22"]
  }

  source_ranges = [var.admin_cidr]
  target_tags   = [var.network_tag]

  depends_on = [google_project_service.apis]
}

resource "google_compute_firewall" "health" {
  count = var.enable_health_firewall ? 1 : 0

  name    = "fxtrade-health"
  project = var.project_id
  network = "default"

  description = "Optional external health endpoint for fxtrade"

  allow {
    protocol = "tcp"
    ports    = [tostring(var.health_port)]
  }

  source_ranges = [var.health_source_cidr]
  target_tags   = [var.network_tag]

  depends_on = [google_project_service.apis]
}

resource "google_compute_instance" "vm" {
  name         = var.vm_name
  project      = var.project_id
  zone         = var.zone
  machine_type = var.machine_type
  tags         = [var.network_tag]

  boot_disk {
    initialize_params {
      image = "ubuntu-os-cloud/ubuntu-2204-lts"
      size  = var.boot_disk_size_gb
      type  = "pd-standard"
    }
  }

  network_interface {
    network = "default"
    access_config {}
  }

  service_account {
    email  = google_service_account.vm.email
    scopes = ["cloud-platform"]
  }

  metadata = merge(
    {
      startup-script = local.startup_script
    },
    var.ssh_public_keys != "" ? { ssh-keys = var.ssh_public_keys } : {}
  )

  allow_stopping_for_update = true

  depends_on = [
    google_project_service.apis,
    google_secret_manager_secret_iam_member.vm_accessor,
  ]

  lifecycle {
    ignore_changes = [
      metadata["ssh-keys"],
    ]
  }
}
