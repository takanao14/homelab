# smartctl-exporter-external

Scrapes TrueNAS [smartctl_exporter](https://github.com/prometheus-community/smartctl_exporter),
the only layer that can see its passed-through SATA disks.

## TrueNAS-side deployment

The TrueNAS UI owns this Custom App; this is its reference definition.

Apps → Discover Apps → ⋮ → Install via YAML:

```yaml
services:
  smartctl-exporter:
    image: prometheuscommunity/smartctl-exporter:v0.14.0
    privileged: true # smartctl needs raw device access
    user: "0"
    command:
      # Exclude the virtual boot disk; labels omit the /dev/ prefix.
      - "--smartctl.device-exclude=^sda$"
    ports:
      - "9633:9633"
    volumes:
      - /dev:/dev:ro
    restart: unless-stopped
```

Notes:

- Standby disks are not woken; their series stay stale until wake-up.
- The image tag lives in the TrueNAS UI and is outside Renovate's reach;
  updates are manual.
- Verify after (re)deploying:
  `curl http://192.168.20.10:9633/metrics | grep smartctl_device_smart_status`
  should list the two ST6000 disks.

## Consumers

- `disk-health` dashboard
- Shared hardware disk-health and temperature rules
