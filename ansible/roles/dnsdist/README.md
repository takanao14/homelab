# dnsdist Role

Installs and configures dnsdist from the official PowerDNS repository on Debian-based systems.

Sets up dnsdist as a DNS load balancer and forwarder: internal domain queries are routed to the PowerDNS Authoritative secondary backends, and external queries go to public resolvers.

## Architecture

dnsdist sits in front of the authoritative DNS servers:

- **Internal forward and reverse zones** (`home.butaco.net.`, `prd.butaco.net.`, `sandbox.butaco.net.`, `168.192.in-addr.arpa.`) → routed to the `internal` pool (PowerDNS secondaries only — the hidden primary is excluded). The single reverse suffix covers every `192.168.x.0/24` segment: the management LAN and each per-node Proxmox SimpleZone.
- **Locally served reverse zones** (`10.in-addr.arpa.`, `254.169.in-addr.arpa.`, `127.in-addr.arpa.`, `16.172.` … `31.172.in-addr.arpa.`) → answered by dnsdist itself with NXDOMAIN, never forwarded (RFC 6303)
- **External domains** → routed to the default pool (public resolvers)

The `internal` pool is built dynamically from `secondary_auth_servers`, so adding or removing a secondary only requires updating that list.

### Why RFC1918 reverse queries are answered locally

The public resolvers in the default pool cannot hold RFC1918 PTR data: those queries reach AS112 and come back NXDOMAIN, after the upstream has seen the internal addressing. Answering locally returns the same NXDOMAIN clients already received, minus the round trip and the disclosure.

This is not only about queries from the LAN. The clusters' CoreDNS runs with `fallthrough in-addr.arpa`, so a reverse lookup for a pod address (Cilium's pool is `10.0.0.0/8`) that its `kubernetes` plugin cannot answer is forwarded here. Service addresses are answered authoritatively by CoreDNS and never arrive.

`dnsdist_internal_domains` and `dnsdist_local_reverse_zones` must stay disjoint — the first routes to pdns, the second is answered locally, and a zone cannot be both. The role asserts this before templating.

`168.192.in-addr.arpa.` belongs to the first list: pdns is authoritative for that whole `/16` as one aggregated zone, so undeclared addresses inside it answer an authoritative NXDOMAIN rather than REFUSED. IPv6 zones are omitted because the network has no IPv6.

## Functionality

- Adds the official PowerDNS APT repository for `dnsdist`.
- Installs the `dnsdist` package.
- Deploys `/etc/dnsdist/dnsdist.yml`, removing any superseded `dnsdist.conf`.
- Enables and starts the `dnsdist` service.

### Configuration format

The configuration is YAML (`/etc/dnsdist/dnsdist.yml`), which dnsdist has
preferred over `dnsdist.conf` since 2.1.

`templates/dnsdist.yml.j2` contains no hand-written YAML. The whole
configuration is the `dnsdist_config` structure in `defaults/main.yaml`, and the
template only renders it through `to_nice_yaml`, so indentation cannot drift and
the rules are reviewable as data. Edit `dnsdist_config`, never the template.

Two non-obvious details in that template and task:

- The render goes through `to_json | from_json` first. `map('combine')` merges
  one literal dict object into every backend, so PyYAML would otherwise emit
  anchors and aliases (`&id001` / `*id001`). The round-trip rebuilds fresh
  objects and keeps the on-host file plain.
- `validate` copies to a `.yml` temp name before calling
  `dnsdist --check-config`. dnsdist picks the config format from the file
  extension, and Ansible hands `validate` an extension-less temp file, which
  would be parsed as Lua and fail.

Note that `--check-config` validates syntax only. A YAML config can load
cleanly and still route differently; changes need behavioural verification.
`dnsdist -c -e "showRules()"` is the most direct check — it prints every rule
with its match count, so two hosts can be compared rule by rule.

**Trust the parser over the YAML reference.** During the migration from Lua the
published reference was wrong twice: it documents a default for the backend
`protocol` field that the binary actually requires, and it shows selector types
hyphen-lowercased (`qname-suffix`) when only the PascalCase enum variant
(`QNameSuffix`) is accepted. When a type or field name is wrong, the parse error
enumerates every valid alternative — that list is authoritative.

