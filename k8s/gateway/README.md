# gateway

Local Helm chart that creates shared Gateway API Gateway resources.

App of Apps manages the chart. Shared definitions live in `values.yaml` and
environment files set the domain and HTTPS toggle.

## Directory Structure

```
gateway/
├── Chart.yaml
├── values.yaml          # Shared GatewayClass / EnvoyProxy / Gateway definitions
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

cert-manager grants cross-namespace TLS Secret access. Environment values toggle
listeners; sandbox disables HTTPS.

### EnvoyProxy

`gateway.infrastructure.parametersRef` attaches `envoyProxy`. Its LoadBalancer
uses Cluster traffic policy to avoid Cilium L2 advertisement blackholes.

## Values

| Key | Description |
|-----|-------------|
| `domain` | Base domain for the environment (e.g. `prd.butaco.net`) |
| `gatewayClass` | GatewayClass definition (name, controllerName). |
| `envoyProxy` | EnvoyProxy definition for Envoy Gateway infrastructure settings. |
| `gateway` | Gateway definition: name, namespace, class, infrastructure, and listener toggles. |

`domain` is required and determines the TLS Secret name.

> `butaco.net` is a personal domain. Replace it in `k8s/gateway/{env}/values.yaml`.

## Notes

- `GatewayClass/cilium` no longer exists: Cilium's ingress and Gateway API
  controllers are disabled in `k0s/values/cilium.yaml.gotmpl`.
- Gateway API CRDs are owned by the `envoy-gateway-crds` ArgoCD app
  (`k8s/envoy-gateway/crds`), which bundles the version matching the pinned
  Envoy Gateway chart (1.8.x → Gateway API v1.5.1 experimental).
- Both environments reference `shared-gateway-envoy`.
- See
  [`ADR-0011`](../../docs/adr/0011-cilium-gateway-to-envoy-gateway-migration.md)
  for the migration decision and rejected alternatives.
