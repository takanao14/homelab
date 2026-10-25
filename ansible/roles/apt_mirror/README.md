# apt_mirror Role

The role directs regular Ubuntu package downloads to Japanese archive mirrors
through apt's `mirror+file:` method, while keeping security updates on their
existing sources. apt tries the mirrors in list order and falls back to the next
one when a download fails.

Supports DEB822 and legacy sources on Ubuntu. Any archive URI ending in
`/ubuntu` is replaced, so hosts already pointing at an earlier single mirror are
migrated; security and `ubuntu-ports` sources are left unchanged.

## Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `apt_mirror_urls` | Yamagata University, JAIST, `jp.archive.ubuntu.com` | Mirrors in fallback order, written with ascending `priority` |
| `apt_mirror_list_path` | `/etc/apt/ubuntu-mirrors.txt` | Mirror list referenced by the sources |

## Usage

Run [playbooks/common/apt_mirror.yaml](../../playbooks/common/apt_mirror.yaml).
