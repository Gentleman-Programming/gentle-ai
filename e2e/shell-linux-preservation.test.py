"""Portable caller-project preservation policy controls; not native Guest qualification.

Only the root .git directory may differ, and only in mtime_ns/ctime_ns: the
native review integration creates and deletes a private review index beside
Git's control files. Every other path, byte and metadata field stays compared,
and review-index or lock residue always refuses.
"""
import ast
import hashlib
import json
import os
import pathlib
import stat
import tempfile
import time
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]
GUEST = ROOT / 'e2e/shell-linux-user-install-guest.py'
SCRATCH = ROOT / '.checks'
FUNCTIONS = {'require', 'physical_inventory', 'project_preservation_violations'}
CONSTANTS = {'REVIEW_INDEX_PREFIX'}
# Row layout: (path, 'metadata', mode, uid, gid, dev, inode, size, mtime_ns, ctime_ns).
FIELDS = {'mode': 2, 'uid': 3, 'gid': 4, 'dev': 5, 'inode': 6, 'size': 7, 'mtime_ns': 8, 'ctime_ns': 9}
DIRECTORY = stat.S_IFDIR | 0o700
REGULAR = stat.S_IFREG | 0o600


def guest_namespace(uid):
    """Execute only the selected guest definitions; importing the guest runs main()."""
    module = ast.parse(GUEST.read_text())
    selected = [node for node in module.body
                if (isinstance(node, ast.FunctionDef) and node.name in FUNCTIONS)
                or (isinstance(node, ast.Assign) and any(isinstance(target, ast.Name) and target.id in CONSTANTS for target in node.targets))]
    namespace = {'hashlib': hashlib, 'json': json, 'os': os, 'pathlib': pathlib, 'stat': stat, 'FIXTURE_UID': uid}
    exec(compile(ast.Module(body=selected, type_ignores=[]), str(GUEST), 'exec'), namespace)
    missing = (FUNCTIONS | CONSTANTS) - set(namespace)
    if missing:
        raise AssertionError('guest preservation contract missing: ' + ', '.join(sorted(missing)))
    return namespace


def metadata(path, mode, inode, size=4096, mtime=10, ctime=20):
    return (path, 'metadata', mode, 1002, 1002, 7, inode, size, mtime, ctime)


def synthetic():
    return [
        metadata('.', DIRECTORY, 1),
        metadata('.git', DIRECTORY, 2),
        metadata('.git/HEAD', REGULAR, 3, size=21),
        ('.git/HEAD', 'file', 'a' * 64),
        metadata('README.md', REGULAR, 4, size=6),
        ('README.md', 'file', 'b' * 64),
        metadata('link', stat.S_IFLNK | 0o777, 5, size=9),
        ('link', 'link', 'README.md'),
        metadata('src', DIRECTORY, 6),
        metadata('src/.git', DIRECTORY, 7),
    ]


def replace(rows, path, kind, **fields):
    result = []
    for row in rows:
        if row[0] == path and row[1] == kind:
            row = list(row)
            for name, value in fields.items():
                row[FIELDS[name] if kind == 'metadata' else 2] = value
            row = tuple(row)
        result.append(row)
    return result


