# longhorn-ui

Authenticated Gateway API route for the Longhorn UI.

Longhorn itself is installed by the k0s bootstrap Helmfile. This chart only
adds a small nginx reverse proxy with Basic Auth and exposes it through the
shared Cilium Gateway.

## Ownership model

Longhorn is cluster infrastructure and remains part of the `k0s/` bootstrap
layer. This chart is intentionally limited to the UI exposure layer:

- `ExternalSecret` for Basic Auth htpasswd content
- nginx reverse proxy with `auth_basic`
- `Service` for the proxy
- `HTTPRoute` attached to the shared Cilium Gateway

Do not move the Longhorn Helm release itself into this chart unless the storage
bootstrap ownership model changes.

## Sandbox route

```text
http://longhorn.sandbox.butaco.net
  -> gateway-system/shared-gateway:http
  -> longhorn-system/longhorn-ui-proxy
  -> longhorn-system/longhorn-frontend:80
```

## Secret

The Basic Auth htpasswd content is read by External Secrets Operator from
OpenBao:

```text
secret/k8s/longhorn-ui/basic-auth
  htpasswd: <htpasswd content>
```

Example value format:

```text
admin:$apr1$...
```

Generate it locally with:

```bash
htpasswd -nbB admin '<password>'
```

Then write it to OpenBao without committing the secret:

```bash
bao kv put secret/k8s/longhorn-ui/basic-auth htpasswd='<generated line>'
```

The sandbox OpenBao Kubernetes auth role must include the `k8s-longhorn-ui`
policy so ESO can read this path.

## Argo CD

The sandbox App of Apps deploys this chart through:

```text
k8s/argocd/sandbox/apps/longhorn-ui.yaml
```

The application syncs to the existing `longhorn-system` namespace. It uses sync
wave `2`, after the sandbox Gateway, ESO, and external-dns applications.

ExternalSecret default fields are pinned in the template to avoid Argo CD
permanent `OutOfSync` caused by ESO API defaulting:

```yaml
conversionStrategy: Default
decodingStrategy: None
metadataPolicy: None
nullBytePolicy: Ignore
```

The Argo CD Application also ignores Cilium/Gateway API defaulting on
`HTTPRoute.spec.parentRefs` and `HTTPRoute.spec.rules`, matching the pattern used
by the sandbox Argo CD route.

## Verification

After Argo CD syncs the application:

```bash
kubectl -n argocd get application longhorn-ui
kubectl -n longhorn-system get externalsecret longhorn-ui-basic-auth
kubectl -n longhorn-system get httproute longhorn-ui
dig longhorn.sandbox.butaco.net
curl -I http://longhorn.sandbox.butaco.net
```

Expected results:

- Argo CD reports `Synced` / `Healthy`.
- `ExternalSecret` reports `SecretSynced` and `READY=True`.
- DNS resolves `longhorn.sandbox.butaco.net` to the sandbox shared Gateway IP.
- unauthenticated `curl` returns `HTTP/1.1 401 Unauthorized` with
  `WWW-Authenticate: Basic realm="Longhorn UI"`.
