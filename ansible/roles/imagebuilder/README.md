# Image builder

Configure `imagebuilder1` with `playbooks/services/imagebuilder.yaml` after
creating and starting the VM. The control account accepts the configured admin
SSH keys and can sudo only the runtime's JSON control entry point. Build jobs
run as the separate unprivileged `imagebuilder` account under systemd. The
playbook then grants the controller's Proxmox token VM-scoped power access on
`pve`; do not limit it to `imagebuilder1` when that access must be reconciled.

Install Go on the Ansible controller. The role builds the repository's Go runtime
there with the toolchain pinned in `scripts/imagebuilder/go.mod`, then copies a
static Linux amd64 binary to the guest. No Go toolchain is installed on the guest.

The work disk must already contain ext4. For a new blank disk only, set
`imagebuilder_format_work_disk=true` explicitly; existing signatures are never
overwritten. The role refuses changes while a build service is active. Enter
maintenance mode before provisioning to prevent admission of new jobs.

Packer's release version and official linux-amd64 archive SHA-256 are pinned
together in role defaults. Update both when upgrading. Initial check mode cannot
validate services, an unformatted disk mount, or files belonging to not-yet-created
accounts. Re-run check mode after provisioning.

The running kernel is copied readably for libguestfs without granting build
jobs sudo. Re-run the role after a kernel upgrade and reboot to refresh this
copy. Runtime services write JSON records to journald with only the command,
control operation, job ID, result code and duration; raw build output and
request data are never logged. Polling operations and idle ticks log only
failures. Inspect them with `journalctl -t imagebuilder -o cat`.
