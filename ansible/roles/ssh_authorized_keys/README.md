# ssh_authorized_keys

Replaces a user's `authorized_keys` with the given list (`exclusive`), so keys
removed from the list are revoked. An empty list fails instead of locking the
user out.

## Role Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `ssh_authorized_keys_user` | User whose keys are managed | `ansible_user`, then the user the connection logs in as |
| `ssh_authorized_keys_list` | Public keys to keep | required |

## Usage

Run [playbooks/common/ssh_authorized_keys.yaml](../../playbooks/common/ssh_authorized_keys.yaml),
which applies the keys in `admin_authorized_keys_file` to the connection user
of `lxc` and `vm` hosts. A host can add keys from its own
`admin_authorized_keys_extra_file` in `host_vars`; a declared file that is
missing fails the run. Run it with `--check --diff` before rotating keys.
