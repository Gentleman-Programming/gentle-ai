"""Native macOS qualification of `gentle-ai shell install` and `shell recover`.

Run with `python3 -I` on an Apple silicon Mac (macOS 14+) as a non-root user:

    TMPDIR=/private/tmp/<fresh dir> python3 -I e2e/shell-macos-qualification.py \
        --gentle-ai /path/to/built/gentle-ai --receipt /path/to/receipt.json

Everything is created inside one new 0700 root under realpath(TMPDIR), which
must use the canonical /private/... spelling. Every installer command runs with
an explicit minimal environment (HOME, TMPDIR and XDG_* inside the root,
PATH=/usr/bin:/bin), its own session, a per-command deadline and the whole-run
deadline. The JSON receipt is written to stdout and to --receipt; the exit code
is non-zero on any failure and the receipt names the failing checkpoint.
Standard library only; compatible with the system Python 3.9.
"""
import argparse
import hashlib
import json
import os
import platform
import pwd
import shutil
import signal
import subprocess
import sys
import tarfile
import tempfile
import time

sys.dont_write_bytecode = True

SCHEMA = 'gentle-shell-macos-qualification/v1'
# Whole-run budget. The workflow step (25 min) and job (45 min) deadlines nest
# around it with margin, so a stuck command is reported here, never by a runner kill.
WHOLE_RUN_SECONDS = 1320
INSPECT_SECONDS = 180
INSTALL_SECONDS = 720
RECOVER_SECONDS = 300
PROBE_SECONDS = 90
TAIL_BYTES = 1024
OUTPUT_BOUND = 1 << 20

# Darwin arm64 pins; e2e/shell-macos-ci-contract.test.py keeps them equal to
# internal/shellinstaller/user_pins_darwin.go.
NODE_PIN = ('ee6fb0e015284d83a91e8ec5213f43a157f8a392b58555301682892ba928c04a', 120965360)
GENTLE_AI_PIN = ('18a9f7fae55d85c95684b6d512a4a148d0cb24a856325f72573c34caf65159eb', 16047186)
TOOL_PINS = {
    'fd': ('b67e1836c468e42e411984b56e52fa7abec08c2bd22c867398e7cc134aac5e12', 1334374, 'fd-v10.5.0-aarch64-apple-darwin/fd', 'fd 10.5.0'),
    'rg': ('3750b2e93f37e0c692657da574d7019a101c0084da05a790c83fd335bad973e4', 1764284, 'ripgrep-15.2.0-aarch64-apple-darwin/rg', 'ripgrep 15.2.0'),
}
PI_VERSION = '1.0.0'
NODE_VERSION = 'v24.18.0'
GENTLE_PI_VERSION = '4.0.0'
CODING_AGENT = 'lib/node_modules/@earendil-works/pi-coding-agent/package.json'
GENTLE_AI_MEMBER = 'lib/node_modules/gentle-pi/.gentle-ai/v4.0.0/gentle-ai'
RESIDUE_PREFIXES = ('.gentle-node-stage.', '.gentle-shell-stage.', '.gentle-user-', '.gentle-go-', '.gentle-native-', '.gentle-unit-')
# Real-HOME locations a leaked HOME would make the installer, npm, Node or Pi touch.
REAL_HOME_WATCH = ('.pi', '.pi/agent', '.npm', '.npmrc', '.gentle-ai', '.gentle-shell', '.node_repl_history',
                   '.config', '.config/gentle-ai', '.config/pi', '.cache', '.cache/gentle-ai', '.local',
                   '.local/share', '.local/state', 'Library/Caches', 'Library/Application Support')

START = time.monotonic()
RECEIPT = {'schema': SCHEMA, 'result': 'fail', 'checkpoints': []}
STATE = {'checkpoint': 'arguments', 'command': None, 'sequence': 0}


class QualificationError(Exception):
    pass


def require(condition, message):
    if not condition:
        raise QualificationError(message)


def elapsed():
    return round(time.monotonic() - START, 1)


def remaining():
    value = WHOLE_RUN_SECONDS - (time.monotonic() - START)
    require(value > 0, 'whole-run deadline of %d s exhausted' % WHOLE_RUN_SECONDS)
    return value


def checkpoint(name):
    STATE['checkpoint'] = name


def reached(name, **details):
    entry = {'name': name, 'elapsedSeconds': elapsed()}
    entry.update(details)
    RECEIPT['checkpoints'].append(entry)


