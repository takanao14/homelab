# longhorn-ui

Authenticated Gateway API route for the Longhorn UI.

Longhorn itself is installed by the k0s bootstrap Helmfile. This chart exposes
the UI through Gateway API. It can use the legacy nginx Basic Auth proxy or,
during the Envoy Gateway migration, Envoy Gateway `SecurityPolicy` Basic Auth.

## Ownership model

Longhorn is cluster infrastructure and remains part of the `k0s/` bootstrap
layer. This chart is intentionally limited to the UI exposure layer:

- `ExternalSecret` for Basic Auth htpasswd content
- optional nginx reverse proxy with `auth_basic`
- optional `Service` for the proxy
- optional Envoy Gateway `SecurityPolicy`
- `HTTPRoute` attached to the configured shared Gateway

Do not move the Longhorn Helm release itself into this chart unless the storage
bootstrap ownership model changes.

## Sandbox route

```text
http://longhorn.sandbox.butaco.net
  -> gateway-system/shared-gateway-envoy:http
  -> longhorn-system/longhorn-ui-proxy
  -> longhorn-system/longhorn-frontend:80
```

The initial sandbox Envoy Gateway canary keeps the nginx proxy in the backend
path. After Envoy Gateway is validated, `securityPolicy.enabled=true`,
`proxy.enabled=false`, and `route.backend.name=longhorn-frontend` move Basic
Auth to the Gateway layer and remove the per-service proxy.

## Secret

The Basic Auth htpasswd content is read by External Secrets Operator from
OpenBao:

```text
secret/k8s/longhorn-ui/basic-auth
  htpasswd: <htpasswd content>
```

Example value format:

```text
admin:{SHA}...
```

Generate it locally with:

```bash
htpasswd -nbs admin '<password>'
```

Envoy Gateway `SecurityPolicy` Basic Auth validates htpasswd entries in `{SHA}`
format. The previous nginx proxy accepted bcrypt/apr1 entries, but Envoy
Gateway rejects them.

OpenBao values are seeded from the encrypted Ansible inventory. Update the
source of truth with SOPS, then seed OpenBao:

```bash
sops ansible/inventories/homelab/group_vars/openbao.sops.yaml
cd ansible
ansible-playbook playbooks/ops-openbao_seed_secrets.yaml
```

Do not keep manual `bao kv put` changes as the final state; the next Ansible
seed would overwrite them.

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
- DNS resolves `longhorn.sandbox.butaco.net` to the configured sandbox Gateway IP.
- unauthenticated `curl` returns `HTTP/1.1 401 Unauthorized` with
  `WWW-Authenticate: Basic realm="Longhorn UI"`.
