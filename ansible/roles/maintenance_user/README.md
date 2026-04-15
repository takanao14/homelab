# maintenance_user

Role to add a maintenance user to a host (especially useful for LXC containers during initial setup).

## Requirements

- `sudo` (this role will install it if not present on Debian-based systems)

## Role Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `maintenance_user` | Name of the maintenance user | OS env `MAINTENANCE_USER` → Ansible var `MAINTENANCE_USER` → `admin` |
| `maintenance_password_hash` | Password hash for the user | OS env `MAINTENANCE_PASSWORD_HASH` → Ansible var `MAINTENANCE_PASSWORD_HASH` → `""` |
| `maintenance_ssh_key_path` | Path to the SSH public key file | OS env `SSH_KEY_PATH` → Ansible var `SSH_KEY_PATH` → `""` |
| `maintenance_ssh_key` | SSH public key content (optional) | `""` |

## Example Playbook

```yaml
- name: Setup LXC container
  hosts: lxc_containers
  roles:
    - role: maintenance_user
      tags: maintenance_user
    - role: timezone
      tags: timezone
```

## Features

- Ensures `sudo` is installed.
- Creates the user and adds to `sudo` group.
- Configures passwordless sudo for the user.
- Adds the specified SSH authorized key for the user.
