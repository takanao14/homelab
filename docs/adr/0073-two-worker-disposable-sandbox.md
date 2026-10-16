# ADR-0073: Use two workers for the disposable sandbox

- **Status:** Accepted
- **Category:** Kubernetes platform
- **Date:** 2026-10-15
- **Related:** [ADR-0033](0033-sandbox-storage-openebs-localpv-and-truenas-nfs.md), [ADR-0070](0070-no-staging-sandbox-validates-the-cluster-platform.md)

## Context

Sandbox validates cluster lifecycle and platform behavior and is rebuilt rather
than migrated in place. Its former Longhorn storage used three replicas across
three workers; OpenEBS LocalPV and NFS no longer require that topology.
A workload snapshot across the three 4 GiB workers used about 0.30 CPU cores and
4.45 GiB of memory, making a single 4 GiB worker too small without further sizing.

## Decision

Keep one controller and two workers, retaining sandbox-k0s-worker1 and
sandbox-k0s-worker2 at four vCPUs and 4 GiB each. Remove worker3 from Terraform,
k0s topology and Ansible inventory and guest startup lists for the next rebuild.
Two workers retain cross-node networking, scheduling, NFS remount and drain
validation. Uninterrupted service while a worker is down is not a sandbox goal.

Use reset and rebuild for this transition. Reset the existing cluster with its
original three-worker topology before deleting worker3 or adopting the reduced
inventory. Prometheus currently has a LocalPV on worker3; its sandbox history is
disposable and is not migrated. The external TrueNAS dataset is not deleted.
Re-register rebuilt sandbox trust with OpenBao after ESO deployment.

## Consequences

The steady allocation drops by four vCPUs, 4 GiB RAM and worker3's virtual disks.
Validate fresh bootstrap, cross-node smoke tests and observed memory headroom on
the two-worker cluster. This snapshot does not establish peak memory or guarantee
that both workers' workloads fit on one worker during maintenance.

A single larger worker would save more per-node overhead but remove cross-node
validation. Three workers add no required storage replica or physical failure
domain for this disposable cluster.
