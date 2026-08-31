# rocm Role

Installs AMD GPU drivers (amdgpu-dkms) and the ROCm toolkit on Ubuntu 24.04 (Noble) hosts.

Starting with ROCm 7.14, AMD moved the ROCm apt repository off
`repo.radeon.com/rocm/apt/<version>` to a single rolling repo, replaced the
generic `rocm` meta-package with per-GPU packages
(`amdrocm<series>-<gfx-target>`), and changed the install layout to versioned
per-component subdirectories under `/opt/rocm` (e.g. `/opt/rocm/core-10.0/`)
instead of a flat `/opt/rocm/{bin,lib}`. This role targets that new layout; it
is not compatible with ROCm 7.2.4 and earlier.

ROCm 10.0 moved that rolling repo again, from
`repo.amd.com/rocm/packages-multi-arch/<distro>` (which stops at 7.14) to
`stable.repo.amd.com/rocm/core/packages/<distro>`. The signing key moved with
it but its contents are unchanged, and the repo still publishes
`Origin: AMD ROCm`, so the apt pin in `files/rocm-pin-600` still applies.

### Coexisting releases

Releases install into version-suffixed directories, declare no `Conflicts`,
and register `update-alternatives` entries from the postinst of
`amdrocm-core<version>-<gfx-target>`. Installing a new release therefore does
not remove the old one: it repoints `/opt/rocm/{core,bin,lib,include,llvm,
amdgcn,libexec,share}` and `/usr/bin/{hipcc,rocminfo,...}` to whichever
release scores highest, and removing it falls back to the older one.

The role does not rely on that coexistence, because the `/opt/rocm/*/lib`
discovery below globs the real directories and would write every installed
release's lib path into `ld.so.conf`, leaving binaries and shared libraries
resolving to different releases. `rocm_previous_version` removes the older
release instead, after the new one is installed.

## Functionality

- Installs prerequisites (`wget`, `python3-debian`, `libatomic1`, `libquadmath0`).
- Purges any legacy `rocm`/`rocm-core` packages from the old repo.radeon.com
  ROCm repo, since they are not upgraded in place by the new packages.
- Purges `amdgpu-install` first, before any task refreshes the apt cache. It
  owns `/etc/apt/sources.list.d/{amdgpu,rocm}.list` as conffiles, which
  duplicate the amdgpu repository this role configures in deb822 form and add
  unmanaged Radeon Software graphics repos (`graphics/<ver>`,
  `rocmradeon/apt/<ver>`) that `rocm-pin-600` then elevates to priority 600.
  Nothing depends on the package. It also owns `/etc/apt/keyrings/rocm.gpg`,
  the key the amdgpu and device-metrics-exporter sources verify against, so the
  purge must run before the keyring tasks — they skip when the keyring already
  exists, and purging later would delete the key they declined to write and
  leave apt unable to verify repo.radeon.com.
- Configures the ROCm apt repository (stable.repo.amd.com) and the AMD GPU driver
  and device-metrics-exporter repositories (repo.radeon.com) in Deb822 format.
- Installs a specific AMD-supported HWE kernel (`rocm_hwe_kernel`) with its
  headers and extra modules, holds the rolling `linux-generic-hwe-24.04`
  metapackages so apt cannot pull an unsupported kernel, and asserts that
  kernel is the one currently running before building the driver.