class SyntheticPreservationPolicy(unittest.TestCase):
    def setUp(self):
        self.compare = guest_namespace(1002)['project_preservation_violations']
        self.before = synthetic()

    def assertPreserved(self, after):
        self.assertEqual(self.compare(self.before, after), [])

    def assertRefused(self, after, before=None):
        self.assertNotEqual(self.compare(self.before if before is None else before, after), [])

    def test_identical_inventory_is_preserved(self):
        self.assertPreserved(list(self.before))

    def test_only_root_git_directory_times_may_differ(self):
        self.assertPreserved(replace(self.before, '.git', 'metadata', mtime_ns=11, ctime_ns=21))
        self.assertPreserved(replace(self.before, '.git', 'metadata', mtime_ns=11))
        self.assertPreserved(replace(self.before, '.git', 'metadata', ctime_ns=21))

    def test_every_other_root_git_metadata_field_refuses(self):
        changes = {'mode': DIRECTORY | 0o055, 'uid': 1003, 'gid': 1003, 'dev': 8, 'inode': 99, 'size': 8192}
        for name, value in changes.items():
            with self.subTest(field=name):
                self.assertRefused(replace(self.before, '.git', 'metadata', **{name: value}))
                self.assertRefused(replace(self.before, '.git', 'metadata', mtime_ns=11, ctime_ns=21, **{name: value}))

    def test_other_directory_times_refuse(self):
        for path in ['.', 'src', 'src/.git']:
            for name in ['mtime_ns', 'ctime_ns']:
                with self.subTest(path=path, field=name):
                    self.assertRefused(replace(self.before, path, 'metadata', **{name: 11}))

    def test_other_file_times_refuse(self):
        for path in ['.git/HEAD', 'README.md', 'link']:
            with self.subTest(path=path):
                self.assertRefused(replace(self.before, path, 'metadata', mtime_ns=11, ctime_ns=21))

    def test_root_git_file_is_not_a_directory_exception(self):
        before = replace(self.before, '.git', 'metadata', mode=REGULAR)
        after = replace(before, '.git', 'metadata', mtime_ns=11, ctime_ns=21)
        self.assertRefused(after, before=before)

    def test_root_git_creation_or_removal_refuses(self):
        without = [row for row in self.before if not row[0].startswith('.git')]
        self.assertRefused(without)
        self.assertRefused(self.before, before=without)

    def test_content_link_addition_and_deletion_refuse(self):
        self.assertRefused(replace(self.before, 'README.md', 'file', sha='c' * 64))
        self.assertRefused(replace(self.before, '.git/HEAD', 'file', sha='c' * 64))
        self.assertRefused(replace(self.before, 'link', 'link', target='elsewhere'))
        added = self.before + [metadata('new.txt', REGULAR, 9, size=1), ('new.txt', 'file', 'd' * 64)]
        self.assertRefused(added)
        self.assertRefused([row for row in self.before if row[0] != 'README.md'])

    def test_review_index_and_lock_residue_refuse_even_when_unchanged(self):
        for name in ['.gentle-ai-review-index-123', '.gentle-ai-review-index-123.lock']:
            residue = [metadata('.git/' + name, REGULAR, 10, size=0), ('.git/' + name, 'file', hashlib.sha256(b'').hexdigest())]
            with self.subTest(name=name):
                self.assertRefused(self.before + residue)
                self.assertRefused(replace(self.before, '.git', 'metadata', mtime_ns=11) + residue)
                self.assertRefused(self.before + residue, before=self.before + residue)

    def test_violations_name_the_refused_paths(self):
        after = replace(replace(self.before, '.git', 'metadata', size=8192), 'README.md', 'file', sha='c' * 64)
        violations = self.compare(self.before, after)
        self.assertEqual(sorted({row[0] for row in violations}), ['.git', 'README.md'])


class PhysicalPreservationPolicy(unittest.TestCase):
    """Actual isolated fixtures under .checks, inventoried by the guest itself."""

    def setUp(self):
        SCRATCH.mkdir(exist_ok=True)
        self.scratch = tempfile.TemporaryDirectory(prefix='shell-linux-preservation-', dir=SCRATCH)
        self.project = pathlib.Path(self.scratch.name) / 'project'
        (self.project / '.git/objects').mkdir(parents=True, mode=0o700)
        (self.project / '.git/HEAD').write_text('ref: refs/heads/main\n')
        (self.project / 'README.md').write_text('fixture\n')
        self.namespace = guest_namespace(os.getuid())

    def tearDown(self):
        self.scratch.cleanup()

    def inventory(self):
        details = []
        digest = self.namespace['physical_inventory'](self.project, details)
        return digest, details

    def touch_git_like_review_index(self):
        """Create/delete a private index beside Git control files until both times move."""
        git = self.project / '.git'
        before = git.lstat()
        for attempt in range(200):
            transient = git / f'.gentle-ai-review-index-{attempt}'
            transient.write_bytes(b'index')
            (git / (transient.name + '.lock')).write_bytes(b'lock')
            (git / (transient.name + '.lock')).unlink()
            transient.unlink()
            after = git.lstat()
            if after.st_mtime_ns != before.st_mtime_ns and after.st_ctime_ns != before.st_ctime_ns:
                return
            time.sleep(0.01)
        self.fail('fixture filesystem did not advance .git directory times')

    def test_review_index_cycle_breaks_strict_inventory_but_meets_policy(self):
        strict_before, before = self.inventory()
        self.touch_git_like_review_index()
        strict_after, after = self.inventory()
        # The former strict comparator (whole-inventory hash equality) refuses this.
        self.assertNotEqual(strict_after, strict_before)
        self.assertEqual(self.namespace['project_preservation_violations'](before, after), [])

    def test_physical_mutations_refuse(self):
        def content():
            (self.project / 'README.md').write_text('changed\n')

        def added():
            (self.project / 'new.txt').write_text('new\n')

        def deleted():
            (self.project / 'README.md').unlink()

        def git_mode():
            (self.project / '.git').chmod(0o750)

        def git_inode():
            (self.project / '.git').rename(self.project / 'old')
            (self.project / '.git').mkdir(mode=0o700)
            (self.project / 'old/HEAD').rename(self.project / '.git/HEAD')
            (self.project / 'old/objects').rename(self.project / '.git/objects')
            (self.project / 'old').rmdir()

        def root_times():
            os.utime(self.project, ns=(1, 1))

        def nested_git_times():
            os.utime(self.project / '.git/objects', ns=(1, 1))

        def index_residue():
            (self.project / '.git/.gentle-ai-review-index-9').write_bytes(b'')

        def lock_residue():
            (self.project / '.git/.gentle-ai-review-index-9.lock').write_bytes(b'')

        for mutate in [content, added, deleted, git_mode, git_inode, root_times, nested_git_times, index_residue, lock_residue]:
            with self.subTest(mutation=mutate.__name__):
                self.tearDown()
                self.setUp()
                _, before = self.inventory()
                mutate()
                _, after = self.inventory()
                self.assertNotEqual(self.namespace['project_preservation_violations'](before, after), [])

    def test_preexisting_residue_refuses_without_any_change(self):
        (self.project / '.git/.gentle-ai-review-index-7.lock').write_bytes(b'')
        _, before = self.inventory()
        _, after = self.inventory()
        self.assertEqual(after, before)
        self.assertNotEqual(self.namespace['project_preservation_violations'](before, after), [])

    def test_inventory_still_refuses_foreign_fixture_objects(self):
        foreign = guest_namespace(os.getuid() + 1)
        with self.assertRaisesRegex(RuntimeError, 'foreign fixture object'):
            foreign['physical_inventory'](self.project, [])


