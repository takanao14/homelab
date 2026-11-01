#!/usr/bin/env python3
import contextlib
import csv
import fcntl
import hashlib
import json
import os
from pathlib import Path
import pwd
import re
import shutil
import signal
import socket
import stat
import subprocess
import sys
import threading
import time
import uuid


CONFIG = Path('/etc/imagebuilder/config.json')
PHASES = frozenset(('preparing', 'building', 'verifying', 'publishing',
                    'succeeded', 'failed', 'interrupted'))
TERMINAL = frozenset(('succeeded', 'failed', 'interrupted'))
CODES = frozenset(('accepted', 'preparing', 'building', 'verifying', 'publishing',
                   'published', 'verified', 'invalid_request', 'invalid_state',
                   'invalid_config', 'missing_credentials', 'command_timeout',
                   'invalid_command_output', 'command_failed',
                   'work_disk_not_mounted', 'kvm_unavailable', 'kernel_unavailable', 'insufficient_space',
                   'missing_tool', 'stopping', 'maintenance', 'busy',
                   'publication_blocked', 'start_failed', 'job_not_finished',
                   'no_verified_artifact', 'shutdown_failed', 'service_failed',
                   'invalid_progress', 'host_restarted', 'revision_mismatch',
                   'unknown_target', 'ssh_agent_failed', 'ssh_key_failed',
                   'invalid_artifact', 'checksum_mismatch', 'no_artifacts',
                   'smoke_boot_failed', 'smoke_timeout', 'remote_pair_incomplete',
                   'remote_checksum_mismatch', 'rollback_failed',
                   'publication_failed_restored', 'artifact_changed',
                   'internal_error', 'invalid_branch', 'branch_resolution_failed'))
CREDENTIALS = frozenset(('PKR_VAR_user_password', 'SEAWEEDFS_S3_ENDPOINT',
                         'SEAWEEDFS_S3_ACCESS_KEY', 'SEAWEEDFS_S3_SECRET_KEY',
                         'SEAWEEDFS_CLOUD_IMAGES_BUCKET'))
BASE_ENV = {'PATH': '/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin',
            'LANG': 'C.UTF-8', 'LIBGUESTFS_BACKEND': 'direct',
            'GIT_TERMINAL_PROMPT': '0'}
LOG_STEPS = frozenset(('checkout', 'ssh-setup', 'build', 'packer-init', 'packer-build',
                       'import', 'compress', 'artifact-check', 'smoke-boot',
                       'publish', 'rollback'))
LOG_EVENTS = frozenset(('started', 'completed', 'failed', 'packer-error',
                        'artifact-count'))
LOG_CATEGORIES = frozenset(('command-exit', 'timeout', 'packer-error',
                            'hcl-validation', 'ssh-timeout', 'provisioner-exit',
                            'hcl-unsupported-argument', 'hcl-missing-required-argument',
                            'hcl-unsupported-attribute', 'hcl-invalid-expression',
                            'hcl-unknown-variable', 'hcl-invalid-reference',
                            'hcl-invalid-function-argument'))
HCL_HEADERS = {
    'Error: Unsupported argument': 'hcl-unsupported-argument',
    'Error: Missing required argument': 'hcl-missing-required-argument',
    'Error: Unsupported attribute': 'hcl-unsupported-attribute',
    'Error: Invalid expression': 'hcl-invalid-expression',
    'Error: Unknown variable': 'hcl-unknown-variable',
    'Error: Invalid reference': 'hcl-invalid-reference',
    'Error: Invalid function argument': 'hcl-invalid-function-argument',
}
MAX_LOG_RECORDS = 1000
MAX_LOG_BYTES = 196608


def validate_branch(value):
    require(isinstance(value, str) and
            re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9._/-]{0,199}', value),
            'invalid_branch')
    require(not any(token in value for token in ('..', '//', '@{')) and
            not value.endswith(('/', '.')) and
            all(not part.startswith('.') and not part.endswith('.lock')
                for part in value.split('/')), 'invalid_branch')
    return value


def validate_log_record(record):
    require(isinstance(record, dict) and
            isinstance(record.get('step'), str) and record['step'] in LOG_STEPS and
            isinstance(record.get('event'), str) and record['event'] in LOG_EVENTS,
            'invalid_state')
    result = {key: record[key] for key in ('step', 'event')}
    for key in ('sequence', 'time'):
        require(type(record.get(key)) is int and 0 <= record[key] < 2 ** 63,
                'invalid_state')
        result[key] = record[key]
    require(result['sequence'] > 0, 'invalid_state')
    for key in ('exit_code', 'duration_ms', 'count', 'provisioner_exit_code',
                'line', 'column'):
        if key in record:
            require(type(record[key]) is int, 'invalid_state')
            require((-255 <= record[key] <= 255) if key == 'exit_code'
                    else (0 <= record[key] < 2 ** 63), 'invalid_state')
            if key == 'provisioner_exit_code':
                require(0 <= record[key] <= 255, 'invalid_state')
            if key in ('line', 'column'):
                require(record[key] > 0, 'invalid_state')
            result[key] = record[key]
    if 'category' in record:
        require(isinstance(record['category'], str) and record['category'] in LOG_CATEGORIES,
                'invalid_state')
        result['category'] = record['category']
    if 'source' in record:
        require(isinstance(record['source'], str) and len(record['source']) <= 512 and
                re.fullmatch(r'packer/(?:[A-Za-z0-9_-][A-Za-z0-9._-]*\.pkr\.hcl|'
                             r'vars/[A-Za-z0-9_-][A-Za-z0-9._-]*\.pkrvars\.hcl)',
                             record['source']), 'invalid_state')
        require('line' in result, 'invalid_state')
        result['source'] = record['source']
    require('column' not in result or 'source' in result, 'invalid_state')
    require('line' not in result or 'source' in result, 'invalid_state')
    return result


class SafeError(Exception):
    pass


def error_code(error):
    value = str(error)
    return value if value in CODES else 'internal_error'


def require(condition, code='invalid_request'):
    if not condition:
        raise SafeError(code)


def read_json(path):
    require(path.is_file() and not path.is_symlink(), 'invalid_state')
    require(path.stat().st_size <= 262144, 'invalid_state')
    return json.loads(path.read_bytes())


def write_json(path, data, mode=0o600):
    temporary = path.with_name(path.name + '.' + uuid.uuid4().hex + '.new')
    descriptor = os.open(temporary, os.O_CREAT | os.O_EXCL | os.O_WRONLY, mode)
    try:
        with os.fdopen(descriptor, 'w') as stream:
            json.dump(data, stream, separators=(',', ':'))
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, path)
    finally:
        temporary.unlink(missing_ok=True)


def job_id(value):
    require(isinstance(value, str) and re.fullmatch(r'[0-9a-f]{32}', value))
    return value


