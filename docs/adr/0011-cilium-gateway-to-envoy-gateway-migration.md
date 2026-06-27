# ADR-0011: Use Envoy Gateway for shared Gateway API ingress

- **Status:** Accepted
- **Date:** 2026-06-27
- **Related:** [`k8s/README.md`](../../k8s/README.md), [`k8s/gateway/README.md`](../../k8s/gateway/README.md), [`k8s/longhorn-ui/README.md`](../../k8s/longhorn-ui/README.md)

## Context

The homelab clusters originally used Cilium Gateway API support for shared
ingress. Cilium remains the CNI and continues to provide Kubernetes networking,
load-balancer IP handling, and L2 announcement, but the Gateway API controller
responsibility needed to move to Envoy Gateway.

The migration was motivated by a few long-lived requirements:

- use Envoy Gateway features such as `SecurityPolicy` for Longhorn UI basic
  authentication;
- keep Gateway API behavior consistent across sandbox, dev, and prd;
- separate ingress-controller concerns from Cilium CNI concerns;
- validate the new ingress path in sandbox before touching dev and prd;
- remove temporary dual-Gateway state after the cutover.

## Decision

Use Envoy Gateway as the Gateway API controller for shared application ingress
in sandbox, dev, and prd.

Each migrated cluster uses:

- `GatewayClass/envoy-gateway`;
- `Gateway/gateway-system/shared-gateway-envoy`;
- `EnvoyProxy/gateway-system/shared-gateway-envoy`;
- HTTPRoutes that reference `shared-gateway-envoy`;
- Gateway API CRDs v1.5.1 experimental;
- Envoy Gateway 1.8.x.

Cilium remains installed and remains the CNI. The Cilium-owned
`GatewayClass/cilium` may remain if it is managed by the Cilium Helm release,
but application HTTPRoutes should not target a Cilium Gateway, and the old
`Gateway/gateway-system/shared-gateway` should not exist after migration.

The shared Envoy proxy LoadBalancer Service uses
`externalTrafficPolicy: Cluster`. This is kept as the normal post-migration
configuration because Cilium L2 announcement can advertise a VIP from a node
that does not host the Envoy proxy pod. Cluster traffic policy keeps the VIP
reachable regardless of which node announces it.

## Migration record

The migration was executed in this order:

1. Prove the Envoy Gateway path in sandbox with HTTP-only routing.
2. Migrate Longhorn UI in sandbox, replacing the proxy-side basic auth pattern
   with Envoy Gateway `SecurityPolicy` basic auth.
3. Migrate sandbox Argo CD and verify direct service access through Envoy.
4. Migrate dev by adding Envoy Gateway, switching HTTPRoutes to
   `shared-gateway-envoy`, verifying service behavior, then deleting the old
   Cilium Gateway.
5. Migrate prd with the same sequence as dev.
6. Remove temporary GitOps ignore rules and Cilium Gateway references from chart
   defaults after all clusters were on Envoy.

## Alternatives considered

- **Continue using Cilium Gateway for ingress** — simpler operationally, but
  keeps Gateway API controller behavior tied to Cilium and does not provide the
  Envoy Gateway policy surface needed by Longhorn UI. *Rejected.*
- **Switch every environment in one step** — faster, but makes DNS, L2
  announcement, TLS, and route-controller issues harder to isolate. *Rejected.*
- **Keep Cilium Gateway and Envoy Gateway in parallel long term** — useful
  during migration, but creates ambiguous defaults and makes HTTPRoute ownership
  harder to reason about. *Rejected.*
- **Remove Cilium entirely** — outside the scope of this change. Cilium remains
  the CNI and load-balancer/L2 announcement provider. *Rejected.*

## Consequences

- New HTTPRoutes should default to `shared-gateway-envoy`.
- The shared Gateway chart owns Envoy Gateway API resources and no longer
  renders the old Cilium `shared-gateway`.
- Temporary `ignoreDifferences` rules for Gateway API defaulting should not be
  used for migrated routes. Templates should render explicit `group`, `kind`,
  and `weight` fields when needed to avoid drift.
- External Secrets Operator defaulted fields should be rendered explicitly in
  charts where Argo CD diff would otherwise remain noisy.
- Sandbox intentionally remains HTTP-only, as described in ADR-0010.
- For Envoy Gateway basic authentication, htpasswd data must use a supported
  `{SHA}` format.
