# rocm Role

Installs AMD GPU drivers (amdgpu-dkms) and the ROCm toolkit on Ubuntu amd64 hosts.
Ubuntu 24.04 uses the native AMD metrics exporter package. Ubuntu 26.04 uses
AMD's pinned v1.5.3 exporter container because AMD publishes no Resolute exporter
APT suite. Rebuild the guest for this transition; native package migration is
rejected before package changes.

The role uses `stable.repo.amd.com/rocm/core/packages/<distro>`, per-GPU
`amdrocm<series>-<gfx-target>` packages, and versioned component directories
under `/opt/rocm`. It is incompatible with ROCm 7.2.4 and earlier.

Releases coexist and `update-alternatives` selects the highest version.
After installing the new release, the role removes `rocm_previous_version`
to prevent library discovery from mixing releases in `ld.so.conf`.

It purges the legacy installer and metapackages, pins an AMD-supported kernel
before DKMS builds, and holds release-specific rolling kernel metapackages.
Driver changes trigger a reboot; normal runs leave the driver unchanged.

On 24.04 the role enables `gpuagent` and `amd-metrics-exporter`. On 26.04 a
rootful Podman system service runs AMD's official privileged container, whose
entrypoint starts both processes. Host networking preserves port 5000, and
`/var/lib/amd-metrics-exporter` shares the GPU health socket with the Kubernetes
device plugin. The fixed upstream configuration keeps GPU health enabled and
removes Kubernetes pod labels and non-GPU agents, matching standalone packaging.

## Variables

| Variable | Default | Description |
| --- | --- | --- |
| `rocm_version` | `10.0.0` | Expected installed ROCm release; combined with `rocm_gpu_target` to form the apt package name (e.g. `amdrocm10.0-gfx1200`) |
| `rocm_previous_version` | `7.14` | Major.minor of the release removed after `rocm_version` is installed; empty string to skip |
| `rocm_package_state` | `latest` | Desired state of the ROCm meta-package |
| `rocm_gpu_target` | `gfx1200` | GPU-specific ROCm package suffix (gfx1200 = RX 9060 XT); see the [ROCm install docs](https://rocm.docs.amd.com/en/latest/install/rocm.html) for the marketing-name-to-gfx-target mapping |
| `rocm_ubuntu_releases` | OS release map | Repository, kernel ABI, modules, hold targets, udev rules and exporter mode for each supported release |
| `rocm_ubuntu_codename` | From OS release map | Ubuntu codename used by the amdgpu repo |
| `rocm_hwe_kernel` | From OS release map | Reviewed kernel ABI: `6.17.0-35-generic` on 24.04, `7.0.0-42-generic` on 26.04; must be running before DKMS installation |
| `rocm_amdgpu_version` | `31.50` | AMD GPU driver repository version; AMD pairs 31.50 with ROCm 10.0, so keep it in step with `rocm_version` |
| `rocm_amdgpu_package_state` | `present` | Desired state of `amdgpu-dkms`; use `latest` only for an explicit driver upgrade |
| `rocm_amdgpu_minimum_boot_free_mb` | `300` | Minimum `/boot` free space required for an explicit driver upgrade |
| `rocm_udev_rules` | From OS release map | GPU access udev rules package matching the OS and driver release |
| `rocm_device_metrics_exporter_version` | `1.5.1` | Device metrics exporter repository version |
| `rocm_device_metrics_exporter_package_state` | `latest` | Desired state of the 24.04 `amdgpu-exporter` package |
| `rocm_device_metrics_exporter_image` | v1.5.3, digest pinned | Official amd64 exporter image for 26.04 |
| `rocm_device_metrics_exporter_config_url` / `rocm_device_metrics_exporter_config_sha256` | Fixed upstream commit and SHA-256 | Standalone configuration source for the container |

The default upgrade path keeps the installed AMD GPU driver version unchanged.
After validating a ROCm userspace upgrade, explicitly upgrade the driver with:

```bash
ansible-playbook playbooks/services/gpuvm.yaml --tags rocm --limit gpuvm1 \
  -e rocm_amdgpu_package_state=latest
```

Update `rocm_amdgpu_version` and every OS-specific `udev_rules` entry together.
Update the container image tag/digest and upstream configuration commit/checksum
together. A driver upgrade stops the container first; a failed upgrade leaves it
stopped until the role completes successfully.

### Kernel pinning

The role pins the host to `rocm_hwe_kernel` (an AMD-supported kernel) and
holds the rolling kernel metapackages. Review available Ubuntu packages and AMD's
compatibility matrix before updating each pinned ABI; holds prevent automatic ABI
updates. Prepare and boot the selected ABI before running the role; installing
the kernel does not select its GRUB entry. The role does **not** remove an
already-installed unsupported kernel. If a **24.04** host has been carried onto an unsupported
kernel by the rolling HWE meta, boot the pinned ABI before running the role:

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
in the corresponding driver PR, for every OS release entry.

`rocm_version`, `rocm_previous_version`, and `rocm_gpu_target` are not tracked
by renovate. The rolling repo publishes no version-numbered index to scrape,
and the `ROCm/ROCm` GitHub releases stop at 7.14.0, so neither is a usable
datasource for 10.0 and later. Bump `rocm_version` manually after checking the
[ROCm release notes](https://rocm.docs.amd.com/en/latest/about/release-notes.html),
and set `rocm_previous_version` to the release being replaced.

## Usage

Run [playbooks/services/gpuvm.yaml](../../playbooks/services/gpuvm.yaml).