def sha256_file(path):
    digest = hashlib.sha256()
    with open(path, 'rb') as handle:
        for block in iter(lambda: handle.read(1 << 20), b''):
            digest.update(block)
    return digest.hexdigest()


def tail(path):
    with open(path, 'rb') as handle:
        handle.seek(0, os.SEEK_END)
        size = handle.tell()
        handle.seek(max(0, size - TAIL_BYTES))
        return handle.read().decode('utf-8', 'replace')


class Context:
    def __init__(self, root, supervisor):
        self.root = root
        self.supervisor = supervisor
        self.home = os.path.join(root, 'home')
        self.tmp = os.path.join(root, 'tmp')
        self.xdg = os.path.join(root, 'xdg')
        self.work = os.path.join(root, 'work')
        self.logs = os.path.join(root, 'logs')
        self.env = {
            'HOME': self.home, 'TMPDIR': self.tmp + '/', 'PATH': '/usr/bin:/bin', 'TERM': 'dumb', 'LANG': 'C',
            'XDG_CONFIG_HOME': os.path.join(self.xdg, 'config'), 'XDG_STATE_HOME': os.path.join(self.xdg, 'state'),
            'XDG_CACHE_HOME': os.path.join(self.xdg, 'cache'), 'XDG_DATA_HOME': os.path.join(self.xdg, 'data'),
        }

    def run(self, label, args, limit, good=True):
        """Run one command in its own session with stdin closed and output in files."""
        STATE['sequence'] += 1
        stem = os.path.join(self.logs, '%02d-%s' % (STATE['sequence'], label))
        budget = min(limit, remaining())
        started = time.monotonic()
        with open(stem + '.out', 'wb') as out, open(stem + '.err', 'wb') as err:
            child = subprocess.Popen(args, cwd=self.work, env=self.env, stdin=subprocess.DEVNULL, stdout=out,
                                     stderr=err, start_new_session=True, close_fds=True)
            try:
                code = child.wait(timeout=budget)
            except subprocess.TimeoutExpired:
                code = None
                self.kill_session(child)
        seconds = round(time.monotonic() - started, 1)
        command = {'label': label, 'limitSeconds': round(budget), 'elapsedSeconds': seconds, 'exit': code,
                   'expectedSuccess': good, 'startedAtSeconds': round(started - START, 1)}
        if code is None:
            command.update(timeout=True, wholeRunBudget=budget < limit, stdoutTail=tail(stem + '.out'), stderrTail=tail(stem + '.err'))
            STATE['command'] = command
            raise QualificationError('%s exceeded its %d s deadline after %.1f s' % (label, round(budget), seconds))
        sizes = [os.path.getsize(stem + suffix) for suffix in ('.out', '.err')]
        if (code == 0) != good or sum(sizes) > OUTPUT_BOUND:
            command.update(stdoutTail=tail(stem + '.out'), stderrTail=tail(stem + '.err'), outputBytes=sum(sizes))
            STATE['command'] = command
            raise QualificationError('%s exited %d, expected %s' % (label, code, 'success' if good else 'refusal'))
        with open(stem + '.out', 'rb') as handle:
            stdout = handle.read().decode('utf-8', 'replace')
        with open(stem + '.err', 'rb') as handle:
            stderr = handle.read().decode('utf-8', 'replace')
        STATE['command'] = None
        return stdout, stderr

    @staticmethod
    def kill_session(child):
        # The supervisor puts owned commands in their own process groups, so
        # killing only the session leader's group could leave them running.
        try:
            os.killpg(child.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        try:
            subprocess.run(['/usr/bin/pkill', '-KILL', '-s', str(child.pid)], stdin=subprocess.DEVNULL,
                           stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=10)
        except (OSError, subprocess.TimeoutExpired):
            pass
        try:
            child.wait(timeout=10)
        except subprocess.TimeoutExpired:
            pass


def gentle(context, label, args, limit, good=True):
    return context.run(label, [context.supervisor, 'shell'] + args, limit, good)


def version_line(text, expected):
    lines = [line.strip() for line in text.splitlines()]
    require(expected in lines, 'expected version line %r, observed %r' % (expected, lines[:4]))


def inspect(context, label, target, shared=None, good=True):
    args = ['install', '--target', target, '--mode', 'shared' if shared else 'separate']
    if shared:
        args += ['--prefix', shared[0], '--agent', shared[1]]
    stdout, stderr = gentle(context, label, args + ['--inspect'], INSPECT_SECONDS, good)
    if not good:
        return args, None, stdout + stderr
    lines = [line for line in stdout.splitlines() if line.startswith('Confirmation: ')]
    require(len(lines) == 1, 'inspect printed %d confirmation lines' % len(lines))
    token = lines[0].split(': ', 1)[1]
    require(len(token) == 64 and token.strip('0123456789abcdef') == '', 'malformed confirmation token')
    return args, token, stdout


def install(context, label, target, shared=None):
    checkpoint(label + '-inspect')
    args, token, _ = inspect(context, label + '-inspect', target, shared)
    reached(label + '-inspect')
    checkpoint(label + '-confirm')
    started = time.monotonic()
    stdout, _ = gentle(context, label + '-confirm', args + ['--confirm', token], INSTALL_SECONDS)
    require('Installed %s;' % target in stdout, 'installation receipt line missing')
    with open(os.path.join(target, 'installation.json'), 'rb') as handle:
        manifest = json.loads(handle.read())
    require(manifest.get('Destination') == target, 'manifest destination differs')
    require(manifest.get('Mode') == ('shared' if shared else 'separate'), 'manifest mode differs')
    require(manifest.get('SupervisorSHA') == RECEIPT['candidate']['sha256'], 'manifest supervisor differs from candidate')
    require(manifest.get('NodeSHA') == NODE_PIN[0], 'manifest Node pin differs')
    reached(label + '-confirm', installSeconds=round(time.monotonic() - started, 1))
    return manifest


def pinned(path, pin):
    observed = {'sha256': sha256_file(path), 'bytes': os.path.getsize(path)}
    observed['pinSHA256'] = pin[0]
    observed['matchesPin'] = observed['sha256'] == pin[0] and observed['bytes'] == pin[1]
    require(observed['matchesPin'], '%s differs from its darwin pin' % os.path.basename(path))
    return observed


def launch_versions(context, target, label):
    pi, _ = context.run(label + '-pi-version', [os.path.join(target, 'bin/pi'), '--version'], PROBE_SECONDS)
    version_line(pi, PI_VERSION)
    shell, _ = context.run(label + '-gentle-shell-version', [os.path.join(target, 'bin/gentle-shell'), '--version'], PROBE_SECONDS)
    version_line(shell, PI_VERSION)
    node, _ = context.run(label + '-node-version', [os.path.join(target, 'runtime/node/bin/node'), '--version'], PROBE_SECONDS)
    version_line(node, NODE_VERSION)
    return {'pi': PI_VERSION, 'gentleShell': PI_VERSION, 'node': NODE_VERSION}


def tool_evidence(context, target, agent, label):
    """Retained archives must equal their pins and the installed tools their members."""
    evidence = {}
    for name, (sha, size, member, version) in sorted(TOOL_PINS.items()):
        archive = os.path.join(target, 'runtime/tools', name + '.tgz')
        record = {'archive': pinned(archive, (sha, size))}
        with tarfile.open(archive, 'r:gz') as bundle:
            entry = bundle.getmember(member)
            require(entry.isfile() and entry.size < 64 << 20, '%s archive member is not a bounded file' % name)
            data = bundle.extractfile(entry).read()
        installed = os.path.join(agent, 'bin', name)
        info = os.lstat(installed)
        require(oct(info.st_mode & 0o7777) == '0o700' and os.path.isfile(installed) and not os.path.islink(installed),
                '%s is not an owned 0700 regular file' % name)
        record['memberSHA256'] = hashlib.sha256(data).hexdigest()
        record['installedSHA256'] = sha256_file(installed)
        record['matchesArchiveMember'] = record['installedSHA256'] == record['memberSHA256']
        require(record['matchesArchiveMember'], 'installed %s differs from its pinned archive member' % name)
        stdout, _ = context.run('%s-%s-version' % (label, name), [installed, '--version'], PROBE_SECONDS)
        first = (stdout.splitlines() or [''])[0].strip()
        # ripgrep appends its build revision: "ripgrep 15.2.0 (rev e89fff89ac)".
        require(first == version or first.startswith(version + ' '), '%s version differs: %r' % (name, first[:80]))
        record['version'] = first[:80]
        evidence[name] = record
    return evidence


def inventory(root):
    """Names, types, modes, inodes, sizes, times, link targets and file bytes of a tree."""
    rows = []
    for directory, names, files in os.walk(root):
        names.sort()
        for name in sorted(names + files):
            path = os.path.join(directory, name)
            info = os.lstat(path)
            row = [os.path.relpath(path, root), info.st_mode, info.st_ino, info.st_size, info.st_mtime_ns, info.st_ctime_ns]
            if os.path.islink(path):
                row.append(os.readlink(path))
            elif os.path.isfile(path):
                row.append(sha256_file(path))
            rows.append(row)
    return hashlib.sha256(json.dumps(rows).encode()).hexdigest()


def residue(work):
    found = []
    for directory, names, files in os.walk(work):
        for name in names + files:
            if name.startswith(RESIDUE_PREFIXES):
                found.append(os.path.relpath(os.path.join(directory, name), work))
    return sorted(found)


def real_home_snapshot(home):
    rows = {'.': sorted(os.listdir(home))}
    for relative in REAL_HOME_WATCH:
        try:
            info = os.lstat(os.path.join(home, relative))
            rows[relative] = [info.st_ino, info.st_mtime_ns, info.st_ctime_ns]
        except FileNotFoundError:
            rows[relative] = None
    return rows


def write_file(path, data, mode):
    descriptor = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, mode)
    with os.fdopen(descriptor, 'wb') as handle:
        handle.write(data)


