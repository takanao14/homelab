# dnsdist Role

Installs and configures dnsdist from the official PowerDNS repository on Debian-based systems.

Sets up dnsdist as a DNS load balancer and forwarder: internal domain queries
are routed to the PowerDNS Authoritative secondary backends, and external
queries go to dedicated Knot Resolver backends.

## Architecture

dnsdist sits in front of the authoritative DNS servers:

- **Internal forward and reverse zones** (`home.butaco.net.`, `prd.butaco.net.`, `sandbox.butaco.net.`, `168.192.in-addr.arpa.`) → routed to the `internal` pool (PowerDNS secondaries only — the hidden primary is excluded). The single reverse suffix covers every `192.168.x.0/24` segment: the management LAN and each per-node Proxmox SimpleZone.
- **Other domains, including RFC 6303 reverse zones** → routed to the default pool (resolver1/resolver2); Knot Resolver owns recursive and special-use-zone policy

The `internal` pool is built dynamically from `secondary_auth_servers`, so
adding or removing a secondary only requires updating that list. The default
pool is built from `dns_resolver_servers`; each dnsdist frontend assigns order
1 to its node-local resolver and order 2 to the peer, then uses
`firstAvailable`.

`168.192.in-addr.arpa.` remains an explicit internal-zone exception: pdns is
authoritative for that whole `/16` as one aggregated zone, so undeclared
addresses inside it answer an authoritative NXDOMAIN rather than REFUSED.

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

The config holds the console key and the webserver password, so it is deployed
`0640` rather than world readable. dnsdist reads it as its own service account,
not as root, so the owning group has to be that account's primary group. The
role resolves it at run time with two `getent` lookups (user, then group by GID)
instead of hard-coding a name — if the package renames or drops the account the
lookup fails before the config is rewritten, leaving the running service alone.

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

#### Reverting to Lua

Pointing the role back at a `dnsdist.conf` template is **not** enough on its own.
dnsdist 2.1 prefers `dnsdist.yml` and only falls back to `dnsdist.conf` when no
YAML file exists, so a leftover `/etc/dnsdist/dnsdist.yml` would keep running
while the `.conf` sat there looking authoritative. Any revert has to delete the
YAML file as well.

#### Rule order is load-bearing

`query_rules` is a list and its order is preserved through rendering:

1. `dnstap-queries` — `type: All`. `DnstapLogAction` has no `stop_processing`
   field and always falls through, so this runs first deliberately: it logs
   the query exactly as the client sent it, before rule 2 rewrites the RD bit.
2. `internal-no-recurse` — non-terminal, falls through to 3
3. `internal-pool` — `stop_processing: true`, the routing decision is final

`response_rules` is a **separate chain** that dnsdist evaluates on every
response regardless of what happened in `query_rules` — a `stop_processing`
query rule only stops the query-side chain, it does not skip response
processing for that query. Its own `dnstap-responses` rule (`type: All`)
therefore already logs internal-zone responses, complete with the real client
address in `network.query-ip` (dnsdist's own address is only the
`network.response-ip`).

This was verified against production Loki data (2026-08-05): before the
`dnstap-queries` reorder above, internal-zone qnames appeared in Loki only as
`CLIENT_RESPONSE` dnstap operations, never `CLIENT_QUERY`, while default-pool
qnames appeared as matched `CLIENT_QUERY`/`CLIENT_RESPONSE` pairs — confirming
the query and response chains are independent, and that internal-zone traffic
was already attributable to a real client before this change. The reorder
closes the remaining gap by also emitting the paired `CLIENT_QUERY` message
for internal zones.

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
| `dns_resolver_servers` | Canonical resolver1/resolver2 `{name, address}` list shared by both frontends |
| `dnsdist_preferred_resolver` | Host-specific name of the node-local preferred resolver |

`dnsdist_internal_backends` and `dnsdist_default_backends` are derived
automatically in `defaults/main.yaml`. Internal backends get a shared health
check built from `dnsdist_internal_check_name`; resolver backends share
`dnsdist_default_check_name` and differ only in their per-frontend order.

### Role defaults (in `defaults/main.yaml`)

| Variable | Default | Description |
|----------|---------|-------------|
| `dnsdist_server_addr` | Host primary IPv4 | IP address dnsdist binds to (frontend + web UI) |
| `dnsdist_service_user` | `_dnsdist` | Service account the package creates. Used only to resolve the group that owns the config — see below |
| `dnsdist_mgmt_acl` | `[127.0.0.1/32, 192.168.0.0/16, 10.0.0.0/8]` | ACL for web UI access. A list, not a comma-separated string — the YAML config takes a sequence |
| `dnsdist_console_acl` | `[127.0.0.1/32]` | ACL for console access (list) |
| `dnsdist_internal_domains` | `home.butaco.net.`, `prd.butaco.net.`, `sandbox.butaco.net.`, `168.192.in-addr.arpa.` | Forward and reverse zones routed to the `internal` pool (suffix match, so one reverse entry covers all `192.168.x.0/24`) |
| `dnsdist_internal_check_name` | `ns1.home.butaco.net.` | Health check FQDN for internal backends |
| `dnsdist_default_check_name` | `dnssec.works.` | Health check FQDN for resolver backends |
| `dnsdist_repo_channel` | `dnsdist-21` | PowerDNS APT repository channel (release train, not a version pin) |
| `dnsdist_repo_release` | `{{ ansible_facts['distribution_release'] }}` | Ubuntu release codename |
| `dnsdist_repo_key_sha256` | `efeb5b14…decae8` | Checksum of the repository signing key. Verifies the APT trust anchor, and is what lets `get_url` skip the network once the key is in place — `force: false` alone does not, because the skip is gated on a checksum being set. Shared with the `pdns_auth` role, which fetches the same file |

## Moving the release train

`dnsdist_repo_channel` selects a PowerDNS release train, not a version. Packages
inside a train update on their own; crossing to the next train is a deliberate
change, because PowerDNS supports a train only for about a year after its
successor ships. Check <https://repo.powerdns.com/> for the available channels
and which is stable, <https://www.dnsdist.org/eol.html> for the dates, and
<https://dnsdist.org/upgrade_guide.html> before moving — the upgrade guide is
organised by version pair, so read only the section for the hop being made.

The keyring and pin paths deliberately carry no channel name: the signing key is
the same for every train and the pin matches on origin rather than suite, so a
train move is a one-line change here. The `pdns_auth` role follows the same
convention and fetches the same key file.

Moving the train only repoints the repository — the apt task uses
`state: present`, so the package itself is upgraded separately by
`ops-package_upgrade.yaml`, which runs the `dns` group one host at a time and
stops before the second host if the first fails to come back.

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
