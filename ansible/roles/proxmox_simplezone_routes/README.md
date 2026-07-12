# proxmox_simplezone_routes Role

Installs persistent static routes between per-node Proxmox SDN SimpleZone
segments. Routes are written as an ifupdown2 drop-in under
`/etc/network/interfaces.d/`; applying the live network reload remains an
operator action.

## Functionality

- Requires the current Proxmox inventory host to have an entry in
  `proxmox_simplezone_routes_map`.
- Renders routes to every other Proxmox node's SimpleZone segment.
- Writes `post-up ip route replace ...` and matching `pre-down ip route del ...`
  hooks for the management bridge.
- Validates that ifupdown2 can parse the configured interface with
  `ifquery --check <interface>` after rendering.
- Does not run `ifreload -a`; the operator applies network changes manually.

## Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `proxmox_simplezone_routes_interface` | `vmbr0` | Interface stanza that receives route hooks. |
| `proxmox_simplezone_routes_file` | `/etc/network/interfaces.d/ansible-simplezone-routes` | Managed ifupdown2 drop-in path. |
| `proxmox_simplezone_routes_map` | `{}` | Map of Proxmox host name to `{ segment, via }`. |
| `proxmox_simplezone_routes_require_local_entry` | `true` | Fail when the current host has no route map entry. |

## Apply Flow

Dry-run first:

```bash
ANSIBLE_ROLES_PATH=$PWD/ansible/roles \
ansible-playbook --check --diff -i ansible/inventories/homelab/hosts.yaml \
  ansible/playbooks/proxmox.yaml --limit node3 --tags proxmox_simplezone_routes
```

After applying the Ansible file change, reload networking manually on the target
node:

```bash
ifreload -a
```

Verify a remote SimpleZone next-hop:

```bash
ip route get 192.168.60.11
```
