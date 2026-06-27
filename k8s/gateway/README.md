# gateway

Local Helm chart that creates shared Gateway API Gateway resources.

Managed by ArgoCD. Each environment has its own ArgoCD Application in
`k8s/argocd/{env}/apps/gateway.yaml`.

## Directory Structure

```
gateway/
├── Chart.yaml
├── values.yaml          # GatewayClass and Gateway definitions
└── templates/
    ├── gatewayclasses.yaml
    └── gateways.yaml
```

## Resources Created

### GatewayClass

Optional `GatewayClass` resources can be rendered from `gatewayClasses`.
The Cilium `GatewayClass/cilium` is owned by the Cilium Helm release, so this
chart does not render it. Sandbox renders `GatewayClass/envoy-gateway` for the
Envoy Gateway migration.

### Gateway

Gateway resources are rendered from `gateways`. The default render preserves the
existing Cilium Gateway:

```yaml
name: shared-gateway
namespace: gateway-system
gatewayClassName: cilium
```

Sandbox renders the Envoy Gateway during the migration:

```yaml
name: shared-gateway-envoy
namespace: gateway-system
gatewayClassName: envoy-gateway
```

### EnvoyProxy

Envoy Gateway proxy settings can be rendered from `envoyProxies` and attached
to a Gateway through `gateway.infrastructure.parametersRef`. Sandbox uses this
to set the generated Envoy proxy LoadBalancer Service to
`externalTrafficPolicy: Cluster`, avoiding Cilium L2 announcement problems when
the VIP is advertised by a node that does not host the Envoy proxy pod.

| Listener | Port | Protocol | TLS Secret |
|----------|------|----------|------------|
| https | 443 | HTTPS | `wildcard-{domain-dashes}-tls` in `cert-manager` |
| http | 80 | HTTP | — |

The TLS secret is referenced cross-namespace via a `ReferenceGrant` created by the `cert-manager` chart.

## Values

| Key | Description |
|-----|-------------|
| `domain` | Base domain for the environment (e.g. `prd.butaco.net`) |
| `gatewayClasses` | Optional GatewayClass definitions to render. |
| `envoyProxies` | Optional EnvoyProxy definitions for Envoy Gateway infrastructure settings. |
| `gateways` | Gateway definitions, including name, namespace, class, and listeners. |

`domain` has no default value and must be explicitly provided. It is used to construct the TLS secret name.

> `butaco.net` is a personal domain. Replace it in `k8s/argocd/{env}/apps/gateway.yaml`.

## Notes

- `GatewayClass/cilium` is owned by the Cilium Helm release.
- Requires Gateway API CRDs v1.4.1 experimental
- Sandbox HTTPRoutes reference `shared-gateway-envoy` after the migration PoC.
- Dev/prd services continue to reference `shared-gateway` until those
  environments are migrated.
