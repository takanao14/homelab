# seaweedfs Role

Installs and configures a standalone [SeaweedFS](https://github.com/seaweedfs/seaweedfs)
server with the S3 gateway enabled, on Debian-based systems (LXC or VM).

Intended as a self-hosted backend for Terraform/OpenTofu remote state. SeaweedFS
is chosen over MinIO because it is actively maintained (roughly weekly releases)
and supports both bucket **versioning** (state history) and **conditional writes**
on versioned buckets, so Terraform native S3 state locking (`use_lockfile = true`)
works without DynamoDB. The conditional-write-on-versioned-bucket bug
([#8073](https://github.com/seaweedfs/seaweedfs/issues/8073)) was fixed in
[#8080](https://github.com/seaweedfs/seaweedfs/pull/8080) — pin a recent version.

The role runs SeaweedFS in all-in-one mode (`weed server -filer -s3`), which
starts master, volume, filer, and the S3 gateway in a single process — suitable
for a single-node homelab backend.

## Functionality

- Creates the `seaweedfs` system user/group and required directories.
- Downloads the pinned `weed` binary from GitHub releases.
- Deploys the S3 identity config (`/etc/seaweedfs/s3.json`) and a systemd unit.
- Ensures the service is started and enabled.

## Variables

### Secrets (must be set in `group_vars/seaweedfs.sops.yaml`)

| Variable | Description |
|----------|-------------|
| `seaweedfs_s3_access_key` | S3 access key for the Terraform identity |
| `seaweedfs_s3_secret_key` | S3 secret key for the Terraform identity |
| `seaweedfs_admin_password` | Admin UI password (empty = auth disabled) |
| `seaweedfs_backup_r2_endpoint` | Cloudflare R2 S3 endpoint (`https://<account_id>.r2.cloudflarestorage.com`) |
| `seaweedfs_backup_r2_access_key` | R2 access key (read-only on the state bucket is enough) |
| `seaweedfs_backup_r2_secret_key` | R2 secret key |

### Non-secret variables (in `defaults/main.yaml`)

| Variable | Default | Description |
|----------|---------|-------------|
| `seaweedfs_version` | `4.32` | SeaweedFS release tag (managed by Renovate) |
| `seaweedfs_user` / `seaweedfs_group` | `seaweedfs` | System user/group |
| `seaweedfs_binary` | `/usr/local/bin/weed` | Binary path |
| `seaweedfs_download_dir` | `/opt/seaweedfs` | Release archive cache directory |
| `seaweedfs_data_dir` | `/var/lib/seaweedfs` | Data directory (master/volume/filer state) |
| `seaweedfs_config_dir` | `/etc/seaweedfs` | Config directory |
| `seaweedfs_s3_config` | `/etc/seaweedfs/s3.json` | S3 identity config path |
| `seaweedfs_ip` | host primary IP | Address advertised to clients |
| `seaweedfs_bind_ip` | `0.0.0.0` | Interface the services bind to |
| `seaweedfs_master_port` | `9333` | Master port (used by the admin UI) |
| `seaweedfs_s3_port` | `8333` | S3 gateway port |
| `seaweedfs_volume_size_limit_mb` | `1024` | Per-volume size limit (small so volumes fit a small disk) |
| `seaweedfs_volume_max` | `0` | Max volumes per server (`0` = auto from free disk) |
| `seaweedfs_s3_identity_name` | `terraform` | Name of the default (Admin) S3 identity in `s3.json` |
| `seaweedfs_s3_extra_identities` | `[]` | Extra credentialed identities (`name`/`access_key`/`secret_key`/`actions`) |
| `seaweedfs_public_buckets` | `[]` | Buckets exposed anonymously (Read-only, no List) for HTTP download |
| `seaweedfs_admin_enabled` | `true` | Deploy the `weed admin` UI as a separate service |
| `seaweedfs_admin_port` | `23646` | Admin UI HTTP port |
| `seaweedfs_admin_data_dir` | `/var/lib/seaweedfs-admin` | Admin state directory |
| `seaweedfs_admin_user` | `admin` | Admin UI username |
| `seaweedfs_backup_enabled` | `true` | Deploy the R2 -> SeaweedFS state backup timer |
| `seaweedfs_backup_r2_bucket` | `homelab-tfstate` | Source R2 bucket (primary state) |
| `seaweedfs_backup_dest_bucket` | `homelab-tfstate-backup` | Destination SeaweedFS bucket (versioned) |
| `seaweedfs_backup_schedule` | `*:0/15` | systemd `OnCalendar` sync interval |
| `seaweedfs_backup_rclone_config` | `/etc/seaweedfs/rclone.conf` | rclone config path |

## Post-install: provisioning the Terraform state bucket

The role does not create buckets (deferred until the migration target is
decided). Once ready, create the bucket and enable versioning with any S3 client,
e.g. the AWS CLI pointed at the gateway:

```bash
export AWS_ACCESS_KEY_ID=<seaweedfs_s3_access_key>
export AWS_SECRET_ACCESS_KEY=<seaweedfs_s3_secret_key>
ENDPOINT=http://<host>:8333

aws --endpoint-url "$ENDPOINT" s3api create-bucket --bucket homelab-terraform-state
aws --endpoint-url "$ENDPOINT" s3api put-bucket-versioning \
  --bucket homelab-terraform-state \
  --versioning-configuration Status=Enabled
```

Then point `tf/root.hcl` at the S3 backend. As with any non-AWS S3 endpoint, set
`skip_s3_checksum = true`, `skip_credentials_validation`, `skip_region_validation`,
and `skip_requesting_account_id`, plus `use_lockfile = true` for native locking.

## State backup (R2 -> SeaweedFS)

The decided topology is **Cloudflare R2 as the primary** Terraform state backend
with **SeaweedFS holding a DR copy**. R2 has no object versioning, so SeaweedFS
also provides the version history: a one-way `rclone sync` (R2 -> SeaweedFS)
runs on the `seaweedfs-backup.timer` systemd timer (default every 15 minutes),
and the destination bucket is versioned so overwrites keep history.

The role installs `rclone`, writes `/etc/seaweedfs/rclone.conf` (an `r2` and a
`seaweedfs` remote), and enables the timer. It does **not** create the
destination bucket. Create it once with versioning enabled, using the SeaweedFS
S3 gateway (the `terraform` identity has `Admin`):

```bash
export AWS_ACCESS_KEY_ID=<seaweedfs_s3_access_key>
export AWS_SECRET_ACCESS_KEY=<seaweedfs_s3_secret_key>
ENDPOINT=http://<host>:8333

aws --endpoint-url "$ENDPOINT" s3api create-bucket --bucket homelab-tfstate-backup
aws --endpoint-url "$ENDPOINT" s3api put-bucket-versioning \
  --bucket homelab-tfstate-backup \
  --versioning-configuration Status=Enabled
```

Trigger a sync immediately and inspect it:

```bash
sudo systemctl start seaweedfs-backup.service   # run once now
systemctl list-timers seaweedfs-backup.timer    # next scheduled run
journalctl -u seaweedfs-backup.service          # logs
```

## Public download buckets

Buckets listed in `seaweedfs_public_buckets` get an `anonymous` S3 identity with
`Read:<bucket>` only (no `List`), so any client can fetch an object by key but
cannot enumerate the bucket. This replaces Garage's public web serving. The
buckets are fronted over HTTPS by the central Caddy:

```
https://s3.home.butaco.net/firmware/<file>      # -> SeaweedFS :8333, path-style
```

Direct (LAN, no TLS) access also works: `http://seaweedfs1.home.butaco.net:8333/firmware/<file>`.
Create the buckets out of band (the role does not create buckets):

```bash
export AWS_ACCESS_KEY_ID=<seaweedfs_s3_access_key>
export AWS_SECRET_ACCESS_KEY=<seaweedfs_s3_secret_key>
ENDPOINT=http://seaweedfs1.home.butaco.net:8333
aws --endpoint-url "$ENDPOINT" s3api create-bucket --bucket firmware
aws --endpoint-url "$ENDPOINT" s3api create-bucket --bucket cloud-images
```

The `cloud-images` bucket distributes the custom Proxmox images built by Packer.
`packer/push.sh` uploads each `<image>.img` and its `<image>.img.sha256`, and the
Terragrunt stack in `tf/customimage` makes Proxmox download them from
`https://s3.home.butaco.net/cloud-images/<file>` (pinned by the published sha256).

> **Sizing note (LXC):** serving multi-GB images is memory-heavy. In an
> unprivileged LXC the page cache counts against the memory cgroup — RAM and file
> cache share one cap — so a node large enough to *store* the images can still
> OOM-kill `weed` while *serving* them (symptom: `volume server has been killed`
> in `journalctl -u seaweedfs`, followed by a systemd restart; the cgroup OOM
> fires on the host, so it is absent from the container's `journalctl -k`). The
> `cloud-images` host (`tf/lxc/node3/seaweedfs`) runs **8GB RAM + 4GB swap** for
> this reason; size new deployments accordingly if they serve large objects.
The build host uses a dedicated upload key rather than the global Admin
`terraform` identity. Scope Admin to `cloud-images` only: large images are
uploaded through S3 multipart upload, and SeaweedFS may reject multipart setup
for a non-owner identity that only has plain `Write:<bucket>`.

```yaml
seaweedfs_s3_extra_identities:
  - name: imagebuilder
    access_key: "{{ seaweedfs_imagebuilder_access_key }}"   # in seaweedfs.sops.yaml
    secret_key: "{{ seaweedfs_imagebuilder_secret_key }}"
    actions: ["Admin:cloud-images", "Read:cloud-images", "Write:cloud-images", "List:cloud-images"]
```

## Single-node deployment

This role deploys SeaweedFS as a **single node, single process** (`weed server
-filer -s3` runs master, volume, filer, and the S3 gateway together on one host).
There is **no replication**, so the node — and the Proxmox host it runs on — is a
single point of failure for the Terraform state backend.

Bucket versioning protects against accidental overwrite/delete (version restore),
but **not** against disk or node loss. For single-node operation, back up the data
directory (`/var/lib/seaweedfs`) on a schedule, or mirror the state out separately
(e.g. a `terragrunt state pull` cron). State is small, so the backup cost is
negligible. If node-level durability is a hard requirement, use an external
backend (Cloudflare R2) instead.

SeaweedFS itself supports multi-node replication; expanding this role to a
replicated cluster (separate master/volume nodes, `-defaultReplication=001`) is a
future option.

## Operations and management

- **S3 level (buckets/objects):** any S3 client works against the gateway
  (`:8333`) — the MinIO client `mc`, the AWS CLI, `rclone`, etc.
- **Admin GUI:** deployed as a separate `seaweedfs-admin` systemd service
  (`weed admin`) on `:{{ seaweedfs_admin_port | default(23646) }}`, providing
  cluster health, node status, and S3 bucket management. Unlike MinIO, this
  console is part of the open project. Disable with `seaweedfs_admin_enabled: false`.
  Authentication is set via `seaweedfs_admin_user` / `seaweedfs_admin_password`,
  passed through `WEED_ADMIN_PASSWORD` in the environment file so the password is
  not visible in the process arguments. **If `seaweedfs_admin_password` is empty,
  the admin UI runs without authentication** — always set it.
- **Status pages:** master UI on `:9333` (topology/volumes), filer file browser on
  `:8888`.
- **CLI admin:** `weed shell` for advanced volume/maintenance operations.

`weed mini` (an all-in-one command that also bundles the admin UI and a
maintenance worker) was evaluated but not used: it is newer (introduced
Dec 2025) and still rapidly changing, so the more established
`weed server` + separate `weed admin` split is preferred for a state backend.

## Dependencies

None.

## Usage

```yaml
- name: Setup SeaweedFS
  hosts: seaweedfs
  roles:
    - role: seaweedfs
```
