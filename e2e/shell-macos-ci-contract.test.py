"""Portable controls for the native macOS qualification workflow and script.

Pure file parsing with the standard library (no YAML package), so it runs on
Linux and macOS alike: `python3 -I e2e/shell-macos-ci-contract.test.py`.
"""
import ast
import pathlib
import re
import subprocess
import sys
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]
WORKFLOW = pathlib.Path(sys.argv.pop()) if len(sys.argv) == 2 else ROOT / '.github/workflows/shell-macos-qualification.yml'
SCRIPT = ROOT / 'e2e/shell-macos-qualification.py'
PINS = ROOT / 'internal/shellinstaller/user_pins_darwin.go'
REQUIRED_PATHS = [
    'internal/shellinstaller/**',
    'internal/cli/shell_install*',
    'scripts/bootstrap-gentle-shell-private-node.sh',
    'scripts/provision-gentle-shell-private-global.mjs',
    'scripts/runtime_helpers.go',
    'scripts/user_helpers.go',
    'scripts/user-global-graph.mjs',
    'scripts/user-locks/**',
    'scripts/complete-generated-lock-sri.mjs',
    'scripts/normalize-private-optional-platform-closure.mjs',
    'internal/app/app.go',
    'cmd/gentle-ai/main.go',
    'go.mod',
    'go.sum',
    'e2e/shell-macos-*',
    'docs/gentle-shell-macos-install.md',
    '.github/workflows/shell-macos-qualification.yml',
]
# Margin between the script's whole-run deadline and its step deadline, so the
# script reports the stuck command before the runner kills the step.
DEADLINE_MARGIN_SECONDS = 180


def indent(line):
    return len(line) - len(line.lstrip(' '))


def children(lines, index):
    """Lines nested under lines[index] (deeper indentation; blank lines kept)."""
    base, nested = indent(lines[index]), []
    for line in lines[index + 1:]:
        if line.strip() and indent(line) <= base:
            break
        nested.append(line)
    return nested


def find(lines, text, start=0):
    for index in range(start, len(lines)):
        if lines[index].strip() == text:
            return index
    raise AssertionError('missing workflow key %r' % text)


def scalar(lines, key):
    values = [line.split(':', 1)[1].strip() for line in lines if line.strip().startswith(key + ':')]
    return values[0] if values else None


def steps(lines):
    body = children(lines, find(lines, 'steps:'))
    starts = [index for index, line in enumerate(body) if line.lstrip().startswith('- ')]
    parsed = []
    for position, start in enumerate(starts):
        chunk = body[start:starts[position + 1] if position + 1 < len(starts) else len(body)]
        first = chunk[0].lstrip()[2:]
        chunk = [' ' * (indent(chunk[0]) + 2) + first] + chunk[1:]
        own = [line for line in chunk if indent(line) == indent(chunk[0])]
        step = {'lines': chunk, 'name': scalar(own, 'name'), 'id': scalar(own, 'id'), 'uses': scalar(own, 'uses'),
                'if': scalar(own, 'if'), 'timeout': scalar(own, 'timeout-minutes'), 'run': None}
        for index, line in enumerate(chunk):
            if indent(line) == indent(chunk[0]) and line.strip().startswith('run:'):
                inline = line.split(':', 1)[1].strip()
                if inline == '|':
                    step['run'] = '\n'.join(text[indent(line) + 2:] for text in children(chunk, index)) + '\n'
                else:
                    step['run'] = inline + '\n'
        parsed.append(step)
    return parsed


def script_constants():
    tree = ast.parse(SCRIPT.read_text())
    values = {}
    for node in tree.body:
        if isinstance(node, ast.Assign) and len(node.targets) == 1 and isinstance(node.targets[0], ast.Name):
            try:
                values[node.targets[0].id] = ast.literal_eval(node.value)
            except ValueError:
                pass
    return tree, values


