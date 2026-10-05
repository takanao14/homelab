# ADR-0066: Expose Hermes Dashboard through Caddy

- **Status:** Accepted
- **Date:** 2026-10-05
- **Related:** [ADR-0064](0064-hermes-evaluation-on-dedicated-vm.md), [ADR-0065](0065-agent-mcp-caller-credentials.md)

## Context

The evaluation VM initially exposed only an SSH CLI. Browser access needs the
same VPN and Caddy path as OpenClaw, with authenticated Dashboard APIs and chat
WebSockets. The Dashboard can edit agent settings, files, and secrets.

## Decision

Run the pinned Hermes image as a systemd-managed Podman Dashboard container.
Expose `https://hermes.home.butaco.net` through Caddy. Publish VM loopback and
net20 port `9119`; nftables restricts net20 access to Caddy. Retain native
username/password authentication and set the external public URL explicitly.
Store the password, scrypt hash, and session-signing key in SOPS; deploy only
the hash and signing key in a root-owned `0600` environment file. No Nous OAuth
registration is required. Both browser chat and SSH CLI use the same container
and state directory. MCP credential changes restart the Dashboard.

## Consequences

Dashboard login grants administrative access to the Hermes instance. VM backups
include the runtime credential file and state. Ansible remains authoritative for
managed settings and can replace Dashboard edits on redeployment. Avoid
concurrent inference requests when comparing agents on the shared GPU.
