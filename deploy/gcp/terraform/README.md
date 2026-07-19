# fxtrade GCP infrastructure (Terraform)

Single-stack Terraform for an **infra-only** e2-micro VM (service account, Secret Manager shell, firewall). Apply does **not** attach a startup script and does **not** auto-install or enable fxtrade bots.

**Project ID (default):** `fxtrade-prod-12345`  
**VM:** `fxtrade-vm` in `us-east1-b`

## Prerequisites

- [Terraform](https://developer.hashicorp.com/terraform/install) >= 1.0
- [gcloud CLI](https://cloud.google.com/sdk/docs/install) authenticated with permission to manage Compute, IAM, and Secret Manager
- Billing enabled on the GCP project

```bash
gcloud auth application-default login
gcloud config set project fxtrade-prod-12345
```

## Quick start

```bash
cd deploy/gcp/terraform
cp terraform.tfvars.example terraform.tfvars
# Edit terraform.tfvars — set admin_cidr and ssh_public_keys at minimum

terraform init
terraform plan
terraform apply
```

After apply, note the outputs (especially `vm_external_ip`) for GitHub Actions secrets.

## What this stack creates

| Resource | Name / notes |
|----------|----------------|
| APIs | `compute`, `secretmanager`, `iam` |
| Service account | `fxtrade-vm@PROJECT.iam.gserviceaccount.com` |
| Secret Manager | `fxtrade-credentials` (empty shell unless you set `credentials_secret_data`) |
| IAM | SA → `secretAccessor` on that secret only |
| VM | `fxtrade-vm`, e2-micro, Ubuntu 22.04, 10 GB, tag `fxtrade` |
| Firewall | SSH (port 22); optional health port |
| Startup script | **Not attached** — `startup.sh.tpl` is kept in-repo only; apply leaves a blank VM for Tradex (or manual fxtrade) install |

Install apps after SSH: Tradex via `tradex/deploy/gcp/install.sh`, or fxtrade via `deploy/gcp/install.sh` (see [../DEPLOY.md](../DEPLOY.md)).

## Import existing resources

If you already created the VM or secret manually, import before `terraform apply` so Terraform adopts them instead of failing on duplicate names.

```bash
cd deploy/gcp/terraform
terraform init

# APIs and IAM are usually safe to create fresh; import the VM if it exists:
terraform import google_compute_instance.vm fxtrade-prod-12345/us-east1-b/fxtrade-vm

# Secret (if already created):
terraform import google_secret_manager_secret.credentials projects/fxtrade-prod-12345/secrets/fxtrade-credentials

# Service account (if already created):
terraform import google_service_account.vm projects/fxtrade-prod-12345/serviceAccounts/fxtrade-vm@fxtrade-prod-12345.iam.gserviceaccount.com

# Firewall rules (if names match):
terraform import google_compute_firewall.ssh projects/fxtrade-prod-12345/global/firewalls/fxtrade-allow-ssh
```

Run `terraform plan` after imports. Expect drift on metadata (e.g. old `startup-script`) and attached service account until you apply once — apply will clear startup-script if present.

To attach an existing VM to the new service account, `terraform apply` may stop/replace the instance metadata — review the plan carefully.

## Upload credentials (Secret Manager)

**Do not commit `.credentials` or put real JSON in git.**

Recommended — from your laptop:

```bash
gcloud secrets versions add fxtrade-credentials \
  --project=fxtrade-prod-12345 \
  --data-file=.credentials
```

Alternative — one-time via Terraform (local only, never commit `terraform.tfvars` with this set):

```hcl
credentials_secret_data = file("/path/to/.credentials")
```

Then on the VM (after a manual install has placed `/opt/fxtrade`):

```bash
sudo GCP_PROJECT=fxtrade-prod-12345 /opt/fxtrade/deploy/gcp/fetch-credentials.sh fxtrade-credentials
sudo systemctl restart fxtrade@universe_scanner.service
```

## GitHub Actions secrets

After `terraform apply`:

```bash
terraform output vm_external_ip
terraform output -raw vm_external_ip   # copy-friendly
```

Configure in **GitHub → Settings → Secrets and variables → Actions**:

| Secret | Value |
|--------|--------|
| `GCP_VM_HOST` | `terraform output -raw vm_external_ip` |
| `GCP_VM_USER` | SSH username (match `ssh_public_keys` prefix, e.g. `deploy`) |
| `GCP_SSH_KEY` | Private key matching the public key on the VM |

Optional: `GCP_VM_PATH` (default `/opt/fxtrade/bin/fxtrade`).

Deploy workflow: `.github/workflows/deploy.yml` — pushes binary via SSH on `main`.

## Post-apply checklist

1. SSH to the VM (`terraform output` / `gcloud compute ssh`)
2. Install the app you want (Tradex: `tradex/deploy/gcp/install.sh`; fxtrade bots: [../DEPLOY.md](../DEPLOY.md) §2)
3. Upload secret version if using Secret Manager (above)
4. Deploy binary via CI or manual `scp` as needed
5. Verify services/health for that app

## Variables reference

| Variable | Default | Description |
|----------|---------|-------------|
| `project_id` | `fxtrade-prod-12345` | GCP project |
| `zone` | `us-east1-b` | VM zone |
| `admin_cidr` | `0.0.0.0/0` | SSH source CIDR — **restrict in production** |
| `ssh_public_keys` | `""` | `user:ssh-ed25519 AAAA...` metadata |
| `enable_health_firewall` | `false` | Open health port externally |
| `health_port` | `8080` | Health TCP port when firewall enabled |
| `credentials_secret_data` | `null` | Sensitive; prefer `gcloud secrets versions add` |

See `terraform.tfvars.example` for a full template.

## Files (gitignored)

- `.terraform/`
- `*.tfstate`, `*.tfstate.*`
- `terraform.tfvars` (use `terraform.tfvars.example` as template)

## Destroy

```bash
terraform destroy
```

This removes the VM, firewall rules, secret **metadata** (not necessarily all secret versions if retention policies apply), and the service account. Back up `.credentials` and state before destroying.
