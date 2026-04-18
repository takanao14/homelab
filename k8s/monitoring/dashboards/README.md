# Grafana Dashboards

Grafana dashboards are defined as Go code using [grafana-foundation-sdk](https://github.com/grafana/grafana-foundation-sdk) and generated as JSON.

## Dashboards

| Name | Description |
|---|---|
| `node-overview` | CPU, memory, temperature, disk I/O, network I/O (including ZFS ARC) |
| `proxmox-overview` | Proxmox VE cluster: VM/LXC counts, node and guest resources, storage |
| `gpu-overview` | AMD RX 9060 XT: activity, VRAM, temperature, power |
| `dns-overview` | dnsdist + pdns-auth: QPS, cache hit rate, latency, response codes |
| `network-overview` | SNMP MIB-II: traffic, errors, discards, interface status |
| `uptime` | ICMP/DNS probe availability timeline |

## Structure

```
.
├── cmd/generate/main.go          # Dashboard definitions (Go)
├── generated/                    # Generated JSON output (git-ignored)
├── provisioning/                 # Local Grafana provisioning config
│   ├── datasources/              # Prometheus datasource
│   └── dashboards/               # Dashboard file provider
├── docker-compose.yml            # Local development Grafana
└── Makefile
```

To add a new dashboard, add an entry to the `dashboards` map in `cmd/generate/main.go`.
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

Edit `cmd/generate/main.go`, then re-run `make dev` to reload.

### Stop

```bash
make dev-stop
```

## Production

`make deploy` generates JSON and copies it to `charts/prometheus/dashboards/`. Commit the diff and ArgoCD will sync the updated ConfigMaps to the cluster.

```
Edit cmd/generate/main.go
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
