"""Guest-only syntax and source-binding controls; not hosted CI execution."""
import ast
import ctypes
import errno
import hashlib
import os
import pathlib
import re
import struct
import tempfile
import subprocess
import sys
import unittest
import yaml

ROOT = pathlib.Path(__file__).resolve().parents[1]
WORKFLOW = pathlib.Path(sys.argv.pop()) if len(sys.argv) == 2 else ROOT / '.github/workflows/shell-linux-first-ci.yml'


class LinuxCIContract(unittest.TestCase):
    def setUp(self):
        self.text = WORKFLOW.read_text()
        self.workflow = yaml.load(self.text, Loader=yaml.BaseLoader)

    def test_upstream_pull_request_uses_exact_public_head(self):
        self.assertIn('pull_request', self.workflow['on'])
        self.assertIn('main', self.workflow['on']['pull_request']['branches'])
        self.assertEqual(self.workflow['permissions'], {})
        job = self.workflow['jobs']['user-vm-laboratory']
        self.assertIn("github.repository == 'Gentleman-Programming/gentle-ai'", job['if'])
        source = job['steps'][0]
        self.assertIn('github.event.pull_request.head.sha', source['env']['SOURCE_SHA'])
        self.assertIn('github.event.pull_request.head.repo.full_name', source['env']['SOURCE_REPOSITORY'])
        self.assertIn('credential.helper=', source['run'])
        self.assertNotIn('actions/checkout', self.text)
        self.assertNotIn('pull_request_target', self.text)

    def test_full_journey_not_historical_or_opening_only_smoke(self):
        self.assertEqual(set(self.workflow['jobs']), {'user-vm-laboratory'})
        self.assertIn('/fixture/harness.py full', self.text)
        self.assertNotIn('/fixture/harness.py smoke', self.text)
        self.assertNotIn('322de52', self.text)
        self.assertNotIn('3.7.0', self.text)

    def test_full_fixture_uses_guarded_internal_kernel_probe(self):
        fixture = ast.parse((ROOT / 'e2e/shell-linux-user-install-guest.py').read_text())
        probes = []
        for node in ast.walk(fixture):
            if not isinstance(node, ast.Call) or not isinstance(node.func, ast.Name) or node.func.id != 'run' or not node.args:
                continue
            args = node.args[0]
            if not isinstance(args, ast.List) or len(args.elts) != 3:
                continue
            binary, command, selector = args.elts
            if isinstance(binary, ast.Name) and binary.id == 'SUPERVISOR' and isinstance(command, ast.Constant) and command.value == 'shell' and isinstance(selector, ast.Constant):
                probes.append(selector.value)
        self.assertIn('internal-check', probes)
        self.assertNotIn('check', probes)

    def test_compiled_package_controls_use_their_source_working_directory(self):
        fixture = ast.parse((ROOT / 'e2e/shell-linux-user-install-guest.py').read_text())
        controls = []
        for node in ast.walk(fixture):
            if not isinstance(node, ast.Call) or not isinstance(node.func, ast.Name) or node.func.id != 'run' or not node.args:
                continue
            args = node.args[0]
            if isinstance(args, ast.List) and len(args.elts) > 1 and isinstance(args.elts[0], ast.Name) and args.elts[0].id == 'TESTS' and isinstance(args.elts[1], ast.Constant) and args.elts[1].value == '-test.run=^TestUser':
                controls.append(node)
        self.assertEqual(len(controls), 1)
        cwd = next((keyword.value for keyword in controls[0].keywords if keyword.arg == 'cwd'), None)
        self.assertIsInstance(cwd, ast.BinOp)
        self.assertIsInstance(cwd.op, ast.Div)
        self.assertEqual(cwd.left.id, 'WORK')
        self.assertEqual(cwd.right.value, 'src/internal/shellinstaller')

    def test_pty_failure_tail_is_bounded_encoded_partial_evidence(self):
        fixture = ast.parse((ROOT / 'e2e/shell-linux-user-install-guest.py').read_text())
        function = next(node for node in fixture.body if isinstance(node, ast.FunctionDef) and node.name == 'pty_status')
        evidence = next(node for node in ast.walk(function) if isinstance(node, ast.Dict) and any(isinstance(key, ast.Constant) and key.value == 'tailBase64' for key in node.keys))
        fields = {key.value: value for key, value in zip(evidence.keys, evidence.values) if isinstance(key, ast.Constant)}
        self.assertIn('not complete process stream', fields['observation'].value)
        encoded = fields['tailBase64']
        self.assertEqual(encoded.func.attr, 'decode')
        self.assertEqual(encoded.args[0].value, 'ascii')
        encode = encoded.func.value
        self.assertEqual(encode.func.value.id, 'base64')
        self.assertEqual(encode.func.attr, 'b64encode')
        partial = encode.args[0]
        self.assertEqual(partial.value.id, 'snapshot')
        self.assertIsInstance(partial.slice.lower.op, ast.USub)
        self.assertEqual(partial.slice.lower.operand.value, 1024)
        self.assertIsNone(partial.slice.upper)
        self.assertEqual(fields['tailBytes'].func.id, 'min')
        self.assertEqual(fields['tailBytes'].args[1].value, 1024)

    def test_status_notification_fits_the_pty_without_lowering_success_checks(self):
        text = (ROOT / 'e2e/shell-linux-user-install-guest.py').read_text()
        fixture = ast.parse(text)
        function = next(node for node in fixture.body if isinstance(node, ast.FunctionDef) and node.name == 'pty_status')
        geometry = next(node for node in ast.walk(function) if isinstance(node, ast.Call) and isinstance(node.func, ast.Attribute) and node.func.attr == 'pack' and isinstance(node.args[0], ast.Constant) and node.args[0].value == 'HHHH')
        self.assertEqual([value.value for value in geometry.args[1:]], [80, 72, 0, 0])
        self.assertIn("b'el Gentleman package is active.'", text)
        self.assertIn("require(expected in raw,", text)
        self.assertIn("min(45, remaining())", text)
        self.assertIn("len(raw) <= 65536", text)
        self.assertIn("require(not violations, 'fixture blank caller project changed during launch')", text)

    def test_project_preservation_policy_controls_pass(self):
        # The build step only names this file; run the narrow .git times policy here.
        result = subprocess.run([sys.executable, '-B', str(ROOT / 'e2e/shell-linux-preservation.test.py')],
                                text=True, capture_output=True, timeout=60)
        self.assertEqual(result.returncode, 0, result.stderr[-1000:])

    def test_kernel_notifications_are_read_only_bounded_partial_evidence(self):
        fixture = ast.parse((ROOT / 'e2e/shell-linux-user-install-guest.py').read_text())
        names = {'require', 'project_watch', 'project_events'}
        selected = [node for node in fixture.body if isinstance(node, ast.FunctionDef) and node.name in names]
        self.assertEqual(len(selected), len(names))
        namespace = {'ctypes': ctypes, 'errno': errno, 'os': os, 'struct': struct, 're': re, 'hashlib': hashlib}
        exec(compile(ast.Module(body=selected, type_ignores=[]), '<actual-kernel-observer>', 'exec'), namespace)
        with tempfile.TemporaryDirectory() as temporary:
            project = pathlib.Path(temporary)
            git = project / '.git'
            # Installer PTYs run before the fixture initializes its Git repo.
            self.assertIsNone(namespace['project_watch'](project))
            unavailable = namespace['project_events'](None)
            self.assertTrue(unavailable['partial'])
            self.assertFalse(unavailable['available'])
            self.assertEqual(list(project.iterdir()), [])
            git.mkdir(mode=0o700)
            before = git.stat()
            fd = namespace['project_watch'](project)
            try:
                self.assertFalse(os.get_inheritable(fd))
                self.assertFalse(os.get_blocking(fd))
                empty = namespace['project_events'](fd)
                self.assertEqual(empty['queueBytes'], 0)
                fresh = git.stat()
                self.assertEqual((before.st_mtime_ns, before.st_ctime_ns), (fresh.st_mtime_ns, fresh.st_ctime_ns))
                transient = git / 'actual-transient.lock'
                transient.write_bytes(b'fixture')
                transient.unlink()
                observed = namespace['project_events'](fd)
                self.assertTrue(observed['partial'])
                self.assertLessEqual(observed['queueBytes'], 4096)
                self.assertLessEqual(len(observed['firstEight']), 8)
                self.assertIn([0x100, transient.name], [list(row) for row in observed['firstEight']])
                self.assertIn([0x200, transient.name], [list(row) for row in observed['firstEight']])
            finally:
                os.close(fd)

    def test_shell_blocks_parse_without_execution(self):
        for step in self.workflow['jobs']['user-vm-laboratory']['steps']:
            if 'run' not in step:
                continue
            with self.subTest(step=step['name']):
                result = subprocess.run(['/bin/bash', '-n'], input=step['run'], text=True,
                                        capture_output=True, timeout=5)
                self.assertEqual(result.returncode, 0, result.stderr[:1000])


if __name__ == '__main__':
    unittest.main()