def guest_runner(ceiling):
    """Execute only run() and its budget helpers, with a portable working directory."""
    import base64
    import signal
    import subprocess
    module = ast.parse(GUEST.read_text())
    selected = [node for node in module.body if isinstance(node, ast.FunctionDef) and node.name in {'require', 'remaining', 'run'}]
    namespace = {'base64': base64, 'hashlib': hashlib, 'os': os, 'pathlib': pathlib, 'signal': signal, 'subprocess': subprocess, 'time': time,
                 'START': time.monotonic(), 'CEILING': ceiling, 'WORK': pathlib.Path(tempfile.gettempdir()),
                 'TESTS': '/fixture/user-install.test', 'SUPERVISOR': '/fixture/supervisor', 'COMMAND_FAILURE': None}
    exec(compile(ast.Module(body=selected, type_ignores=[]), str(GUEST), 'exec'), namespace)
    return namespace


class OwnedCommandTimeout(unittest.TestCase):
    def test_timeout_identifies_the_killed_command_with_bounded_tail(self):
        guest = guest_runner(ceiling=850)
        with self.assertRaisesRegex(RuntimeError, 'owned command deadline'):
            guest['run'](['/bin/sh', '-c', 'printf partial; sleep 5', 'prepare-prior'], timeout=1)
        failure = guest['COMMAND_FAILURE']
        self.assertEqual((failure['timeout'], failure['limitSeconds'], failure['wholeBudget']), (True, 1, False))
        self.assertEqual(failure['operation'], 'prepare-prior')
        self.assertEqual(failure['stdoutTailBase64'], 'cGFydGlhbA==')
        self.assertNotIn('args', failure)

    def test_timeout_reports_whole_budget_exhaustion(self):
        guest = guest_runner(ceiling=1)
        with self.assertRaisesRegex(RuntimeError, 'owned command deadline'):
            guest['run'](['/bin/sh', '-c', 'sleep 5'], timeout=180)
        self.assertTrue(guest['COMMAND_FAILURE']['wholeBudget'])


class DeadlineNesting(unittest.TestCase):
    def test_guest_budget_fits_inside_service_and_wrapper_limits(self):
        import re
        workflow = (ROOT / '.github/workflows/shell-linux-first-ci.yml').read_text()
        ceiling = int(re.search(r'^CEILING = (\d+)$', GUEST.read_text(), re.M).group(1))
        service = int(re.search(r'RuntimeMaxSec=(\d+)', workflow).group(1))
        wrapper = int(re.search(r'timeout --kill-after=5 (\d+) ', workflow).group(1))
        self.assertLess(ceiling, service)
        self.assertLess(service, wrapper)
        # The full qualification reached its final fault step with only 66s left at 850s.
        self.assertGreaterEqual(ceiling, 1300)


class LaunchWiring(unittest.TestCase):
    def test_launch_requires_the_policy_with_the_original_failure(self):
        module = ast.parse(GUEST.read_text())
        function = next(node for node in module.body if isinstance(node, ast.FunctionDef) and node.name == 'pty_status')
        text = ast.unparse(function)
        self.assertIn('project_preservation_violations(project_before_entries, project_after_entries)', text)
        self.assertIn("require(not violations, 'fixture blank caller project changed during launch')", text)
        self.assertIn("'notifications': project_events(observer)", text)
        calls = [node for node in ast.walk(function) if isinstance(node, ast.Call) and isinstance(node.func, ast.Name) and node.func.id == 'physical_inventory']
        self.assertEqual([call.args[1].id for call in calls], ['project_before_entries', 'project_after_entries'])


if __name__ == '__main__':
    unittest.main()
