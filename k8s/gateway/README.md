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
chart does not render it. The migrated clusters render
`GatewayClass/envoy-gateway` for Envoy Gateway.

### Gateway

Gateway resources are rendered from `gateways`. The chart default renders the
shared Envoy Gateway:

```yaml
name: shared-gateway-envoy
namespace: gateway-system
gatewayClassName: envoy-gateway
```

Environments may still override `gateways` to select listeners, domains, or
future per-cluster infrastructure settings.

### EnvoyProxy

Envoy Gateway proxy settings can be rendered from `envoyProxies` and attached
to a Gateway through `gateway.infrastructure.parametersRef`. Migrated
environments use this to set the generated Envoy proxy LoadBalancer Service to
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
- Requires Gateway API CRDs v1.5.1 experimental for Envoy Gateway 1.8.x.
- Sandbox, dev, and prd HTTPRoutes reference `shared-gateway-envoy` after the
  migration.
