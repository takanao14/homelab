# proxmox_metric_server

Manages Proxmox VE external metric server definitions with `pvesh`.

The definitions are cluster-scoped and stored by Proxmox in
`/etc/pve/status.cfg`, so this role should run on one representative host per
Proxmox cluster. For standalone Proxmox installations, list each standalone host
because each host owns its own `status.cfg`. It supports Ansible check mode:
reads still execute, while create/update/delete operations are reported without
changing Proxmox.

## Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `proxmox_metric_server_manage_hosts` | `[]` | Inventory hosts allowed to manage cluster-scoped metric servers |
| `proxmox_metric_server_definitions` | `[]` | Desired metric server definitions |

Each `proxmox_metric_server_definitions` item uses Proxmox API option names directly:

```yaml
proxmox_metric_server_manage_hosts:
  - pve
  - node1
  - node2
  - node3
  - node4

proxmox_metric_server_definitions:
  - id: alloy-otlp
    type: opentelemetry
    server: "{{ alloy_otlp_server }}"
    port: 4318
    otel-protocol: http
    otel-path: /v1/metrics
    otel-compression: gzip
    state: present
```

Avoid putting credentials or authorization headers in plaintext. If
`otel-headers` is ever needed, store it in SOPS-managed vars.