def replace_bytes(path, data):
    mode = os.lstat(path).st_mode & 0o7777
    os.chmod(path, mode | 0o200)
    with open(path, 'wb') as handle:
        handle.write(data)
    os.chmod(path, mode)


def host():
    checkpoint('host')
    require(sys.platform == 'darwin', 'qualification requires macOS')
    machine = os.uname().machine
    release = platform.mac_ver()[0]
    require(machine == 'arm64', 'qualification requires native arm64, observed %s' % machine)
    require(int(release.split('.')[0]) >= 14, 'qualification requires macOS 14+, observed %s' % release)
    require(os.getuid() != 0, 'qualification must not run as root')
    RECEIPT['host'] = {'macOS': release, 'arch': machine, 'darwin': os.uname().release, 'python': platform.python_version()}
    reached('host')


def prepare(arguments):
    checkpoint('root')
    base = os.path.realpath(os.environ.get('TMPDIR', ''))
    require(os.environ.get('TMPDIR') and base.startswith('/private/') and os.path.isdir(base),
            'TMPDIR must name an existing directory whose real path starts with /private/')
    root = tempfile.mkdtemp(prefix='gentle-macos-qualification.', dir=base)
    require(os.path.realpath(root) == root, 'qualification root is not canonical')
    supervisor_source = os.path.realpath(arguments.gentle_ai)
    require(os.path.isfile(supervisor_source), 'candidate gentle-ai binary is missing')
    context = Context(root, os.path.join(root, 'bin/gentle-ai'))
    for directory in [context.home, context.tmp, context.work, context.logs, os.path.join(root, 'bin')] + \
            [context.env[key] for key in ('XDG_CONFIG_HOME', 'XDG_STATE_HOME', 'XDG_CACHE_HOME', 'XDG_DATA_HOME')]:
        os.makedirs(directory, mode=0o700)
    shutil.copyfile(supervisor_source, context.supervisor)
    os.chmod(context.supervisor, 0o755)
    RECEIPT['root'] = root
    RECEIPT['candidate'] = {'sha256': sha256_file(context.supervisor), 'bytes': os.path.getsize(context.supervisor)}
    reached('root')
    return context


