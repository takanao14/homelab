# apt_mirror Role

Replaces the default Ubuntu apt mirror URL with a local mirror on Ubuntu hosts.

Supports DEB822 and legacy sources on Ubuntu, leaves security sources unchanged,
and updates the APT cache after replacement.

## Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `apt_mirror_url` | `http://ftp.udx.icscoe.jp/Linux/ubuntu` | Mirror URL to substitute for the default Ubuntu archive |

## Usage

Run [playbooks/common/apt_mirror.yaml](../../playbooks/common/apt_mirror.yaml).
