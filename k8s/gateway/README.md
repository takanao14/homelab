# gateway

Local Helm chart that creates a shared Cilium Gateway API gateway for TLS termination.

Managed by ArgoCD. Each environment (prd/dev) has its own ArgoCD Application in `k8s/argocd/{env}/apps/gateway.yaml` with the domain set inline.

## Directory Structure

```
gateway/
├── Chart.yaml
├── values.yaml          # Schema: domain (no default — must be set explicitly)
└── templates/
    ├── gatewayclass.yaml  # GatewayClass: cilium
    └── gateway.yaml       # shared-gateway with HTTPS + HTTP listeners
```

## Resources Created

### GatewayClass

```
name: cilium
controllerName: io.cilium/gateway-controller
```

### Gateway

```
name: shared-gateway
namespace: gateway-system
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

`domain` has no default value and must be explicitly provided. It is used to construct the TLS secret name.

> `butaco.net` is a personal domain. Replace it in `k8s/argocd/{env}/apps/gateway.yaml`.

## Notes

- Cilium 1.19.x does NOT auto-create a GatewayClass — it is created explicitly by this chart
- Requires Gateway API CRDs v1.4.1 experimental
- Each service exposes itself via HTTPRoute referencing `shared-gateway`