def qualify(context):
    real_home = pwd.getpwuid(os.getuid()).pw_dir
    home_before = real_home_snapshot(real_home)

    separate = os.path.join(context.work, 'separate')
    manifest = install(context, 'separate', separate)
    checkpoint('separate-launch')
    versions = launch_versions(context, separate, 'separate')
    reached('separate-launch', **versions)
    checkpoint('separate-artifacts')
    prefix, agent = manifest['Prefix'], manifest['Agent']
    with open(os.path.join(prefix, 'lib/node_modules/gentle-pi/package.json'), 'rb') as handle:
        require(json.loads(handle.read()).get('version') == GENTLE_PI_VERSION, 'Gentle package version differs')
    artifacts = {
        'node': pinned(os.path.join(separate, 'runtime/node/bin/node'), NODE_PIN),
        'gentleAi': pinned(os.path.join(prefix, GENTLE_AI_MEMBER), GENTLE_AI_PIN),
    }
    artifacts.update(tool_evidence(context, separate, agent, 'separate'))
    RECEIPT['separate'] = {'versions': dict(versions, gentlePi=GENTLE_PI_VERSION), 'artifacts': artifacts}
    reached('separate-artifacts')

    checkpoint('shared-fixture')
    shared_prefix = os.path.join(context.work, 'shared-prefix')
    shared_agent = os.path.join(context.work, 'shared-agent')
    conflict_agent = os.path.join(context.work, 'conflict-agent')
    shutil.copytree(prefix, shared_prefix, symlinks=True)
    os.chmod(shared_prefix, 0o700)
    for directory in (shared_agent, conflict_agent, os.path.join(conflict_agent, 'bin')):
        os.mkdir(directory, 0o700)
    settings = b'{}\n'
    write_file(os.path.join(shared_agent, 'settings.json'), settings, 0o600)
    write_file(os.path.join(conflict_agent, 'settings.json'), settings, 0o600)
    write_file(os.path.join(conflict_agent, 'bin/fd'), b'#!/bin/sh\nexit 0\n', 0o700)
    coding_agent = os.path.join(shared_prefix, CODING_AGENT)
    with open(coding_agent, 'rb') as handle:
        original_metadata = handle.read()
    require(json.loads(original_metadata).get('version') == PI_VERSION, 'copied coding-agent version differs')
    reached('shared-fixture')

    checkpoint('shared-fd-refusal')
    refused_target = os.path.join(context.work, 'shared-refused')
    before = (inventory(shared_prefix), inventory(conflict_agent), sorted(os.listdir(context.work)))
    _, _, output = inspect(context, 'shared-fd-refusal', refused_target, (shared_prefix, conflict_agent), good=False)
    require('already contains fd' in output, 'Shared refusal did not name the pre-existing AGENT/bin/fd')
    after = (inventory(shared_prefix), inventory(conflict_agent), sorted(os.listdir(context.work)))
    require(before == after, 'refused Shared inspect changed the prefix, agent or parent inventory')
    require(not os.path.lexists(refused_target), 'refused Shared inspect created its target')
    reached('shared-fd-refusal', inventoriesUnchanged=True)

    shared = os.path.join(context.work, 'shared')
    shared_manifest = install(context, 'shared', shared, (shared_prefix, shared_agent))
    require(shared_manifest.get('Prefix') == shared_prefix and shared_manifest.get('Agent') == shared_agent,
            'Shared manifest does not bind the selected prefix and agent')
    checkpoint('shared-launch')
    shared_versions = launch_versions(context, shared, 'shared')
    reached('shared-launch', **shared_versions)
    checkpoint('shared-artifacts')
    shared_artifacts = {
        'node': pinned(os.path.join(shared, 'runtime/node/bin/node'), NODE_PIN),
        'gentleAi': pinned(os.path.join(shared_prefix, GENTLE_AI_MEMBER), GENTLE_AI_PIN),
    }
    shared_artifacts.update(tool_evidence(context, shared, shared_agent, 'shared'))
    RECEIPT['shared'] = {'versions': shared_versions, 'artifacts': shared_artifacts}
    reached('shared-artifacts')

    checkpoint('recover-inspect')
    stdout, _ = gentle(context, 'recover-inspect', ['recover', shared, 'inspect'], INSPECT_SECONDS)
    tokens = [line.split(': ', 1)[1] for line in stdout.splitlines() if line.startswith('Recovery confirmation: ')]
    require(len(tokens) == 1 and len(tokens[0]) == 64 and tokens[0].strip('0123456789abcdef') == '', 'malformed recovery token')
    token = tokens[0]
    reached('recover-inspect')

    checkpoint('recover-damage')
    damaged = json.loads(original_metadata)
    damaged['version'] = '9.9.9'
    damaged_metadata = json.dumps(damaged).encode()
    replace_bytes(coding_agent, damaged_metadata)
    context.run('damaged-pi-version', [os.path.join(shared, 'bin/pi'), '--version'], PROBE_SECONDS, good=False)
    reached('recover-damage', piRefusedDamagedGraph=True)

    checkpoint('recover-wrong-token')
    wrong = token[:-1] + ('0' if token[-1] != '0' else '1')
    gentle(context, 'recover-wrong-token', ['recover', shared, wrong], RECOVER_SECONDS, good=False)
    with open(coding_agent, 'rb') as handle:
        require(handle.read() == damaged_metadata, 'wrong recovery token changed the prefix')
    stdout, _ = gentle(context, 'recover-reinspect', ['recover', shared, 'inspect'], INSPECT_SECONDS)
    require('Recovery confirmation: %s' % token in stdout.splitlines(), 'recovery token changed after damage')
    reached('recover-wrong-token', tokenStableAfterDamage=True)

    checkpoint('recover-restore')
    stdout, _ = gentle(context, 'recover-restore', ['recover', shared, token], RECOVER_SECONDS)
    require('Restored selected shared preimages' in stdout, 'recovery receipt line missing')
    with open(coding_agent, 'rb') as handle:
        require(handle.read() == original_metadata, 'recovery did not restore the original coding-agent bytes')
    with open(os.path.join(shared_agent, 'settings.json'), 'rb') as handle:
        require(handle.read() == settings, 'recovery did not restore the original settings bytes')
    quarantines = sorted(name for name in os.listdir(os.path.join(shared, 'state')) if '.quarantine-' in name)
    require(len(quarantines) == 2, 'recovery did not retain prefix and agent quarantines')
    reached('recover-restore', originalBytesRestored=True, retainedQuarantines=len(quarantines))

    checkpoint('residue')
    leftovers = residue(context.work)
    require(not leftovers, 'stage or workspace residue: %s' % leftovers[:8])
    expected = ['conflict-agent', 'separate', 'shared', 'shared-agent', 'shared-prefix']
    require(sorted(os.listdir(context.work)) == expected, 'unexpected entries beside the targets')
    for directory in [context.home, context.tmp] + [context.env[key] for key in ('XDG_CONFIG_HOME', 'XDG_STATE_HOME', 'XDG_CACHE_HOME', 'XDG_DATA_HOME')]:
        require(os.listdir(directory) == [], 'redirected %s is not empty' % os.path.relpath(directory, context.root))
    reached('residue', stageOrWorkspaceResidue=0)

    checkpoint('real-home')
    home_after = real_home_snapshot(real_home)
    changed = sorted(key for key in home_before if home_before[key] != home_after[key])
    require(not changed, 'real HOME changed at %s' % changed)
    reached('real-home', watchedEntries=len(REAL_HOME_WATCH))


