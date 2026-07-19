# smartctl-exporter-external

ScrapeConfig for [smartctl_exporter](https://github.com/prometheus-community/smartctl_exporter)
instances running on hosts outside the cluster. Currently the only target is
TrueNAS SCALE: its SATA controller is passed through from the Proxmox host
(pve), so the ST6000 data disks are invisible to the host's node-exporter
smartmon collector and can only be read from inside the TrueNAS guest.

## TrueNAS-side deployment

The exporter runs as a TrueNAS SCALE Custom App (Docker). It is configured in
the TrueNAS UI, not managed from this repo — this section is the master copy
of that definition.

Apps → Discover Apps → ⋮ → Install via YAML:

```yaml
services:
  smartctl-exporter:
    image: prometheuscommunity/smartctl-exporter:v0.14.0
    privileged: true # smartctl needs raw device access
    user: "0"
    command:
      # The boot disk is a QEMU virtual drive without S.M.A.R.T. support;
      # only the passed-through SATA disks are worth scanning.
      - "--smartctl.device-exclude=^/dev/sda$"
    ports:
      - "9633:9633"
    volumes:
      - /dev:/dev:ro
    restart: unless-stopped
```

Notes:

- `--smartctl.powermode-check` defaults to `standby`, so polling never wakes
  a spun-down disk; a disk in standby is skipped and its series go stale
  until it wakes (visible as a nonzero `smartctl_device_smartctl_exit_status`).
- The image tag lives in the TrueNAS UI and is outside Renovate's reach;
  updates are manual.
- Verify after (re)deploying:
  `curl http://192.168.20.10:9633/metrics | grep smartctl_device_smart_status`
  should list the two ST6000 disks.

## Consumers

- `disk-health` dashboard: "TrueNAS (smartctl_exporter)" row plus the shared
  SMART Health / Summary / Temperature panels.
- `hardware-alerts` PrometheusRule: `SmartctlDiskUnhealthy` and the shared
  disk-temperature alert.
