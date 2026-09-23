# ADR-0054: Update the ComfyUI image through pinned tags and CI-opened pull requests

- **Status:** Accepted
- **Date:** 2026-09-23
- **Related:** [ADR-0014](0014-argocd-app-of-apps-shared-helm-chart.md),
  [ADR-0027](0027-gpu-workload-switching-web-ui.md),
  [ADR-0045](0045-deploy-slack-bot-from-its-own-repository.md),
  [`k8s/comfyui/README.md`](../../k8s/comfyui/README.md)

## Context

The comfyui chart deployed `comfyui-docker:latest` with the implicit `Always`
pull policy, and the image cloned ComfyUI and ComfyUI-Manager from their
default branches. Every gpu-switch start therefore pulled about 3 GB and could
run a different, unrecorded build; nothing rebuilt the image when upstream
released; and there was no version to roll back to.

ADR-0027 keeps this multi-gigabyte ROCm image on the Forgejo self-hosted runner
and LAN registry. Mend-hosted Renovate, which drives this repository, cannot see
either, so the ADR-0045 pattern of a Renovate bump of a published tag does not
apply. A self-hosted Renovate already runs on Forgejo (`takanao/renovate`) but
only against Forgejo repositories.

## Decision

**Pin the image source, publish immutable tags, and let the build open the
deployment pull request here.**

1. `comfyui-docker` pins `COMFYUI_VERSION`; the Forgejo self-hosted Renovate
   bumps it weekly. ComfyUI-Manager is installed from ComfyUI's own
   `manager_requirements.txt`, so it needs no separate pin.
2. Each build pushes `<COMFYUI_VERSION>-<short sha>`.
3. The same workflow force-pushes one rolling branch here that sets
   `image.tag` and opens or refreshes a `manual-review` pull request, using a
   fine-grained PAT limited to this repository. Merging it is the deployment.
4. The chart pulls with `IfNotPresent` and keeps inputs, outputs, user data, and
   custom nodes on a second PVC, so recreating the pod no longer loses them.

## Alternatives considered

- **Move the image to GitHub Actions and ghcr.io** (ADR-0045). Rejected by
  ADR-0027's capacity constraint.
- **A second, self-hosted Renovate against this repository.** It would need the
  same GitHub credential as the chosen option while colliding with Mend
  Renovate's dashboard and branches. *Rejected.*
- **Argo CD Image Updater.** Adds a controller and puts a GitHub write
  credential in the cluster to replace one pull request per build. *Rejected.*
- **Keep `latest` and restart to update.** No record of what runs and no
  rollback. *Rejected.*

## Consequences

- A GitHub PAT with write access to this repository lives in Forgejo Actions
  secrets and must be rotated before it expires.
- Only the newest unmerged build has an open pull request; older ones are
  replaced.
- The torch/ROCm wheel trio is still bumped by hand in `comfyui-docker`.
- Custom-node pip dependencies are not persisted; nodes that need them must be
  baked into the image.
