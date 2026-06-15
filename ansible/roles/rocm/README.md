# rocm Role

Installs AMD GPU drivers (amdgpu-dkms) and the ROCm toolkit on Ubuntu 24.04 (Noble) hosts.

## Functionality

- Installs prerequisites (`wget`, `python3-debian`, `python3-setuptools`, `python3-wheel`).
- Configures independent ROCm, AMD GPU driver, and exporter repositories in Deb822 format.
- Installs the HWE kernel (`linux-generic-hwe-24.04`) and reboots when it changes.
- Installs kernel headers for the running kernel.
- Installs `amdgpu-dkms` without automatically upgrading it.
- Installs or upgrades `rocm` (long-running step with 1-hour async timeout).
- Adds the current user to the `render` and `video` groups.
- Writes `/etc/ld.so.conf.d/rocm.conf` with ROCm library paths and runs `ldconfig`.
- Reboots when the AMD GPU driver changes.
- Adds the AMD device-metrics-exporter APT repository and installs `amdgpu-exporter`.
- Enables and starts `gpuagent` and `amd-metrics-exporter` services.
- Verifies the installed ROCm version, HIP compiler, and GPU detection.

## Variables

| Variable | Default | Description |
| --- | --- | --- |
| `rocm_version` | `7.2.4` | ROCm repository version and expected installed release |
| `rocm_package_state` | `latest` | Desired state of the `rocm` meta-package |
| `rocm_ubuntu_codename` | `noble` | Ubuntu repository codename |
| `rocm_amdgpu_version` | `30.30.4` | AMD GPU driver repository version; managed independently from ROCm |
| `rocm_amdgpu_package_state` | `present` | Desired state of `amdgpu-dkms`; use `latest` only for an explicit driver upgrade |
| `rocm_amdgpu_minimum_boot_free_mb` | `300` | Minimum `/boot` free space required for an explicit driver upgrade |
| `rocm_udev_rules` | `amdgpu-insecure-instinct-udev-rules_30.30.4.0-2341068.24.04_all.deb` | GPU access udev rules package |
| `rocm_device_metrics_exporter_version` | `1.4.0` | Device metrics exporter repository version |

The default upgrade path keeps the installed AMD GPU driver version unchanged.
After validating a ROCm userspace upgrade, explicitly upgrade the driver with:

```bash
ansible-playbook playbooks/gpuvm.yaml --tags rocm --limit gpuvm01 \
  -e rocm_amdgpu_package_state=latest
```

Update `rocm_amdgpu_version` and `rocm_udev_rules` together.

Renovate tracks `rocm_version`, `rocm_amdgpu_version`, and
`rocm_device_metrics_exporter_version` from the AMD repository indexes.
AMD GPU driver updates are limited to the `30.30.x` release line and are not
automerged. Update `rocm_udev_rules` manually in the corresponding driver PR.

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
