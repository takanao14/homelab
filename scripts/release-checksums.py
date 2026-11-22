#!/usr/bin/env python3
"""Verify pinned release binary checksums, or refresh them with --update.

Run from the repository root; CI runs a trusted copy outside the checkout.
"""

import argparse
import hashlib
import re
import sys
import urllib.request
from pathlib import Path

import yaml

SHA_RE = r'[a-f0-9]{64}'

# checksum_url marks targets whose digest Renovate cannot update natively
# (non-GitHub hosts); --update refreshes only those.
TARGETS = [
    {
        'name': 'dnscollector',
        'path': 'ansible/roles/dnscollector/defaults/main.yaml',
        'version_key': 'dnscollector_version',
        'sha_key': 'dnscollector_sha256',
        'version_re': r'v\d+\.\d+\.\d+',
        'url': lambda v: f'https://github.com/dmachard/DNS-collector/releases/download/{v}/DNS-collector_{v[1:]}_linux_amd64.tar.gz',
    },
    {
        'name': 'seaweedfs',
        'path': 'ansible/roles/seaweedfs/defaults/main.yaml',
        'version_key': 'seaweedfs_version',
        'sha_key': 'seaweedfs_sha256',
        'version_re': r'\d+\.\d+(?:\.\d+)?',
        'url': lambda v: f'https://github.com/seaweedfs/seaweedfs/releases/download/{v}/linux_amd64.tar.gz',
    },
    {
        'name': 'forgejo',
        'path': 'ansible/roles/forgejo/defaults/main.yaml',
        'version_key': 'forgejo_version',
        'sha_key': 'forgejo_sha256',
        'version_re': r'\d+\.\d+\.\d+(?:-\d+)?',
        'url': lambda v: f'https://codeberg.org/forgejo/forgejo/releases/download/v{v}/forgejo-{v}-linux-amd64',
        'checksum_url': lambda v: f'https://codeberg.org/forgejo/forgejo/releases/download/v{v}/forgejo-{v}-linux-amd64.sha256',
    },
    {
        'name': 'forgejo_runner',
        'path': 'ansible/roles/forgejo_runner/defaults/main.yaml',
        'version_key': 'forgejo_runner_version',
        'sha_key': 'forgejo_runner_sha256',
        'version_re': r'\d+\.\d+\.\d+',
        'url': lambda v: f'https://code.forgejo.org/forgejo/runner/releases/download/v{v}/forgejo-runner-{v}-linux-amd64',
        'checksum_url': lambda v: f'https://code.forgejo.org/forgejo/runner/releases/download/v{v}/forgejo-runner-{v}-linux-amd64.sha256',
    },
]


def fail(message):
    sys.exit(message)


def read_pins(target):
    pins = yaml.safe_load(Path(target['path']).read_text())
    version = str(pins[target['version_key']])
    sha = str(pins[target['sha_key']])
    if not re.fullmatch(target['version_re'], version) or not re.fullmatch(SHA_RE, sha):
        fail(f"Invalid {target['name']} release pins")
    return version, sha


def fetch_sha256(url, name):
    try:
        with urllib.request.urlopen(url, timeout=60) as archive:
            return hashlib.file_digest(archive, 'sha256').hexdigest()
    except Exception:
        fail(f'Could not fetch {name} release archive')


def fetch_upstream_sha256(url, name):
    try:
        with urllib.request.urlopen(url, timeout=60) as response:
            fields = response.read(1024).decode('ascii').split()
    except Exception:
        fail(f'Could not fetch {name} upstream checksum')
    if not fields or not re.fullmatch(SHA_RE, fields[0]):
        fail(f'Invalid {name} upstream checksum')
    return fields[0]


def check(target):
    version, expected = read_pins(target)
    if fetch_sha256(target['url'](version), target['name']) != expected:
        fail(f"{target['name']} release checksum mismatch")
    print(f"{target['name']} release checksum verified")


def update(target):
    version, current = read_pins(target)
    upstream = fetch_upstream_sha256(target['checksum_url'](version), target['name'])
    if fetch_sha256(target['url'](version), target['name']) != upstream:
        fail(f"{target['name']} release archive does not match upstream checksum")
    if upstream == current:
        print(f"{target['name']} release checksum already current")
        return
    path = Path(target['path'])
    text, count = re.subn(
        rf'^{target["sha_key"]}: "{SHA_RE}"$',
        f'{target["sha_key"]}: "{upstream}"',
        path.read_text(),
        flags=re.MULTILINE,
    )
    if count != 1:
        fail(f"Could not locate {target['name']} checksum pin")
    path.write_text(text)
    print(f"{target['name']} release checksum updated")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--update', action='store_true', help='refresh checksums from upstream')
    args = parser.parse_args()
    for target in TARGETS:
        if not args.update:
            check(target)
        elif 'checksum_url' in target:
            update(target)


if __name__ == '__main__':
    main()
