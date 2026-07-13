# meshcentral Role

Runs MeshCentral as a rootful Podman container managed by systemd. The service
is placed on `rpi4`, outside the Proxmox and Kubernetes failure domains that it
is intended to recover.

During cutover it listens on `192.168.10.241:8443`; production DNS and Caddy
must not be switched until the fresh standalone instance has been configured
and verified. There is no Kubernetes state to migrate: the previous prd move
lost the MeshCentral configuration, so devices are registered again after this
deployment.

## Persistent data

The four directories below mirror the mounts used by the former Kubernetes
Deployment:

| Host path | Container path |
|---|---|
| `/opt/meshcentral/meshcentral-data` | `/opt/meshcentral/meshcentral-data` |
| `/opt/meshcentral/meshcentral-files` | `/opt/meshcentral/meshcentral-files` |
| `/opt/meshcentral/meshcentral-backups` | `/opt/meshcentral/meshcentral-backups` |
| `/opt/meshcentral/meshcentral-web` | `/opt/meshcentral/meshcentral-web` |

The container is limited to 1 GiB RAM and one CPU so that a failure cannot
starve Kea DHCP on the same recovery host.

## Deployment

Preview the host changes first:

```bash
ansible-playbook playbooks/meshcentral.yaml --check --diff
```

The user performs the actual deployment:

```bash
ansible-playbook playbooks/meshcentral.yaml
```

Configure the administrator and device groups on the fresh instance, then
verify it directly before the ingress cutover:

```bash
curl --header 'Host: meshcentral.home.butaco.net' \
  http://192.168.10.241:8443/
```

Port 8443 is intentionally plain HTTP because MeshCentral runs with
`TLS_OFFLOAD=true`; Caddy terminates TLS after cutover. Do not expose this port
outside the trusted LAN.

The canonical hostname is `meshcentral.home.butaco.net`. It is outside the
Kubernetes environment namespace and is managed by the existing Caddy and
DNSControl home-zone workflow.
