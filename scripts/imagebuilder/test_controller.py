import contextlib
import io
import json
import subprocess
import unittest
from unittest.mock import Mock, patch

from controller import Controller, ControlError, parser, safe_response


ENV = {'IMAGEBUILDER_PROXMOX_TOKEN': 'test!power=SECRET-MARKER'}
JOB = 'a' * 32
REVISION = 'b' * 40


class ControllerTests(unittest.TestCase):
    def controller(self):
        return Controller(ENV)

    def test_response_discards_unapproved_fields(self):
        self.assertEqual(safe_response({'ok': True, 'phase': 'idle',
                                        'credentials': 'SECRET-MARKER'}),
                         {'ok': True, 'phase': 'idle'})

    def test_response_rejects_secret_in_error_or_events(self):
        for data in ({'ok': False, 'error': 'SECRET-MARKER'},
                     {'ok': True, 'events': [{'phase': 'failed',
                                              'code': 'SECRET-MARKER', 'time': 1}]}):
            with self.assertRaisesRegex(ControlError, '^invalid_builder_response$'):
                safe_response(data)

    def test_stopped_status_does_not_wake(self):
        control = self.controller()
        control.power = Mock(return_value='stopped')
        control.wake = Mock()
        self.assertEqual(control.execute(parser().parse_args(['status'])),
                         {'ok': True, 'power': 'stopped'})
        control.wake.assert_not_called()

    def test_existing_vm_identity_is_checked(self):
        control = self.controller()
        control.vmid = '100'
        control.api = Mock(return_value={'name': 'another-vm', 'status': 'running',
                                          'secret': 'SECRET-MARKER'})
        with self.assertRaisesRegex(ControlError, '^unexpected_vm_identity$'):
            control.power()

    def test_wake_starts_then_checks_readiness(self):
        control = self.controller()
        control.vmid = '100'
        control.power = Mock(side_effect=['stopped', 'running'])
        control.api = Mock()
        control.remote = Mock(side_effect=[ControlError('builder_unreachable'),
                                           {'ok': True, 'phase': 'ready'}])
        with patch('controller.time.sleep'):
            self.assertEqual(control.wake()['phase'], 'ready')
        control.api.assert_called_once_with('qemu/100/status/start', post=True)

    def test_wake_restarts_after_an_in_progress_shutdown(self):
        control = self.controller()
        control.vmid = '100'
        control.power = Mock(side_effect=['running', 'stopped'])
        control.api = Mock()
        control.remote = Mock(side_effect=[ControlError('stopping'),
                                           {'ok': True, 'phase': 'ready'}])
        with patch('controller.time.sleep'):
            self.assertEqual(control.wake()['phase'], 'ready')
        control.api.assert_called_once_with('qemu/100/status/start', post=True)

    def test_kvm_failure_is_not_retried(self):
        control = self.controller()
        control.power = Mock(return_value='running')
        control.remote = Mock(side_effect=ControlError('kvm_unavailable'))
        with self.assertRaisesRegex(ControlError, '^kvm_unavailable$'):
            control.wake()
        self.assertEqual(control.remote.call_count, 1)

    def test_remote_keeps_credentials_out_of_arguments_and_diagnostics(self):
        control = self.controller()
        result = subprocess.CompletedProcess([], 1, b'not-json SECRET-MARKER',
                                               b'SECRET-MARKER')
        request = {'op': 'submit', 'credentials': {'PKR_VAR_user_password': 'SECRET-MARKER'}}
        with patch('controller.subprocess.run', return_value=result) as run:
            with self.assertRaisesRegex(ControlError, '^builder_unreachable$'):
                control.remote(request)
        args = run.call_args
        self.assertNotIn('SECRET-MARKER', repr(args.args[0]))
        self.assertIn(b'SECRET-MARKER', args.kwargs['input'])
        self.assertIn('ForwardAgent=no', args.args[0])
        self.assertIn('SendEnv=-*', args.args[0])
        self.assertIn('StrictHostKeyChecking=yes', args.args[0])
        self.assertNotIn('IMAGEBUILDER_PROXMOX_TOKEN', args.kwargs['env'])

    def test_default_build_never_forwards_s3_credentials(self):
        control = self.controller()
        control.env.update(PKR_VAR_user_password='SECRET-MARKER',
                           SEAWEEDFS_S3_SECRET_KEY='SECRET-MARKER')
        control.wake = Mock()
        control.remote = Mock(return_value={'ok': True, 'job_id': JOB,
                                             'phase': 'preparing'})
        args = parser().parse_args(['run', 'ubuntu26-base', '--revision',
                                    REVISION, '--detach'])
        with contextlib.redirect_stdout(io.StringIO()):
            control.execute(args)
        request = control.remote.call_args.args[0]
        self.assertFalse(request['publish'])
        self.assertEqual(set(request['credentials']), {'PKR_VAR_user_password'})

    def test_invalid_revision_cannot_wake_vm(self):
        control = self.controller()
        control.wake = Mock()
        args = parser().parse_args(['run', 'ubuntu26-base', '--revision', 'main'])
        with self.assertRaisesRegex(ControlError, '^invalid_revision$'):
            control.execute(args)
        control.wake.assert_not_called()

    def test_publish_missing_credentials_cannot_wake_vm(self):
        control = self.controller()
        control.wake = Mock()
        args = parser().parse_args(['run', 'ubuntu26-base', '--revision',
                                    REVISION, '--publish'])
        with self.assertRaisesRegex(ControlError, '^missing_publish_credentials$'):
            control.execute(args)
        control.wake.assert_not_called()

    def test_wait_observes_root_finalization(self):
        control = self.controller()
        control.remote = Mock(side_effect=[
            {'ok': True, 'job_id': JOB, 'phase': 'succeeded', 'finished': False},
            {'ok': True, 'job_id': JOB, 'phase': 'succeeded', 'finished': True}])
        with patch('controller.time.sleep'), contextlib.redirect_stdout(io.StringIO()):
            self.assertTrue(control.wait_job(JOB)['finished'])
        self.assertEqual(control.remote.call_count, 2)

    def test_wait_failure_is_value_free(self):
        control = self.controller()
        control.remote = Mock(return_value={
            'ok': True, 'job_id': JOB, 'phase': 'failed', 'finished': True})
        with contextlib.redirect_stdout(io.StringIO()):
            with self.assertRaisesRegex(ControlError, '^job_failed$'):
                control.wait_job(JOB)

    def test_shutdown_never_uses_force_stop(self):
        control = self.controller()
        control.power = Mock(return_value='running')
        control.remote = Mock(return_value={'ok': True, 'phase': 'stopping'})
        control.wait_stopped = Mock(return_value={'ok': True, 'power': 'stopped'})
        control.execute(parser().parse_args(['shutdown']))
        control.remote.assert_called_once_with({'op': 'shutdown'})


if __name__ == '__main__':
    unittest.main()
