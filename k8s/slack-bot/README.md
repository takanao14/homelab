# slack-bot

Slack Socket Mode bot that renders channel messages and emojis to the LED matrix
on rpi3. Source and image live in
[takanao14/slack-bot](https://github.com/takanao14/slack-bot); this directory
only deploys a published tag. See
[ADR-0045](../../docs/adr/0045-deploy-slack-bot-from-its-own-repository.md).

App of Apps deploys it to prd at wave 1. **Do not enable it in sandbox**: both
clusters would open a Socket Mode connection with the same app token and Slack
would deliver each event to only one of them, so sandbox would silently render
part of the production traffic. A sandbox deployment needs its own Slack app.

## Layout

```text
slack-bot/
├── chart/
│   ├── Chart.yaml
│   ├── values.yaml         # Image, LED address, OpenBao secret coordinates
│   └── templates/
├── values.yaml             # Common resource reservations
└── prd/values.yaml         # LED address
```

## No routed traffic

The bot dials out to Slack and to the LED service; nothing routes requests to
it. There is no HTTPRoute and no NetworkPolicy — the ClusterIP Service exists
only so Prometheus can scrape `/metrics`, and it fronts no authentication
boundary worth defending. `/healthz` is reached by the kubelet liveness probe.
Add a NetworkPolicy if metrics gain channel or user labels, or when the cluster
adopts default-deny.

`SLACK_BOT_LED_ADDR` is an IP because the LED service runs outside the cluster
on rpi3 (`192.168.10.240:50051`) over plaintext gRPC, and cluster DNS cannot
complete the short hostname used on the LAN.

## Secrets

`SLACK_BOT_TOKEN` and `SLACK_APP_TOKEN` come from OpenBao
`secret/k8s/slack-bot/tokens` (`bot-token`, `app-token`) through ESO. Seed them
from the encrypted `openbao_secrets` inventory with
`ops-openbao_seed_secrets.yaml`; do not use manual `bao kv put`. Reading them
requires the `k8s-slack-bot` policy on the prd Kubernetes auth role.

Reloader restarts the Deployment when the Secret changes.

## Monitoring

Pod-level failures are already covered by the kube-prometheus-stack defaults
(`KubePodCrashLooping`, `KubePodNotReady`). The chart adds what those cannot
see, because the process stays healthy while the function stops:

| Alert | Condition |
|---|---|
| `SlackBotSocketDisconnected` | `slack_bot_socket_connected == 0` for `alerts.socketDisconnectedFor` |
| `SlackBotLedSendFailing` | any failed LED send in 15 minutes |

There is deliberately no alert on zero rendered messages; a quiet night is
indistinguishable from a stalled pipeline.

The LED service itself is probed separately from rpi4
(`TcpServiceUnreachable` for `led-rpi3`), so a send failure can be attributed
to the device or to the bot.

## Image release

The Deployment uses the image repository and tag defined by `image.repository`
and `image.tag` in `chart/values.yaml`.

The public image needs no pull Secret. Renovate updates `image.tag` after the
source repository publishes a supported release tag.

## Render

Render with the same values used by Argo CD:

```bash
helm template slack-bot k8s/slack-bot/chart \
  --namespace slack-bot \
  -f k8s/slack-bot/values.yaml \
  -f k8s/slack-bot/prd/values.yaml
```

Before changing values, verify the live Argo CD value-file contract:

```bash
kubectl -n argocd get application slack-bot \
  -o jsonpath='{.spec.sources[*].helm.valueFiles}'
```

## Runtime properties

- One replica. Rolling updates are safe: Slack allows the app up to 10 Socket
  Mode connections and delivers each event to exactly one of them.
- `terminationGracePeriodSeconds: 60` must stay above
  `SLACK_BOT_LED_OPERATION_TIMEOUT_SECONDS` (image default 30); shutdown drains
  an in-flight LED send before closing the font face and gRPC connection.
- `SLACK_BOT_FONT_PATH` stays unset so the font bundled in the image is used.
- Non-root distroless container with a read-only root filesystem, all Linux
  capabilities dropped, and no ServiceAccount token mounted.
- Initial requests are `10m` CPU and `32Mi` memory, with no limits. Replace
  these with seven-day observations after deployment.
