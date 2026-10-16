# apt_mirror Role

The role directs regular Ubuntu package downloads to a Japanese archive mirror
by replacing the default archive URL with `apt_mirror_url`, while keeping
security updates on their existing sources.

Supports DEB822 and legacy sources on Ubuntu, leaves security sources unchanged,
and updates the APT cache after replacement.

## Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `apt_mirror_url` | `http://ftp.udx.icscoe.jp/Linux/ubuntu` | Mirror URL to substitute for the default Ubuntu archive |

## Usage

Run [playbooks/common/apt_mirror.yaml](../../playbooks/common/apt_mirror.yaml).
