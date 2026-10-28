# meshcentral Role

MeshCentral runs on `rpi4` so it remains available to recover Proxmox and
Kubernetes during their outages. systemd manages its rootful Podman container
with `no-new-privileges`.

## Persistent data

Persistent container mounts:

| Host path | Container path |
|---|---|
| `/opt/meshcentral/meshcentral-data` | `/opt/meshcentral/meshcentral-data` |
| `/opt/meshcentral/meshcentral-files` | `/opt/meshcentral/meshcentral-files` |
| `/opt/meshcentral/meshcentral-backups` | `/opt/meshcentral/meshcentral-backups` |
| `/opt/meshcentral/meshcentral-web` | `/opt/meshcentral/meshcentral-web` |

The container has a 1 GiB RAM limit and a one-CPU limit to protect Kea DHCP on
the same recovery host from MeshCentral resource exhaustion.

The container uses the dedicated `homelab-meshcentral` AppArmor profile, based
on containers/common v0.57.4. It permits TERM/KILL from crun and self-signals
and ptrace read under this host's stacked crun/container label. The generated
default profile otherwise prevents shutdown and entrypoint log access.
The unit loads the dedicated profile before startup; Kea's confinement is unchanged.
Revalidate startup, TERM/KILL and restart after AppArmor/crun updates.

The role also keeps `settings.cert` in the generated `config.json` aligned with
`meshcentral_hostname`. The container's dynamic configuration is first-start
generation and does not update this persisted value after a hostname change;
a mismatch causes MeshCentral to reject the login WebSocket origin.

## Deployment

Preview the host changes first:

```bash
ansible-playbook playbooks/services/meshcentral.yaml --check --diff
```

The user performs the actual deployment:

```bash
ansible-playbook playbooks/services/meshcentral.yaml
```

Configure the administrator and device groups on the fresh instance, then
verify the backend directly:

```bash
curl --header 'Host: meshcentral.home.butaco.net' \
  http://192.168.10.241:8443/
```

Port 8443 is intentionally plain HTTP because MeshCentral runs with
`TLS_OFFLOAD=true`; Caddy terminates TLS. Do not expose this port
outside the trusted LAN.

The canonical hostname is `meshcentral.home.butaco.net`. It is outside the
Kubernetes environment namespace; manage it through the existing Caddy and
DNSControl home-zone workflow.

The container receives an `/etc/hosts` entry for the canonical hostname using
the `caddy1` inventory address. This lets MeshCentral retrieve the TLS-offload
certificate from Caddy even when the recovery host itself does not use the
internal `home.butaco.net` resolver.
