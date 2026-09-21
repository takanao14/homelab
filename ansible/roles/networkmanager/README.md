# networkmanager Role

Declares NetworkManager connection profiles with `community.general.nmcli`.

## Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `networkmanager_connections` | `[]` | Connection definitions; each entry supplies `conn_name` plus the `nmcli` settings to enforce |

Supported keys per entry: `conn_name`, `type`, `ifname`, `method4`, `ip4`,
`gw4`, `routes4`, `dns4`, `dns4_search`, `method6`. Anything omitted is left
untouched on the profile.

## Why not a netplan file

On Raspberry Pi OS, NetworkManager owns the persisted configuration: it rewrites
whatever is under `/etc/netplan` into its own `90-NM-<uuid>.yaml` when a profile
changes, so an Ansible-managed netplan file is deleted on first apply and the
run never converges. Managing the profile itself is idempotent.

That round-trip is also why `ifname` is normally left out. Netplan writes
`match: {}`, which clears `connection.interface-name`, so pinning it makes every
run report a change. The module only falls back to `conn_name` for the interface
name when it *creates* a connection, so entries here must name profiles that
already exist.

## Behavior

The nmcli module edits a dormant profile, so a changed connection is brought up
by a handler. That drops the SSH session, which the handlers start detached and
then wait out.

## Dependencies

- `community.general` Ansible collection.

## Usage

Run [playbooks/common-networkmanager.yaml](../../playbooks/common-networkmanager.yaml).
