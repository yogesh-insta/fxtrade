output "vm_external_ip" {
  description = "External IP of the fxtrade VM"
  value       = google_compute_instance.vm.network_interface[0].access_config[0].nat_ip
}

output "vm_name" {
  description = "Compute Engine instance name"
  value       = google_compute_instance.vm.name
}

output "zone" {
  description = "Zone where the VM runs"
  value       = google_compute_instance.vm.zone
}

output "service_account_email" {
  description = "Service account attached to the VM"
  value       = google_service_account.vm.email
}

output "secret_id" {
  description = "Secret Manager secret ID for fxtrade credentials"
  value       = google_secret_manager_secret.credentials.secret_id
}
