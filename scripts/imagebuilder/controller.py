#!/usr/bin/env python3

import argparse
import contextlib
import json
import os
import re
import ssl
import subprocess
import sys
import time
import urllib.request

from runtime import (CODES, CREDENTIALS, PHASES, TERMINAL, SafeError,
                     validate_branch, validate_log_record)


class ControlError(Exception):
    pass


def require(condition, code):
    if not condition:
        raise ControlError(code)


def safe_response(data):
    require(isinstance(data, dict) and type(data.get('ok')) is bool,
            'invalid_builder_response')
    if not data['ok']:
        require(data.get('error') in CODES, 'invalid_builder_response')
        raise ControlError(data['error'])
    result = {'ok': True}
    for key in ('maintenance', 'publication_blocked', 'publish', 'finished',
                'truncated', 'has_more'):
        if key in data:
            require(type(data[key]) is bool, 'invalid_builder_response')
            result[key] = data[key]
    if 'branch' in data:
        try:
            result['branch'] = validate_branch(data['branch'])
        except SafeError:
            raise ControlError('invalid_builder_response') from None
    for key, pattern in (('job_id', r'[0-9a-f]{32}'),
                         ('revision', r'[0-9a-f]{40}|[0-9a-f]{64}'),
                         ('target', r'[a-z][a-z0-9-]{0,63}'),
                         ('sha256', r'[0-9a-f]{64}')):
        if key in data:
            require(isinstance(data[key], str) and
                    re.fullmatch(pattern, data[key]), 'invalid_builder_response')
            result[key] = data[key]
    if 'phase' in data:
        require(data['phase'] in PHASES | {'idle', 'ready', 'stopping'},
                'invalid_builder_response')
        result['phase'] = data['phase']
    if 'code' in data:
        require(data['code'] in CODES, 'invalid_builder_response')
        result['code'] = data['code']
    if 'created' in data:
        require(type(data['created']) is int, 'invalid_builder_response')
        result['created'] = data['created']
    if 'events' in data:
        require(isinstance(data['events'], list) and len(data['events']) <= 100,
                'invalid_builder_response')
        events = []
        for event in data['events']:
            require(isinstance(event, dict) and event.get('phase') in PHASES
                    and event.get('code') in CODES
                    and type(event.get('time')) is int, 'invalid_builder_response')
            events.append({key: event[key] for key in ('phase', 'code', 'time')})
        result['events'] = events
    if 'cursor' in data:
        require(type(data['cursor']) is int and 0 <= data['cursor'] < 2 ** 63,
                'invalid_builder_response')
        result['cursor'] = data['cursor']
    if 'records' in data:
        require(isinstance(data['records'], list) and len(data['records']) <= 256,
                'invalid_builder_response')
        try:
            result['records'] = [validate_log_record(record) for record in data['records']]
        except (SafeError, TypeError, KeyError):
            raise ControlError('invalid_builder_response') from None
    return result


