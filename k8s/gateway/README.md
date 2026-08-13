# gateway

Creates shared Gateway API resources. `values.yaml` defines them; environment
files set the domain and HTTPS toggle.

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

The shared Gateway is:

```yaml
name: shared-gateway-envoy
namespace: gateway-system
gatewayClassName: envoy-gateway
```

| Listener | Port | Protocol | TLS Secret |
|----------|------|----------|------------|
| https | 443 | HTTPS | `wildcard-{domain-dashes}-tls` in `cert-manager` |
| http | 80 | HTTP | — |

cert-manager grants cross-namespace Secret access. Sandbox disables HTTPS.

### EnvoyProxy

`gateway.infrastructure.parametersRef` attaches `envoyProxy`. Cluster traffic
policy avoids Cilium L2 advertisement blackholes.

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

- Cilium ingress and Gateway controllers are disabled; `GatewayClass/cilium`
  is retired.
- `envoy-gateway-crds` owns the CRDs matching the pinned controller version.
- Both environments reference `shared-gateway-envoy`.
- See
  [`ADR-0011`](../../docs/adr/0011-cilium-gateway-to-envoy-gateway-migration.md)
  for the migration decision and rejected alternatives.