#### Rule order is load-bearing

`query_rules` is a list and its order is preserved through rendering:

1. `internal-no-recurse` — non-terminal, falls through to 2
2. `internal-pool` — `stop_processing: true`, the routing decision is final
3. `rfc6303-local-reverse` — terminal, these never reach a backend
4. `dnstap-queries` — last

Rules 2 and 3 are both terminal, so **dnstap only sees queries that reach the
default pool**. Neither internal-zone queries nor locally answered RFC 6303
NXDOMAINs are logged to dnscollector. `showRules()` confirms this on both the
Lua and YAML configurations — rule 4's match count is lower than rule 1's by
exactly the number of internal queries.

That is inherited behaviour, not a design decision made here. If dnscollector
should see the internal traffic, move rule 4 to the front deliberately rather
than letting an edit change it as a side effect.

## Variables

### Secrets (from SOPS-encrypted `group_vars/dnsdist.sops.yaml`)

Loaded via `community.sops.sops` lookup.

| Variable | Description |
|----------|-------------|
| `DNSDIST_WEB_PASSWORD` | Web UI password |
| `DNSDIST_WEB_API_KEY` | Web UI API key |
| `DNSDIST_CONSOLE_KEY` | Console key (generate with `dnsdist --gen-key`) |

These are mapped to `dnsdist_web_password`, `dnsdist_web_api_key`, and `dnsdist_console_key` in `defaults/main.yaml`.

### Backend server lists (from `group_vars/dns.yaml` and `group_vars/dnsdist.yaml`)

| Variable | Description |
|----------|-------------|
| `secondary_auth_servers` | List of `{name, address}` dicts for all secondary auth servers (single source of truth for the `internal` pool) |
| `dnsdist_default_servers` | List of `{name, address, health_checks}` dicts for public resolvers (defined in `group_vars/dnsdist.yaml`) |

`dnsdist_internal_backends` and `dnsdist_default_backends` are derived automatically in `defaults/main.yaml` by merging the above lists with a `pools` sequence. Internal backends also get a shared `health_checks` mapping built from `dnsdist_internal_check_name`; the public resolvers carry their own, because they do not all answer for the same probe name.

### Role defaults (in `defaults/main.yaml`)

| Variable | Default | Description |
|----------|---------|-------------|
| `dnsdist_server_addr` | Host primary IPv4 | IP address dnsdist binds to (frontend + web UI) |
| `dnsdist_mgmt_acl` | `[127.0.0.1/32, 192.168.0.0/16, 10.0.0.0/8]` | ACL for web UI access. A list, not a comma-separated string — the YAML config takes a sequence |
| `dnsdist_console_acl` | `[127.0.0.1/32]` | ACL for console access (list) |
| `dnsdist_internal_domains` | `home.butaco.net.`, `prd.butaco.net.`, `sandbox.butaco.net.`, `168.192.in-addr.arpa.` | Forward and reverse zones routed to the `internal` pool (suffix match, so one reverse entry covers all `192.168.x.0/24`) |
| `dnsdist_local_reverse_zones` | `10.`, `254.169.`, `127.`, `16.172.` … `31.172.in-addr.arpa.` | Reverse zones answered locally with NXDOMAIN instead of being forwarded (RFC 6303). Must be disjoint from `dnsdist_internal_domains` |
| `dnsdist_internal_check_name` | `ns1.home.butaco.net.` | Health check FQDN for internal backends |
| `dnsdist_packet_cache_size` | `10000` | Packet cache entry limit (default pool only) |
| `dnsdist_packet_cache_max_ttl` | `86400` | Maximum TTL for cached entries (seconds) |
| `dnsdist_repo_channel` | `dnsdist-21` | PowerDNS APT repository channel (release train, not a version pin) |
| `dnsdist_repo_release` | `{{ ansible_facts['distribution_release'] }}` | Ubuntu release codename |

## Dependencies

None.

## Usage

```yaml
# playbooks/dnsdist.yaml
- name: Setup dnsdist
  hosts: dnsdist
  roles:
    - role: dnsdist
```