- Installs `amdgpu-dkms` without automatically upgrading it.
- Installs or upgrades `rocm_package_name` (e.g. `amdrocm10.0-gfx1200`; long-running
  step with 1-hour async timeout), then removes the `rocm_previous_version`
  packages left behind by that install.
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
| `rocm_version` | `10.0.0` | Expected installed ROCm release; combined with `rocm_gpu_target` to form the apt package name (e.g. `amdrocm10.0-gfx1200`) |
| `rocm_previous_version` | `7.14` | Major.minor of the release removed after `rocm_version` is installed; empty string to skip |
| `rocm_package_state` | `latest` | Desired state of the ROCm meta-package |
| `rocm_gpu_target` | `gfx1200` | GPU-specific ROCm package suffix (gfx1200 = RX 9060 XT); see the [ROCm install docs](https://rocm.docs.amd.com/en/latest/install/rocm.html) for the marketing-name-to-gfx-target mapping |
| `rocm_ubuntu_codename` | `noble` | Ubuntu codename used by the amdgpu and device-metrics-exporter repos |
| `rocm_hwe_kernel` | `6.17.0-35-generic` | Specific AMD-supported HWE kernel to pin; the role installs this exact kernel and refuses to proceed unless it is the running kernel, because amdgpu-dkms fails to build on unsupported kernels (e.g. 7.x from the rolling HWE meta) |
| `rocm_amdgpu_version` | `31.50` | AMD GPU driver repository version; AMD pairs 31.50 with ROCm 10.0, so keep it in step with `rocm_version` |
| `rocm_amdgpu_package_state` | `present` | Desired state of `amdgpu-dkms`; use `latest` only for an explicit driver upgrade |
| `rocm_amdgpu_minimum_boot_free_mb` | `300` | Minimum `/boot` free space required for an explicit driver upgrade |
| `rocm_udev_rules` | `amdgpu-insecure-instinct-udev-rules_31.50.0.0-2390945.24.04_all.deb` | GPU access udev rules package |
| `rocm_device_metrics_exporter_version` | `1.5.1` | Device metrics exporter repository version |
| `rocm_device_metrics_exporter_package_state` | `latest` | Desired state of `amdgpu-exporter`; `present` would pin the installed package and make a version bump a no-op |

The default upgrade path keeps the installed AMD GPU driver version unchanged.
After validating a ROCm userspace upgrade, explicitly upgrade the driver with:

```bash
ansible-playbook playbooks/gpuvm.yaml --tags rocm --limit gpuvm1 \
  -e rocm_amdgpu_package_state=latest
```

Update `rocm_amdgpu_version` and `rocm_udev_rules` together.

### Kernel pinning

The role pins the host to `rocm_hwe_kernel` (an AMD-supported kernel) and
holds the rolling HWE metapackages. It does **not** remove an already-installed
unsupported kernel, because the running kernel cannot remove itself. If a host
has been carried onto an unsupported kernel (e.g. `7.x`) by the rolling meta,
do this one-time migration before running the role:

```bash
# On the target host, boot the supported kernel and drop the unsupported one.
sudo apt-get install -y linux-image-6.17.0-35-generic \
  linux-headers-6.17.0-35-generic linux-modules-extra-6.17.0-35-generic
sudo sed -i 's/^GRUB_DEFAULT=.*/GRUB_DEFAULT="Advanced options for Ubuntu>Ubuntu, with Linux 6.17.0-35-generic"/' /etc/default/grub
sudo update-grub
sudo reboot
# After it comes back on 6.17 (verify with `uname -r`), purge the 7.x kernel:
sudo apt-get purge -y 'linux-image-7.0.0-*' 'linux-headers-7.0.0-*' \
  'linux-modules-7.0.0-*' 'linux-hwe-7.0-*'
```

Renovate tracks `rocm_amdgpu_version` and `rocm_device_metrics_exporter_version`
from the AMD repository indexes. AMD GPU driver updates are limited to the
`31.x` release line and are not automerged. Update `rocm_udev_rules` manually
in the corresponding driver PR.

`rocm_version`, `rocm_previous_version`, and `rocm_gpu_target` are not tracked
by renovate. The rolling repo publishes no version-numbered index to scrape,
and the `ROCm/ROCm` GitHub releases stop at 7.14.0, so neither is a usable
datasource for 10.0 and later. Bump `rocm_version` manually after checking the
[ROCm release notes](https://rocm.docs.amd.com/en/latest/about/release-notes.html),
and set `rocm_previous_version` to the release being replaced.

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
