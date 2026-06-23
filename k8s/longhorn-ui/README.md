# longhorn-ui

Authenticated Gateway API route for the Longhorn UI.

Longhorn itself is installed by the k0s bootstrap Helmfile. This chart only
adds a small nginx reverse proxy with Basic Auth and exposes it through the
shared Cilium Gateway.

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
