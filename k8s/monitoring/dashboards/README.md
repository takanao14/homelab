# Grafana Dashboards

Grafana dashboards are defined as Go code using [grafana-foundation-sdk](https://github.com/grafana/grafana-foundation-sdk) and generated as JSON.

## Dashboards

| Name | Description |
|---|---|
| `node-overview` | Bare-metal nodes: CPU, memory, temperature, disk I/O, network I/O, ZFS ARC |
| `k8s-node-overview` | Kubernetes nodes: CPU, memory, disk, network (filtered by cluster/node) |
| `kubernetes-overview` | Kubernetes cluster health, resource usage, pod lifecycle, network, PVC |
| `proxmox-overview` | Proxmox VE cluster: VM/LXC counts, node and guest resources, storage |
| `gpu-overview` | AMD RX 9060 XT: activity, VRAM, temperature, power, clock speed |
| `dns-overview` | dnsdist + pdns-auth: QPS, cache hit rate, latency, response codes, drop rate |
| `dns-logs` | DNS query logs via Loki: query rate, response codes, top domains, per-host breakdown |
| `network-overview` | SNMP MIB-II (bgw1/c1200): traffic, errors, discards, interface status |
| `monitoring-overview` | Prometheus and Loki self-monitoring: scrape targets, TSDB, ingestion rate |
| `syslog` | Syslog log volume and error rate via Loki |
| `uptime` | ICMP/DNS probe availability timeline |

## Structure

```
.
├── cmd/generate/               # Dashboard definitions (Go)
│   ├── main.go                 # Entrypoint and common functions
│   ├── node.go                 # node-overview
│   ├── k8s_node.go             # k8s-node-overview
│   ├── kubernetes.go           # kubernetes-overview
│   ├── proxmox.go              # proxmox-overview
│   ├── gpu.go                  # gpu-overview
│   ├── dns.go                  # dns-overview
│   ├── dns_logs.go             # dns-logs
│   ├── network.go              # network-overview
│   ├── monitoring.go           # monitoring-overview
│   ├── syslog.go               # syslog
│   └── uptime.go               # uptime
├── generated/                  # Generated JSON output (git-ignored)
├── provisioning/               # Local Grafana provisioning config
│   ├── datasources/            # Prometheus datasource
│   └── dashboards/             # Dashboard file provider
├── docker-compose.yml          # Local development Grafana
└── Makefile
```

To add a new dashboard, create a new `.go` file in `cmd/generate/` (e.g., `new_dashboard.go`) and add an entry to the `dashboards` map in `cmd/generate/main.go`.
The Helm template auto-discovers all JSON files in `charts/prometheus/dashboards/`, so no template changes are needed.

## Development

### Setup

Copy `.env.example` to `.env` and set your Prometheus URL:

```bash
cp .env.example .env
# Edit .env
```

### Start local Grafana

```bash
make dev
```

Opens at http://localhost:3000. Dashboards and the Prometheus datasource are provisioned automatically.

Edit `.go` files in `cmd/generate/`, then re-run `make dev` to reload.

### Stop

```bash
make dev-stop
```

## Production

`make deploy` generates JSON and copies it to `charts/prometheus/dashboards/`. Commit the diff and ArgoCD will sync the updated ConfigMaps to the cluster.

```
Edit .go files in cmd/generate/
  → make deploy  (generate JSON + copy to Helm chart)
  → git commit & push
  → ArgoCD syncs ConfigMaps
  → Grafana sidecar reloads dashboards
```

```bash
make deploy
git add ../charts/prometheus/dashboards/
git commit -m "..."
```
