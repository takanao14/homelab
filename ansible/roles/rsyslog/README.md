# rsyslog Role

Installs and configures rsyslog to forward logs to a remote Vector log collector.

## Functionality

- Installs `rsyslog` from APT.
- Deploys `/etc/rsyslog.d/99-forward.conf` from a Jinja2 template.
- Ensures the `rsyslog` service is started and enabled.

## Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `rsyslog_forward_host` | *(required)* | Destination syslog host; set in the inventory (`group_vars/all.yaml`) |
| `rsyslog_forward_port` | `514` | Destination port |
| `rsyslog_forward_protocol` | `udp` | Protocol (`udp` or `tcp`) |
| `rsyslog_journal_input` | `false` | Read journald directly with `imjournal` and use an isolated forwarding ruleset |
| `rsyslog_ignore_previous_journal` | `true` | Start at the end of the journal on the first deployment |
| `rsyslog_queue_size` | `10000` | Maximum number of messages in the memory queue |
| `rsyslog_queue_high_watermark` | `9000` | Queue depth that starts disk-assisted spooling |
| `rsyslog_queue_low_watermark` | `7000` | Queue depth that stops disk-assisted spooling |
| `rsyslog_queue_max_disk_space` | `256m` | Maximum disk space for the forwarding queue |
| `rsyslog_file_inputs` | `[]` | File logs to collect via `imfile` (see below) |
| `rsyslog_work_directory` | `/var/spool/rsyslog` | State-file directory for `imfile` (emitted only when file inputs are present) |
| `rsyslog_file_queue_size` | `5000` | Max messages in the file-forwarding memory queue |
| `rsyslog_file_queue_high_watermark` | `4000` | File-queue depth that starts disk-assisted spooling |
| `rsyslog_file_queue_low_watermark` | `2000` | File-queue depth that stops disk-assisted spooling |
| `rsyslog_file_queue_max_disk_space` | `128m` | Maximum disk space for the file-forwarding queue |

When `rsyslog_journal_input` is enabled, the journal input is connected directly
to a dedicated forwarding ruleset. The forwarding action uses a bounded
disk-assisted memory queue and drops new remote-forwarding entries when the
queue is full rather than blocking the source host.

## File log collection (`imfile`)

`rsyslog_file_inputs` adds `imfile` inputs for log files the journal does not
carry. Each item is:

```yaml
rsyslog_file_inputs:
  - path: /var/log/pveproxy/access.log  # exact path, no wildcards
    tag: pveproxy-access                # becomes the RFC 5424 APP-NAME
    facility: local6                    # optional (default local6)
    severity: info                      # optional (default info)
```

Behavior and assumptions:

- Inputs are bound to a separate `ForwardFilesToLoki` ruleset with its **own**
  disk-assisted, non-blocking queue (`loki_forward_files`, sized smaller than the
  journal queue), so a high-volume file (e.g. an access log) burst cannot starve
  journal forwarding.
- inotify mode (default) follows **rename-based rotation**; only the current file
  is matched — rotated files (`.1`, `.gz`, `index.1`, `pveam.log.0`) are not.
- `PersistStateInterval=100` keeps durable read positions, so on first deployment
  each current file is replayed once (bounded) and not re-read afterwards.
  `freshStartTail` is intentionally **not** used (it can lose lines at startup);
  `reopenOnTruncate` is off (files rotate by rename, and the option is
  experimental).
- The default is an empty list, so hosts without file inputs (e.g. `rpi3`) keep
  their journal/legacy-only behavior unchanged.

> **Privacy:** access logs contain source IPs, authenticated users, request
> paths, VMIDs, and task identifiers. These stay in the message body and must
> never be promoted to Loki index labels. Per-line size is bounded by rsyslog's
> global `maxMessageSize` default; no global override is set here so the
> hypervisor's shared rsyslog limits are left untouched.

## Dependencies

None.

## Usage

```yaml
- name: Configure rpi3
  hosts: rpi3
  roles:
    - role: rsyslog
```
