#!/usr/bin/env python3
"""Initialize a rebuilt OpenBao host and store its root token and recovery keys in SOPS.

The init output stays in this process and reaches SOPS through --value-stdin, so it
is never printed or placed in a process listing. Recovery keys cannot be regenerated,
so the SOPS writes are rehearsed on a copy first and the output is kept in an
encrypted backup until both writes are verified.
"""
import argparse
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

DEFAULT_SOPS_FILE = (Path(__file__).resolve().parent.parent
                     / "ansible/inventories/homelab/group_vars/openbao.sops.yaml")
BAO = "BAO_ADDR=http://127.0.0.1:8200 bao"


def fail(message):
    print(f"openbao-init: {message}", file=sys.stderr)
    sys.exit(1)


def run(args, stdin=None):
    try:
        result = subprocess.run(args, input=stdin, capture_output=True, text=True, timeout=120)
    except (OSError, subprocess.TimeoutExpired):
        return None
    return result.stdout if result.returncode == 0 else None


def remote_json(host, command):
    output = run(["ssh", "-o", "BatchMode=yes", host, command])
    try:
        return json.loads(output) if output is not None else fail("ssh or bao failed")
    except ValueError:
        fail("unexpected bao output")


# sops set rejects array values, so the recovery keys are written one element at a time.
def store(sops_file, root, keys):
    writes = [('["openbao_root_token"]', root)]
    writes += [(f'["openbao_recovery_keys"][{i}]', key) for i, key in enumerate(keys)]
    for index, value in writes:
        if run(["sops", "set", "--value-stdin", str(sops_file), index], stdin=json.dumps(value)) is None:
            return False
    stored = run(["sops", "decrypt", "--extract", '["openbao_recovery_keys"]',
                  "--output-type", "json", str(sops_file)])
    try:
        return stored is not None and len(json.loads(stored)) == len(keys)
    except ValueError:
        return False


# The override name lets a repository .sops.yaml match; --age covers files outside it.
def encrypt(payload, name, recipients):
    return run(["sops", "encrypt", "--age", ",".join(recipients), "--filename-override", name,
                "--input-type", "json", "--output-type", "yaml", "/dev/stdin"],
               stdin=json.dumps(payload))


def main():
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("host", help="SSH destination of the OpenBao host")
    parser.add_argument("--sops-file", type=Path, default=DEFAULT_SOPS_FILE)
    parser.add_argument("--recovery-shares", type=int, default=5)
    args = parser.parse_args()
    if not args.sops_file.is_file():
        fail("SOPS file not found")
    recipients = sorted(set(re.findall(r"age1[0-9a-z]{58}", args.sops_file.read_text())))
    if not recipients:
        fail("no age recipient found in the SOPS file metadata")

    backup = args.sops_file.with_name(args.sops_file.name.removesuffix(".sops.yaml")
                                      + ".init-backup.sops.yaml")
    if backup.exists():
        fail(f"{backup.name} exists; recover from it before initializing again")
    with tempfile.TemporaryDirectory() as tmp:
        rehearsal = Path(tmp) / args.sops_file.name
        shutil.copyfile(args.sops_file, rehearsal)
        dummy = ["rehearsal"] * args.recovery_shares
        if encrypt({"recovery_keys_b64": dummy}, str(backup), recipients) is None \
                or not store(rehearsal, "rehearsal", dummy):
            fail("SOPS writes failed on a copy; nothing was initialized")

    # bao status exits 2 while sealed, which is the expected pre-init state.
    status = remote_json(args.host, f"{BAO} status -format=json || true")
    if status.get("initialized") is not False:
        fail("OpenBao is already initialized; refusing to run operator init")

    init = remote_json(args.host, f"{BAO} operator init -format=json "
                                  f"-recovery-shares={args.recovery_shares} "
                                  f"-recovery-threshold={(args.recovery_shares + 1) // 2}")
    root = init.get("root_token")
    keys = init.get("recovery_keys_b64")
    if not isinstance(root, str) or not root:
        fail("init output has no root token")
    if not isinstance(keys, list) or len(keys) != args.recovery_shares \
            or not all(isinstance(k, str) and k for k in keys):
        fail("init output has unexpected recovery keys")

    encrypted = encrypt({"root_token": root, "recovery_keys_b64": keys}, str(backup), recipients)
    if encrypted is None:
        fail("could not encrypt the init output; it was not stored, re-initialize the host")
    fd = os.open(backup, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, "w") as handle:
        handle.write(encrypted)

    if not store(args.sops_file, root, keys):
        fail(f"SOPS update failed; the init output is kept encrypted in {backup}")
    backup.unlink()

    status = remote_json(args.host, f"{BAO} status -format=json || true")
    print(f"Stored the root token and {len(keys)} recovery keys in {args.sops_file.name}; "
          f"initialized={status.get('initialized')} sealed={status.get('sealed')}")


if __name__ == "__main__":
    main()
