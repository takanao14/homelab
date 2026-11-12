# Homepage

[Homepage](https://gethomepage.dev/) dashboard for prd, managed by Argo CD.

## Directory Structure

```
homepage/
├── prd/values.yaml        # hostname
└── chart/                 # Custom Helm chart
    ├── Chart.yaml
    ├── values.yaml        # Kubernetes deployment values (hostname, image, Gateway)
    ├── config/            # Homepage YAML configs mounted as-is
    │   ├── settings.yaml
    │   ├── services.yaml
    │   ├── widgets.yaml
    │   ├── bookmarks.yaml
    │   ├── kubernetes.yaml
    │   └── proxmox.yaml
    └── templates/
        ├── secret-config.yaml  # Mounts config/*.yaml as a Secret
        ├── external-secret.yaml # ESO ExternalSecret for API credentials
        ├── deployment.yaml
        ├── service.yaml        # ClusterIP
        ├── httproute.yaml      # HTTPRoute → shared-gateway-envoy
        └── rbac.yaml
```

## Access

An HTTPRoute uses the environment hostname and listener.

## Configuration

Keep application configuration in native Homepage YAML to allow edits
without Helm template syntax. `secret-config.yaml` packages `chart/config/`
verbatim as `homepage-config` for mounting. Helm and environment differences
belong in values files.

## Secrets

Use `HOMEPAGE_VAR_*` references to keep API credentials out of committed
Homepage YAML; ESO supplies their values from OpenBao.

OpenBao KV paths:

| OpenBao path | Property | Description |
|-------------|----------|-------------|
| `k8s/homepage/proxmox` | `{host}-token` | API token ID (`pve`, `node1`–`node5`; `user@pam!tokenname`) |
| `k8s/homepage/proxmox` | `{host}-secret` | Matching API token secret |
| `k8s/homepage/truenas` | `key` | TrueNAS API key |
| `k8s/monitoring/grafana` | `adminPassword` | `HOMEPAGE_VAR_GRAFANA_PRD_PASSWORD` |
| `k8s-sandbox/grafana` | `adminPassword` | `HOMEPAGE_VAR_GRAFANA_SANDBOX_PASSWORD` |

The Grafana widgets read the same environment-specific secrets as Grafana.
Apply the Homepage policy in `ansible/playbooks/ops/openbao/configure.yaml`
before deploying these references. Restart Homepage after ESO updates
`homepage-secrets`; its environment variables are read at process startup.
