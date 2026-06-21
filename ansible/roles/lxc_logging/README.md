# lxc_logging Role

Meta-role that aggregates the standard log-shipping stack applied to
Vector-enabled LXC guests: the [`vector`](../vector/README.md) agent plus the
[`journald`](../journald/README.md) policy.

It exists purely to remove the repeated

```yaml
  - role: vector
  - role: journald
```

block from every service playbook (caddy, dnsdist, pdns_auth, forgejo, netbox,
seaweedfs, log_collector). Use it after the service role:

```yaml
roles:
  - role: timezone
  - role: seaweedfs
  - role: lxc_logging   # vector + journald
    tags: logging
```

## Ordering

The bundled roles are declared as `meta/main.yaml` dependencies. Ansible runs a
role's dependencies immediately **before** the role itself, so listing
`lxc_logging` after the service role yields `service -> vector -> journald`.

## Tags

`vector` and `journald` carry their own tags inside the meta, so all of these
keep working:

- `--tags logging` — both roles
- `--tags vector` — Vector only
- `--tags journald` — journald only

## When not to use it

Hosts that run Vector but not the journald policy (e.g. the rpi4 DHCP host, which
is outside `vector_lxc`) keep a plain `- role: vector` instead.
