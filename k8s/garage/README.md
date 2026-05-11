# garage

Garage S3-compatible object storage deployed on the dev cluster.

## Access

| Endpoint | URL | Purpose |
|----------|-----|---------|
| S3 API (HTTPS) | `https://s3.dev.butaco.net` | mc / AWS CLI / applications |
| Public HTTP | `http://192.168.20.250/firmware/<file>` | Router firmware download |
| Public HTTP | `http://192.168.20.250/cloud-images/<file>` | VM cloud image download |

## Initial Setup (one-time)

Required after first deployment or after Pod is recreated.

Run the Ansible playbook from the repo root:

```bash
cd ansible
ansible-playbook playbooks/garage_init.yaml
```

To target a specific kubectl context:

```bash
ansible-playbook playbooks/garage_init.yaml -e kubectl_context=dev-homelab
```

The playbook is idempotent and handles: cluster layout, S3 key import, bucket creation, and public web access configuration.

## Secrets (OpenBao)

Stored at `k8s/garage/s3` with the following properties:

| Property | Description |
|----------|-------------|
| `access-key` | S3 access key ID |
| `secret-key` | S3 secret key |
| `rpc-secret` | Cluster RPC secret (`openssl rand -hex 32`) |
