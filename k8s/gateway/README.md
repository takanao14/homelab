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
The Cilium `GatewayClass/cilium` is still owned by the Cilium Helm release, so
this chart does not render it. During the Envoy Gateway migration, sandbox
renders `GatewayClass/envoy-gateway`.

### Gateway

Gateway resources are rendered from `gateways`. The default render preserves the
existing Cilium Gateway:

```yaml
name: shared-gateway
namespace: gateway-system
gatewayClassName: cilium
```

Sandbox can also render a parallel Envoy Gateway:

```yaml
name: shared-gateway-envoy
namespace: gateway-system
gatewayClassName: envoy-gateway
```

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
| `gateways` | Gateway definitions, including name, namespace, class, and listeners. |

`domain` has no default value and must be explicitly provided. It is used to construct the TLS secret name.

> `butaco.net` is a personal domain. Replace it in `k8s/argocd/{env}/apps/gateway.yaml`.

## Notes

- `GatewayClass/cilium` is owned by the Cilium Helm release.
- Requires Gateway API CRDs v1.4.1 experimental
- Existing services expose themselves via HTTPRoute referencing `shared-gateway`
  until they are explicitly migrated to `shared-gateway-envoy`.
