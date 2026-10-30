# pdm Role

Installs [Proxmox Datacenter Manager](https://pdm.proxmox.com/) on Debian 13
from the `pdm-no-subscription` repository (ADR-0082). The Proxmox archive
keyring is verified against `pdm_keyring_sha256` on first download only; the
`proxmox-archive-keyring` package owns the file afterwards.

The role does not configure PDM itself. Remotes, their API tokens, realms and
ACLs live in PDM's own state and are set up in the web UI
(`https://pdm.home.butaco.net`) after the first `root@pam` sign-in, which needs
a root password set on the host. Recreate them after a rebuild.

Updates arrive through `ops/package_upgrade.yaml` like every other guest; do
not apply updates to the Proxmox hosts from PDM, which bypasses the drain
orchestration in ADR-0032.
