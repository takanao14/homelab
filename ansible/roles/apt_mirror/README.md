# apt_mirror Role

Replaces the default Ubuntu apt mirror URL with a local mirror on Ubuntu hosts.

## Functionality

- Skips non-Ubuntu systems automatically.
- Detects DEB822 format (`/etc/apt/sources.list.d/ubuntu.sources`) and legacy format (`/etc/apt/sources.list`).
- Replaces `http://archive.ubuntu.com/ubuntu` with `apt_mirror_url` in the detected sources file.
- Leaves security entries (`-security`) untouched.
- Triggers `apt-get update` after replacement.

## Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `apt_mirror_url` | `http://ftp.udx.icscoe.jp/Linux/ubuntu` | Mirror URL to substitute for the default Ubuntu archive |

## Dependencies

None.

## Usage

```yaml
- name: Configure apt mirror
  hosts: all
  roles:
    - apt_mirror
```
