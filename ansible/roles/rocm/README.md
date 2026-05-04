# rocm Role

Installs AMD GPU drivers (amdgpu-dkms) and the ROCm toolkit on Ubuntu 24.04 (Noble) hosts.

## Functionality

- Installs prerequisites (`wget`, `python3-setuptools`, `python3-wheel`).
- Downloads and installs the `amdgpu-install` meta-package from the AMD repository (ROCm 7.2.1).
- Installs the HWE kernel (`linux-generic-hwe-24.04`) and reboots to apply it.
- Installs kernel headers for the running kernel.
- Installs `amdgpu-dkms` and `rocm` (long-running step with 1-hour async timeout).
- Adds the current user to the `render` and `video` groups.
- Writes `/etc/ld.so.conf.d/rocm.conf` with ROCm library paths and runs `ldconfig`.
- Reboots to load the GPU driver.
- Adds the AMD device-metrics-exporter APT repository and installs `amdgpu-exporter`.
- Enables and starts `gpuagent` and `amd-metrics-exporter` services.

## Variables

None. The ROCm version and repository URL are hardcoded in the tasks (`7.2.1` / `noble`).

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