def credentials(value, publish=False):
    require(isinstance(value, dict) and set(value) <= CREDENTIALS)
    require(all(isinstance(v, str) and 0 < len(v) <= 16384 and '\0' not in v
                for v in value.values()))
    if publish:
        require({'SEAWEEDFS_S3_ENDPOINT', 'SEAWEEDFS_S3_ACCESS_KEY',
                 'SEAWEEDFS_S3_SECRET_KEY'} <= set(value), 'missing_credentials')
        require(re.fullmatch(r'https://[A-Za-z0-9.-]+(?::[0-9]+)?/?',
                             value['SEAWEEDFS_S3_ENDPOINT']) is not None)
        require(re.fullmatch(r'[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]',
                             value.get('SEAWEEDFS_CLOUD_IMAGES_BUCKET',
                                       'cloud-images')) is not None)
    return value


def command(args, *, cwd=None, env=None, timeout=60, capture=False,
            digest=False, observer=None):
    """Drain child output in memory; no raw output or exception reaches callers."""
    started = time.monotonic()
    if observer:
        observer({'event': 'started'})
    try:
        process = subprocess.Popen(args, cwd=cwd, env=env or BASE_ENV,
                                   stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                                   stderr=(subprocess.STDOUT if observer and not capture and not digest
                                           else subprocess.DEVNULL),
                                   start_new_session=True)
    except OSError:
        if observer:
            observer({'event': 'failed', 'duration_ms': int((time.monotonic() - started) * 1000),
                      'category': 'command-exit'})
        raise SafeError('command_failed') from None
    output = bytearray()
    checksum = hashlib.sha256()
    overflow = False
    observer_failed = False

    def drain():
        nonlocal overflow, observer_failed
        pending = bytearray()
        dropping = False
        try:
            while chunk := process.stdout.read1(65536):
                if digest:
                    checksum.update(chunk)
                elif capture:
                    if len(output) + len(chunk) <= 2097152:
                        output.extend(chunk)
                    else:
                        overflow = True
                if observer and not digest and not observer_failed:
                    for index, part in enumerate(chunk.split(b'\n')):
                        if index:
                            if not dropping:
                                observer({'event': 'output', 'line': bytes(pending)})
                            pending.clear()
                            dropping = False
                        if len(pending) + len(part) <= 16384 and not dropping:
                            pending.extend(part)
                        else:
                            pending.clear()
                            dropping = True
            if observer and not digest and pending and not dropping and not observer_failed:
                observer({'event': 'output', 'line': bytes(pending)})
        except Exception:
            observer_failed = True
            with contextlib.suppress(OSError):
                os.killpg(process.pid, signal.SIGKILL)

    reader = threading.Thread(target=drain, daemon=True)
    reader.start()
    timed_out = False
    try:
        process.wait(timeout=timeout)
    except subprocess.TimeoutExpired:
        os.killpg(process.pid, signal.SIGKILL)
        process.wait()
        timed_out = True
    finally:
        reader.join(timeout=5)
        process.stdout.close()
    require(not reader.is_alive() and not overflow and not observer_failed,
            'invalid_command_output')
    if observer:
        observer({'event': 'failed' if timed_out or process.returncode else 'completed',
                  'exit_code': process.returncode,
                  'duration_ms': int((time.monotonic() - started) * 1000),
                  **({'category': 'timeout' if timed_out else 'command-exit'}
                     if timed_out or process.returncode else {})})
    require(not timed_out, 'command_timeout')
    require(process.returncode == 0, 'command_failed')
    return checksum.hexdigest() if digest else bytes(output)


