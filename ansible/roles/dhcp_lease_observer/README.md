# DHCP Lease Observer Role

Deploys the immutable `dhcp-lease-observer` release artifact as a hardened
systemd oneshot service on the `dhcp_lease_observer` inventory group.

The role verifies the pinned archive SHA-256, installs the release under a
versioned `/opt` directory, renders strict JSON configuration, and supplies the
IX2106 password and device identity key through systemd credentials. It writes
last-good state under `/var/lib/dhcp-lease-observer` and Prometheus metrics to
the node_exporter textfile directory.

Run `playbooks/common-node_exporter.yaml` first so the shared textfile writer
group and directory exist. The observer user joins that group; this role does
not change ownership of the shared directory.

The timer is disabled by default. After deployment, verify `--version`,
`--check-config`, one manual service invocation, state, metrics, and structured
journal delivery before enabling `dhcp_lease_observer_timer_enabled`.

Secrets are required from SOPS inventory:

- `dhcp_lease_observer_ix2106_password`
- `dhcp_lease_observer_identity_key`
- `dhcp_lease_observer_ix2106_host_key_sha256`

Never place these values in role defaults, non-secret group variables, process
arguments, or logs.
