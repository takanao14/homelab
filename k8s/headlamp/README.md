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

Two separate settings are involved, and it is easy to assume the first does the
second's job:

- `-unsafe-use-service-account-token` is what removes the token prompt. Headlamp
  reaches the API server as its own ServiceAccount for every visitor. Upstream
  calls it unsafe because by itself it gives cluster-admin to anyone who can
  reach the Service, so it is only sound behind the forward auth above — enable
  and disable the two together.
- `-proxy-auth=true` with authentik's header names does **not** bypass the login
  screen. In v0.44.0 it only feeds `/clusters/{name}/me`, so the top bar shows
  who Authentik authenticated, and it gates `-proxy-auth-token-header`, which is
  unused here because forwarding a token to the API server needs the OIDC work
  deferred by decision 19.

Client-supplied identity headers cannot be trusted on their own: the defence is
that `headersToBackend` overwrites coexisting headers, so a forged
`X-authentik-username` is replaced by the outpost's value. Do not add an
HTTPRoute `RequestHeaderModifier` to remove those headers — route-level header
mutation runs after ext_authz and would strip the injected values too.

A NetworkPolicy closes the path around all of this. ext_authz only guards
traffic arriving through the Gateway, so any pod that could reach the Headlamp
Service directly would get a cluster-admin session without authenticating.
Ingress is therefore limited to the proxy pods of the Gateway named in the
HTTPRoute. kubelet probes still work: Cilium exempts host-to-pod traffic while
the host firewall is off, which is the default here.

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

## Login Token (removed)

There is no login token any more. The `headlamp-token` Secret — a long-lived
`kubernetes.io/service-account-token` for the `cluster-admin` `headlamp`
ServiceAccount — was deleted from prd on 2026-08-20; sandbox had already lost it
in a rebuild and ran without it. **Do not recreate it.**

Forward auth does not protect that Secret: the gate sits in front of Headlamp,
not in front of kube-apiserver, so anyone holding the token could have used it
against the API directly. Removing it is the part of the ADR-0015 cleanup that
did not have to wait for OIDC (stage 13 of the identity plan).

Nothing consumes it: Headlamp reaches the API server with the projected,
pod-bound token of its own ServiceAccount, which is short-lived and rotated by
the kubelet. Access is granted by Authentik group membership instead — see
`ansible/roles/authentik/files/blueprints/proxy.yaml`.

The `headlamp` ServiceAccount is still bound to `cluster-admin` by the chart,
which is why the forward auth and NetworkPolicy above are load-bearing. Splitting
that binding into per-user roles needs OIDC (decision 19).

## Design Note

Headlamp moved from a prd multi-cluster kubeconfig design to per-cluster
instances, allowing App of Apps alone to restore it. See
[ADR-0015](../../docs/adr/0015-headlamp-per-cluster-in-cluster-deployment.md)
for the rationale. OpenBao kubeconfigs remain workstation-only.
