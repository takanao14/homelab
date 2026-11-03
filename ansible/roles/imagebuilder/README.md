# Image builder

Configure the Ubuntu 26.04 tool VM for headless Packer/QEMU builds.
The tool image supplies Packer, OpenBao, SOPS, direnv, tmux and rclone.
This role installs QEMU, libguestfs and the ISO creation tool, grants KVM
access and checks nested KVM availability. Chezmoi manages user configuration,
including shell hooks and the SSH agent.

Run `playbooks/bootstrap.yaml --limit imagebuilder1`, then
`playbooks/services/imagebuilder.yaml --limit imagebuilder1` from `ansible/`.
`imagebuilder_user` defaults to the SSH connection user. Reconnect after the
first run for KVM group membership to take effect.

Apply chezmoi to the builder user and clone this repository into `~/homelab`.
From its root, run `scripts/secrets/get-env.sh` and
`scripts/secrets/get-sops-key.sh` to retrieve `~/.env` and the SOPS AGE key
from OpenBao. Authorize the repository's `.envrc` files with direnv after
reviewing them, then run the existing scripts from `~/homelab/packer`.

Provide a local SSH agent with the build key loaded and its matching public
key via `PKR_VAR_ssh_pubkey` or the default `~/.ssh/id_ed25519.pub`.
Ansible does not deploy private keys or credentials. `build.sh` also needs
sudo to make the host kernel readable by libguestfs.
