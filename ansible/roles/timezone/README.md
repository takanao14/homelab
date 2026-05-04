# timezone Role

Sets the system timezone using the `community.general.timezone` module.

## Functionality

- Sets the system timezone to the value of `timezone`.

## Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `timezone` | `Asia/Tokyo` | Timezone name (must be a valid `timedatectl` timezone identifier) |

## Dependencies

- `community.general` Ansible collection.

## Usage

```yaml
- name: Configure timezone
  hosts: all
  roles:
    - timezone
```
