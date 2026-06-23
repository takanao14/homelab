# gateway

Local Helm chart that creates a shared Cilium Gateway API Gateway.

Managed by ArgoCD. Each environment has its own ArgoCD Application in
`k8s/argocd/{env}/apps/gateway.yaml`.

## Directory Structure

```
gateway/
├── Chart.yaml
├── values.yaml          # Domain and listener settings
└── templates/
    └── gateway.yaml      # shared-gateway with configurable listeners
```

## Resources Created

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
| `listeners.http.enabled` | Enable the HTTP listener. |
| `listeners.https.enabled` | Enable the HTTPS listener and certificate reference. |

`domain` has no default value and must be explicitly provided. It is used to construct the TLS secret name.

> `butaco.net` is a personal domain. Replace it in `k8s/argocd/{env}/apps/gateway.yaml`.

## Notes

- `GatewayClass/cilium` is owned by the Cilium Helm release.
- Requires Gateway API CRDs v1.4.1 experimental
- Each service exposes itself via HTTPRoute referencing `shared-gateway`
