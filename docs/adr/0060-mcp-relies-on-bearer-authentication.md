# ADR-0060: Rely on bearer authentication for MCP access

- **Status:** Accepted
- **Date:** 2026-10-02
- **Related:** [ADR-0057](0057-cluster-hosted-authenticated-mcp.md), [ADR-0011](0011-cilium-gateway-to-envoy-gateway-migration.md)

## Context

The MCP routes used Envoy Gateway source CIDR policies in addition to each
server's bearer token. The shared Gateway Service uses
`externalTrafficPolicy: Cluster` so Cilium L2 announcement remains reachable
when its leader has no Envoy Pod. Cross-node forwarding replaces the client
address with the leader's Cilium internal address, causing the CIDR policies
to deny authorized clients.

## Decision

Remove the MCP route source CIDR policies. Retain HTTPS, separate caller bearer
tokens, read-only MCP configuration, and Pod ingress limited to the Gateway.
Any client able to reach the Gateway VIP can attempt authentication; token
secrecy and rotation are the access boundary for these routes.

## Consequences

The MCP routes no longer distinguish LAN subnets at the Gateway. Verify that
requests without a token or with the other server's token are rejected before
accepting the deployment. The shared Gateway traffic policy remains unchanged.
