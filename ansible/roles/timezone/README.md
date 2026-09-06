# timezone Role

Sets the system timezone using the `community.general.timezone` module.

## Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `timezone` | `Asia/Tokyo` | Timezone name (must be a valid `timedatectl` timezone identifier) |

## Dependencies

- `community.general` Ansible collection.

## Usage

Run [playbooks/common-timezone.yaml](../../playbooks/common-timezone.yaml).
