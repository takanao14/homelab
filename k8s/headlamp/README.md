# Headlamp

In-cluster Kubernetes UI for each environment, authenticated by ServiceAccount
RBAC without cross-cluster kubeconfig Secrets.

## Directory Structure

```
headlamp/
├── chart/
│   ├── Chart.yaml        # Wrapper chart with headlamp as dependency
│   ├── values.yaml       # Common values (in-cluster mode, HTTPRoute, forward auth)
│   └── templates/
│       └── forward-auth.yaml  # Backend + outpost HTTPRoute + SecurityPolicy
├── prd/values.yaml       # hostname: headlamp.prd.butaco.net, https listener
└── sandbox/values.yaml   # hostname: headlamp.sandbox.butaco.net, http listener (ADR-0010)
```

App of Apps enables Headlamp per environment.

## Access

- prd: https://headlamp.prd.butaco.net
- sandbox: http://headlamp.sandbox.butaco.net

## Authentication (forward auth)

Both environments sit behind Authentik. Envoy Gateway calls the Authentik proxy
outpost on `authentik1` over ext_authz, so every request on the Headlamp
hostname is authenticated before it reaches the Service, and there is no second
entry point to guard. Admission is by Authentik group
(`files/blueprints/proxy.yaml` in the `authentik` role).

```text
browser -> Envoy Gateway -> /outpost.goauthentik.io/*  -> authentik1:9100 (login/callback)
                         -> /  --ext_authz--> authentik1:9100
                                200 + X-authentik-* -> Headlamp Service
                                302                 -> auth.home.butaco.net
```

Headlamp runs with `-proxy-auth=true` and reads authentik's own header names, so
the token prompt is gone. Client-supplied identity headers cannot be trusted on
their own: the defence is that `headersToBackend` overwrites coexisting headers,
so a forged `X-authentik-username` is replaced by the outpost's value. Do not add
an HTTPRoute `RequestHeaderModifier` to remove those headers — route-level header
mutation runs after ext_authz and would strip the injected values too.

Requirements and limits:

- Envoy Gateway **1.7.1 or newer**. Envoy 1.37.0 stripped the `Location` header
  from ext_authz 302 responses, which breaks the login redirect
  (envoyproxy/gateway#8202, fixed in EG v1.7.1 / Envoy 1.37.1).
- `extensionApis.enableBackend: true` on the controller, since the outpost lives
  outside the cluster (`k8s/envoy-gateway/controller/values.yaml`).
- `failOpen: false` — an Authentik outage takes Headlamp down rather than opening
  it. The break-glass path is the X.509 admin kubeconfig, which does not depend
  on Authentik.
- Headlamp still reaches the API server as its own `cluster-admin`
  ServiceAccount, so admission is all-or-nothing and the API server audit log
  sees one identity. Per-user RBAC needs OIDC, deferred by decision 19.

See `docs/plans/identity-authentication-architecture.md` decision 20.

## Login Token (per cluster)

**Obsolete once forward auth is verified, and worth deleting.** With
`-proxy-auth=true` the login screen is skipped, so nothing consumes this Secret.
It remains a long-lived `cluster-admin` credential that works directly against
the API server, which forward auth does not protect — the gate is in front of
Headlamp, not in front of kube-apiserver. Deleting it is the part of the
ADR-0015 cleanup that does not have to wait for OIDC (stage 13).

The chart binds `headlamp` to `cluster-admin`. After sync, create its login token:

```bash
kubectl apply -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: headlamp-token
  namespace: headlamp
  annotations:
    kubernetes.io/service-account.name: headlamp
type: kubernetes.io/service-account-token
EOF

# Retrieve the token
kubectl get secret headlamp-token -n headlamp \
  -o jsonpath='{.data.token}' | base64 -d
```

To revoke access, delete and recreate the Secret:

```bash
kubectl delete secret headlamp-token -n headlamp
```

## Design Note

Headlamp moved from a prd multi-cluster kubeconfig design to per-cluster
instances, allowing App of Apps alone to restore it. See
[ADR-0015](../../docs/adr/0015-headlamp-per-cluster-in-cluster-deployment.md)
for the rationale. OpenBao kubeconfigs remain workstation-only.