def emit(path):
    RECEIPT['elapsedSeconds'] = elapsed()
    data = (json.dumps(RECEIPT, indent=2, sort_keys=True) + '\n').encode()
    sys.stdout.buffer.write(data)
    sys.stdout.flush()
    if path:
        write_file(path, data, 0o600)


def main():
    parser = argparse.ArgumentParser(description='Native macOS Gentle Shell qualification')
    parser.add_argument('--gentle-ai', required=True, help='built candidate gentle-ai binary')
    parser.add_argument('--receipt', required=True, help='new file for the JSON evidence receipt')
    parser.add_argument('--source-sha', default='', help='candidate source commit, recorded only')
    arguments = parser.parse_args()
    receipt = os.path.abspath(arguments.receipt)
    RECEIPT['sourceSHA'] = arguments.source_sha
    RECEIPT['budgets'] = {'wholeRunSeconds': WHOLE_RUN_SECONDS, 'inspectSeconds': INSPECT_SECONDS,
                          'installSeconds': INSTALL_SECONDS, 'recoverSeconds': RECOVER_SECONDS, 'probeSeconds': PROBE_SECONDS}
    try:
        require(not os.path.lexists(receipt), 'receipt path already exists')
        host()
        qualify(prepare(arguments))
        RECEIPT['result'] = 'pass'
    except Exception as error:  # every failure becomes receipt evidence
        RECEIPT['failure'] = {'checkpoint': STATE['checkpoint'], 'errorType': type(error).__name__,
                              'reason': str(error)[:400], 'command': STATE['command']}
    if os.path.lexists(receipt):
        receipt = None
    emit(receipt)
    if RECEIPT['result'] != 'pass':
        sys.stderr.write('QUALIFICATION-FAIL checkpoint=%s\n' % RECEIPT['failure']['checkpoint'])
        return 1
    return 0


if __name__ == '__main__':
    sys.exit(main())
