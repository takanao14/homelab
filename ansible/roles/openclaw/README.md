# OpenClaw

`openclaw1` runs Ubuntu 26.04 and the pinned OpenClaw image through a
systemd-managed Podman container. Open the Web UI at
`https://openclaw.home.butaco.net` over VPN.
The VM firewall admits Gateway traffic only from Caddy. SSH forwarding remains
available for recovery:

```sh
ssh -L 18789:127.0.0.1:18789 takanao@192.168.20.23
```

Browse to `http://127.0.0.1:18789` through the tunnel, or use the Caddy URL,
and enter the Gateway token from the
SOPS-encrypted `openclaw.sops.yaml` inventory file. Do not print the token in
logs or copy it to another repository. The token authenticates the UI; the
`sk-local` provider marker is not a Lemonade credential.
Approve a new browser's device pairing on the VM after entering the token.

OpenClaw uses Lemonade's OpenAI-compatible endpoint and
`Gemma-4-12B-it-MTP-GGUF`. The initial `minimal` tool profile permits chat
without filesystem or shell tools. The Gateway state, including sessions, is
stored under `/var/lib/openclaw`; back it up before replacing the VM.

Lemonade must be selected in gpu-switch before a model request can succeed.
Switching the GPU to another workload, or completing a planned shutdown,
leaves OpenClaw unable to answer until Lemonade is selected again.

Deploy with `ansible-playbook playbooks/services/openclaw.yaml` from `ansible/`.
Afterward, check the systemd unit, `openclaw security audit`, a direct model
response, and a Web UI conversation.
