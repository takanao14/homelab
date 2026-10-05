# ADR-0062: Compose VM initialization with optional cloud-init snippets

- **Status:** Accepted
- **Date:** 2026-10-03
- **Related:** [Terraform README](../../tf/README.md)

## Context

FreeBSD 15.1 BASIC-CLOUDINIT uses nuageinit. A standalone VM verified that
custom NoCloud user-data and version 2 network-data configure the account,
SSH keys, hostname, static address, gateway, and DNS. Proxmox's generated
version 1 network-data is incompatible with nuageinit's network parser.

## Decision

Keep one `proxmox-vm` module. Each VM may supply user-data and network-data
independently; omitted payloads retain Proxmox's generated initialization.
A `cloudinit-snippets` child module uploads only supplied payloads and returns
file IDs. Keeping it under `proxmox-vm/modules` allows Terragrunt to copy it
with existing module source paths. Snippets and the VM share one state.

FreeBSD account and interface settings belong in the caller's HCL, shared by
the FreeBSD stack and its verification stack. SSH upload authentication is
configured through a caller-provided provider alias. Do not place plaintext
passwords or other decrypted secrets in snippets, which persist on Proxmox
and in Terraform state.

Retain the existing initialization lifecycle policy: changing a snippet does
not reconfigure an initialized guest. Explicit VM replacement is required to
apply changed first-boot settings; no automatic replacement is introduced.

## Alternatives

- Embedding payload generation and upload in the VM resource mixes OS settings
  with hardware provisioning.
- A separate Terragrunt snippet stack adds state and deletion-order coordination
  for files that belong to a single VM.
- A FreeBSD-specific VM module would duplicate hardware provisioning.
