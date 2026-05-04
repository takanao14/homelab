# lemonade Role

Installs and configures [Lemonade](https://github.com/lemonade-command/lemonade) AI inference server with AMD ROCm backend.

## Functionality

- Adds the Lemonade stable PPA (`ppa:lemonade-team/stable`).
- Installs `lemonade-server` from APT.
- Adds the `lemonade` user to the `video` group (required for GPU access).
- Installs the `llamacpp:rocm` backend binary.
- Configures host, port, and backend via `lemonade config set`.
- Ensures `lemonade-server` is started and enabled.
- Optionally pulls models listed in `lemonade_models`.

## Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `lemonade_host` | `0.0.0.0` | Listen address |
| `lemonade_port` | `13305` | Listen port |
| `lemonade_llamacpp_backend` | `rocm` | Inference backend (`rocm` for AMD GPUs) |
| `lemonade_models` | `[]` | List of model names to pull via `lemonade pull` |

## Dependencies

- `rocm` role (AMD GPU drivers must be installed before this role runs).

## Usage

```yaml
- name: Setup Lemonade
  hosts: gpuvm
  roles:
    - role: rocm
    - role: lemonade
      vars:
        lemonade_models:
          - llama3.2:3b
```
