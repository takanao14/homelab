#!/usr/bin/env python3
import contextlib
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import pwd
import sys
import tempfile
import threading
import unittest
from unittest.mock import patch


SPEC = importlib.util.spec_from_file_location('imagebuilder_runtime',
                                             Path(__file__).with_name('runtime.py'))
runtime = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(runtime)


class RuntimeTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        root = Path(self.temporary.name)
        config = {'work_dir': str(root / 'work'), 'runtime_dir': str(root / 'run'),
                  'repo_url': 'https://github.com/takanao14/homelab.git',
                  'user': pwd.getpwuid(os.getuid()).pw_name, 'require_mount': False,
                  'minimum_free_bytes': 1, 'idle_seconds': 600,
                  'timeout_seconds': 60, 'retention_days': 7,
                  'smoke_timeout_seconds': 30}
        self.chown = patch.object(runtime.os, 'chown')
        self.chown.start()
        self.addCleanup(self.chown.stop)
        self.backend = runtime.Runtime(config)
        self.backend.initialize()
        self.ready = patch.object(self.backend, 'readiness',
                                  return_value={'ok': True, 'phase': 'ready'})
        self.ready.start()
        self.addCleanup(self.ready.stop)
        self.commands = patch.object(runtime, 'command', return_value=b'')
        self.command = self.commands.start()
        self.addCleanup(self.commands.stop)

    def submit(self, **fields):
        request = {'op': 'submit', 'revision': 'a' * 40,
                   'target': 'ubuntu26-base',
                   'credentials': {'PKR_VAR_user_password': 'SECRET_MARKER'}}
        request.update(fields)
        return self.backend.control(request)['job_id']

    def complete(self, identifier, phase='succeeded', code='verified'):
        self.backend.update(identifier, phase, code)
        self.backend.finish(identifier)

    def assert_code(self, expected, operation):
        with self.assertRaises(runtime.SafeError) as error:
            operation()
        self.assertEqual(str(error.exception), expected)

    def test_secrets_are_tmpfs_only_and_removed_after_success(self):
        identifier = self.submit()
        self.assertIn('SECRET_MARKER',
                      self.backend.request_path(identifier).read_text())
        self.assertNotIn('SECRET_MARKER',
                         self.backend.record_path(identifier).read_text())
        self.complete(identifier)
        self.assertFalse(self.backend.request_path(identifier).parent.exists())
        self.assertFalse(self.backend.worker_path(identifier).exists())
        self.assertEqual(self.backend.control({'op': 'status'})['phase'], 'succeeded')

    def test_concurrent_requests_have_one_winner(self):
        barrier = threading.Barrier(2)
        results = []

        def submit():
            barrier.wait()
            try:
                results.append(self.submit())
            except runtime.SafeError as error:
                results.append(str(error))

        workers = [threading.Thread(target=submit) for _ in range(2)]
        for worker in workers:
            worker.start()
        for worker in workers:
            worker.join(timeout=5)
            self.assertFalse(worker.is_alive())
        self.assertEqual(len(results), 2)
        self.assertEqual(results.count('busy'), 1)
        self.assertEqual(self.command.call_count, 1)

    def test_shutdown_refuses_active_job_and_new_job_refuses_stopping(self):
        identifier = self.submit()
        self.assert_code('busy', lambda: self.backend.control({'op': 'shutdown'}))
        self.complete(identifier)
        self.backend.control({'op': 'shutdown'})
        self.assert_code('stopping', self.submit)
        self.assertIn(['systemctl', 'poweroff', '--no-block'],
                      [call.args[0] for call in self.command.call_args_list])

    def test_start_failure_does_not_leave_busy_or_credentials(self):
        self.command.side_effect = runtime.SafeError('command_failed')
        self.assert_code('start_failed', self.submit)
        state = runtime.read_json(self.backend.control_path)
        self.assertIsNone(state['active'])
        self.assertFalse(list((self.backend.run_dir / 'jobs').iterdir()))
        self.assertEqual(self.backend.control({'op': 'status'})['code'], 'start_failed')

    def test_terminal_disconnect_does_not_own_systemd_job(self):
        identifier = self.submit()
        self.command.assert_called_once_with(
            ['systemctl', 'start', '--no-block',
             f'imagebuilder-job@{identifier}.service'])
        state = runtime.read_json(self.backend.control_path)
        self.assertEqual(state['active'], identifier)

    def test_idle_timer_stops_only_idle_nonmaintenance_vm(self):
        with self.backend.locked() as state:
            state['idle_since'] = 0
            self.backend.save_control(state)
        self.backend.control({'op': 'maintenance', 'enabled': True})
        self.backend.idle()
        self.command.assert_not_called()
        self.backend.control({'op': 'maintenance', 'enabled': False})
        with self.backend.locked() as state:
            state['idle_since'] = 0
            self.backend.save_control(state)
        self.backend.idle()
        self.command.assert_called_once_with(['systemctl', 'poweroff', '--no-block'])

    def test_successful_readiness_reserves_idle_grace_for_submission(self):
        with self.backend.locked() as state:
            state['idle_since'] = 0
            self.backend.save_control(state)
        self.backend.control({'op': 'ready'})
        self.backend.idle()
        self.command.assert_not_called()
        self.assertGreater(runtime.read_json(self.backend.control_path)['idle_since'], 0)

    def test_explicit_maintenance_survives_reboot_until_disabled(self):
        self.backend.control({'op': 'maintenance', 'enabled': True})
        self.backend.control_path.unlink()
        self.backend.boot()
        self.assertTrue(self.backend.control({'op': 'status'})['maintenance'])
        self.assert_code('maintenance', self.submit)
        with self.backend.locked() as state:
            state['idle_since'] = 0
            self.backend.save_control(state)
        self.backend.idle()
        self.command.assert_not_called()
        self.backend.control({'op': 'maintenance', 'enabled': False})
        self.backend.control_path.unlink()
        self.backend.boot()
        self.assertFalse(self.backend.control({'op': 'status'})['maintenance'])

    def test_boot_marks_interrupted_and_discards_runtime_secrets(self):
        identifier = self.submit()
        self.backend.update(identifier, 'building', 'building')
        self.backend.control_path.unlink()
        self.backend.boot()
        status = self.backend.control({'op': 'status', 'job_id': identifier})
        self.assertEqual(status['phase'], 'interrupted')
        self.assertFalse(self.backend.request_path(identifier).parent.exists())
        self.assertIsNone(runtime.read_json(self.backend.control_path)['active'])

    def test_interrupted_publication_blocks_other_jobs_across_boot(self):
        identifier = self.submit()
        self.backend.update(identifier, 'publishing', 'publishing')
        self.backend.control_path.unlink()
        self.backend.boot()
        self.assertEqual(runtime.read_json(self.backend.publication_guard)['job_id'],
                         identifier)
        self.assert_code('publication_blocked', self.submit)

    def test_boot_recovery_refuses_to_disrupt_running_job(self):
        identifier = self.submit()
        self.command.return_value = b'active\n'
        self.assert_code('busy', self.backend.boot)
        self.assertEqual(runtime.read_json(self.backend.control_path)['active'],
                         identifier)
        self.assertTrue(self.backend.request_path(identifier).exists())

    def test_timeout_finalizer_cleans_secrets_and_retains_failure(self):
        identifier = self.submit()
        self.backend.update(identifier, 'building', 'building')
        self.backend.finish(identifier)
        status = self.backend.control({'op': 'status', 'job_id': identifier})
        self.assertEqual(status['code'], 'service_failed')
        self.assertFalse(self.backend.worker_path(identifier).exists())

    def test_invalid_progress_cannot_leak_values(self):
        identifier = self.submit()
        runtime.write_json(self.backend.progress_path(identifier),
                           {'phase': 'failed', 'code': 'SECRET_MARKER'})
        self.backend.finish(identifier)
        response = self.backend.control({'op': 'status', 'job_id': identifier})
        self.assertEqual(response['code'], 'invalid_progress')
        self.assertNotIn('SECRET_MARKER', json.dumps(response))

    def test_publishing_timeout_blocks_future_submissions(self):
        identifier = self.submit()
        self.backend.update(identifier, 'publishing', 'publishing')
        self.backend.finish(identifier)
        self.assertEqual(self.backend.control({'op': 'status'})['code'],
                         'publication_blocked')
        self.assert_code('publication_blocked', self.submit)

    def test_missing_mount_is_rejected_before_creating_state(self):
        self.backend.config['require_mount'] = True
        self.assert_code('work_disk_not_mounted', self.backend.initialize)

    def test_kernel_configuration_rejects_paths_and_version_mismatch(self):
        for fields in ({'kernel_version': 'SECRET_MARKER\n'},
                       {'kernel_path': '/var/lib/../SECRET_MARKER'},
                       {'kernel_modules': '/lib/modules/..'},
                       {'kernel_version': '6.1-test',
                        'kernel_modules': '/lib/modules/6.2-test'}):
            self.assert_code('invalid_config', lambda: runtime.Runtime(
                dict(self.backend.config, **fields)))

    def test_readiness_checks_kernel_readability_as_worker(self):
        self.backend.config.update(kernel_path='/var/lib/imagebuilder-kernel/vmlinuz',
                                   kernel_version='6.1-test',
                                   kernel_modules='/lib/modules/6.1-test')
        with (patch.object(runtime.os, 'access', return_value=True),
              patch.object(runtime.shutil, 'which', return_value='/usr/bin/tool')):
            runtime.Runtime.readiness(self.backend)
        calls = [call.args[0] for call in self.command.call_args_list]
        self.assertIn(['runuser', '-u', self.backend.user.pw_name, '--',
                       'test', '-r', '/var/lib/imagebuilder-kernel/vmlinuz'], calls)
        self.assertIn(['runuser', '-u', self.backend.user.pw_name, '--',
                       'test', '-d', '/lib/modules/6.1-test'], calls)

    def test_logs_emit_only_approved_event_fields(self):
        identifier = self.submit()
        self.backend.update(identifier, 'building', 'building', secret='SECRET_MARKER')
        response = self.backend.control({'op': 'logs', 'job_id': identifier})
        self.assertNotIn('SECRET_MARKER', json.dumps(response))
        self.assertEqual(set(response['events'][0]), {'phase', 'code', 'time'})

    def test_invalid_control_json_is_value_free(self):
        config_path = Path(self.temporary.name) / 'config.json'
        runtime.write_json(config_path, self.backend.config)
        output = io.StringIO()
        source = io.TextIOWrapper(io.BytesIO(b'{"SECRET_MARKER":'))
        with (patch.object(runtime, 'CONFIG', config_path),
              patch.object(sys, 'argv', ['imagebuilder', 'control']),
              patch.object(sys, 'stdin', source),
              contextlib.redirect_stdout(output),
              contextlib.redirect_stderr(output)):
            self.assertEqual(runtime.main(), 1)
        self.assertEqual(json.loads(output.getvalue()),
                         {'ok': False, 'error': 'internal_error'})
        self.assertNotIn('SECRET_MARKER', output.getvalue())

    def test_unknown_credential_names_and_shell_tokens_are_rejected(self):
        self.assert_code('invalid_request', lambda: self.submit(
            credentials={'PATH': 'SECRET_MARKER'}))
        self.assert_code('invalid_request', lambda: self.submit(target='x;id'))
        self.assert_code('invalid_request', lambda: self.submit(revision='main'))
        self.command.assert_not_called()

    def test_target_list_is_derived_from_checkout(self):
        checkout = self.backend.work / 'checkout'
        (checkout / 'packer' / 'vars').mkdir(parents=True)
        (checkout / 'packer' / 'vars' / 'example-base.pkrvars.hcl').write_text('')
        (checkout / 'packer' / 'import-upstream.sh').write_text(
            'import_target() {\n case "$1" in\n freebsd151)\n ;;\n esac\n}\n')
        self.assertEqual(self.backend.target_command(checkout, 'example-base'),
                         ['bash', './build.sh', '-y', 'example-base'])
        self.assertEqual(self.backend.target_command(checkout, 'freebsd151'),
                         ['bash', './import-upstream.sh', '-y', 'freebsd151'])
        self.assert_code('unknown_target',
                         lambda: self.backend.target_command(checkout, 'unknown'))

    def test_retry_reuses_job_and_validated_artifacts(self):
        identifier = self.submit()
        runtime.write_json(self.backend.jobs / identifier / 'verified.json',
                           [{'name': 'ubuntu.img', 'sha256': 'b' * 64}])
        self.complete(identifier, 'failed', 'publication_failed_restored')
        request = {'op': 'publish-retry', 'job_id': identifier,
                   'credentials': {'SEAWEEDFS_S3_ENDPOINT': 'https://s3.example.test',
                                   'SEAWEEDFS_S3_ACCESS_KEY': 'SECRET_MARKER',
                                   'SEAWEEDFS_S3_SECRET_KEY': 'SECRET_MARKER'}}
        response = self.backend.control(request)
        self.assertEqual(response['job_id'], identifier)
        self.assertEqual(runtime.read_json(self.backend.control_path)['active'], identifier)
        self.assertEqual(runtime.read_json(self.backend.request_path(identifier))['op'],
                         'publish-retry')
        self.assertEqual(self.backend.control({'op': 'status'})['phase'], 'preparing')

    def test_retry_start_failure_releases_lock_and_does_not_deadlock(self):
        identifier = self.submit()
        runtime.write_json(self.backend.jobs / identifier / 'verified.json', [])
        self.complete(identifier)
        self.command.side_effect = runtime.SafeError('command_failed')
        self.assert_code('start_failed', lambda: self.backend.control(
            {'op': 'publish-retry', 'job_id': identifier,
             'credentials': {'SEAWEEDFS_S3_ENDPOINT': 'https://s3.example.test',
                             'SEAWEEDFS_S3_ACCESS_KEY': 'SECRET_MARKER',
                             'SEAWEEDFS_S3_SECRET_KEY': 'SECRET_MARKER'}}))
        self.assertIsNone(runtime.read_json(self.backend.control_path)['active'])

    def test_failed_build_never_publishes_or_emits_child_output(self):
        identifier = self.submit()
        checkout = self.backend.jobs / identifier / 'repo'
        checkout.mkdir()

        def agent(_identifier, env):
            env['SSH_AGENT_PID'] = '123456789'

        with (patch.object(self.backend, 'checkout', return_value=checkout),
              patch.object(self.backend, 'target_command',
                           return_value=['bash', './build.sh', '-y', 'ubuntu26-base']),
              patch.object(self.backend, 'ssh_environment', side_effect=agent),
              patch.object(self.backend, 'publish') as publish,
              patch.object(runtime.os, 'kill')):
            self.command.side_effect = runtime.SafeError('command_failed')
            self.assertEqual(self.backend.run(identifier), 1)
            publish.assert_not_called()
        self.assertEqual(self.backend.progress(identifier)['code'], 'command_failed')
        self.assertNotIn('SECRET_MARKER',
                         self.backend.progress_path(identifier).read_text())

    def test_full_dry_run_never_reads_root_control_state_as_worker(self):
        identifier = self.submit()
        checkout = self.backend.jobs / identifier / 'repo'
        (checkout / 'packer' / 'vars').mkdir(parents=True)
        (checkout / 'packer' / 'vars' / 'ubuntu26-base.pkrvars.hcl').write_text('')
        self.backend.config.update(kernel_path='/var/lib/imagebuilder-kernel/vmlinuz',
                                   kernel_version='6.1-test',
                                   kernel_modules='/lib/modules/6.1-test')

        def execute(args, **_kwargs):
            if args[:2] == ['bash', './build.sh']:
                self.assertEqual(_kwargs['env']['SUPERMIN_KERNEL_VERSION'], '6.1-test')
                self.assertEqual(_kwargs['env']['SUPERMIN_MODULES'], '/lib/modules/6.1-test')
                images = checkout / 'packer' / 'images'
                images.mkdir()
                image = images / 'ubuntu-26.04-base.img'
                image.write_bytes(b'image-bytes')
                Path(str(image) + '.sha256').write_text(
                    hashlib.sha256(b'image-bytes').hexdigest() + '\n')
            return b''

        original_read = runtime.read_json

        def worker_read(path):
            self.assertNotIn(self.backend.control_dir, path.parents)
            return original_read(path)

        def agent(_identifier, env):
            env.update(SSH_AGENT_PID='123456789', PKR_VAR_ssh_pubkey='ssh-ed25519 AAAA')

        self.command.side_effect = execute
        with (patch.object(self.backend, 'checkout', return_value=checkout),
              patch.object(self.backend, 'ssh_environment', side_effect=agent),
              patch.object(self.backend, 'smoke') as smoke,
              patch.object(runtime, 'read_json', side_effect=worker_read),
              patch.object(runtime.os, 'kill')):
            self.assertEqual(self.backend.run(identifier), 0)
            smoke.assert_called_once()
        status = self.backend.control({'op': 'status', 'job_id': identifier})
        self.assertEqual(status['code'], 'verified')
        self.assertFalse(status['finished'])
        self.backend.finish(identifier)
        status = self.backend.control({'op': 'status', 'job_id': identifier})
        self.assertTrue(status['finished'])
        self.assertEqual(status['sha256'], hashlib.sha256(b'image-bytes').hexdigest())

    def test_run_publication_and_retry_reuse_verified_image(self):
        secret_values = {'PKR_VAR_user_password': 'SECRET_MARKER',
                         'SEAWEEDFS_S3_ENDPOINT': 'https://s3.example.test',
                         'SEAWEEDFS_S3_ACCESS_KEY': 'SECRET_MARKER',
                         'SEAWEEDFS_S3_SECRET_KEY': 'SECRET_MARKER'}
        identifier = self.submit(publish=True, credentials=secret_values)
        checkout = self.backend.jobs / identifier / 'repo'
        images = checkout / 'packer' / 'images'
        images.mkdir(parents=True)
        (checkout / 'packer' / 'vars').mkdir()
        (checkout / 'packer' / 'vars' / 'ubuntu26-base.pkrvars.hcl').write_text('')
        payload = b'image-bytes'
        name = 'ubuntu-26.04-base.img'
        digest = hashlib.sha256(payload).hexdigest()
        remote = {}
        builds = []

        def execute(args, **kwargs):
            if args[:2] == ['bash', './build.sh']:
                self.assertIn('PKR_VAR_user_password', kwargs['env'])
                self.assertNotIn('SEAWEEDFS_S3_SECRET_KEY', kwargs['env'])
                builds.append(True)
                (images / name).write_bytes(payload)
                (images / (name + '.sha256')).write_text(digest + '\n')
            elif args[:2] == ['bash', './push.sh']:
                self.assertNotIn('PKR_VAR_user_password', kwargs['env'])
                self.assertIn('SEAWEEDFS_S3_SECRET_KEY', kwargs['env'])
                remote['seaweedfs:cloud-images/' + name] = payload
                remote['seaweedfs:cloud-images/' + name + '.sha256'] = (digest + '\n').encode()
            elif args[:2] == ['rclone', 'lsf']:
                return b''
            elif args[:2] == ['rclone', 'cat']:
                contents = remote[args[2]]
                return hashlib.sha256(contents).hexdigest() if kwargs.get('digest') else contents
            return b''

        def agent(_identifier, env):
            env.update(SSH_AGENT_PID='123456789', PKR_VAR_ssh_pubkey='ssh-ed25519 AAAA')

        self.command.side_effect = execute
        with (patch.object(self.backend, 'checkout', return_value=checkout) as clone,
              patch.object(self.backend, 'ssh_environment', side_effect=agent),
              patch.object(self.backend, 'smoke') as smoke,
              patch.object(runtime.os, 'kill')):
            self.assertEqual(self.backend.run(identifier), 0)
            self.backend.finish(identifier)
            self.assertEqual(self.backend.control({'op': 'status'})['code'], 'published')
            self.backend.control({'op': 'publish-retry', 'job_id': identifier,
                                  'credentials': secret_values})
            self.assertEqual(self.backend.run(identifier), 0)
            self.backend.finish(identifier)
            clone.assert_called_once()
            smoke.assert_called_once()
            self.assertFalse(set(clone.call_args.args[2]) & runtime.CREDENTIALS)
            self.assertFalse(set(smoke.call_args.args[2]) & runtime.CREDENTIALS)
        self.assertEqual(len(builds), 1)
        self.assertTrue(self.backend.control({'op': 'status'})['finished'])
        self.assertFalse(self.backend.publication_guard.exists())

    def test_invalid_worker_digest_is_not_copied_into_root_record(self):
        identifier = self.submit()
        runtime.write_json(self.backend.progress_path(identifier),
                           {'phase': 'succeeded', 'code': 'verified',
                            'sha256': 'SECRET_MARKER'})
        self.backend.finish(identifier)
        self.assertNotIn('SECRET_MARKER',
                         self.backend.record_path(identifier).read_text())
        self.assertEqual(self.backend.control({'op': 'status'})['code'], 'invalid_progress')

    def test_freebsd_smoke_uses_native_nuageinit_without_sudo(self):
        configuration, check = self.backend.smoke_configuration(
            'a' * 32, Path('freebsd-15.1-cloudinit-ufs.img'), 'ssh-ed25519 AAAA')
        self.assertNotIn('sudo', configuration['users'][0])
        self.assertNotIn('cloud-init', check)
        self.assertNotIn('sudo', check)
        self.assertEqual(configuration['runcmd'],
                         ['touch /var/tmp/imagebuilder-smoke-' + 'a' * 32])
        self.assertEqual(check, 'test -f /var/tmp/imagebuilder-smoke-' + 'a' * 32)

    def test_linux_smoke_checks_guest_agent_and_cloud_init(self):
        configuration, check = self.backend.smoke_configuration(
            'a' * 32, Path('ubuntu-26.04-base.img'), 'ssh-ed25519 AAAA')
        self.assertEqual(configuration['users'][0]['sudo'], 'ALL=(ALL) NOPASSWD:ALL')
        self.assertIn('sudo -n cloud-init status --wait', check)
        self.assertIn('systemctl is-active --quiet qemu-guest-agent', check)

    def test_cache_eviction_runs_only_as_worker_and_keeps_small_recent_cache(self):
        cache = self.backend.cache / 'packer'
        cache.mkdir()
        (cache / 'image').write_bytes(b'cache')
        self.backend.config['cache_max_bytes'] = 10
        self.backend.prune_cache()
        self.assertTrue(cache.exists())
        self.backend.config['cache_max_bytes'] = 1
        self.backend.prune_cache()
        self.assertFalse(cache.exists())

    def test_idle_cache_pruning_drops_privileges_and_never_runs_mid_job(self):
        cache = self.backend.cache / 'packer'
        cache.mkdir()
        identifier = self.submit()
        self.backend.idle()
        self.assertFalse(any(call.args[0][0] == 'runuser'
                             for call in self.command.call_args_list))
        self.complete(identifier)
        self.backend.idle()
        self.assertIn(['runuser', '-u', self.backend.user.pw_name, '--',
                       '/usr/local/libexec/imagebuilder', 'cache-prune'],
                      [call.args[0] for call in self.command.call_args_list])

    def test_publication_failure_restores_old_pair(self):
        identifier = self.submit()
        checkout = self.backend.jobs / identifier / 'repo'
        checkout.mkdir()
        env = {'SEAWEEDFS_S3_ENDPOINT': 'https://s3.example.test',
               'SEAWEEDFS_S3_ACCESS_KEY': 'SECRET_MARKER',
               'SEAWEEDFS_S3_SECRET_KEY': 'SECRET_MARKER'}
        old = 'a' * 64

        def execute(args, **_kwargs):
            if args[:2] == ['rclone', 'lsf']:
                return b'ubuntu.img\nubuntu.img.sha256\n'
            if args[:2] == ['rclone', 'cat']:
                return (old + '\n').encode()
            if args[:2] == ['bash', './push.sh']:
                raise runtime.SafeError('command_failed')
            return b''

        self.command.side_effect = execute
        with patch.object(self.backend, 'remote_digest', return_value=old):
            self.assert_code('publication_failed_restored', lambda: self.backend.publish(
                identifier, checkout, [{'name': 'ubuntu.img', 'sha256': 'b' * 64}], env))
        copies = [call.args[0] for call in self.command.call_args_list
                  if call.args[0][:2] == ['rclone', 'copyto']]
        self.assertEqual(len(copies), 4)
        self.assertIn('.imagebuilder-backups/', copies[0][3])
        self.assertIn('.imagebuilder-backups/', copies[2][2])

    def test_first_publication_failure_accepts_already_absent_rollback_objects(self):
        identifier = self.submit()
        checkout = self.backend.jobs / identifier / 'repo'
        checkout.mkdir()
        env = {'SEAWEEDFS_S3_ENDPOINT': 'https://s3.example.test',
               'SEAWEEDFS_S3_ACCESS_KEY': 'SECRET_MARKER',
               'SEAWEEDFS_S3_SECRET_KEY': 'SECRET_MARKER'}

        def execute(args, **_kwargs):
            if args[:2] == ['rclone', 'lsf']:
                return b''
            if args[:2] in (['bash', './push.sh'], ['rclone', 'deletefile']):
                raise runtime.SafeError('command_failed')
            return b''

        self.command.side_effect = execute
        self.assert_code('publication_failed_restored', lambda: self.backend.publish(
            identifier, checkout, [{'name': 'ubuntu.img', 'sha256': 'b' * 64}], env))
        self.assertEqual(sum(call.args[0][:2] == ['rclone', 'lsf']
                             for call in self.command.call_args_list), 2)

    def test_failed_first_publication_cannot_claim_rollback_while_object_remains(self):
        identifier = self.submit()
        checkout = self.backend.jobs / identifier / 'repo'
        checkout.mkdir()
        env = {'SEAWEEDFS_S3_ENDPOINT': 'https://s3.example.test',
               'SEAWEEDFS_S3_ACCESS_KEY': 'SECRET_MARKER',
               'SEAWEEDFS_S3_SECRET_KEY': 'SECRET_MARKER'}
        written = False

        def execute(args, **_kwargs):
            nonlocal written
            if args[:2] == ['rclone', 'lsf']:
                return b'ubuntu.img\n' if written else b''
            if args[:2] == ['bash', './push.sh']:
                written = True
                raise runtime.SafeError('command_failed')
            if args[:2] == ['rclone', 'deletefile']:
                raise runtime.SafeError('command_failed')
            return b''

        self.command.side_effect = execute
        self.assert_code('publication_blocked', lambda: self.backend.publish(
            identifier, checkout, [{'name': 'ubuntu.img', 'sha256': 'b' * 64}], env))

    def test_changed_publication_snapshot_cannot_target_unrelated_objects(self):
        identifier = self.submit()
        checkout = self.backend.jobs / identifier / 'repo'
        checkout.mkdir()
        runtime.write_json(self.backend.jobs / identifier / 'publication.json',
                           {'artifacts': [{'name': '../unrelated.img',
                                           'present': False, 'sha256': None}]})
        env = {'SEAWEEDFS_S3_ENDPOINT': 'https://s3.example.test',
               'SEAWEEDFS_S3_ACCESS_KEY': 'SECRET_MARKER',
               'SEAWEEDFS_S3_SECRET_KEY': 'SECRET_MARKER'}
        self.command.reset_mock()
        self.assert_code('invalid_state', lambda: self.backend.publish(
            identifier, checkout, [{'name': 'ubuntu.img', 'sha256': 'b' * 64}], env))
        self.command.assert_not_called()

    def test_failed_rollback_persists_block_until_successful_retry(self):
        identifier = self.submit()
        self.complete(identifier, 'failed', 'publication_blocked')
        self.assertTrue(self.backend.publication_guard.exists())
        self.assert_code('publication_blocked', self.submit)
        self.backend.update(identifier, 'succeeded', 'published')
        record = runtime.read_json(self.backend.record_path(identifier))
        record['finished'] = False
        runtime.write_json(self.backend.record_path(identifier), record)
        self.backend.finish(identifier)
        self.assertFalse(self.backend.publication_guard.exists())

    def test_retention_preserves_active_and_blocked_jobs(self):
        identifier = self.submit()
        record = runtime.read_json(self.backend.record_path(identifier))
        record['created'] = 0
        runtime.write_json(self.backend.record_path(identifier), record)
        self.complete(identifier, 'failed', 'publication_blocked')
        self.backend.idle()
        self.assertTrue(self.backend.record_path(identifier).exists())