class MacOSQualificationContract(unittest.TestCase):
    def setUp(self):
        self.text = WORKFLOW.read_text()
        self.lines = self.text.splitlines()
        self.steps = steps(self.lines)
        self.by_id = {step['id']: step for step in self.steps if step['id']}

    def test_triggers_keep_installer_paths_and_manual_dispatch(self):
        on = children(self.lines, find(self.lines, 'on:'))
        events = {line.strip().rstrip(':') for line in on if indent(line) == 2}
        self.assertEqual(events, {'pull_request', 'push', 'workflow_dispatch'})
        self.assertNotIn('pull_request_target', self.text)
        self.assertNotIn('schedule', self.text)
        selected = {}
        for event in ['pull_request', 'push']:
            index = find(self.lines, event + ':')
            block = children(self.lines, index)
            self.assertIn('branches: [main]', [line.strip() for line in block])
            paths_index = find(self.lines, 'paths:', index)
            selected[event] = [line.strip()[2:].strip("'") for line in children(self.lines, paths_index)]
            for path in REQUIRED_PATHS:
                self.assertIn(path, selected[event], event)
        self.assertEqual(selected['pull_request'], selected['push'])

    def test_native_runner_without_credentials_or_permissions(self):
        self.assertEqual(scalar([line for line in self.lines if indent(line) == 0], 'permissions'), '{}')
        self.assertEqual(self.text.count('runs-on:'), 1)
        self.assertEqual(scalar(self.lines, 'runs-on'), 'macos-latest')
        # Only superseded pull request runs may be cancelled; each push keeps its own receipt.
        self.assertIn("cancel-in-progress: ${{ github.event_name == 'pull_request' }}", self.text)
        self.assertIn('github.event.pull_request.number || github.sha }}', self.text)
        self.assertNotIn('cancel-in-progress: true', self.text)
        self.assertIn("github.repository == 'Gentleman-Programming/gentle-ai'", self.text)
        self.assertNotIn('actions/checkout', self.text)
        self.assertNotIn('secrets.', self.text)
        source = self.steps[0]
        self.assertIn('credential.helper=', source['run'])
        self.assertIn('github.event.pull_request.head.sha', '\n'.join(source['lines']))
        self.assertIn('test "$(uname -s)/$(uname -m)" = Darwin/arm64', source['run'])
        for step in self.steps:
            if step['uses']:
                self.assertRegex(step['uses'], r'^[\w.-]+/[\w.-]+@[0-9a-f]{40}(\s+#.*)?$')

    def test_go_tests_and_build_precede_qualification(self):
        runs = [step['run'] or '' for step in self.steps]
        order = []
        for marker in ['go test ./internal/shellinstaller ./scripts -count=1',
                       'go test ./internal/cli -run Shell -count=1',
                       'go build -mod=readonly -o "$RUNNER_TEMP/candidate/gentle-ai" ./cmd/gentle-ai',
                       'e2e/shell-macos-qualification.py']:
            order.append(next(index for index, run in enumerate(runs) if marker in run))
        self.assertEqual(order, sorted(order))
        setup = next(step for step in self.steps if step['uses'] and step['uses'].startswith('actions/setup-go@'))
        self.assertIn('go-version-file: go.mod', '\n'.join(setup['lines']))

    def test_qualification_script_is_invoked_isolated(self):
        run = self.by_id['qualify']['run']
        self.assertIn('base="$(mktemp -d /private/tmp/gentle-macos-qualification.XXXXXX)"', run)
        self.assertIn('/usr/bin/env -i PATH=/usr/bin:/bin TMPDIR="$base" /usr/bin/python3 -I e2e/shell-macos-qualification.py', run)
        self.assertIn('--gentle-ai "$RUNNER_TEMP/candidate/gentle-ai"', run)
        self.assertIn('--receipt "$RUNNER_TEMP/macos-qualification/receipt.json"', run)
        self.assertNotIn('|| true', run)

    def test_deadlines_nest_inside_the_job(self):
        job = int(scalar(self.lines, 'timeout-minutes'))
        budgets = [int(step['timeout']) for step in self.steps]
        self.assertEqual(len(budgets), len(self.steps), 'every step needs its own timeout-minutes')
        self.assertLessEqual(sum(budgets), job)
        _, constants = script_constants()
        whole = constants['WHOLE_RUN_SECONDS']
        step = int(self.by_id['qualify']['timeout'])
        self.assertLessEqual(whole + DEADLINE_MARGIN_SECONDS, step * 60)
        for name in ['INSPECT_SECONDS', 'INSTALL_SECONDS', 'RECOVER_SECONDS', 'PROBE_SECONDS']:
            self.assertLess(constants[name], whole, name)

    def test_receipt_artifact_is_uploaded_even_after_failure(self):
        upload = next(step for step in self.steps if step['uses'] and step['uses'].startswith('actions/upload-artifact@'))
        self.assertEqual(upload['if'], "always() && steps.qualify.outcome != 'skipped'")
        body = '\n'.join(upload['lines'])
        self.assertIn('path: ${{ runner.temp }}/macos-qualification/receipt.json', body)
        self.assertIn('if-no-files-found: error', body)
        self.assertGreater(self.steps.index(upload), self.steps.index(self.by_id['qualify']))

    def test_script_is_stdlib_only_and_reports_timeouts(self):
        tree, constants = script_constants()
        allowed = {'argparse', 'hashlib', 'json', 'os', 'platform', 'pwd', 're', 'shutil', 'signal', 'subprocess', 'sys',
                   'tarfile', 'tempfile', 'time'}
        imported = set()
        for node in ast.walk(tree):
            if isinstance(node, ast.Import):
                imported.update(alias.name.split('.')[0] for alias in node.names)
            elif isinstance(node, ast.ImportFrom):
                imported.add(node.module.split('.')[0])
        self.assertLessEqual(imported, allowed)
        text = SCRIPT.read_text()
        for marker in ["command.update(timeout=True", "stdoutTail=tail(", "'elapsedSeconds': seconds",
                       "'checkpoint': STATE['checkpoint']", "start_new_session=True", "stdin=subprocess.DEVNULL",
                       "'PATH': '/usr/bin:/bin'"]:
            self.assertIn(marker, text)
        self.assertLessEqual(constants['TAIL_BYTES'], 4096)

    def test_script_pins_equal_the_darwin_installer_pins(self):
        go = PINS.read_text()
        _, constants = script_constants()
        node = re.search(r'privateNativeNodeSize int64 = (\d+)\nconst privateNativeNodeSHA = "([0-9a-f]{64})"', go)
        self.assertEqual(constants['NODE_PIN'], (node.group(2), int(node.group(1))))
        binary = re.search(r'userBinarySize int64 = (\d+)\nconst userBinarySHA = "([0-9a-f]{64})"', go)
        self.assertEqual(constants['GENTLE_AI_PIN'], (binary.group(2), int(binary.group(1))))
        rows = re.findall(r'\{"(\w+)", "[\w/]+", "v?[\d.]+", "([\w.-]+)", "([0-9a-f]{64})", (\d+)\}', go)
        tools = {name: (sha, int(size), stem + '/' + name) for name, stem, sha, size in rows}
        self.assertEqual(set(tools), {'fd', 'rg'})
        for name, pin in constants['TOOL_PINS'].items():
            self.assertEqual(pin[:3], tools[name], name)

    def test_shell_blocks_parse_without_execution(self):
        for step in self.steps:
            if step['run'] is None:
                continue
            with self.subTest(step=step['name']):
                result = subprocess.run(['/bin/bash', '-n'], input=step['run'], text=True, capture_output=True, timeout=5)
                self.assertEqual(result.returncode, 0, result.stderr[:1000])


if __name__ == '__main__':
    unittest.main()
