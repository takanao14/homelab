# gateway

Creates shared Gateway API resources. `values.yaml` defines them; environment
files set the domain and HTTPS toggle.

## Directory Structure

```
gateway/
├── Chart.yaml
├── values.yaml          # Shared GatewayClass / EnvoyProxy / Gateway definitions
├── prd/values.yaml      # domain: prd.butaco.net
├── sandbox/values.yaml  # domain: sandbox.butaco.net, HTTP listener, pinned address
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

`gateway.listener` selects the environment's only listener, so HTTPRoutes omit
`sectionName` (ADR-0072). prd has no port 80 and no HTTP redirect.

| `gateway.listener` | Environment | Port | Protocol | TLS Secret |
|--------------------|-------------|------|----------|------------|
| `https` | prd | 443 | HTTPS | `wildcard-{domain-dashes}-tls` in `cert-manager` |
| `http` | sandbox | 80 | HTTP | — |

cert-manager grants cross-namespace Secret access.

### EnvoyProxy

`gateway.infrastructure.parametersRef` attaches `envoyProxy`. Cluster traffic
policy avoids Cilium L2 advertisement blackholes. Sandbox pins the Service to
`192.168.20.128` with `io.cilium/lb-ipam-ips` for its wildcard DNS record.

## Values

| Key | Description |
|-----|-------------|
| `domain` | Base domain for the environment (e.g. `prd.butaco.net`) |
| `gatewayClass` | GatewayClass definition (name, controllerName). |
| `envoyProxy` | EnvoyProxy definition for Envoy Gateway infrastructure settings. |
| `gateway` | Gateway definition: name, namespace, class, infrastructure, and `listener` (`https` or `http`). |

`domain` is required and determines the TLS Secret name.

## Notes

- Cilium ingress and Gateway controllers are disabled; `GatewayClass/cilium`
  is retired.
- `envoy-gateway-crds` owns the CRDs matching the pinned controller version.
- Both environments reference `shared-gateway-envoy`.
- See
  [`ADR-0011`](../../docs/adr/0011-cilium-gateway-to-envoy-gateway-migration.md)
  for the migration decision and rejected alternatives.
