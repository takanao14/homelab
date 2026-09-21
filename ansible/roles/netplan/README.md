# netplan Role

Declares a host's interface configuration as a single Ansible-managed netplan
file that overrides whatever the image shipped.

## Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `netplan_file` | `99-ansible.yaml` | File under `/etc/netplan`; the name must sort after the files it overrides, such as cloud-init's `50-cloud-init.yaml` |
| `netplan_renderer` | `networkd` | Backend netplan configures |
| `netplan_ethernets` | `{}` | Rendered verbatim under `network.ethernets`; at least one interface is required |
| `netplan_obsolete_files` | `[]` | Files under `/etc/netplan` to delete, such as the image's `50-cloud-init.yaml` |
| `netplan_disable_cloud_init_network` | `false` | Write the cloud-init drop-in that stops it regenerating a removed file |

Netplan concatenates list values such as `nameservers.addresses` across files
rather than replacing them, so overriding a shipped file is not enough to drop
an entry from it. Delete the shipped file through `netplan_obsolete_files` and
declare the whole interface here.

## Not for NetworkManager hosts

Where NetworkManager renders the configuration it also owns it, rewriting
`/etc/netplan` into its own `90-NM-<uuid>.yaml` and deleting the Ansible-managed
file on the first apply. Use the [networkmanager](../networkmanager/README.md)
role there instead.

## Behavior

`netplan generate` runs as a regular task so an invalid configuration fails the
play instead of surviving until the next boot.

## Usage

Run [playbooks/common-netplan.yaml](../../playbooks/common-netplan.yaml).
