# Butako voice chat

The prd Argo CD Application deploys the Go/TypeScript app, VOICEVOX CPU
engine, Go search MCP service, and SearXNG in the `butako-voice` namespace.
The HTTPRoute exposes only the app at
`https://butako.prd.butaco.net`; ExternalDNS creates its PowerDNS record.
The app calls Lemonade, VOICEVOX, and search MCP through ClusterIP Services.
Search MCP calls SearXNG, which contacts public search engines; the MCP
service also fetches selected public result pages. NetworkPolicies limit
ingress to search MCP from the app and to SearXNG from search MCP. Neither
search component has an HTTPRoute or PVC.

`values.yaml` pins the app release tag and the official VOICEVOX 0.25.2 amd64
digest. VOICEVOX's entrypoint prints requests containing the synthesis text,
so the container redirects stdout and stderr to `/dev/null`. Inspect Pod and
node log files after deployment to confirm no conversation text is retained.

The app and search images share release tag `v0.2.0`. The GitHub tag workflow
publishes both images, including the separate `Dockerfile.search` build.
SearXNG uses a fixed upstream digest and accepts JSON searches. Its secret
key is seeded from the SOPS-encrypted OpenBao inventory at
`secret/k8s/butako-voice/searxng` and synced by External Secrets. Apply the
OpenBao policy and prd Kubernetes auth role before syncing the chart, then
seed the encrypted inventory through `ops-openbao_seed_secrets.yaml`. Do not
write the key manually to OpenBao. Check `ExternalSecret/searxng-secret` is
Ready before expecting SearXNG to start.

From `ansible/`, apply the OpenBao changes in this order, using the normal
inventory and SOPS key setup:

```sh
ansible-playbook playbooks/ops-openbao_configure.yaml
ansible-playbook playbooks/ops-openbao_register_cluster.yaml -e cluster=prd
ansible-playbook playbooks/ops-openbao_seed_secrets.yaml
```

The route, API, and browser use a 120-second conversation deadline. The API
rejects input over 2 MiB and responses over 2 MiB. The UI requires HTTPS for
Safari microphone access. A SecurityPolicy denies requests outside
`192.168.10.0/24`; confirm the Gateway sees the expected client IP before
accepting the unauthenticated route.
