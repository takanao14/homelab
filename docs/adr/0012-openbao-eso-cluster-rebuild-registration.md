# ADR-0012: Re-register rebuilt clusters with OpenBao for ESO

- **Status:** Accepted
- **Date:** 2026-06-27
- **Related:** [`ansible/README.md`](../../ansible/README.md), [`ansible/roles/openbao/README.md`](../../ansible/roles/openbao/README.md), [`k8s/eso/README.md`](../../k8s/eso/README.md)

## Context

OpenBao runs outside the k0s clusters and persists its KV secret values across a
k0s cluster rebuild. Rebuilding a Kubernetes cluster therefore does not require
re-seeding application secrets into OpenBao.

What does change is the Kubernetes auth trust relationship used by External
Secrets Operator (ESO). A rebuilt cluster can have a new cluster CA and a new
service-account identity context, so OpenBao's per-cluster Kubernetes auth
configuration must be refreshed before ESO can authenticate and sync
`ExternalSecret` resources again.

The previous manual workflow required harvesting cluster trust data, editing
SOPS-encrypted OpenBao variables, and re-running the broader OpenBao
configuration. That made cluster rebuild recovery slower and mixed runtime
cluster identity with durable secret inventory.

## Decision

Keep OpenBao persistent and external to the Kubernetes clusters. After a k0s
cluster rebuild, refresh only the OpenBao Kubernetes auth configuration for the
rebuilt cluster.

The steady-state design is:

- ESO uses `ClusterSecretStore/openbao` with Kubernetes auth.
- Each cluster uses its own OpenBao Kubernetes auth mount:
  - `kubernetes` for prd;
  - `kubernetes-dev` for dev;
  - `kubernetes-sandbox` for sandbox.
- The ESO ServiceAccount is granted `system:auth-delegator` through the
  `external-secrets-auth-delegator` ClusterRoleBinding.
- OpenBao Kubernetes auth is configured with `disable_local_ca_jwt=true`.
- No long-lived `token_reviewer_jwt` is stored in SOPS for ESO authentication.
- The rebuilt cluster CA is harvested from the target kubeconfig at runtime by
  `ansible/playbooks/ops-openbao_register_cluster.yaml`.

Run the registration playbook after Argo CD has reconciled the ESO application
and the auth-delegator binding exists:

```bash
ansible-playbook playbooks/ops-openbao_register_cluster.yaml -e cluster=sandbox
ansible-playbook playbooks/ops-openbao_register_cluster.yaml -e cluster=dev
ansible-playbook playbooks/ops-openbao_register_cluster.yaml -e cluster=prd
```

The playbook updates the selected OpenBao Kubernetes auth mount, restarts ESO,
and validates `ExternalSecret` readiness in the target cluster.

## Alternatives considered

- **Continue storing reviewer JWTs and CAs in SOPS** — works, but requires
  SOPS hand-editing after every rebuild and stores runtime cluster identity in
  durable inventory. *Rejected.*
- **Re-seed all OpenBao KV values after every k0s rebuild** — unnecessary
  because OpenBao is external and persistent. *Rejected.*
- **Pin the k0s cluster CA across rebuilds** — could reduce or eliminate
  re-registration, but it is a separate bootstrap decision. *Deferred.*
- **Run OpenBao inside each Kubernetes cluster** — would couple secret storage
  durability to the cluster being rebuilt. *Rejected.*

## Consequences

- Cluster rebuild recovery for ESO is a single human-run Ansible command per
  rebuilt cluster.
- OpenBao secret values remain independent from k0s cluster lifecycle.
- SOPS no longer needs to carry rotating ESO reviewer JWT material.
- The ESO Argo CD application must reconcile before registration so the
  `external-secrets` ServiceAccount and auth-delegator binding exist.
- If a cluster API endpoint changes, update the corresponding OpenBao host value
  before running registration.
