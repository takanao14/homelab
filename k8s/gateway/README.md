# gateway

Local Helm chart that creates shared Gateway API Gateway resources.

Managed by ArgoCD. Each environment has its own ArgoCD Application in
`k8s/argocd/{env}/apps/gateway.yaml`. Shared definitions live in this chart's
`values.yaml`; per-environment differences (`domain`, and the HTTPS listener
toggle for sandbox) live in `{env}/values.yaml`.

## Directory Structure

```
gateway/
├── Chart.yaml
├── values.yaml          # Shared GatewayClass / EnvoyProxy / Gateway definitions
├── dev/values.yaml      # domain: dev.butaco.net
├── prd/values.yaml      # domain: prd.butaco.net
├── sandbox/values.yaml  # domain: sandbox.butaco.net, HTTPS listener disabled
└── templates/
    ├── gatewayclasses.yaml
    ├── envoyproxies.yaml
    └── gateways.yaml
```

## Resources Created

### GatewayClass

`GatewayClass/envoy-gateway` is rendered from `gatewayClass`.

### Gateway

The shared Gateway is rendered from `gateway`:

```yaml
name: shared-gateway-envoy
namespace: gateway-system
gatewayClassName: envoy-gateway
```

| Listener | Port | Protocol | TLS Secret |
|----------|------|----------|------------|
| https | 443 | HTTPS | `wildcard-{domain-dashes}-tls` in `cert-manager` |
| http | 80 | HTTP | — |

The TLS secret is referenced cross-namespace via a `ReferenceGrant` created by
the `cert-manager` chart. Listeners are toggled per environment via
`gateway.listeners.{http,https}.enabled` (sandbox disables https).

### EnvoyProxy

Envoy Gateway proxy settings are rendered from `envoyProxy` and attached to the
Gateway through `gateway.infrastructure.parametersRef`. The generated Envoy
proxy LoadBalancer Service uses `externalTrafficPolicy: Cluster`, avoiding
Cilium L2 announcement problems when the VIP is advertised by a node that does
not host the Envoy proxy pod.

## Values

| Key | Description |
|-----|-------------|
| `domain` | Base domain for the environment (e.g. `prd.butaco.net`) |
| `gatewayClass` | GatewayClass definition (name, controllerName). |
| `envoyProxy` | EnvoyProxy definition for Envoy Gateway infrastructure settings. |
| `gateway` | Gateway definition: name, namespace, class, infrastructure, and listener toggles. |

`domain` has no default value and must be explicitly provided. It is used to construct the TLS secret name.

> `butaco.net` is a personal domain. Replace it in `k8s/argocd/{env}/apps/gateway.yaml`.

## Notes

- `GatewayClass/cilium` no longer exists: Cilium's ingress and Gateway API
  controllers are disabled in `k0s/values/cilium.yaml.gotmpl`.
- Gateway API CRDs are owned by the `envoy-gateway-crds` ArgoCD app
  (`k8s/envoy-gateway/crds`), which bundles the version matching the pinned
  Envoy Gateway chart (1.8.x → Gateway API v1.5.1 experimental).
- Sandbox, dev, and prd HTTPRoutes reference `shared-gateway-envoy` after the
  migration.
- See
  [`ADR-0011`](../../docs/adr/0011-cilium-gateway-to-envoy-gateway-migration.md)
  for the migration decision and rejected alternatives.
