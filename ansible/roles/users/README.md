# users Role

Manages local user accounts on shared VMs, including passwords, login state,
shells, and optional sudo access.

## Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `users_accounts` | `[]` | Accounts to manage. Each item requires `name` and `password`; optional keys are `shell`, `sudo`, and `state` |
| `users_default_shell` | `/bin/bash` | Default shell for enabled accounts |
| `users_create_home` | `true` | Create home directories |
| `users_password_scheme` | `sha512` | Password hashing scheme used on the control node |

Store `users_accounts` in a SOPS-encrypted
`inventories/homelab/group_vars/shared_vms.sops.yaml` file when the
`shared_vms` inventory group is enabled. Passwords are accepted as plaintext
inside the encrypted file and are hashed before being passed to the user module.

Setting an account's `state` to `disabled` expires and locks the account,
assigns a nologin shell, and removes supplementary groups while preserving its
home directory. Returning the state to `enabled` reverses those restrictions.

## Usage

```yaml
- name: Create user accounts on shared VMs
  hosts: shared_vms
  roles:
    - role: users
```
