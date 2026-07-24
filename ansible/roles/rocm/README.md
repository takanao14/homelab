# rocm Role

Installs AMD GPU drivers (amdgpu-dkms) and the ROCm toolkit on Ubuntu 24.04 (Noble) hosts.

Starting with ROCm 7.14, AMD moved the ROCm apt repository from
`repo.radeon.com/rocm/apt/<version>` to a single rolling
`repo.amd.com/rocm/packages-multi-arch/<distro>` repo, replaced the generic
`rocm` meta-package with per-GPU packages (`amdrocm<series>-<gfx-target>`),
and changed the install layout to versioned per-component subdirectories
under `/opt/rocm` (e.g. `/opt/rocm/core-7.14/`) instead of a flat
`/opt/rocm/{bin,lib}`. This role targets that new layout; it is not
compatible with ROCm 7.2.4 and earlier.

## Functionality

- Installs prerequisites (`wget`, `python3-debian`, `libatomic1`, `libquadmath0`).
- Purges any legacy `rocm`/`rocm-core` packages from the old repo.radeon.com
  ROCm repo, since they are not upgraded in place by the new packages.
- Configures the ROCm apt repository (repo.amd.com) and the AMD GPU driver
  and device-metrics-exporter repositories (repo.radeon.com) in Deb822
  format; removes the now-obsolete `rocm-graphics` repo.
- Installs the HWE kernel (`linux-generic-hwe-24.04`) and reboots when it changes.
- Installs kernel headers for the running kernel.
- Installs `amdgpu-dkms` without automatically upgrading it.
- Installs or upgrades `rocm_package_name` (e.g. `amdrocm7.14-gfx1200`; long-running
  step with 1-hour async timeout).
- Adds the current user to the `render` and `video` groups.
- Discovers the installed `/opt/rocm/*/lib` and `/opt/rocm/*/bin`
  directories, writes `/etc/ld.so.conf.d/rocm.conf`, and runs `ldconfig`.
- Reboots when the AMD GPU driver changes.
- Adds the AMD device-metrics-exporter APT repository and installs `amdgpu-exporter`.
- Enables and starts `gpuagent` and `amd-metrics-exporter` services.
- Verifies the installed ROCm version, HIP compiler, and GPU detection.

## Variables

| Variable | Default | Description |
| --- | --- | --- |
| `rocm_version` | `7.14.0` | Expected installed ROCm release; combined with `rocm_gpu_target` to form the apt package name (e.g. `amdrocm7.14-gfx1200`) |
| `rocm_package_state` | `latest` | Desired state of the ROCm meta-package |
| `rocm_gpu_target` | `gfx1200` | GPU-specific ROCm package suffix (gfx1200 = RX 9060 XT); see the [ROCm install docs](https://rocm.docs.amd.com/en/latest/install/rocm.html) for the marketing-name-to-gfx-target mapping |
| `rocm_ubuntu_codename` | `noble` | Ubuntu codename used by the amdgpu and device-metrics-exporter repos |
| `rocm_amdgpu_version` | `31.40` | AMD GPU driver repository version; managed independently from ROCm |
| `rocm_amdgpu_package_state` | `present` | Desired state of `amdgpu-dkms`; use `latest` only for an explicit driver upgrade |
| `rocm_amdgpu_minimum_boot_free_mb` | `300` | Minimum `/boot` free space required for an explicit driver upgrade |
| `rocm_udev_rules` | `amdgpu-insecure-instinct-udev-rules_31.40.0.0-2364437.24.04_all.deb` | GPU access udev rules package |
| `rocm_device_metrics_exporter_version` | `1.5.0` | Device metrics exporter repository version |

The default upgrade path keeps the installed AMD GPU driver version unchanged.
After validating a ROCm userspace upgrade, explicitly upgrade the driver with:

```bash
ansible-playbook playbooks/gpuvm.yaml --tags rocm --limit gpuvm1 \
  -e rocm_amdgpu_package_state=latest
```

Update `rocm_amdgpu_version` and `rocm_udev_rules` together.

Renovate tracks `rocm_amdgpu_version` and `rocm_device_metrics_exporter_version`
from the AMD repository indexes. AMD GPU driver updates are limited to the
`31.x` release line and are not automerged. Update `rocm_udev_rules` manually
in the corresponding driver PR. `rocm_version` and `rocm_gpu_target` are no
longer tracked by renovate's `custom.rocm` datasource: the new repo.amd.com
repo has no version-numbered index to scrape, so bump `rocm_version` manually
after checking the [ROCm release notes](https://rocm.docs.amd.com/en/latest/about/release-notes.html).

## Dependencies

None.

## Usage

```yaml
- name: Setup ROCm on GPU VM
  hosts: gpuvm
  roles:
    - role: timezone
    - role: rocm
```
