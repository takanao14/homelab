# maintenance_user

Creates a maintenance account with SSH access and passwordless sudo, installing
`sudo` on Debian-based systems when needed. It runs on LXC hosts by default;
set `maintenance_user_force: true` for other hosts.

## Role Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `maintenance_user` | Name of the maintenance user | OS env `MAINTENANCE_USER` → Ansible var `MAINTENANCE_USER` → `admin` |
| `maintenance_password_hash` | Password hash for the user | OS env `MAINTENANCE_PASSWORD_HASH` → Ansible var `MAINTENANCE_PASSWORD_HASH` → `""` |
| `maintenance_ssh_key_path` | Path to the SSH public key file | OS env `SSH_KEY_PATH` → Ansible var `SSH_KEY_PATH` → `""` |
| `maintenance_ssh_key` | SSH public key content (optional) | `""` |
| `maintenance_user_force` | Run outside LXC | `false` |

## Usage

Run [playbooks/common/maintenance_user.yaml](../../playbooks/common/maintenance_user.yaml).
