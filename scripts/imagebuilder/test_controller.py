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

    def test_diagnostic_startup_does_not_require_a_healthy_build_environment(self):
        control = self.controller()
        control.power = Mock(return_value='running')
        control.remote = Mock(return_value={'ok': True, 'phase': 'idle'})
        self.assertEqual(control.wake(build_ready=False)['phase'], 'idle')
        control.remote.assert_called_once_with({'op': 'ready', 'build_ready': False})

    def test_logs_and_maintenance_use_diagnostic_startup(self):
        control = self.controller()
        control.wake = Mock()
        control.remote = Mock(return_value={'ok': True})
        for arguments in (['logs', JOB], ['maintenance', 'on'], ['start']):
            control.execute(parser().parse_args(arguments))
            control.wake.assert_called_with(build_ready=False)

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

    def test_branch_is_resolved_once_and_submitted_as_a_pinned_commit(self):
        control = self.controller()
        control.wake = Mock()
        control.remote = Mock(side_effect=[
            {'ok': True, 'branch': 'fix/packer', 'revision': REVISION},
            {'ok': True, 'job_id': JOB, 'phase': 'preparing'}])
        args = parser().parse_args(['run', 'ubuntu26-base', '--branch',
                                    'fix/packer', '--detach'])
        with contextlib.redirect_stdout(io.StringIO()) as output:
            control.execute(args)
        self.assertEqual(control.remote.call_args_list[0].args[0],
                         {'op': 'resolve', 'branch': 'fix/packer'})
        request = control.remote.call_args_list[1].args[0]
        self.assertEqual(request['revision'], REVISION)
        self.assertEqual(request['branch'], 'fix/packer')
        self.assertIn(REVISION, output.getvalue())
        self.assertFalse(request['publish'])

    def test_invalid_branch_cannot_wake_vm(self):
        control = self.controller()
        control.wake = Mock()
        for branch in ('../main', 'main*', '-main', 'fix//packer', 'main\nSECRET-MARKER'):
            args = parser().parse_args(['run', 'ubuntu26-base', '--branch=' + branch])
            with self.assertRaisesRegex(ControlError, '^invalid_branch$'):
                control.execute(args)
        control.wake.assert_not_called()

    def test_source_selection_is_required_and_exclusive(self):
        with contextlib.redirect_stderr(io.StringIO()):
            for arguments in (['run', 'ubuntu26-base'],
                              ['run', 'ubuntu26-base', '--branch', 'main',
                               '--revision', REVISION]):
                with self.assertRaises(SystemExit):
                    parser().parse_args(arguments)

    def test_branch_resolution_failure_never_submits_a_job(self):
        control = self.controller()
        control.wake = Mock()
        control.remote = Mock(side_effect=ControlError('command_failed'))
        args = parser().parse_args(['run', 'ubuntu26-base', '--branch', 'missing'])
        with self.assertRaisesRegex(ControlError, '^command_failed$'):
            control.execute(args)
        self.assertEqual(control.remote.call_count, 1)

    def test_branch_resolution_response_must_match_requested_branch(self):
        control = self.controller()
        control.wake = Mock()
        control.remote = Mock(return_value={
            'ok': True, 'branch': 'another', 'revision': REVISION})
        args = parser().parse_args(['run', 'ubuntu26-base', '--branch', 'main'])
        with self.assertRaisesRegex(ControlError, '^invalid_builder_response$'):
            control.execute(args)
        self.assertEqual(control.remote.call_count, 1)

    def test_log_response_reconstructs_only_approved_fields(self):
        response = safe_response({'ok': True, 'cursor': 1, 'has_more': False,
                                  'truncated': False, 'records': [
                                      {'sequence': 1, 'time': 1, 'step': 'packer-init',
                                       'event': 'started', 'raw': 'SECRET-MARKER'}]})
        self.assertNotIn('SECRET-MARKER', json.dumps(response))
        self.assertEqual(response['records'][0]['step'], 'packer-init')

    def test_follow_drains_pages_before_stopping_at_finalization(self):
        control = self.controller()
        base = {'ok': True, 'job_id': JOB, 'finished': True,
                'phase': 'failed', 'truncated': False}
        control.remote = Mock(side_effect=[
            dict(base, cursor=1, has_more=True, records=[
                {'sequence': 1, 'time': 1, 'step': 'packer-init', 'event': 'started'}]),
            dict(base, cursor=2, has_more=False, records=[
                {'sequence': 2, 'time': 2, 'step': 'packer-init',
                 'event': 'failed', 'exit_code': 1, 'duration_ms': 10}])])
        with contextlib.redirect_stdout(io.StringIO()) as output:
            result = control.follow_logs(JOB)
        self.assertTrue(result['finished'])
        self.assertEqual(result['cursor'], 2)
        self.assertEqual(control.remote.call_args_list[1].args[0]['after'], 1)
        self.assertEqual(len(output.getvalue().splitlines()), 2)

    def test_follow_rejects_repeated_records(self):
        control = self.controller()
        control.remote = Mock(return_value={
            'ok': True, 'job_id': JOB, 'finished': False,
            'cursor': 1, 'has_more': True, 'truncated': False,
            'records': [{'sequence': 1, 'time': 1, 'step': 'checkout',
                         'event': 'started'}]})
        with contextlib.redirect_stdout(io.StringIO()):
            with self.assertRaisesRegex(ControlError, '^invalid_builder_response$'):
                control.follow_logs(JOB, after=1)

    def test_invalid_log_request_cannot_wake_vm(self):
        control = self.controller()
        control.wake = Mock()
        for arguments, code in ((['logs', 'invalid'], 'invalid_job_id'),
                                (['logs', JOB, '--after', '-1'], 'invalid_log_cursor'),
                                (['logs', JOB, '--after', str(2 ** 63)], 'invalid_log_cursor')):
            with self.assertRaisesRegex(ControlError, '^' + code + '$'):
                control.execute(parser().parse_args(arguments))
        control.wake.assert_not_called()

    def test_follow_accepts_a_retention_gap_and_reports_it(self):
        control = self.controller()
        control.remote = Mock(return_value={
            'ok': True, 'job_id': JOB, 'finished': True, 'phase': 'succeeded',
            'cursor': 100, 'has_more': False, 'truncated': True,
            'records': [{'sequence': 100, 'time': 1, 'step': 'packer-build',
                         'event': 'completed'}]})
        with contextlib.redirect_stdout(io.StringIO()) as output:
            result = control.follow_logs(JOB)
        records = [json.loads(line) for line in output.getvalue().splitlines()]
        self.assertTrue(records[0]['truncated'])
        self.assertEqual(records[1]['sequence'], 100)
        self.assertEqual(result['cursor'], 100)

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
            {'ok': True, 'job_id': JOB, 'phase': 'succeeded', 'finished': False,
             'records': [], 'cursor': 0, 'has_more': False, 'truncated': False},
            {'ok': True, 'job_id': JOB, 'phase': 'succeeded', 'finished': True,
             'records': [], 'cursor': 0, 'has_more': False, 'truncated': False}])
        with patch('controller.time.sleep'), contextlib.redirect_stdout(io.StringIO()):
            self.assertTrue(control.wait_job(JOB)['finished'])
        self.assertEqual(control.remote.call_count, 2)

    def test_wait_failure_is_value_free(self):
        control = self.controller()
        control.remote = Mock(return_value={
            'ok': True, 'job_id': JOB, 'phase': 'failed', 'finished': True,
            'records': [], 'cursor': 0, 'has_more': False, 'truncated': False})
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