class Controller:
    def __init__(self, environ=None):
        self.env = dict(os.environ if environ is None else environ)
        self.url = self.env.get('IMAGEBUILDER_PROXMOX_URL',
                                'https://192.168.10.10:8006').rstrip('/')
        require(re.fullmatch(r'https://[a-zA-Z0-9.-]+(?::[0-9]+)?', self.url),
                'invalid_proxmox_url')
        self.node = self.env.get('IMAGEBUILDER_NODE', 'pve')
        require(re.fullmatch(r'[a-zA-Z0-9-]+', self.node), 'invalid_node')
        self.token = self.env.get('IMAGEBUILDER_PROXMOX_TOKEN', '')
        require(self.token and '\n' not in self.token and '\r' not in self.token,
                'missing_proxmox_token')
        self.host = self.env.get('IMAGEBUILDER_SSH_HOST', '192.168.20.25')
        self.user = self.env.get('IMAGEBUILDER_SSH_USER', 'imagebuilder-control')
        require(re.fullmatch(r'[A-Za-z0-9.-]+', self.host), 'invalid_ssh_host')
        require(re.fullmatch(r'[a-z_][a-z0-9_-]*', self.user), 'invalid_ssh_user')
        self.context = ssl.create_default_context(
            cafile=self.env.get('IMAGEBUILDER_PROXMOX_CA_FILE'))
        self.vmid = self.env.get('IMAGEBUILDER_VMID')
        if self.vmid is not None:
            require(re.fullmatch(r'[1-9][0-9]{2,8}', self.vmid), 'invalid_vmid')

    def api(self, suffix, *, post=False):
        request = urllib.request.Request(
            f'{self.url}/api2/json/nodes/{self.node}/{suffix}',
            data=b'' if post else None,
            method='POST' if post else 'GET')
        request.add_unredirected_header('Authorization', 'PVEAPIToken=' + self.token)
        try:
            with urllib.request.urlopen(request, context=self.context,
                                        timeout=30) as response:
                raw = response.read(1048577)
            require(len(raw) <= 1048576, 'invalid_proxmox_response')
            data = json.loads(raw)
            require(isinstance(data, dict) and 'data' in data,
                    'invalid_proxmox_response')
            return data['data']
        except ControlError:
            raise
        except Exception:
            raise ControlError('proxmox_request_failed') from None

    def resolve(self):
        if self.vmid is None:
            data = self.api('qemu')
            require(isinstance(data, list), 'invalid_proxmox_response')
            matches = [item for item in data if isinstance(item, dict)
                       and item.get('name') == 'imagebuilder1']
            require(len(matches) == 1, 'builder_not_found')
            value = matches[0].get('vmid')
            require(type(value) is int and 100 <= value <= 999999999,
                    'invalid_proxmox_response')
            self.vmid = str(value)
        data = self.api(f'qemu/{self.vmid}/status/current')
        require(isinstance(data, dict) and data.get('name') == 'imagebuilder1',
                'unexpected_vm_identity')
        require(data.get('status') in ('running', 'stopped'),
                'invalid_proxmox_response')
        return data['status']

    def power(self):
        return self.resolve()

    def remote(self, request):
        args = ['ssh', '-o', 'BatchMode=yes', '-o', 'ForwardAgent=no',
                '-o', 'SendEnv=-*',
                '-o', 'ConnectTimeout=10', '-o', 'StrictHostKeyChecking=yes',
                '-T', self.user + '@' + self.host,
                'sudo -n /usr/local/libexec/imagebuilder control']
        try:
            ssh_env = {key: self.env[key] for key in
                       ('HOME', 'PATH', 'SSH_AUTH_SOCK') if key in self.env}
            result = subprocess.run(args, input=json.dumps(request).encode(),
                                    stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                    timeout=180, check=False, env=ssh_env)
            require(len(result.stdout) <= 262144, 'invalid_builder_response')
            data = json.loads(result.stdout)
            response = safe_response(data)
            require(result.returncode == 0, 'builder_request_failed')
            return response
        except ControlError:
            raise
        except Exception:
            raise ControlError('builder_unreachable') from None

    def wake(self, *, build_ready=True):
        deadline = time.monotonic() + 900
        last_error = 'builder_start_timeout'
        while time.monotonic() < deadline:
            if self.power() == 'stopped':
                try:
                    self.api(f'qemu/{self.vmid}/status/start', post=True)
                except ControlError:
                    # Another controller may have started the same VM.
                    if self.power() != 'running':
                        raise
            try:
                return self.remote({'op': 'ready', **(
                    {} if build_ready else {'build_ready': False})})
            except ControlError as error:
                last_error = str(error)
                if last_error not in ('builder_unreachable', 'internal_error',
                                      'stopping', 'work_disk_not_mounted',
                                      'command_failed', 'command_timeout'):
                    raise
            time.sleep(5)
        raise ControlError(last_error)

    def secret_values(self, publish, *, build=True):
        values = {key: self.env[key] for key in CREDENTIALS if self.env.get(key)}
        if build:
            require('PKR_VAR_user_password' in values, 'missing_build_password')
        if publish:
            require({'SEAWEEDFS_S3_ENDPOINT', 'SEAWEEDFS_S3_ACCESS_KEY',
                     'SEAWEEDFS_S3_SECRET_KEY'} <= set(values),
                    'missing_publish_credentials')
        else:
            values = {key: value for key, value in values.items()
                      if key == 'PKR_VAR_user_password'}
        return values

    def wait_job(self, identifier):
        state = self.follow_logs(identifier)
        if state['phase'] != 'succeeded':
            emit(state)
            raise ControlError('job_failed')
        return state

    def wait_stopped(self):
        deadline = time.monotonic() + 900
        while time.monotonic() < deadline:
            if self.power() == 'stopped':
                return {'ok': True, 'power': 'stopped'}
            time.sleep(5)
        raise ControlError('shutdown_timeout')

    def follow_logs(self, identifier, after=0):
        deadline = time.monotonic() + 14700
        while time.monotonic() < deadline:
            page = self.remote({'op': 'logs', 'job_id': identifier, 'after': after})
            require(page.get('job_id') == identifier, 'unexpected_job_identity')
            require(type(page.get('cursor')) is int and page['cursor'] >= after
                    and type(page.get('has_more')) is bool,
                    'invalid_builder_response')
            if page.get('truncated'):
                emit({'ok': True, 'job_id': identifier, 'truncated': True,
                      'cursor': after})
            for record in page.get('records', []):
                require(record['sequence'] > after, 'invalid_builder_response')
                emit({'job_id': identifier, **record})
                after = record['sequence']
            require(page['cursor'] == after, 'invalid_builder_response')
            if page['has_more']:
                require(bool(page.get('records')), 'invalid_builder_response')
                continue
            if page.get('finished'):
                require(page.get('phase') in TERMINAL, 'invalid_builder_response')
                return {key: page[key] for key in (
                    'ok', 'job_id', 'phase', 'code', 'finished', 'cursor',
                    'branch', 'revision', 'publication_blocked') if key in page}
            time.sleep(2)
        raise ControlError('log_wait_timeout')

    def execute(self, args):
        if args.command == 'status':
            if self.power() == 'stopped':
                return {'ok': True, 'power': 'stopped'}
            return self.remote({'op': 'status', **(
                {'job_id': args.job_id} if args.job_id else {})})
        if args.command == 'shutdown':
            if self.power() == 'stopped':
                return {'ok': True, 'power': 'stopped'}
            self.remote({'op': 'shutdown'})
            return self.wait_stopped()
        if args.command == 'run':
            if args.branch is not None:
                try:
                    validate_branch(args.branch)
                except SafeError:
                    raise ControlError('invalid_branch') from None
            else:
                require(re.fullmatch(r'[0-9a-f]{40}|[0-9a-f]{64}', args.revision),
                        'invalid_revision')
            require(re.fullmatch(r'[a-z][a-z0-9-]{0,63}', args.target),
                    'invalid_target')
            # Import-only targets do not consume the cloud-init password.
            values = self.secret_values(args.publish, build=False)
            request = {'op': 'submit', 'revision': args.revision,
                       'target': args.target, 'publish': args.publish,
                       'credentials': values}
        elif args.command == 'publish-retry':
            require(re.fullmatch(r'[0-9a-f]{32}', args.job_id), 'invalid_job_id')
            request = {'op': 'publish-retry', 'job_id': args.job_id,
                       'credentials': self.secret_values(True, build=False)}
        else:
            request = None
        if args.command == 'logs':
            require(re.fullmatch(r'[0-9a-f]{32}', args.job_id), 'invalid_job_id')
            require(0 <= args.after < 2 ** 63, 'invalid_log_cursor')
        self.wake(build_ready=args.command in ('run', 'publish-retry'))
        if args.command == 'start':
            if args.keep_running:
                return self.remote({'op': 'maintenance', 'enabled': True})
            return {'ok': True, 'power': 'running'}
        if args.command == 'maintenance':
            return self.remote({'op': 'maintenance', 'enabled': args.enabled == 'on'})
        if args.command == 'logs':
            if args.follow:
                return self.follow_logs(args.job_id, args.after)
            return self.remote({'op': 'logs', 'job_id': args.job_id,
                                'after': args.after})
        if args.command == 'run' and args.branch is not None:
            resolved = self.remote({'op': 'resolve', 'branch': args.branch})
            require(resolved.get('branch') == args.branch and
                    re.fullmatch(r'[0-9a-f]{40}|[0-9a-f]{64}',
                                 resolved.get('revision', '')),
                    'invalid_builder_response')
            request.update(revision=resolved['revision'], branch=args.branch)
            emit(resolved)
        result = self.remote(request)
        require('job_id' in result, 'invalid_builder_response')
        if args.detach:
            return result
        emit(result)
        return self.wait_job(result['job_id'])


