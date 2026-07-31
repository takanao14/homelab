# knot_resolver

Installs Knot Resolver 6 from the official CZ.NIC Labs repository and configures
it as a private full-service recursive resolver.

The role:

- listens on the host's fixed service address on port 53;
- permits recursion only from the two dnsdist hosts;
- keeps the management API on its package-default Unix socket;
- enables DNSSEC validation and a persistent 100 MB cache;
- exports Prometheus metrics through the node_exporter textfile collector.

The repository signing key is downloaded with a pinned SHA-256 checksum. The
expected upstream fingerprint is:

```text
9C71 D59C D4CE 8BD2 966A 7A3E AB6A 3031 2401 9B64
```

Update the checksum and verification date in `defaults/main.yaml` only after
checking both the replacement key and fingerprint through an independent
official CZ.NIC source.

The role follows the current Knot Resolver 6 package from the repository with
APT `state: present`; it does not pin a minor or patch version. Package
candidate drift is reported by `playbooks/ops-version_audit.yaml`.

Configuration is stored at `/etc/knot-resolver/config.yaml`. Candidate and
installed files are validated with `kresctl validate`, and valid changes are
applied with the package-supported systemd reload. The package-created account
and primary group are resolved at runtime before the configuration is written.

`knot-resolver-metrics.timer` runs once per minute. It obtains Prometheus output
through the management Unix socket and atomically replaces
`/var/lib/node_exporter/textfile_collector/knot_resolver.prom`; the last good
file survives a failed collection. The metrics service is ordered after Knot
Resolver when both start together, but does not require or restart the resolver;
an intentional resolver stop must remain stopped for failover testing.

## Required variables

```yaml
knot_resolver_server_addr: "192.0.2.53"
knot_resolver_allowed_clients:
  - "192.0.2.10/32"
  - "192.0.2.11/32"
```

Run the homelab playbook in check mode before provisioning:

```bash
ansible-playbook -i inventories/homelab playbooks/knot-resolver.yaml --check --diff
```

On a pristine host, package-dependent checks are intentionally deferred because
the package is not installed during Ansible check mode.

Provision or update one resolver at a time:

```bash
ansible-playbook -i inventories/homelab playbooks/knot-resolver.yaml --limit resolver1
ansible-playbook -i inventories/homelab playbooks/knot-resolver.yaml --limit resolver2
```

Useful read-only diagnostics:

```bash
kresctl validate /etc/knot-resolver/config.yaml
kresctl metrics --prometheus
systemctl status knot-resolver knot-resolver-metrics.timer
journalctl -u knot-resolver --since today
dig @192.0.2.53 dnssec.works A +dnssec
```