class Runtime:
    def __init__(self, config):
        require(isinstance(config, dict), 'invalid_config')
        self.config = config
        self.work = Path(config['work_dir'])
        self.run_dir = Path(config['runtime_dir'])
        self.user = pwd.getpwnam(config.get('user', 'imagebuilder'))
        self.control_dir = self.work / 'control'
        self.records = self.control_dir / 'jobs'
        self.jobs = self.work / 'jobs'
        self.cache = self.work / 'cache'
        self.control_path = self.run_dir / 'control.json'
        self.current_job = None
        self.current_step = None
        require(self.work.is_absolute() and self.run_dir.is_absolute(), 'invalid_config')
        require(re.fullmatch(r'https://[A-Za-z0-9.-]+/[A-Za-z0-9_./-]+',
                             config['repo_url']) is not None, 'invalid_config')
        for name in ('minimum_free_bytes', 'idle_seconds', 'timeout_seconds',
                     'retention_days', 'smoke_timeout_seconds'):
            require(type(config.get(name)) is int and config[name] > 0,
                    'invalid_config')
        cache_limit = config.get('cache_max_bytes', 20 * 1024 ** 3)
        require(type(cache_limit) is int and cache_limit > 0, 'invalid_config')
        if 'kernel_version' in config:
            require(isinstance(config['kernel_version'], str) and
                    re.fullmatch(r'[0-9][A-Za-z0-9._+-]{0,127}',
                                 config['kernel_version']), 'invalid_config')
        if 'kernel_modules' in config:
            require(isinstance(config['kernel_modules'], str) and
                    re.fullmatch(r'/(?:usr/)?lib/modules/[0-9][A-Za-z0-9._+-]*',
                                 config['kernel_modules']), 'invalid_config')
            if 'kernel_version' in config:
                require(Path(config['kernel_modules']).name == config['kernel_version'],
                        'invalid_config')
        if 'kernel_path' in config:
            require(isinstance(config['kernel_path'], str) and
                    re.fullmatch(r'/[A-Za-z0-9._/+-]+', config['kernel_path']) and
                    '..' not in Path(config['kernel_path']).parts, 'invalid_config')

    def initialize(self):
        require(not self.config.get('require_mount', True) or self.work.is_mount(),
                'work_disk_not_mounted')
        for path, owner, mode in (
                (self.control_dir, 0, 0o700), (self.records, 0, 0o700),
                (self.jobs, self.user.pw_uid, 0o700),
                (self.cache, self.user.pw_uid, 0o700),
                (self.run_dir, 0, 0o750),
                (self.run_dir / 'jobs', 0, 0o750),
                (self.run_dir / 'worker', 0, 0o750)):
            require(not path.is_symlink(), 'invalid_state')
            path.mkdir(parents=True, exist_ok=True)
            os.chown(path, owner, self.user.pw_gid)
            path.chmod(mode)
        if not self.control_path.exists():
            write_json(self.control_path,
                       {'active': None, 'maintenance': False, 'stopping': False,
                        'idle_since': int(time.time())})

    @contextlib.contextmanager
    def locked(self):
        descriptor = os.open(self.run_dir / 'control.lock',
                             os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
        try:
            fcntl.flock(descriptor, fcntl.LOCK_EX)
            yield read_json(self.control_path)
        finally:
            os.close(descriptor)

    def save_control(self, state):
        write_json(self.control_path, state)

    def record_path(self, identifier):
        return self.records / (job_id(identifier) + '.json')

    @property
    def publication_guard(self):
        return self.control_dir / 'publication-blocked.json'

    @property
    def maintenance_path(self):
        return self.control_dir / 'maintenance.json'

    def worker_path(self, identifier):
        return self.run_dir / 'worker' / job_id(identifier)

    def request_path(self, identifier):
        return self.run_dir / 'jobs' / job_id(identifier) / 'request.json'

    def progress_path(self, identifier):
        return self.jobs / job_id(identifier) / 'progress.json'

    def diagnostic_path(self, identifier):
        return self.jobs / job_id(identifier) / 'diagnostics.json'

    def known_sources(self, identifier):
        packer = self.jobs / job_id(identifier) / 'repo' / 'packer'
        if not packer.is_dir() or packer.is_symlink():
            return {}
        sources = list(packer.glob('*.pkr.hcl'))
        variables = packer / 'vars'
        if variables.is_dir() and not variables.is_symlink():
            sources += list(variables.glob('*.pkrvars.hcl'))
        return {str(path.relative_to(packer)): 'packer/' + str(path.relative_to(packer))
                for path in sources if path.is_file() and not path.is_symlink()
                and re.fullmatch(r'[A-Za-z0-9_./-]+', str(path.relative_to(packer)))}

    def diagnostics(self, identifier):
        path = self.diagnostic_path(identifier)
        if not path.exists():
            return {'next_sequence': 1, 'records': []}
        data = read_json(path)
        require(isinstance(data, dict) and type(data.get('next_sequence')) is int
                and 1 <= data['next_sequence'] < 2 ** 63 and
                isinstance(data.get('records'), list) and
                len(data['records']) <= MAX_LOG_RECORDS, 'invalid_state')
        records = [validate_log_record(record) for record in data['records']]
        previous = 0
        sources = set(self.known_sources(identifier).values())
        for record in records:
            require(previous < record['sequence'] < data['next_sequence'], 'invalid_state')
            require('source' not in record or record['source'] in sources, 'invalid_state')
            previous = record['sequence']
        return {'next_sequence': data['next_sequence'], 'records': records}

    def append_diagnostic(self, identifier, step, event, **fields):
        data = self.diagnostics(identifier)
        record = validate_log_record(dict(fields, step=step, event=event,
                                          sequence=data['next_sequence'],
                                          time=int(time.time())))
        if 'source' in record:
            require(record['source'] in set(self.known_sources(identifier).values()),
                    'invalid_state')
        data['next_sequence'] += 1
        data['records'] = (data['records'] + [record])[-MAX_LOG_RECORDS:]
        while len(json.dumps(data, separators=(',', ':')).encode()) > MAX_LOG_BYTES:
            data['records'].pop(0)
        write_json(self.diagnostic_path(identifier), data)

    def diagnostic_page(self, identifier, after):
        require(type(after) is int and 0 <= after < 2 ** 63)
        data = self.diagnostics(identifier)
        pending = [record for record in data['records'] if record['sequence'] > after]
        records = pending[:256]
        return {'records': records,
                'cursor': records[-1]['sequence'] if records else after,
                'has_more': len(pending) > len(records),
                'truncated': bool(data['records'] and
                                  after < data['records'][0]['sequence'] - 1)}

    def diagnostic_output(self, identifier, context, raw):
        text = raw.decode('utf-8', errors='replace').strip()
        marker = re.fullmatch(r'IMAGEBUILDER_STEP=(packer-init|packer-build|compress)', text)
        message = None
        if not marker:
            try:
                fields = next(csv.reader([text]))
            except (csv.Error, StopIteration):
                fields = []
            if (len(fields) >= 4 and re.fullmatch(r'[0-9]+', fields[0])
                    and fields[1] in ('', 'qemu.custom')):
                if fields[2] == 'artifact-count' and len(fields) == 4:
                    if re.fullmatch(r'[0-9]{1,5}', fields[3]):
                        self.append_diagnostic(identifier, context['step'],
                                               'artifact-count', count=int(fields[3]))
                    return
                if fields[2] == 'ui' and len(fields) >= 5:
                    value = ','.join(fields[4:]).replace('%!(PACKER_COMMA)', ',')
                    value = value.replace('\\n', '\n').replace('\\r', '\r')
                    value = re.sub(r'^(?:==> )?qemu\.custom(?: \(shell-local\))?:\s*',
                                   '', value.strip())
                    marker = re.fullmatch(
                        r'IMAGEBUILDER_STEP=(packer-init|packer-build|compress)', value)
                    if fields[3] == 'error':
                        message = value
            if message is None and (text in HCL_HEADERS or
                                    re.match(r'^on [A-Za-z0-9_./-]+\.pkr(?:vars)?\.hcl line ', text)):
                message = text
        if marker:
            now = time.monotonic()
            if context.get('substep'):
                self.append_diagnostic(identifier, context['step'], 'completed',
                                       exit_code=0,
                                       duration_ms=int((now - context['step_started']) * 1000))
            context.update(step=marker[1], substep=True, step_started=now)
            self.append_diagnostic(identifier, marker[1], 'started')
            return
        if message is None:
            return
        details = {'category': 'packer-error'}
        sources = self.known_sources(identifier)
        for line in message.splitlines():
            line = line.strip()
            if line in HCL_HEADERS:
                details['category'] = HCL_HEADERS[line]
            elif line == 'Timeout waiting for SSH.':
                details['category'] = 'ssh-timeout'
            status = re.search(r'(?:^|: )Script exited with non-zero exit status: '
                               r'([0-9]{1,3})\. Allowed exit codes are: \[[0-9, ]+\]$', line)
            if status and 0 <= int(status[1]) <= 255:
                details.update(category='provisioner-exit', provisioner_exit_code=int(status[1]))
            location = re.fullmatch(r'on ([A-Za-z0-9_./-]+\.pkr(?:vars)?\.hcl) line '
                                    r'([0-9]{1,9})(?:, column ([0-9]{1,9}))?(?:[,:].*)?', line)
            if location:
                source = sources.get(location[1])
                if source is None and location[1] in sources.values():
                    source = location[1]
                if source and int(location[2]) > 0:
                    details.update(source=source, line=int(location[2]))
                    if location[3] and int(location[3]) > 0:
                        details['column'] = int(location[3])
                    if details['category'] == 'packer-error':
                        details['category'] = 'hcl-validation'
        self.append_diagnostic(identifier, context['step'], 'packer-error', **details)

    def invoke(self, args, **kwargs):
        if self.current_job is None or self.current_step is None:
            return command(args, **kwargs)
        identifier, step = self.current_job, self.current_step
        context = {'step': step, 'substep': False}

        def observer(event):
            if event['event'] == 'output':
                if step in ('build', 'packer-init', 'packer-build', 'compress'):
                    self.diagnostic_output(identifier, context, event['line'])
                return
            if event['event'] in ('completed', 'failed') and context['substep']:
                facts = {key: event[key] for key in ('exit_code', 'category') if key in event}
                facts['duration_ms'] = int((time.monotonic() - context['step_started']) * 1000)
                self.append_diagnostic(identifier, context['step'], event['event'], **facts)
            facts = {key: event[key] for key in ('exit_code', 'category', 'duration_ms')
                     if key in event}
            self.append_diagnostic(identifier, step, event['event'], **facts)

        return command(args, observer=observer, **kwargs)

    def progress(self, identifier):
        path = self.progress_path(identifier)
        if not path.exists():
            return {'phase': 'preparing', 'code': 'accepted', 'events': []}
        result = read_json(path)
        require(result.get('phase') in PHASES, 'invalid_state')
        require(result.get('code') in CODES, 'invalid_state')
        safe = {'phase': result['phase'], 'code': result['code'], 'events': []}
        if 'sha256' in result:
            require(isinstance(result['sha256'], str) and
                    re.fullmatch(r'[0-9a-f]{64}', result['sha256']), 'invalid_state')
            safe['sha256'] = result['sha256']
        events = result.get('events', [])
        require(isinstance(events, list) and len(events) <= 100, 'invalid_state')
        for event in events:
            require(isinstance(event, dict) and event.get('phase') in PHASES and
                    event.get('code') in CODES and type(event.get('time')) is int,
                    'invalid_state')
            safe['events'].append({key: event[key] for key in ('phase', 'code', 'time')})
        return safe

    def update(self, identifier, phase, code, **fields):
        old = self.progress(identifier)
        event = {'phase': phase, 'code': code, 'time': int(time.time())}
        data = {'phase': phase, 'code': code,
                'events': (old.get('events', []) + [event])[-100:]}
        data.update(fields)
        write_json(self.progress_path(identifier), data)

    def safe_job(self, identifier):
        record = read_json(self.record_path(identifier))
        progress = (self.progress(identifier) if not record.get('finished')
                    else record['result'])
        require(progress.get('phase') in PHASES, 'invalid_state')
        result = {'ok': True, 'job_id': job_id(identifier),
                  'revision': record['revision'], 'target': record['target'],
                  'phase': progress['phase'], 'code': progress['code'],
                  'publish': record['publish'], 'created': record['created'],
                  'finished': record['finished']}
        if 'branch' in record:
            result['branch'] = validate_branch(record['branch'])
        require(re.fullmatch(r'[0-9a-f]{40}|[0-9a-f]{64}', result['revision']))
        require(re.fullmatch(r'[a-z][a-z0-9-]{0,63}', result['target']))
        require(result['code'] in CODES)
        if 'sha256' in progress:
            require(re.fullmatch(r'[0-9a-f]{64}', progress['sha256']))
            result['sha256'] = progress['sha256']
        return result

    def readiness(self):
        require(not self.config.get('require_mount', True)
                or self.work.is_mount(), 'work_disk_not_mounted')
        require(os.access('/dev/kvm', os.R_OK | os.W_OK), 'kvm_unavailable')
        for permission in ('-r', '-w'):
            try:
                self.invoke(['runuser', '-u', self.user.pw_name, '--',
                         'test', permission, '/dev/kvm'])
            except SafeError:
                raise SafeError('kvm_unavailable') from None
        for key, permission in (('kernel_path', '-r'), ('kernel_modules', '-x')):
            if key in self.config:
                try:
                    self.invoke(['runuser', '-u', self.user.pw_name, '--',
                             'test', '-f' if key == 'kernel_path' else '-d',
                             self.config[key]])
                    self.invoke(['runuser', '-u', self.user.pw_name, '--',
                             'test', permission, self.config[key]])
                except SafeError:
                    raise SafeError('kernel_unavailable') from None
        require(shutil.disk_usage(self.work).free >=
                self.config['minimum_free_bytes'], 'insufficient_space')
        for tool in ('git', 'packer', 'qemu-img', 'qemu-system-x86_64',
                     'virt-sparsify', 'rclone', 'ssh', 'ssh-agent', 'ssh-add',
                     'ssh-keygen', 'cloud-localds', 'curl', 'xz'):
            require(shutil.which(tool, path=BASE_ENV['PATH']) is not None,
                    'missing_tool')
        self.invoke(['cloud-init', 'status', '--wait'], timeout=120)
        return {'ok': True, 'phase': 'ready'}

    def store_request(self, identifier, request):
        directory = self.request_path(identifier).parent
        directory.mkdir(mode=0o750)
        os.chown(directory, 0, self.user.pw_gid)
        write_json(directory / 'request.json', request, 0o640)
        os.chown(directory / 'request.json', 0, self.user.pw_gid)
        worker = self.worker_path(identifier)
        worker.mkdir(mode=0o700)
        os.chown(worker, self.user.pw_uid, self.user.pw_gid)

    def cleanup_secrets(self, identifier):
        for directory in (self.request_path(identifier).parent,
                          self.worker_path(identifier)):
            if directory.is_symlink():
                directory.unlink()
            elif directory.exists():
                shutil.rmtree(directory)

    def submit(self, request, state):
        require(not state['stopping'], 'stopping')
        require(not state['maintenance'], 'maintenance')
        require(state['active'] is None, 'busy')
        require(not self.publication_guard.exists(), 'publication_blocked')
        require(set(request) <= {'op', 'revision', 'target', 'publish', 'branch',
                                 'credentials'})
        revision, target = request.get('revision'), request.get('target')
        require(isinstance(revision, str) and
                re.fullmatch(r'[0-9a-f]{40}|[0-9a-f]{64}', revision))
        require(isinstance(target, str) and
                re.fullmatch(r'[a-z][a-z0-9-]{0,63}', target))
        publish = request.get('publish', False)
        require(type(publish) is bool)
        secret_values = credentials(request.get('credentials', {}), publish)
        request = dict(request, publish=publish, credentials=secret_values)
        if 'branch' in request:
            validate_branch(request['branch'])
        self.readiness()
        identifier = uuid.uuid4().hex
        work = self.jobs / identifier
        work.mkdir(mode=0o700)
        os.chown(work, self.user.pw_uid, self.user.pw_gid)
        record = {'revision': revision, 'target': target, 'publish': publish,
                  'created': int(time.time()), 'finished': False}
        if 'branch' in request:
            record['branch'] = request['branch']
        write_json(self.record_path(identifier), record)
        self.store_request(identifier, request)
        state['active'] = identifier
        self.save_control(state)
        try:
            self.invoke(['systemctl', 'start', '--no-block',
                     f'imagebuilder-job@{identifier}.service'])
        except Exception:
            self.cleanup_secrets(identifier)
            record.update(finished=True, result={'phase': 'failed',
                                                'code': 'start_failed'})
            write_json(self.record_path(identifier), record)
            state.update(active=None, idle_since=int(time.time()))
            self.save_control(state)
            raise SafeError('start_failed') from None
        return {'ok': True, 'job_id': identifier, 'phase': 'preparing'}

    def retry(self, request, state):
        require(set(request) <= {'op', 'job_id', 'credentials'})
        identifier = job_id(request.get('job_id'))
        require(not state['stopping'], 'stopping')
        require(not state['maintenance'], 'maintenance')
        require(state['active'] is None, 'busy')
        if self.publication_guard.exists():
            require(read_json(self.publication_guard)['job_id'] == identifier,
                    'publication_blocked')
        record = read_json(self.record_path(identifier))
        require(record.get('finished') is True and
                record['result']['phase'] in TERMINAL, 'job_not_finished')
        require((self.jobs / identifier / 'verified.json').is_file(),
                'no_verified_artifact')
        credentials(request.get('credentials', {}), True)
        self.readiness()
        self.store_request(identifier,
                           {'op': 'publish-retry', 'revision': record['revision'],
                            'target': record['target'], 'publish': True,
                            'credentials': request['credentials']})
        self.progress_path(identifier).unlink(missing_ok=True)
        record.update(finished=False, publish=True)
        write_json(self.record_path(identifier), record)
        state['active'] = identifier
        self.save_control(state)
        try:
            self.invoke(['systemctl', 'reset-failed',
                     f'imagebuilder-job@{identifier}.service'])
            self.invoke(['systemctl', 'start', '--no-block',
                     f'imagebuilder-job@{identifier}.service'])
        except Exception:
            self.finish_locked(identifier, state)
            raise SafeError('start_failed') from None
        return {'ok': True, 'job_id': identifier, 'phase': 'preparing'}

    def control(self, request):
        require(isinstance(request, dict))
        operation = request.get('op')
        if operation == 'resolve':
            require(set(request) == {'op', 'branch'})
            branch = validate_branch(request['branch'])
            reference = 'refs/heads/' + branch
            try:
                output = self.invoke(['runuser', '-u', self.user.pw_name, '--',
                                  'git', 'ls-remote', '--exit-code', '--refs',
                                  self.config['repo_url'], reference],
                                 capture=True).decode().splitlines()
                require(len(output) == 1, 'branch_resolution_failed')
                fields = output[0].split('\t')
                require(len(fields) == 2 and fields[1] == reference and
                        re.fullmatch(r'[0-9a-f]{40}|[0-9a-f]{64}', fields[0]),
                        'branch_resolution_failed')
            except Exception:
                raise SafeError('branch_resolution_failed') from None
            return {'ok': True, 'revision': fields[0], 'branch': branch}
        with self.locked() as state:
            if operation == 'submit':
                return self.submit(request, state)
            if operation == 'publish-retry':
                return self.retry(request, state)
            if operation == 'ready':
                require(set(request) <= {'op', 'build_ready'} and
                        type(request.get('build_ready', True)) is bool)
                require(not state['stopping'], 'stopping')
                result = (self.readiness() if request.get('build_ready', True)
                          else {'ok': True, 'phase': 'ready'})
                if state['active'] is None:
                    state['idle_since'] = int(time.time())
                    self.save_control(state)
                return result
            if operation in ('status', 'logs'):
                require(set(request) <= ({'op', 'job_id', 'after'} if operation == 'logs'
                                         else {'op', 'job_id'}))
                identifier = request.get('job_id') or state['active']
                if identifier is None:
                    latest = sorted(self.records.glob('*.json'),
                                    key=lambda p: p.stat().st_mtime)
                    identifier = latest[-1].stem if latest else None
                if identifier is None:
                    return {'ok': True, 'phase': 'stopping' if state['stopping']
                            else 'idle', 'maintenance': state['maintenance'],
                            'publication_blocked': self.publication_guard.exists()}
                result = self.safe_job(identifier)
                result['maintenance'] = state['maintenance']
                result['publication_blocked'] = self.publication_guard.exists()
                if operation == 'logs':
                    result['events'] = self.progress(identifier)['events']
                    result.update(self.diagnostic_page(identifier, request.get('after', 0)))
                return result
            if operation == 'maintenance':
                require(set(request) == {'op', 'enabled'} and
                        type(request['enabled']) is bool)
                require(not state['stopping'], 'stopping')
                state.update(maintenance=request['enabled'],
                             idle_since=int(time.time()))
                write_json(self.maintenance_path, {'enabled': request['enabled']})
                self.save_control(state)
                return {'ok': True, 'maintenance': state['maintenance']}
            if operation == 'shutdown':
                require(set(request) == {'op'})
                return self.shutdown(state)
            raise SafeError('invalid_request')

    def shutdown(self, state):
        require(state['active'] is None, 'busy')
        require(not state['stopping'], 'stopping')
        state['stopping'] = True
        self.save_control(state)
        try:
            self.invoke(['systemctl', 'poweroff', '--no-block'])
        except Exception:
            state['stopping'] = False
            self.save_control(state)
            raise SafeError('shutdown_failed') from None
        return {'ok': True, 'phase': 'stopping'}

    def finish_locked(self, identifier, state):
        record = read_json(self.record_path(identifier))
        try:
            result = self.progress(identifier)
            if result['phase'] not in TERMINAL:
                result = {'phase': 'failed', 'code':
                          'publication_blocked' if result['phase'] == 'publishing'
                          else 'service_failed'}
            result = {'phase': result['phase'], 'code': result['code'],
                      **({'sha256': result['sha256']} if 'sha256' in result else {})}
        except Exception:
            result = {'phase': 'failed', 'code': 'publication_blocked'
                      if record['publish'] else 'invalid_progress'}
        self.cleanup_secrets(identifier)
        if result['code'] == 'publication_blocked':
            write_json(self.publication_guard, {'job_id': identifier})
        elif (result['phase'] == 'succeeded' and result['code'] == 'published'
              and self.publication_guard.exists()
              and read_json(self.publication_guard)['job_id'] == identifier):
            self.publication_guard.unlink()
        record.update(finished=True, result=result)
        write_json(self.record_path(identifier), record)
        if state['active'] == identifier:
            state.update(active=None, idle_since=int(time.time()))
            self.save_control(state)

    def finish(self, identifier):
        with self.locked() as state:
            self.finish_locked(job_id(identifier), state)

    def boot(self):
        self.initialize()
        with self.locked() as state:
            if state['active'] is not None:
                identifier = job_id(state['active'])
                active = self.invoke(['systemctl', 'show',
                                  f'imagebuilder-job@{identifier}.service',
                                  '--property=ActiveState', '--value'],
                                 capture=True).decode().strip()
                require(active in ('active', 'activating', 'reloading',
                                   'deactivating', 'inactive', 'failed'),
                        'invalid_state')
                require(active in ('inactive', 'failed'), 'busy')
            for path in self.records.glob('*.json'):
                record = read_json(path)
                if not record.get('finished'):
                    try:
                        progress = self.progress(path.stem)
                    except Exception:
                        progress = {'phase': 'publishing' if record['publish']
                                    else 'preparing'}
                    if (progress['phase'] == 'publishing' or
                            (self.jobs / path.stem / 'publication.json').exists()):
                        write_json(self.publication_guard, {'job_id': path.stem})
                    record.update(finished=True,
                                  result={'phase': 'interrupted',
                                          'code': 'host_restarted'})
                    write_json(path, record)
            for parent in ('jobs', 'worker'):
                for path in (self.run_dir / parent).iterdir():
                    if re.fullmatch(r'[0-9a-f]{32}', path.name):
                        if path.is_symlink():
                            path.unlink()
                        elif path.is_dir():
                            shutil.rmtree(path)
            maintenance = (read_json(self.maintenance_path)['enabled']
                           if self.maintenance_path.exists() else False)
            require(type(maintenance) is bool, 'invalid_state')
            state.update(active=None, maintenance=maintenance, stopping=False,
                         idle_since=int(time.time()))
            self.save_control(state)

    def idle(self):
        with self.locked() as state:
            self.prune(state)
            if (state['active'] is None and not state['maintenance'] and
                    not state['stopping'] and time.time() - state['idle_since']
                    >= self.config['idle_seconds']):
                self.shutdown(state)

    def prune(self, state):
        cutoff = time.time() - self.config['retention_days'] * 86400
        blocked = (read_json(self.publication_guard)['job_id']
                   if self.publication_guard.exists() else None)
        for path in self.records.glob('*.json'):
            record = read_json(path)
            identifier = job_id(path.stem)
            if (record.get('finished') and record['created'] < cutoff and
                    identifier not in (state['active'], blocked)):
                directory = self.jobs / identifier
                if directory.is_symlink():
                    directory.unlink()
                elif directory.is_dir():
                    shutil.rmtree(directory)
                path.unlink()
        if state['active'] is None and (self.cache / 'packer').exists():
            self.invoke(['runuser', '-u', self.user.pw_name, '--',
                     '/usr/local/libexec/imagebuilder', 'cache-prune'])

    def prune_cache(self):
        require(os.geteuid() == self.user.pw_uid, 'invalid_config')
        cache = self.cache / 'packer'
        if cache.is_symlink():
            cache.unlink()
            return
        if not cache.is_dir():
            return
        total, newest, count = 0, 0, 0
        for directory, _directories, files in os.walk(cache, followlinks=False):
            for name in files:
                info = (Path(directory) / name).lstat()
                if stat.S_ISREG(info.st_mode):
                    total += info.st_size
                    newest = max(newest, info.st_mtime)
                count += 1
                if count > 100000:
                    break
            if count > 100000:
                break
        if (count > 100000 or total > self.config.get('cache_max_bytes', 20 * 1024 ** 3)
                or (count and newest < time.time() -
                    self.config['retention_days'] * 86400)):
            shutil.rmtree(cache)

    def checkout(self, identifier, request, env):
        self.current_step = 'checkout'
        checkout = self.jobs / identifier / 'repo'
        checkout.mkdir(mode=0o700)
        self.invoke(['git', 'init', '--quiet', str(checkout)], env=env)
        self.invoke(['git', '-C', str(checkout), 'remote', 'add', 'origin',
                 self.config['repo_url']], env=env)
        self.invoke(['git', '-C', str(checkout), 'fetch', '--depth=1', 'origin',
                 request['revision']], env=env, timeout=300)
        self.invoke(['git', '-C', str(checkout), 'checkout', '--detach',
                 request['revision']], env=env)
        actual = self.invoke(['git', '-C', str(checkout), 'rev-parse', 'HEAD'],
                         env=env, capture=True).decode().strip()
        require(actual == request['revision'], 'revision_mismatch')
        return checkout

    def target_command(self, checkout, target):
        packer = checkout / 'packer'
        if target == 'all' or (packer / 'vars' / (target + '.pkrvars.hcl')).is_file():
            return ['bash', './build.sh', '-y', target]
        source = (packer / 'import-upstream.sh').read_text()
        match = re.search(r'import_target\(\)\s*\{(.*?)\n\}', source, re.S)
        allowed = re.findall(r'^\s*([a-z][a-z0-9-]*)\)', match[1], re.M) if match else []
        require(target in allowed, 'unknown_target')
        return ['bash', './import-upstream.sh', '-y', target]

    def ssh_environment(self, identifier, env):
        self.current_step = 'ssh-setup'
        directory = self.worker_path(identifier)
        key = directory / 'id_ed25519'
        self.invoke(['ssh-keygen', '-q', '-t', 'ed25519', '-N', '', '-f', str(key)],
                env=env)
        agent = self.invoke(['ssh-agent', '-s'], env=env, capture=True).decode()
        sock = re.search(r'^SSH_AUTH_SOCK=([^;\n]+);', agent, re.M)
        pid = re.search(r'^SSH_AGENT_PID=([0-9]+);', agent, re.M)
        require(sock is not None and pid is not None, 'ssh_agent_failed')
        env.update(SSH_AUTH_SOCK=sock[1], SSH_AGENT_PID=pid[1])
        self.invoke(['ssh-add', str(key)], env=env)
        public = key.with_suffix('.pub').read_text().strip()
        require(re.fullmatch(r'ssh-ed25519 [A-Za-z0-9+/=]+(?: [^\r\n]*)?', public),
                'ssh_key_failed')
        env['PKR_VAR_ssh_pubkey'] = public

    def artifacts(self, checkout):
        self.current_step = 'artifact-check'
        images = checkout / 'packer' / 'images'
        result = []
        for image in sorted(images.glob('*.img')):
            require(not image.is_symlink() and image.is_file(), 'invalid_artifact')
            require(re.fullmatch(r'[a-z0-9][a-z0-9.-]+\.img', image.name),
                    'invalid_artifact')
            with image.open('rb') as stream:
                digest = hashlib.file_digest(stream, 'sha256').hexdigest()
            sidecar = Path(str(image) + '.sha256')
            require(not sidecar.is_symlink() and sidecar.stat().st_size <= 65,
                    'invalid_artifact')
            require(sidecar.read_text().strip() == digest, 'checksum_mismatch')
            self.invoke(['qemu-img', 'check', '-f', 'qcow2', str(image)], timeout=300)
            result.append({'name': image.name, 'sha256': digest})
        require(bool(result), 'no_artifacts')
        return result

    def smoke_configuration(self, identifier, image, public):
        marker = '/var/tmp/imagebuilder-smoke-' + job_id(identifier)
        user = {'name': 'imagecheck', 'shell': '/bin/sh',
                'ssh_authorized_keys': [public]}
        configuration = {'users': [user], 'ssh_pwauth': False,
                         'runcmd': ['touch ' + marker]}
        check = 'test -f ' + marker
        if not image.name.startswith('freebsd-'):
            user.update(sudo='ALL=(ALL) NOPASSWD:ALL', lock_passwd=True)
            check = ('sudo -n cloud-init status --wait && '
                     'systemctl is-active --quiet qemu-guest-agent && ' + check)
        # FreeBSD BASIC-CLOUDINIT uses nuageinit and has neither sudo nor cloud-init.
        return configuration, check

    def smoke(self, identifier, image, env):
        self.current_step = 'smoke-boot'
        directory = self.worker_path(identifier)
        overlay = self.jobs / identifier / 'smoke.qcow2'
        seed = directory / 'seed.img'
        userdata, check = self.smoke_configuration(identifier, image,
                                                 env['PKR_VAR_ssh_pubkey'])
        (directory / 'user-data').write_text('#cloud-config\n' + json.dumps(userdata))
        (directory / 'meta-data').write_text(json.dumps({'instance-id': identifier}))
        self.invoke(['cloud-localds', str(seed), str(directory / 'user-data'),
                 str(directory / 'meta-data')], env=env)
        self.invoke(['qemu-img', 'create', '-f', 'qcow2', '-F', 'qcow2', '-b',
                 str(image), str(overlay)], env=env)
        with socket.socket() as port_socket:
            port_socket.bind(('127.0.0.1', 0))
            port = port_socket.getsockname()[1]
        process = subprocess.Popen(
            ['qemu-system-x86_64', '-enable-kvm', '-cpu', 'host', '-m', '2048',
             '-smp', '2', '-display', 'none', '-serial', 'none', '-monitor', 'none',
             '-drive', f'file={overlay},format=qcow2,if=virtio',
             '-drive', f'file={seed},format=raw,media=cdrom,readonly=on',
             '-netdev', f'user,id=net0,hostfwd=tcp:127.0.0.1:{port}-:22',
             '-device', 'virtio-net-pci,netdev=net0',
             '-device', 'virtio-serial-pci', '-chardev',
             f'socket,path={directory / "agent.sock"},server=on,wait=off,id=agent',
             '-device', 'virtserialport,chardev=agent,name=org.qemu.guest_agent.0'],
            env=env, stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL)
        try:
            deadline = time.monotonic() + self.config['smoke_timeout_seconds']
            while time.monotonic() < deadline:
                require(process.poll() is None, 'smoke_boot_failed')
                try:
                    self.invoke(['ssh', '-F', '/dev/null', '-o', 'BatchMode=yes',
                             '-o', 'ConnectTimeout=5', '-o', 'StrictHostKeyChecking=no',
                             '-o', 'UserKnownHostsFile=/dev/null', '-i',
                             str(directory / 'id_ed25519'), '-p', str(port),
                             'imagecheck@127.0.0.1', check], env=env, timeout=15)
                    return
                except SafeError:
                    time.sleep(2)
            raise SafeError('smoke_timeout')
        finally:
            process.terminate()
            try:
                process.wait(timeout=15)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
            overlay.unlink(missing_ok=True)
            seed.unlink(missing_ok=True)

    def remote_env(self, env):
        result = env.copy()
        result.update(RCLONE_CONFIG_SEAWEEDFS_TYPE='s3',
                      RCLONE_CONFIG_SEAWEEDFS_PROVIDER='Other',
                      RCLONE_CONFIG_SEAWEEDFS_REGION='us-east-1',
                      RCLONE_CONFIG_SEAWEEDFS_NO_CHECK_BUCKET='true',
                      RCLONE_CONFIG_SEAWEEDFS_USE_MULTIPART_UPLOADS='false')
        for key, name in (('ENDPOINT', 'ENDPOINT'), ('ACCESS_KEY_ID', 'ACCESS_KEY'),
                          ('SECRET_ACCESS_KEY', 'SECRET_KEY')):
            result['RCLONE_CONFIG_SEAWEEDFS_' + key] = env['SEAWEEDFS_S3_' + name]
        return result

    def remote_digest(self, path, env):
        return self.invoke(['rclone', 'cat', path], env=env,
                       timeout=self.config['timeout_seconds'], digest=True)

    def publication_snapshot(self, path, manifest):
        snapshot = read_json(path)
        require(isinstance(snapshot, dict) and set(snapshot) == {'artifacts'}
                and isinstance(snapshot['artifacts'], list), 'invalid_state')
        require(len(snapshot['artifacts']) == len(manifest), 'invalid_state')
        safe = []
        for old, current in zip(snapshot['artifacts'], manifest):
            require(isinstance(old, dict) and set(old) == {'name', 'present', 'sha256'}
                    and old['name'] == current['name'] and type(old['present']) is bool,
                    'invalid_state')
            require((not old['present'] and old['sha256'] is None) or
                    (old['present'] and isinstance(old['sha256'], str) and
                     re.fullmatch(r'[0-9a-f]{64}', old['sha256'])), 'invalid_state')
            safe.append({key: old[key] for key in ('name', 'present', 'sha256')})
        return safe

    def publish(self, identifier, checkout, manifest, env):
        self.current_step = 'publish'
        remote_env = self.remote_env(env)
        bucket = env.get('SEAWEEDFS_CLOUD_IMAGES_BUCKET', 'cloud-images')
        remote = f'seaweedfs:{bucket}'
        backup = self.jobs / identifier / 'publication.json'
        if not backup.exists():
            old = []
            # A verified pair is retained before any consumer-visible overwrite.
            listing = self.invoke(['rclone', 'lsf', '--files-only', '--max-depth', '1',
                               remote], env=remote_env, capture=True)
            existing = set(listing.decode().splitlines())
            for artifact in manifest:
                name = artifact['name']
                present = name in existing
                require(present == (name + '.sha256' in existing),
                        'remote_pair_incomplete')
                digest = None
                if present:
                    digest = self.remote_digest(f'{remote}/{name}', remote_env)
                    sidecar = self.invoke(['rclone', 'cat', f'{remote}/{name}.sha256'],
                                      env=remote_env, capture=True).decode().strip()
                    require(sidecar == digest, 'remote_checksum_mismatch')
                    for suffix in ('', '.sha256'):
                        self.invoke(['rclone', 'copyto', f'{remote}/{name}{suffix}',
                                 f'{remote}/.imagebuilder-backups/{identifier}/{name}{suffix}'],
                                env=remote_env, timeout=self.config['timeout_seconds'])
                old.append({'name': name, 'present': present, 'sha256': digest})
            write_json(backup, {'artifacts': old})
        snapshot = self.publication_snapshot(backup, manifest)
        try:
            self.invoke(['bash', './push.sh', read_json(self.request_path(identifier))['target']],
                    cwd=checkout / 'packer', env=env,
                    timeout=self.config['timeout_seconds'])
            for artifact in manifest:
                name = artifact['name']
                require(self.remote_digest(f'{remote}/{name}', remote_env) ==
                        artifact['sha256'], 'remote_checksum_mismatch')
                sidecar = self.invoke(['rclone', 'cat', f'{remote}/{name}.sha256'],
                                  env=remote_env, capture=True).decode().strip()
                require(sidecar == artifact['sha256'], 'remote_checksum_mismatch')
        except Exception:
            self.current_step = 'rollback'
            try:
                for artifact in snapshot:
                    name = artifact['name']
                    for suffix in ('', '.sha256'):
                        if artifact['present']:
                            self.invoke(['rclone', 'copyto',
                                     f'{remote}/.imagebuilder-backups/{identifier}/{name}{suffix}',
                                     f'{remote}/{name}{suffix}'], env=remote_env,
                                    timeout=self.config['timeout_seconds'])
                        else:
                            with contextlib.suppress(SafeError):
                                self.invoke(['rclone', 'deletefile', f'{remote}/{name}{suffix}'],
                                        env=remote_env, timeout=self.config['timeout_seconds'])
                    if artifact['present']:
                        require(self.remote_digest(f'{remote}/{name}', remote_env)
                                == artifact['sha256'], 'rollback_failed')
                        restored = self.invoke(['rclone', 'cat', f'{remote}/{name}.sha256'],
                                           env=remote_env, capture=True).decode().strip()
                        require(restored == artifact['sha256'], 'rollback_failed')
                    else:
                        listing = self.invoke(['rclone', 'lsf', '--files-only',
                                           '--max-depth', '1', remote],
                                          env=remote_env, capture=True)
                        remaining = set(listing.decode().splitlines())
                        require(name not in remaining and name + '.sha256' not in remaining,
                                'rollback_failed')
            except Exception:
                raise SafeError('publication_blocked') from None
            raise SafeError('publication_failed_restored') from None

    def run(self, identifier):
        identifier = job_id(identifier)
        request = read_json(self.request_path(identifier))
        directory = self.jobs / identifier
        env = dict(BASE_ENV, HOME=self.user.pw_dir,
                   PACKER_CACHE_DIR=str(self.cache / 'packer'),
                   TMPDIR=str(self.worker_path(identifier)))
        secret_values = credentials(request.get('credentials', {}), request['publish'])
        for key, variable in (('kernel_path', 'SUPERMIN_KERNEL'),
                              ('kernel_version', 'SUPERMIN_KERNEL_VERSION'),
                              ('kernel_modules', 'SUPERMIN_MODULES')):
            if key in self.config:
                env[variable] = self.config[key]
        agent_pid = None
        self.current_job = identifier
        self.current_step = 'checkout'
        try:
            self.update(identifier, 'preparing', 'preparing')
            if request['op'] == 'publish-retry':
                checkout = directory / 'repo'
                manifest = read_json(directory / 'verified.json')
                require(self.artifacts(checkout) == manifest, 'artifact_changed')
            else:
                checkout = self.checkout(identifier, request, env)
                args = self.target_command(checkout, request['target'])
                require(args[1] == './import-upstream.sh' or
                        'PKR_VAR_user_password' in secret_values, 'missing_credentials')
                self.ssh_environment(identifier, env)
                agent_pid = int(env['SSH_AGENT_PID'])
                self.update(identifier, 'building', 'building')
                build_env = env.copy()
                if 'PKR_VAR_user_password' in secret_values:
                    build_env['PKR_VAR_user_password'] = secret_values['PKR_VAR_user_password']
                build_env['IMAGEBUILDER_LOG_EVENTS'] = '1'
                self.current_step = 'import' if args[1] == './import-upstream.sh' else 'build'
                self.invoke(args, cwd=checkout / 'packer', env=build_env,
                        timeout=self.config['timeout_seconds'])
                self.update(identifier, 'verifying', 'verifying')
                manifest = self.artifacts(checkout)
                for artifact in manifest:
                    self.smoke(identifier, checkout / 'packer' / 'images' /
                               artifact['name'], env)
                write_json(directory / 'verified.json', manifest)
            if request['publish']:
                self.update(identifier, 'publishing', 'publishing')
                publish_env = dict(env, **{key: value for key, value in secret_values.items()
                                          if key != 'PKR_VAR_user_password'})
                self.publish(identifier, checkout, manifest, publish_env)
            fields = {'sha256': manifest[0]['sha256']} if len(manifest) == 1 else {}
            self.update(identifier, 'succeeded',
                        'published' if request['publish'] else 'verified', **fields)
            return 0
        except SafeError as error:
            self.update(identifier, 'failed', error_code(error))
            return 1
        except Exception:
            self.update(identifier, 'failed', 'internal_error')
            return 1
        finally:
            self.current_job = None
            self.current_step = None
            if agent_pid is not None:
                with contextlib.suppress(ProcessLookupError):
                    os.kill(agent_pid, signal.SIGTERM)


def main():
    try:
        runtime = Runtime(read_json(CONFIG))
        require(len(sys.argv) in (2, 3))
        operation = sys.argv[1]
        if operation == 'control' and len(sys.argv) == 2:
            raw = sys.stdin.buffer.read(262145)
            require(len(raw) <= 262144)
            request = json.loads(raw)
            response = runtime.control(request)
            print(json.dumps(response, separators=(',', ':')))
        elif operation in ('boot', 'idle') and len(sys.argv) == 2:
            getattr(runtime, operation)()
        elif operation == 'cache-prune' and len(sys.argv) == 2:
            runtime.prune_cache()
        elif operation in ('run', 'finish') and len(sys.argv) == 3:
            result = getattr(runtime, operation)(job_id(sys.argv[2]))
            return result or 0
        else:
            raise SafeError('invalid_request')
        return 0
    except SafeError as error:
        print(json.dumps({'ok': False, 'error': error_code(error)}, separators=(',', ':')))
    except Exception:
        print('{"ok":false,"error":"internal_error"}')
    return 1


if __name__ == '__main__':
    sys.exit(main())
