# ADR-0065: Connect VM agents to shared read-only MCP servers

- **Status:** Accepted
- **Date:** 2026-10-05
- **Related:** [ADR-0057](0057-cluster-hosted-authenticated-mcp.md), [ADR-0060](0060-mcp-relies-on-bearer-authentication.md), [ADR-0063](0063-openclaw-on-dedicated-vm-with-lemonade.md), [ADR-0064](0064-hermes-evaluation-on-dedicated-vm.md)

## Context

OpenClaw and Hermes need identical Grafana and NetBox tools for evaluation.
Existing cluster MCP servers authenticate callers separately from their upstream
API credentials. Each server currently accepts one shared caller token.

## Decision

Use native HTTPS MCP clients with an explicit shared tool allowlist. Retain the
minimal OpenClaw profile and Hermes write approvals. Enable public Web search and page retrieval through native tools, using
Parallel's anonymous free tier for search and Hermes extraction. Queries and
Hermes extraction URLs leave the lab; private-address protections remain enabled.
Pin the OpenClaw search plugin version and archive checksum. Add no shell tools or MCP
resource and prompt utilities. The shared Ansible `mcp_client` role decrypts the
existing SOPS source on the controller and distributes only caller tokens to
root-owned `0600` files under `/etc/openclaw` and `/etc/hermes`. Podman loads the
files into the container environment; agent configs contain variable references.
VMs receive no AGE keys or upstream API credentials and do not contact OpenBao
at startup. Suppress secret task logs and diffs; do not cache decrypted facts.

## Consequences

VM disks and their backups contain plaintext caller tokens, accessible to root.
Container administrators can inspect runtime credentials. Tokens shared by the
clients cannot be revoked per agent. Rotation requires updating the servers and
both VMs; pause evaluations during the mismatch. OpenClaw restarts on credential
changes, and Hermes reads them on its next launch. MCP outages interrupt tool
access without changing GPU workload selection.