def emit(result):
    print(json.dumps(result, separators=(',', ':')), flush=True)


def parser():
    cli = argparse.ArgumentParser(description='Run image builds on the on-demand VM.')
    sub = cli.add_subparsers(dest='command', required=True)
    run = sub.add_parser('run')
    run.add_argument('target')
    source = run.add_mutually_exclusive_group(required=True)
    source.add_argument('--revision')
    source.add_argument('--branch')
    run.add_argument('--publish', action='store_true')
    run.add_argument('--detach', action='store_true')
    retry = sub.add_parser('publish-retry')
    retry.add_argument('job_id')
    retry.add_argument('--detach', action='store_true')
    status = sub.add_parser('status')
    status.add_argument('job_id', nargs='?')
    logs = sub.add_parser('logs')
    logs.add_argument('job_id')
    logs.add_argument('--follow', action='store_true')
    logs.add_argument('--after', type=int, default=0)
    start = sub.add_parser('start')
    start.add_argument('--keep-running', action='store_true')
    maintenance = sub.add_parser('maintenance')
    maintenance.add_argument('enabled', choices=('on', 'off'))
    sub.add_parser('shutdown')
    return cli


def main():
    # argparse must never echo an unknown argument that could contain a secret.
    with contextlib.redirect_stderr(None):
        try:
            args = parser().parse_args()
        except SystemExit as error:
            if error.code:
                emit({'ok': False, 'error': 'invalid_arguments'})
            return error.code
    try:
        emit(Controller().execute(args))
        return 0
    except ControlError as error:
        emit({'ok': False, 'error': str(error)})
    except KeyboardInterrupt:
        emit({'ok': False, 'error': 'controller_interrupted_job_may_continue'})
    except Exception:
        emit({'ok': False, 'error': 'controller_failed'})
    return 1


if __name__ == '__main__':
    sys.exit(main())
