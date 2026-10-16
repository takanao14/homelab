# timezone Role

The role sets managed hosts to `Asia/Tokyo` by default so local timestamps use
the same timezone during operations and log inspection. It applies `timezone`
through `community.general.timezone`.

## Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `timezone` | `Asia/Tokyo` | Timezone name (must be a valid `timedatectl` timezone identifier) |

## Dependencies

- `community.general` Ansible collection.

## Usage

Run [playbooks/common/timezone.yaml](../../playbooks/common/timezone.yaml).
