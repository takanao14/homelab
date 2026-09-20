# knot_resolver

Installs Knot Resolver 6 from the official CZ.NIC Labs repository and configures
it as a private caching resolver. It performs full-service recursion and local
DNSSEC validation by default, and can use authenticated forwarding during an
external DNS reachability incident.

The role:

- listens on the host's fixed service address on port 53;
- permits recursion only from the two dnsdist hosts;
- keeps the management API on its package-default Unix socket;
- enables local DNSSEC validation in recursive mode and keeps a persistent
  cache (`knot_resolver_cache_size`, 256 MB);
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

`knot-resolver-metrics.timer` runs every 15 seconds. It reads
`/metrics/prometheus` from the management Unix socket and atomically replaces
`/var/lib/prometheus/node-exporter/knot_resolver.prom`; the last good
file survives a failed collection. The metrics service is ordered after Knot
Resolver when both start together, but does not require or restart the resolver;
an intentional resolver stop must remain stopped for failover testing.

The 15s collection interval keeps counters fresh for each 30s Prometheus scrape.
The collector queries the socket directly to avoid loading the kresctl CLI on
every run; use `kresctl metrics --prometheus` for manual inspection.

## Required variables

```yaml
knot_resolver_server_addr: "192.0.2.53"
knot_resolver_allowed_clients:
  - "192.0.2.10/32"
  - "192.0.2.11/32"
```

Set `knot_resolver_mode` to `emergency_forward` to forward cache misses over
authenticated DNS-over-TLS while retaining the local persistent cache:

```yaml
knot_resolver_mode: emergency_forward
knot_resolver_forwarders:
  - address:
      - 8.8.8.8
      - 8.8.4.4
    transport: tls
    hostname: dns.google
```

Set `knot_resolver_forward_dnssec: false` only when an authenticated validating
upstream is responsible for DNSSEC validation.

Return to full-service recursion by setting `knot_resolver_mode: recursive`.
The serial resolver playbook detects a transition from emergency forwarding,
reloads the recursive configuration, and clears that resolver's cache before
continuing to the peer. This prevents answers accepted under the upstream's
validation policy from remaining available in the locally validating mode.

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