class ProcessTests(unittest.TestCase):
    def test_child_output_is_discarded_and_failure_contains_no_secret(self):
        output = io.StringIO()
        with contextlib.redirect_stdout(output), contextlib.redirect_stderr(output):
            result = runtime.command([sys.executable, '-c',
                                      'print("SECRET_MARKER")'])
            with self.assertRaises(runtime.SafeError) as error:
                runtime.command([sys.executable, '-c',
                                 'import sys; print("SECRET_MARKER"); '
                                 'print("SECRET_MARKER", file=sys.stderr); sys.exit(1)'])
        self.assertEqual(result, b'')
        self.assertEqual(str(error.exception), 'command_failed')
        self.assertNotIn('SECRET_MARKER', output.getvalue())

    def test_digest_stream_does_not_capture_payload(self):
        result = runtime.command([sys.executable, '-c',
                                  'import sys; sys.stdout.write("x" * 3000000)'],
                                 digest=True)
        self.assertEqual(result, hashlib.sha256(b'x' * 3000000).hexdigest())

    def test_command_timeout_is_value_free(self):
        with self.assertRaises(runtime.SafeError) as error:
            runtime.command([sys.executable, '-c',
                             'import time; print("SECRET_MARKER"); time.sleep(5)'],
                            timeout=0.1)
        self.assertEqual(str(error.exception), 'command_timeout')


if __name__ == '__main__':
    unittest.main()
