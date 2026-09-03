# ADR-0042: Scheduled OpenBao Raft snapshots kept on-premises with a restore path

- **Status:** Proposed
- **Date:** 2026-09-03
- **Review by:** 2026-12-03
- **Related:** [ADR-0012](0012-openbao-eso-cluster-rebuild-registration.md),
  [ADR-0018](0018-seaweedfs-data-on-usb-ssd-directory-storage.md),
  [ADR-0023](0023-openbao-ansible-userpass-login.md),
  [`ansible/roles/openbao/README.md`](../../ansible/roles/openbao/README.md),
  [`ansible/roles/seaweedfs/README.md`](../../ansible/roles/seaweedfs/README.md)

## Context

ADR-0012 keeps OpenBao outside the k0s clusters so secret values survive a
cluster rebuild. Nothing protects the same values against loss of the OpenBao
host itself.

The only snapshot taken today is the pre-upgrade one in
`ops-openbao_upgrade.yaml`, written to `/var/backups/openbao` on the host being
upgraded. There is no schedule, no copy outside the host, no retention policy,
and no restore procedure. Losing the VM loses both OpenBao and its backups.

Recovery also has a circular dependency. `secret/sops/age` holds the AGE private
key, that key decrypts the SOPS inventory, the inventory holds
`openbao_seal_key`, and a Raft snapshot is encrypted under the seal. If the
OpenBao host and the operator workstation copy of the AGE key are lost together,
every snapshot becomes undecryptable. This is a property of the trust chain, not
something a backup mechanism can fix.

## Decision

Take Raft snapshots on a schedule, keep every copy on-premises, and make restore
a documented operational playbook.

1. **Scheduled snapshots.** The `openbao` role deploys a systemd service and
   timer that run `bao operator raft snapshot save` into `openbao_backup_dir`
   and prune to a bounded number of local generations. This follows the
   `seaweedfs-backup.service` / `.timer` pattern already used for state
   mirroring.
2. **Off-host copy in SeaweedFS.** The same unit copies each snapshot with
   rclone to a dedicated versioned SeaweedFS bucket, written through a
   non-admin S3 identity declared in `seaweedfs_s3_extra_identities` and scoped
   to that bucket.
3. **Snapshot-only credential.** A dedicated `snapshot-agent` userpass account
   with a policy granting `read` on `sys/storage/raft/snapshot` and nothing
   else. Per ADR-0023 the unit logs in at run time rather than holding a token.
4. **Restore playbook.** `ops-openbao_restore.yaml` takes a snapshot path,
   asserts the target is initialized and unsealed, runs
   `bao operator raft snapshot restore`, and verifies the API afterwards. Since
   the static seal key is deployed from SOPS, a rebuilt host uses the same seal
   and needs no `-force`.
5. **Offline escrow, outside this repository.** The AGE private key,
   `openbao_seal_key`, `openbao_root_token`, and the `operator init` recovery
   keys are held on offline media. The role README documents the requirement;
   nothing automates it.

Snapshots stay inside the LAN because OpenBao holds the credentials that reach
external providers — the Cloudflare API token and the SOPS AGE key among them.
Replicating that store to a cloud provider would place the material that
controls an external account inside that same account's blast radius, and would
widen the set of parties who hold a copy of every homelab secret. Bounding the
copies to on-premises storage is worth losing off-site protection.

## Alternatives considered

- **Cloudflare R2, as used for Terraform state** — the only candidate that
  survives loss of the site, and the seal key would remain separate in the Git
  remote. *Rejected:* OpenBao holds the Cloudflare API token, so this
  concentrates the store and the account it controls in one provider, and it
  exports every homelab secret, in sealed form, to a third party. Off-site
  durability does not justify that exposure here.
- **TrueNAS NFS** — outside the Proxmox node failure domain and on-premises, so
  it satisfies the same constraint. *Deferred:* it would add an NFS client mount
  to the host that stores every secret, whereas the SeaweedFS path reuses an
  existing rclone and S3 identity mechanism. Worth revisiting as a second
  on-premises copy.
- **Proxmox VM-level backup** — captures `/etc/openbao/seal.key` in the same
  archive as the data, so one leaked backup is a full compromise, and a running
  Raft store is only crash-consistent. *Rejected.*
- **Copying `/opt/openbao/data` directly** — not a consistent snapshot of an
  active Raft store. *Rejected.*
- **Storing recovery keys in SOPS** — would close the circular dependency by
  making the repository sufficient to unseal a rebuilt store, which defeats the
  purpose of holding them. *Rejected.*

## Consequences

- Loss of the OpenBao host becomes a documented recovery: rebuild the VM, run
  `openbao.yaml` to place the seal key, `bao operator init`, restore the
  snapshot, then re-run `ops-openbao_register_cluster.yaml` per cluster.
- Loss of the site loses the secret values themselves. Only the offline escrow
  survives, and it carries key material, not the KV inventory. Rebuilding from
  scratch and re-seeding from SOPS remains the fallback for that case.
- SeaweedFS becomes a recovery dependency. It is a single LXC on a node3 USB
  SSD (ADR-0018), so the local generations under `openbao_backup_dir` are the
  first line of recovery and the SeaweedFS copy covers host loss, not both at
  once.
- A snapshot-read credential lives on the OpenBao host. The marginal exposure is
  small because the seal key already does.
- Restoring resets the root token and recovery keys to the values in effect when
  the snapshot was taken, so escrowed copies must be refreshed after a rekey.
- The escrow step stays manual and is therefore the weakest link in the chain.
  It needs a periodic verification habit, not just a one-time action.
