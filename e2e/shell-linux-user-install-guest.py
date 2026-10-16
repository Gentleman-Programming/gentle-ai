#!/usr/bin/python3
"""Credentialless Guest ONLY. Direct evidence is not complete manager/update proof."""
import base64
import ctypes
import errno
import hashlib
import http.client
import http.server
import ipaddress
import json
import os
import pathlib
import re
import select
import shutil
import signal
import socket
import ssl
import stat
import struct
import subprocess
import sys
import threading
import time
import urllib.parse
import urllib.request
import fcntl
import termios
import yaml

START = time.monotonic()
CEILING = 1300
WORK = pathlib.Path('/work')
SUPERVISOR = '/fixture/supervisor'
TESTS = '/fixture/user-install.test'
FIXTURE = pathlib.Path('/fixture')
MODE = sys.argv[1] if len(sys.argv) == 2 else 'full'
REPORT = {}
COMMAND_FAILURE = None
FAULT = ''
FAULT_SEEN = threading.Event()
FAULT_RELEASE = threading.Event()
CACHE = {}
CACHE_BYTES = 0
CACHE_LOCK = threading.Lock()
REQUESTS = []
PUBLIC_IPS = {}
ALLOW = {'registry.npmjs.org', 'nodejs.org', 'github.com', 'release-assets.githubusercontent.com', 'pi.dev'}
NODE_SHA = '783130984963db7ba9cbd01089eaf2c2efb055c7c1693c943174b967b3050cb8'
PUBLIC_TLS = ssl.create_default_context()
FIXTURE_UID = 1002
# Native review integration creates and deletes this private index (and Git's
# .lock for it) beside Git control files; any leftover is residue, never state.
REVIEW_INDEX_PREFIX = '.gentle-ai-review-index-'


def require(condition, message):
    if not condition:
        raise RuntimeError(message)


def pnpm_root_receipt(text, name, version, sri):
    """Parse stock lock-v9 roles with the OS YAML SDK; refuse duplicate keys/aliases."""
    class ReceiptLoader(yaml.SafeLoader):
        def compose_node(self, parent, index):
            if self.check_event(yaml.AliasEvent):
                raise ValueError('receipt aliases are not stock root evidence')
            return super().compose_node(parent, index)

        def construct_mapping(self, node, deep=False):
            seen = set()
            for key, _ in node.value:
                value = self.construct_object(key, deep=deep)
                if value in seen:
                    raise ValueError('duplicate receipt key')
                seen.add(value)
            return super().construct_mapping(node, deep=deep)

    try:
        lock = yaml.load(text, Loader=ReceiptLoader)
        imported = lock['importers']['.']['dependencies'][name]
        package = lock['packages'][name + '@' + version]
        selected = imported['version']
        return (str(lock['lockfileVersion']) in ['9', '9.0']
                and imported['specifier'] in [version, '^' + version, '~' + version]
                and isinstance(selected, str)
                and re.fullmatch(re.escape(version) + r'(?:\([^\s]*\))*', selected) is not None
                and package['resolution']['integrity'] == sri)
    except (yaml.YAMLError, ValueError, TypeError, KeyError):
        return False


def remaining():
    value = CEILING - (time.monotonic() - START)
    require(value > 0, 'whole Guest deadline')
    return value


def kernel():
    require(os.getuid() == 1002 and os.uname().machine == 'x86_64', 'real UID1002/linuxamd64 required')
    status = pathlib.Path('/proc/self/status').read_text()
    for key in ['CapInh', 'CapPrm', 'CapEff', 'CapAmb', 'CapBnd']:
        require(f'{key}:\t0000000000000000' in status, 'capability present')
    require('NoNewPrivs:\t1' in status, 'NoNewPrivs missing')
    membership = pathlib.Path('/proc/self/cgroup').read_text()
    require(membership.startswith('0::/'), 'unified actual membership required')
    relative = membership.strip()[3:]
    group = pathlib.Path('/sys/fs/cgroup' + (relative if relative != '/' else ''))
    require(str(group.resolve()) == str(group), 'noncanonical cgroup mapping')
    expected = {'memory.max': '3221225472', 'memory.swap.max': '0', 'cpu.max': '100000 100000', 'pids.max': '64'}
    for name, value in expected.items():
        require((group / name).read_text().strip() == value, 'physical cgroup limit differs')
    require(' - cgroup2 ' in pathlib.Path('/proc/self/mountinfo').read_text(), 'cgroup2 mount missing')
    require(os.statvfs('/').f_flag & os.ST_RDONLY, 'physical Guest root mount must be read-only')
    require(pathlib.Path('/proc/sys/net/ipv4/ip_unprivileged_port_start').read_text().strip() == '0', 'nonprivileged TLS fixture port unavailable')
    REPORT['kernel'] = {'uid': os.getuid(), 'limits': expected}


def run(args, cwd=None, extra=None, timeout=180, good=True, stdout_only=False):
    global COMMAND_FAILURE
    env = {'PATH': '/usr/local/bin:/usr/bin:/bin:/work/personal/prefix/bin', 'HOME': str(WORK / 'home'), 'TMPDIR': str(WORK / 'tmp'),
           'PYTHONDONTWRITEBYTECODE': '1', 'TERM': 'xterm-256color'}
    env.update(extra or {})
    names = {pathlib.Path(arg).name for arg in args[:2]}
    stock = next((name for name in ['pnpm', 'pnpm.mjs', 'npm-cli.js', 'pi', 'cli.js', 'provision.mjs', 'git'] if name in names), 'stock')
    identity = {
        'kind': 'tests' if args[0] == TESTS else 'supervisor' if args[0] == SUPERVISOR else stock,
        'operation': next((op for op in ['check', 'install', 'recover', 'internal-install', 'internal-run', 'internal-verify', 'ci', 'add', 'list', 'update', 'bin', '--version', 'prepare-prior'] if op in args), 'stock'),
        'inspect': '--inspect' in args, 'force': '--force' in args, 'lockOnly': '--package-lock-only' in args,
    }
    limit = min(timeout, remaining())
    child = subprocess.Popen(args, cwd=cwd or WORK, env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True)
    try:
        out, error = child.communicate(timeout=limit)
    except subprocess.TimeoutExpired:
        os.killpg(child.pid, signal.SIGKILL)
        out, error = child.communicate(timeout=3)
        raw = out + error
        # Identify the killed command and when it died; only a bounded tail of
        # its own output is disclosed, never its environment or arguments.
        COMMAND_FAILURE = {**identity, 'timeout': True, 'limitSeconds': round(limit), 'elapsedSeconds': round(time.monotonic() - START),
                           'wholeBudget': limit < timeout, 'rawBytes': len(raw), 'rawSHA256': hashlib.sha256(raw).hexdigest()}
        if b'\0' not in raw:
            COMMAND_FAILURE.update(stdoutTailBase64=base64.b64encode(out[-600:]).decode('ascii'),
                                   stderrTailBase64=base64.b64encode(error[-600:]).decode('ascii'))
        raise RuntimeError('owned command deadline; bounded output tail only')
    raw = out + error
    evidence = {**identity, 'exit': child.returncode, 'expectedSuccess': good, 'rawBytes': len(raw), 'rawSHA256': hashlib.sha256(raw).hexdigest()}
    if len(raw) >= 4096 or b'\0' in raw:
        COMMAND_FAILURE = {**evidence, 'nulBytes': raw.count(b'\0'),
                           'stdoutBytes': len(out), 'stdoutSHA256': hashlib.sha256(out).hexdigest(),
                           'stderrBytes': len(error), 'stderrSHA256': hashlib.sha256(error).hexdigest()}
        raise RuntimeError('whole raw command output withheld: byte/NUL bound')
    text = raw.decode('utf-8', 'strict')
    if (child.returncode == 0) != good:
        COMMAND_FAILURE = {**evidence, 'stdoutBase64': base64.b64encode(out).decode('ascii'),
                           'stderrBase64': base64.b64encode(error).decode('ascii')}
        raise RuntimeError('command outcome differs; bounded complete failure evidence')
    return out.decode('utf-8', 'strict') if stdout_only else text


def physical_inventory(root, details=None):
    info = root.lstat()
    result = [('.', 'metadata', info.st_mode, info.st_uid, info.st_gid, info.st_dev, info.st_ino, info.st_size, info.st_mtime_ns, info.st_ctime_ns)]
    for parent, directories, files in os.walk(root, followlinks=False):
        for name in sorted(directories + files):
            p = pathlib.Path(parent) / name
            info = p.lstat()
            require(info.st_uid == FIXTURE_UID, 'foreign fixture object')
            relative = str(p.relative_to(root))
            result.append((relative, 'metadata', info.st_mode, info.st_uid, info.st_gid, info.st_dev, info.st_ino, info.st_size, info.st_mtime_ns, info.st_ctime_ns))
            if stat.S_ISLNK(info.st_mode):
                result.append((relative, 'link', os.readlink(p)))
            elif stat.S_ISREG(info.st_mode):
                result.append((relative, 'file', hashlib.sha256(p.read_bytes()).hexdigest()))
            else:
                require(stat.S_ISDIR(info.st_mode), 'nonphysical fixture')
    if details is not None:
        details.extend(result)
    return hashlib.sha256(json.dumps(sorted(result)).encode()).hexdigest()


def project_preservation_violations(before_entries, after_entries):
    """Compare physical_inventory rows; empty means the caller project is preserved.

    The only tolerated difference is mtime_ns/ctime_ns of the root .git
    directory, written by the native review index cycle. Paths, bytes, links
    and every other metadata field stay compared, everywhere. Review-index or
    lock residue refuses even when it is unchanged across the launch.
    """
    before = {(row[0], row[1]): row[2:] for row in before_entries}
    after = {(row[0], row[1]): row[2:] for row in after_entries}
    violations = []
    for key in sorted(set(before) | set(after)):
        if pathlib.PurePosixPath(key[0]).name.startswith(REVIEW_INDEX_PREFIX):
            violations.append((key[0], key[1], 'review-index residue'))
            continue
        old, new = before.get(key), after.get(key)
        if old == new:
            continue
        if old is None or new is None:
            violations.append((key[0], key[1], 'created' if old is None else 'removed'))
            continue
        # Row tail: mode, uid, gid, dev, inode, size, mtime_ns, ctime_ns.
        git_times = key == ('.git', 'metadata') and stat.S_ISDIR(old[0]) and old[:6] == new[:6]
        if not git_times:
            violations.append((key[0], key[1], 'modified'))
    return violations


def upstream_ip(host):
    require(host in ALLOW, 'unapproved upstream')
    if host not in PUBLIC_IPS:
        url = 'https://1.1.1.1/dns-query?' + urllib.parse.urlencode({'name': host, 'type': 'A'})
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
        with opener.open(urllib.request.Request(url, headers={'accept': 'application/dns-json'}), timeout=10) as response:
            raw = response.read(8193)
        require(len(raw) <= 8192, 'DNS DATA bound')
        answers = json.loads(raw)['Answer']
        candidates = [entry['data'] for entry in answers if entry['type'] == 1]
        require(candidates, 'no public upstream IPv4')
        PUBLIC_IPS[host] = str(ipaddress.IPv4Address(candidates[0]))
    return PUBLIC_IPS[host]


class DirectTLS(http.client.HTTPSConnection):
    def connect(self):
        sock = socket.create_connection((upstream_ip(self.host), 443), timeout=10)
        self.sock = PUBLIC_TLS.wrap_socket(sock, server_hostname=self.host)


def public_bytes(host, resource):
    global CACHE_BYTES
    key = (host, resource)
    with CACHE_LOCK:
        if key in CACHE:
            return CACHE[key]
    connection = DirectTLS(host, timeout=10)
    try:
        connection.request('GET', resource, headers={'accept-encoding': 'identity', 'user-agent': 'Gentle-Guest-qualification'})
        response = connection.getresponse()
        raw = response.read(67108865)
        require(len(raw) <= 67108864, 'upstream body bound')
        headers = {name.lower(): value for name, value in response.getheaders() if name.lower() in {'content-type', 'location'}}
        if 'location' in headers:
            redirect = urllib.parse.urlsplit(headers['location'])
            require(redirect.scheme == 'https' and redirect.hostname in ALLOW, 'unapproved release redirect')
        with CACHE_LOCK:
            if key not in CACHE:
                require(len(CACHE) < 4096 and CACHE_BYTES + len(raw) <= 536870912, 'upstream aggregate cache bound')
                CACHE[key] = (response.status, headers, raw)
                CACHE_BYTES += len(raw)
            return CACHE[key]
    finally:
        connection.close()


class Mirror(http.server.BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass  # Never expose signed release URLs or arbitrary stock diagnostics.

    def do_GET(self):
        try:
            host = self.headers.get('Host', '').split(':')[0].lower()
            require(host in ALLOW and len(self.path) <= 4096, 'fixture request origin/path')
            require(len(REQUESTS) < 4096, 'fixture request observation bound')
            REQUESTS.append((host, hashlib.sha256(self.path.encode()).hexdigest()))
            if host == 'nodejs.org':
                require(self.path == '/dist/v24.18.0/node-v24.18.0-linux-x64.tar.gz', 'Node path')
                raw = (FIXTURE / 'node.tgz').read_bytes()
                require(len(raw) == 57224421 and hashlib.sha256(raw).hexdigest() == NODE_SHA, 'independent fixture Node pin')
                if FAULT in {'node-cancel', 'node-cleanup'}:
                    FAULT_SEEN.set()
                    require(FAULT_RELEASE.wait(timeout=min(20, remaining())), 'fault barrier deadline')
                if FAULT in {'node-sri', 'node-cleanup'}:
                    raw = raw[:-1] + bytes([raw[-1] ^ 1])
                status, headers = 200, {}
            elif FAULT == 'tool-refuse' and host == 'github.com' and self.path.startswith('/sharkdp/fd/releases/download/'):
                FAULT_SEEN.set()
                status, headers, raw = 404, {}, b''
            elif host == 'pi.dev' and self.path == '/api/latest-version':
                status, headers, raw = 200, {'content-type': 'application/json'}, b'{"version":"1.0.0"}'
            else:
                status, headers, raw = public_bytes(host, self.path)
            self.send_response(status)
            for name, value in headers.items():
                self.send_header(name, value)
            self.send_header('Content-Length', str(len(raw)))
            self.end_headers()
            self.wfile.write(raw)
        except Exception:
            self.send_error(502, 'bounded fixture refusal')


class BoundedServer(http.server.ThreadingHTTPServer):
    slots = threading.BoundedSemaphore(4)
    daemon_threads = True

    def process_request(self, request, address):
        if not self.slots.acquire(timeout=10):
            self.shutdown_request(request)
            return
        super().process_request(request, address)

    def handle_error(self, request, address):
        pass  # Raw unbounded socket tracebacks are not qualification evidence.

    def process_request_thread(self, request, address):
        try:
            super().process_request_thread(request, address)
        finally:
            self.slots.release()


def tls_fixture():
    server = BoundedServer(('127.0.0.1', 443), Mirror)
    context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    context.load_cert_chain(FIXTURE / 'server.crt', FIXTURE / 'server.key')
    server.socket = context.wrap_socket(server.socket, server_side=True)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    return server


def inspect(target, shared=None):
    args = [SUPERVISOR, 'shell', 'install', '--target', str(target), '--mode', 'shared' if shared else 'separate']
    if shared:
        args += ['--prefix', str(shared[0]), '--agent', str(shared[1])]
    response = run(args + ['--inspect'])
    line = next(line for line in response.splitlines() if line.startswith('Confirmation: '))
    token = line.split(': ', 1)[1]
    require(len(token) == 64 and all(c in '0123456789abcdef' for c in token), 'consent token')
    return args, token


def install(target, shared=None):
    args, token = inspect(target, shared)
    result = run(args + ['--confirm', token], timeout=remaining())
    require('Installed ' in result, 'installation receipt missing')
    manifest = json.loads((target / 'installation.json').read_text())
    require(manifest['Destination'] == str(target), 'manifest destination')
    REPORT.setdefault('installs', []).append({'mode': manifest['Mode'], 'manifestSHA': hashlib.sha256((target / 'installation.json').read_bytes()).hexdigest()})
    return manifest


def acquisition_fault(target, cleanup=False):
    global FAULT
    args, token = inspect(target)
    FAULT_SEEN.clear()
    FAULT_RELEASE.clear()
    FAULT = 'node-cleanup' if cleanup else 'node-cancel'
    env = {'PATH': '/usr/local/bin:/usr/bin:/bin', 'HOME': str(WORK / 'home'), 'TMPDIR': str(WORK / 'tmp')}
    child = subprocess.Popen(args + ['--confirm', token], env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True)
    stage = None
    try:
        require(FAULT_SEEN.wait(timeout=min(20, remaining())), 'actual cold acquisition not observed')
        stages = list(target.parent.glob('.gentle-user-*'))
        require(len(stages) == 1 and stages[0].is_dir() and stages[0].lstat().st_uid == 1002, 'owned acquisition stage differs')
        stage = stages[0]
        if cleanup:
            os.chmod(stage, 0o500)  # Real owned-filesystem cleanup refusal, not a mocked return.
        else:
            child.send_signal(signal.SIGINT)
        FAULT_RELEASE.set()
        out, error = child.communicate(timeout=min(40, remaining()))
        raw = out + error
        require(len(raw) < 4096 and b'\0' not in raw, 'fault command raw output withheld')
        text = raw.decode('utf-8', 'strict')
        require(child.returncode != 0 and not target.exists(), 'fault published or reported success')
        if cleanup:
            require(stage.exists() and str(stage) in text and 'uncertain' in text, 'cleanup denial lost owned evidence locator')
            REPORT['cleanupDenial'] = 'actual changed stage permissions; uncertain evidence retained'
        else:
            require(not stage.exists(), 'canceled acquisition stage not cleaned after worker reap')
            REPORT['acquisitionCancel'] = 'actual in-flight request interrupted, worker waited and unpublished stage removed'
    finally:
        FAULT_RELEASE.set()
        FAULT = ''
        if child.poll() is None:
            child.send_signal(signal.SIGINT)
            try:
                child.wait(timeout=3)
            except subprocess.TimeoutExpired:
                os.killpg(child.pid, signal.SIGKILL)
                child.wait(timeout=3)
        if stage is not None and stage.exists():
            os.chmod(stage, 0o700)  # Only this disposable, physically identified Guest-owned stage.


def tool_acquisition_fault(target, shared):
    global FAULT
    before = [physical_inventory(path) for path in shared]
    stages = set(target.parent.glob('.gentle-user-*'))  # Earlier fault evidence stays retained.
    args, token = inspect(target, shared)
    FAULT_SEEN.clear()
    FAULT = 'tool-refuse'
    try:
        text = run(args + ['--confirm', token], timeout=remaining(), good=False)
    finally:
        FAULT = ''
    require(FAULT_SEEN.is_set(), 'actual pinned tool acquisition not observed')
    require('uncertain' not in text and not target.exists() and set(target.parent.glob('.gentle-user-*')) == stages, 'pre-provisioning tool refusal left uncertain evidence')
    require([physical_inventory(path) for path in shared] == before, 'tool acquisition failure changed selected Shared prefix or agent')
    REPORT['toolAcquisitionOrder'] = 'actual pinned fd refusal before Shared provisioning; selected prefix and agent unchanged'


def publication_fault(target, shared):
    args, token = inspect(target, shared)
    changed, stop = threading.Event(), threading.Event()
    node = target / 'runtime/node/bin/node'
    def alter_published_node():
        while not stop.wait(0.002):
            if node.exists():
                info = node.lstat()
                if stat.S_ISREG(info.st_mode) and info.st_uid == 1002:
                    os.chmod(node, 0o600)
                    changed.set()
                return
    observer = threading.Thread(target=alter_published_node, daemon=True)
    observer.start()
    try:
        text = run(args + ['--confirm', token], timeout=remaining(), good=False)
        require(changed.is_set() and target.exists() and str(target) in text and 'uncertain' in text, 'actual post-publication uncertainty not retained')
        REPORT['publicationUncertainty'] = 'actual published Node mode changed during final readback; destination and locator retained'
    finally:
        stop.set()
        observer.join(timeout=3)
        if changed.is_set():
            os.chmod(node, 0o700)


def project_watch(project):
    """Read-only kernel notifications; names/masks, never file contents."""
    libc = ctypes.CDLL(None, use_errno=True)
    libc.inotify_init1.argtypes, libc.inotify_init1.restype = [ctypes.c_int], ctypes.c_int
    libc.inotify_add_watch.argtypes, libc.inotify_add_watch.restype = [ctypes.c_int, ctypes.c_char_p, ctypes.c_uint32], ctypes.c_int
    fd = libc.inotify_init1(os.O_NONBLOCK | os.O_CLOEXEC)
    require(fd >= 0, 'kernel project observer unavailable')
    if libc.inotify_add_watch(fd, os.fsencode(project / '.git'), 0x000003CA) < 0:
        error = ctypes.get_errno()
        os.close(fd)
        if error == errno.ENOENT:
            return None  # Installer cases precede Git initialization; create nothing.
        raise RuntimeError('kernel Git observer unavailable')
    return fd


def project_events(fd):
    """One bounded partial queue read, not a complete mutation receipt."""
    if fd is None:
        return {'partial': True, 'available': False}
    try:
        data = os.read(fd, 4096)
    except BlockingIOError:
        data = b''
    events, offset = set(), 0
    while offset + 16 <= len(data):
        _, mask, _, size = struct.unpack_from('iIII', data, offset)
        require(offset + 16 + size <= len(data), 'kernel observer record truncated')
        name = data[offset + 16:offset + 16 + size].split(b'\0', 1)[0]
        label = name.decode('ascii') if re.fullmatch(rb'[A-Za-z0-9._-]{1,64}', name) else 'sha256:' + hashlib.sha256(name).hexdigest()
        events.add((mask, label))
        offset += 16 + size
    return {'partial': True, 'queueBytes': len(data), 'observedUnique': len(events), 'firstEight': sorted(events)[:8]}


def pty_status(binding, project, command=None, extra=None, installer=None, cancel_installer=False, opening_only=False):
    project_before_entries = []
    project_before, requests_before = physical_inventory(project, project_before_entries), len(REQUESTS)
    master, slave = os.openpty()
    # Stock /gentle:status reports routing for every installed agent. Give its
    # actual notification room; a short viewport can omit the required header.
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH', 80, 72, 0, 0))

    def terminal():
        os.setsid()
        fcntl.ioctl(0, termios.TIOCSCTTY, 0)

    env = {'PATH': '/usr/local/bin:/usr/bin:/bin:/work/personal/prefix/bin', 'HOME': str(WORK / 'home'), 'TMPDIR': str(WORK / 'tmp'), 'TERM': 'xterm-256color'}
    env.update(extra or {})
    selected = command or [str(binding)]
    require(all("'" not in value and '\n' not in value for value in selected), 'unsafe PTY command selection')
    invocation = ' '.join("'" + value + "'" for value in selected)
    script = invocation + "; result=$?; printf '\\nGUEST-STATUS:%d\\n' \"$result\"; read -r finish; exit \"$result\""
    observer = None
    try:
        observer = project_watch(project)
        child = subprocess.Popen(['/bin/bash', '--noprofile', '--norc', '-m', '-c', script], cwd=project, env=env, stdin=slave, stdout=slave, stderr=slave, preexec_fn=terminal)
    except BaseException:
        if observer is not None:
            os.close(observer)
        os.close(master)
        os.close(slave)
        raise
    os.close(slave)
    raw = bytearray()
    deadline = time.monotonic() + min(45, remaining())
    sent = False
    typed = False
    expected = b'type, or / for commands' if opening_only else b'el Gentleman package is active.'
    confirmed = False
    stopped = False
    observed_cwd = ''
    try:
        while time.monotonic() < deadline:
            ready, _, _ = select.select([master], [], [], 0.1)
            if ready:
                try:
                    part = os.read(master, 4096)
                except OSError:
                    break
                if not part:
                    break
                raw.extend(part)
                require(len(raw) <= 65536, 'whole PTY capture bound')
                # Credentialless Pi emits this fallback after installing its editor submit handler.
                input_ready = b'No models available.' in raw if installer is None else b'Gentle Shell Linux user installer' in raw
                if not typed and input_ready and not opening_only:
                    if installer is not None:
                        os.write(master, b'\x1b' if cancel_installer else (str(installer) + '\r').encode())
                        confirmed = cancel_installer
                        sent = True
                    else:
                        command_text = b'/gentle:status'
                        require(os.write(master, command_text) == len(command_text), 'incomplete PTY command typing')
                    typed = True
                if installer is None and typed and not sent and b'/gentle:status' in raw:
                    require(os.write(master, b'\r') == 1, 'incomplete PTY command submission')
                    sent = True
                if installer is not None and not confirmed and b'Confirm this physical selection' in raw:
                    os.write(master, b'y')
                    confirmed = True
                if installer is None and expected in raw and (input_ready or not opening_only) and not stopped:
                    foreground = os.tcgetpgrp(master)
                    observed_cwd = os.readlink(f'/proc/{foreground}/cwd')
                    require(observed_cwd == str(project), 'actual foreground caller CWD differs')
                    if opening_only:
                        require(os.readlink(f'/proc/{foreground}/exe') == str(binding.parent.parent / 'runtime/node/bin/node'), 'actual foreground private Node differs')
                        require(b'Failed to load extension' not in raw, 'Gentle startup extension failure')
                    stop_key = b'\x04' if opening_only else b'\x03\x03'
                    require(os.write(master, stop_key) == len(stop_key), 'incomplete PTY exit input')
                    stopped = True
                if b'GUEST-STATUS:' in raw:
                    require(b'GUEST-STATUS:0' in raw, 'interactive child failed; evidence retained')
                    require(os.tcgetpgrp(master) == child.pid, 'real caller terminal foreground not restored')
                    os.write(master, b'finish\n')
                    break
        if installer is None:
            require(expected in raw, 'actual Gentle UI opening not observed' if opening_only else 'actual Gentle command registration not observed')
            if opening_only:
                require(stopped and b'GUEST-STATUS:0' in raw, 'Gentle opening did not settle successfully')
            require(b'Gentle Shell Linux user installer' not in raw, 'ordinary launch opened installer')
        else:
            require(confirmed and b'GUEST-STATUS:0' in raw, 'installer TUI did not settle successfully')
            REPORT['installerCancel' if cancel_installer else 'installerTUI'] = 'actual PTY idle cancellation' if cancel_installer else 'actual PTY physical review, typed confirmation and settled idempotent install'
        child.wait(timeout=5)
        project_after_entries = []
        project_after = physical_inventory(project, project_after_entries)
        violations = project_preservation_violations(project_before_entries, project_after_entries)
        if violations:
            before = {(row[0], row[1]): row[2:] for row in project_before_entries}
            after = {(row[0], row[1]): row[2:] for row in project_after_entries}
            changed = [key for key in sorted(set(before) | set(after)) if before.get(key) != after.get(key)]
            delta = [{'path': key[0], 'kind': key[1], 'before': before.get(key), 'after': after.get(key)} for key in changed]
            data = json.dumps(delta, sort_keys=True, separators=(',', ':')).encode()
            entries = [{'path': key[0], 'kind': key[1], 'change': 'created' if key not in before else 'removed' if key not in after else 'modified'} for key in changed]
            names = json.dumps(entries, sort_keys=True, separators=(',', ':')).encode()
            REPORT['projectMutation'] = {'notifications': project_events(observer), 'changedRecords': len(delta), 'bytes': len(data), 'sha256': hashlib.sha256(data).hexdigest(),
                                         'metadataFields': ['mode', 'uid', 'gid', 'dev', 'inode', 'size', 'mtime_ns', 'ctime_ns'],
                                         'wholeDelta': delta if len(data) <= 1024 else 'withheld',
                                         'entriesBytes': len(names), 'entriesSHA256': hashlib.sha256(names).hexdigest(),
                                         'wholeEntries': entries if len(names) <= 1024 else 'withheld',
                                         'violationCount': len(violations), 'violationReasons': sorted({row[2] for row in violations})}
        elif project_after != project_before:
            REPORT['projectGitTimesOnly'] = REPORT.get('projectGitTimesOnly', 0) + 1
        require(not violations, 'fixture blank caller project changed during launch')
        if command is None:
            requests = REQUESTS[requests_before:]
            observed = sorted(set(requests))
            origins = {host for host, _ in observed}
            if not origins <= {'pi.dev'}:
                data = json.dumps(observed, separators=(',', ':')).encode()
                REPORT['ordinaryRequestFailure'] = {'requestCount': len(requests), 'uniqueCount': len(observed),
                                                    'fields': ['origin', 'requestTargetSHA256'], 'bytes': len(data),
                                                    'sha256': hashlib.sha256(data).hexdigest(),
                                                    'wholeUnique': observed if len(data) <= 1024 else 'withheld'}
            require(origins <= {'pi.dev'}, 'ordinary launch requested installer/package artifacts')
            REPORT['ordinaryStartup'] = 'blank caller project preserved; fixture-origin installer/package requests absent (not whole-network attestation)'
        REPORT.setdefault('pty', []).append({'binding': binding.name, 'rawBytes': len(raw), 'rawSHA256': hashlib.sha256(raw).hexdigest(), 'nulBytes': raw.count(0), 'GentleRegistered': installer is None and not opening_only, 'GentleUIOpened': opening_only, 'foregroundCWD': observed_cwd, 'callerForegroundRestored': True})
    except Exception:
        if installer is None and command is None:
            snapshot = bytes(raw)
            try:
                snapshot.decode('utf-8', 'strict')
                valid_utf8 = True
            except UnicodeError:
                valid_utf8 = False
            try:
                foreground = os.tcgetpgrp(master)
                lflags = termios.tcgetattr(master)[3]
                with open(f'/proc/{foreground}/stat', 'rb') as stream:
                    process_stat = stream.read(4097)
                require(len(process_stat) <= 4096, 'foreground metadata bound')
                REPORT['ptyKernel'] = {
                    'node': os.readlink(f'/proc/{foreground}/exe') == str(binding.parent.parent / 'runtime/node/bin/node'),
                    'project': os.readlink(f'/proc/{foreground}/cwd') == str(project),
                    'canonical': bool(lflags & termios.ICANON), 'echo': bool(lflags & termios.ECHO),
                    'stopped': process_stat.rpartition(b') ')[2][:1] in (b'T', b't'),
                }
                # Linux TIOCGPTPEER opens this master's slave without a pathname or controlling-tty change.
                peer = fcntl.ioctl(master, 0x5441, os.O_RDONLY | os.O_NOCTTY | os.O_NONBLOCK)
                try:
                    peer_identity = os.fstat(peer)
                    input_identity = os.stat(f'/proc/{foreground}/fd/0')
                    same_terminal = stat.S_ISCHR(input_identity.st_mode) and (input_identity.st_dev, input_identity.st_ino) == (peer_identity.st_dev, peer_identity.st_ino)
                    queued_input = struct.unpack('i', fcntl.ioctl(peer, termios.FIONREAD, b'\0' * 4))[0] > 0
                finally:
                    os.close(peer)
                REPORT['ptyKernel'].update(stdinSameTTY=same_terminal, inputQueued=queued_input)
            except (OSError, termios.error):
                REPORT['ptyKernel'] = {'unavailable': True}
            REPORT['ptyFailure'] = {
                'kernel': REPORT['ptyKernel'],
                'binding': binding.name, 'observation': 'buffer at failed check, not complete process stream',
                # A bounded encoded tail distinguishes editor/command failures;
                # it is neither a complete stream nor a startup/registration proof.
                'tailBytes': min(len(snapshot), 1024),
                'tailBase64': base64.b64encode(snapshot[-1024:]).decode('ascii'),
                'bytes': len(snapshot), 'sha256': hashlib.sha256(snapshot).hexdigest(),
                'nulBytes': snapshot.count(0), 'strictUTF8': valid_utf8,
                'typingSent': typed, 'commandSent': sent, 'registrationObserved': b'el Gentleman package is active.' in snapshot,
                'extensionLoadError': b'Failed to load extension' in snapshot,
                'noModelsMessage': b'No models available.' in snapshot,
                'startupBusyMessage': b'Startup is still in progress' in snapshot,
                'noModelSelectedMessage': b'No model selected.' in snapshot,
                'unknownCommandMessage': b'Unknown command:' in snapshot,
                'commandEchoObserved': b'/gentle:status' in snapshot,
                'gentlePrefixObserved': b'el Gentleman' in snapshot,
                'activeSuffixObserved': b'package is active.' in snapshot,
                'installerObserved': b'Gentle Shell Linux user installer' in snapshot,
                'openingObserved': opening_only and expected in snapshot, 'exitInputSent': stopped,
                'statusObserved': b'GUEST-STATUS:' in snapshot, 'exit': child.poll(),
                'deadlineExpired': time.monotonic() >= deadline, 'foregroundCWD': observed_cwd,
            }
            if valid_utf8 and b'\0' not in snapshot and len(snapshot) < 4096:
                REPORT['ptyFailureBuffer'] = base64.b64encode(snapshot).decode('ascii')
        raise
    finally:
        if child.poll() is None:
            os.killpg(child.pid, signal.SIGKILL)
            child.wait(timeout=3)
        if observer is not None:
            os.close(observer)
        os.close(master)


def alan_pnpm_backend_probe():
    """Reuse Alan's stock pnpm backend and prove Pi's own updater, not an adapter."""
    root = WORK / 'reuse-pnpm-backend'
    root.mkdir(mode=0o700)
    for name in ['runtime', 'home', 'tmp', 'config', 'agent', 'tooling', 'pnpm']:
        (root / name).mkdir(mode=0o700)
    for name in ['user.npmrc', 'global.npmrc']:
        (root / 'config' / name).write_bytes(b'')
    run(['/bin/sh', '/work/src/scripts/bootstrap-gentle-shell-private-node.sh', '--destination', str(root / 'runtime/node'), '--node-archive', str(FIXTURE / 'node.tgz')], timeout=180)
    node = root / 'runtime/node/bin/node'
    npm = root / 'runtime/node/lib/node_modules/npm/bin/npm-cli.js'
    tool = root / 'tooling'
    version = '11.1.1'  # Alan's installer-downloads.mjs at 5ed499ade6c93733990e0298052df477428a90be.
    integrity = 'sha512-0f319zxhe2T6GlaoHDyN/g6WbjOmAQqiVrUXrne+Idk+Ba/8DeGoOw5PKdVp9otEaujwaM1yR8C7PfD7TXvfmg=='
    (tool / 'package.json').write_text(json.dumps({'name': 'gentle-owned-pnpm-probe', 'version': '1.0.0', 'private': True, 'dependencies': {'pnpm': version}}))
    env = {'HOME': str(root / 'home'), 'TMPDIR': str(root / 'tmp'), 'PATH': str(node.parent) + ':/usr/bin:/bin',
           'NODE_USE_SYSTEM_CA': '1', 'NPM_CONFIG_USERCONFIG': str(root / 'config/user.npmrc'),
           'NPM_CONFIG_GLOBALCONFIG': str(root / 'config/global.npmrc'), 'NPM_CONFIG_CACHE': str(root / 'tool-cache'),
           'NPM_CONFIG_PREFIX': str(tool), 'NPM_CONFIG_IGNORE_SCRIPTS': 'true', 'npm_config_ignore_scripts': 'true'}
    flags = ['--ignore-scripts', '--engine-strict', '--no-audit', '--no-fund', '--min-release-age=0', '--registry=https://registry.npmjs.org/', '--loglevel=error']
    run([str(node), str(npm), 'install', '--package-lock-only'] + flags, cwd=tool, extra=env)
    lock_bytes = (tool / 'package-lock.json').read_bytes()
    require(len(lock_bytes) <= 33554432, 'pnpm probe lock bound')
    lock = json.loads(lock_bytes)
    entry = lock.get('packages', {}).get('node_modules/pnpm', {})
    require(lock.get('lockfileVersion') == 3 and lock['packages']['']['dependencies'] == {'pnpm': version}, 'pnpm probe seed differs')
    require(entry.get('version') == version and entry.get('integrity') == integrity and entry.get('resolved') == 'https://registry.npmjs.org/pnpm/-/pnpm-11.1.1.tgz', 'independent pnpm probe pin differs')
    require(all(key in {'', 'node_modules/pnpm'} or (key.startswith('node_modules/pnpm/node_modules/') and record.get('inBundle') is True) for key, record in lock['packages'].items()), 'pnpm probe has unpinned external dependencies')
    run([str(node), str(npm), 'ci'] + flags, cwd=tool, extra=env)
    require((tool / 'package-lock.json').read_bytes() == lock_bytes, 'pnpm probe lock changed')
    package = tool / 'node_modules/pnpm'
    metadata = json.loads((package / 'package.json').read_bytes())
    require(metadata['name'] == 'pnpm' and metadata['version'] == version and metadata['engines']['node'] == '>=22.13', 'pnpm probe package identity/engine')
    cli = package / 'bin/pnpm.mjs'
    require(run([str(node), str(cli), '--version'], extra=env).strip() == version, 'acquired stock pnpm did not execute')
    home = root / 'pnpm'
    binaries = home / 'bin'
    binaries.mkdir(mode=0o700)
    wrapper = binaries / 'pnpm'
    wrapper.write_text(f'#!/bin/sh\nexec \'{node}\' \'{cli}\' --reporter=silent "$@"\n')
    os.chmod(wrapper, 0o700)  # A stock program binding, not an updater implementation.
    env.pop('NPM_CONFIG_PREFIX')
    env.update(PNPM_HOME=str(home), PATH=str(binaries) + ':' + env['PATH'], PI_CODING_AGENT_DIR=str(root / 'agent'),
               npm_config_store_dir=str(root / 'store'), npm_config_engine_strict='true', npm_config_reporter='silent')
    require(run([str(wrapper), 'bin', '-g'], extra=env).strip() == str(binaries), 'pnpm global-bin selection differs')
    pins = {
        'gentle-pi': ('4.0.0', 'sha512-ZG/diWBSKPfjU4MjiHVUXxHWQvDSRS2dvpPCma4ZINuV+8UGdZAPHca6vcK27FLAAG7crlJpwdWOcwVNW4rh/Q=='),
        '@earendil-works/pi-coding-agent': ('0.99.2', 'sha512-6R1BZ2N77CrVcGf3eC2KovTz1Q4RYiAeydvVWQT546N2fi1nBc81aURlbOZCgruWoW9VY/UrLzDynF4YTolpoA=='),
        '@earendil-works/pi-tui': ('1.0.0', 'sha512-JsT7kXnpZA2YOtQu6RyriyxEO0eJIzPyfiH09bH+OLN5+s18HYkwaUD/tBkjhnSfMu6/50CQPRYJagzSP6HdPw=='),
        '@heyhuynhgiabuu/pi-pretty': ('0.6.27', 'sha512-4Jj+n6ZBFdn979fWAA3nMcJ45Q5qtcLeq1Pe6+Oo2LDIpDhqv7heoTKkBpq9G74/pNYc0EaXR28pssQ5Wbc5bg=='),
        'typebox': ('1.3.27', 'sha512-zu+jc1pcy4UiNThxikUr36f0Rybk9PEeCg/NE6adeWr/SKsdNO4EzZHYRDlv2YCVAfj3Odq3dESSo/jNyoBXzA=='),
    }
    run([str(wrapper), 'add', '-g', '--ignore-scripts', '--config.minimumReleaseAge=0', '--registry=https://registry.npmjs.org/'] + [f'{name}@{pin[0]}' for name, pin in pins.items()], extra=env, timeout=remaining())

    def cohort():
        projects = json.loads(run([str(wrapper), 'list', '-g', '--depth', '0', '--json'], extra=env, stdout_only=True))
        require(isinstance(projects, list), 'pnpm project listing shape')
        owners = [project for project in projects if 'gentle-pi' in project.get('dependencies', {})]
        require(len(owners) == 1 and set(owners[0]['dependencies']) == set(pins), 'pnpm stack owner is ambiguous')
        project = pathlib.Path(owners[0]['path']).resolve(strict=True)
        require(project.is_relative_to(home) and project.stat().st_uid == 1002, 'pnpm project escapes selected home')
        def lock_text(project):
            require(project.is_relative_to(home) and project.stat().st_uid == 1002, 'pnpm root project escapes selected home')
            candidates = [project / 'pnpm-lock.yaml', project / 'node_modules/.pnpm/lock.yaml']
            locks = [item for item in candidates if item.is_file() and not item.is_symlink()]
            if not locks or any(os.path.lexists(item) and item not in locks for item in candidates):
                def relative(item):
                    return str(item.relative_to(root)) if item.is_relative_to(root) else 'outside-owned-probe'

                def entry(item):
                    try:
                        status = item.lstat()
                    except FileNotFoundError:
                        return {'kind': 'absent'}
                    kind = 'link' if stat.S_ISLNK(status.st_mode) else 'file' if stat.S_ISREG(status.st_mode) else 'directory' if stat.S_ISDIR(status.st_mode) else 'other'
                    return {'kind': kind, 'uid': status.st_uid, 'mode': oct(stat.S_IMODE(status.st_mode)), 'bytes': status.st_size}

                layout = {'project': relative(project), 'locks': {relative(item): entry(item) for item in candidates}, 'roots': {}, 'directories': {}}
                for name, item in owners[0]['dependencies'].items():
                    reported = pathlib.Path(item['path'])
                    layout['roots'][name] = {'reported': relative(reported), 'resolved': relative(reported.resolve(strict=True))}
                for directory in [project, project / 'node_modules', project / 'node_modules/.pnpm']:
                    details = entry(directory)
                    if details['kind'] == 'directory' and directory.resolve(strict=True).is_relative_to(home) and details['uid'] == 1002:
                        names = sorted(item.name for item in directory.iterdir())
                        require(all(b'\0' not in name.encode('utf-8', 'strict') for name in names), 'pnpm layout name encoding')
                        raw = json.dumps(names, separators=(',', ':')).encode('utf-8')
                        details.update(entries=len(names), namesBytes=len(raw), namesSHA256=hashlib.sha256(raw).hexdigest())
                        details['names'] = names if len(raw) <= 512 else 'whole vector withheld'
                    layout['directories'][relative(directory)] = details
                raw = json.dumps(layout, sort_keys=True, separators=(',', ':')).encode('utf-8')
                REPORT['pnpmLayout'] = layout if len(raw) <= 3072 else {'withheld': True, 'bytes': len(raw), 'sha256': hashlib.sha256(raw).hexdigest()}
                raise RuntimeError('pnpm persisted global lock absent or ambiguous')
            copies = []
            for item in locks:
                lock_path = item.resolve(strict=True)
                require(lock_path.is_relative_to(home), 'pnpm lock escapes selected home')
                with os.fdopen(os.open(lock_path, os.O_RDONLY | os.O_NOFOLLOW), 'rb') as stream:
                    status = os.fstat(stream.fileno())
                    require(stat.S_ISREG(status.st_mode) and status.st_uid == 1002 and status.st_mode & 0o022 == 0 and status.st_size <= 33554432, 'pnpm lock ownership/byte bound')
                    raw = stream.read(33554433)
                require(len(raw) == status.st_size and len(raw) <= 33554432 and b'\0' not in raw, 'pnpm lock changed or exceeds byte/NUL bound')
                copies.append(raw)
            require(all(copy == copies[0] for copy in copies), 'pnpm persisted global lock copies differ')
            return copies[0].decode('utf-8', errors='strict')

        result = {}
        for name, (expected, sri) in pins.items():
            item = owners[0]['dependencies'][name]
            reported = pathlib.Path(item['path'])
            context = reported.parents[len(name.split('/'))]
            require(reported == context / 'node_modules' / name, 'pnpm reported root placement differs')
            text = lock_text(context.resolve(strict=True))
            location = reported.resolve(strict=True)
            require(location.is_relative_to(home) and location.stat().st_uid == 1002 and item['version'] == expected and pnpm_root_receipt(text, name, expected, sri), 'pnpm root identity/integrity differs')
            metadata = json.loads((location / 'package.json').read_bytes())
            require(metadata['name'] == name and metadata['version'] == expected, 'pnpm root metadata differs')
            # Keep pnpm's global entrypoint for Pi's stock ownership check;
            # the physical path, metadata and receipts were checked above.
            result[name] = reported
        if 'pnpmReceiptControls' not in REPORT:
            controls = home / 'receipt-controls'
            for directory in [controls, controls / 'node_modules', controls / 'node_modules/.pnpm']:
                directory.mkdir(mode=0o700)
            public = controls / 'pnpm-lock.yaml'
            hidden = controls / 'node_modules/.pnpm/lock.yaml'
            public.write_bytes(b'receipt\n')
            require(lock_text(controls) == 'receipt\n', 'pnpm single receipt control')
            hidden.write_bytes(b'receipt\n')
            require(lock_text(controls) == 'receipt\n', 'pnpm matching receipt control')
            hidden.write_bytes(b'changed\n')
            try:
                lock_text(controls)
            except RuntimeError as error:
                require(str(error) == 'pnpm persisted global lock copies differ', 'pnpm conflict refused for wrong reason')
            else:
                raise RuntimeError('pnpm conflicting receipt accepted')
            REPORT['pnpmReceiptControls'] = {'single': True, 'matching': True, 'conflictRefused': True}
        count = 0
        for metadata in home.rglob('package.json'):
            count += 1
            require(count <= 4096 and metadata.parent.name != 'aix-ppc64', 'pnpm package bound/nonapplicable AIX package')
        return result

    roots = cohort()
    pi = roots['@earendil-works/pi-coding-agent'] / 'dist/cli.js'
    require('/pnpm/' in str(pi) and run([str(node), str(pi), '--version'], extra=env).strip() == '0.99.2', 'prior Pi did not execute from recognized pnpm layout')
    identity = (home.stat().st_dev, home.stat().st_ino)
    run([str(node), str(pi), 'update', '--self'], extra=env, timeout=180)
    pins['@earendil-works/pi-coding-agent'] = ('1.0.0', 'sha512-/FtbxoSQU/mEv1QnichJjRjqteqaIaMWxmhB4G367+MwZfX7/DI5B9YAg5lqbN7nztFskBEtUSZ+FlmMBECtMw==')
    roots = cohort()
    pi = roots['@earendil-works/pi-coding-agent'] / 'dist/cli.js'
    require(run([str(node), str(pi), '--version'], extra=env).strip() == '1.0.0', 'stock pnpm prior-to-next update did not execute')
    run([str(node), str(pi), 'update', '--self', '--force'], extra=env, timeout=180)
    roots = cohort()
    require((home.stat().st_dev, home.stat().st_ino) == identity, 'stock updater replaced selected pnpm home')
    require(run([str(node), str(roots['@earendil-works/pi-coding-agent'] / 'dist/cli.js'), '--version'], extra=env).strip() == '1.0.0', 'stock pnpm forced reinstall did not execute')
    REPORT['alanBackendProbe'] = {'pnpm': version, 'stockUpgrade': '0.99.2 to 1.0.0', 'stockForce': True, 'independentlyPinnedRoots': len(pins), 'functionalReady': False}


def mvp_smoke(personal):
    source = os.environ.get('GENTLE_GUEST_SOURCE_SHA', '')
    require(len(source) == 40 and all(c in '0123456789abcdef' for c in source), 'exact candidate source revision missing')
    seed = install(WORK / 'parents/personal-seed')
    shutil.copytree(seed['Prefix'], personal / 'prefix', symlinks=True)
    personal_agent = WORK / 'home/.pi/agent'
    personal_agent.mkdir(mode=0o700, parents=True)
    (personal_agent / 'settings.json').write_text('{"theme":"personal-preserved","packages":[]}\n')
    for name in ['.bashrc', '.profile', '.zshrc']:
        (WORK / 'home' / name).write_text('# personal shell configuration must remain unchanged\n')
    protected = [personal, WORK / 'home']
    before = [physical_inventory(root) for root in protected]
    target = WORK / 'parents/mvp-separate'
    require(not target.exists(), 'smoke target is not cold')
    manifest = install(target)
    require(manifest['Mode'] == 'separate', 'smoke installed a different mode')
    prefix = pathlib.Path(manifest['Prefix'])
    require(run([str(target / 'bin/pi'), '--version']).strip() == '1.0.0', 'authenticated Pi version differs')
    require(json.loads((prefix / 'lib/node_modules/gentle-pi/package.json').read_text())['version'] == '4.0.0', 'authenticated Gentle version differs')
    native = prefix / 'lib/node_modules/gentle-pi/.gentle-ai/v4.0.0/gentle-ai'
    require(hashlib.sha256(native.read_bytes()).hexdigest() == '50ba217b5138c1a9c7d5bf2f79931b1bb89b89c4cf650dcd7ee037657c88158d', 'authenticated native bytes differ')
    require('4.0.0' in run([str(native), '--version']), 'authenticated native v4 did not execute')
    pty_status(pathlib.Path(SUPERVISOR), WORK / 'project', [SUPERVISOR, 'shell', 'install'], installer=target)
    for name in ['gentle-shell', 'pi']:
        pty_status(target / 'bin' / name, WORK / 'project', opening_only=True)
    require(before == [physical_inventory(root) for root in protected], 'personal Pi, agent or shell configuration changed')
    require((WORK / 'gofmt.data').is_file() and (WORK / 'gofmt.data').stat().st_size == 0, 'candidate source formatting differs')
    REPORT.update(functionalReady=False, mvpSmoke={'outcome': 'PASS', 'sourceSHA': source, 'pi': '1.0.0', 'gentle': '4.0.0', 'native': '4.0.0', 'personalPiAndShellPreserved': True, 'coldInstallerTUI': 'deferred', 'fullJourney': 'deferred'})


def main():
    global FAULT
    require(MODE in {'probe', 'direct', 'full', 'smoke'}, 'explicit qualification mode')
    if MODE == 'probe':
        kernel()
        print(json.dumps({'physicalWorker': REPORT['kernel'], 'functionalReady': False}, sort_keys=True))
        return
    if MODE in {'full', 'smoke'}:
        require(pathlib.Path('/run/user/1002/systemd/private').is_socket() and pathlib.Path('/run/user/1002/bus').is_socket(), 'STOP: pre-existing delegated manager and bus unavailable')
    kernel()
    require(not any(key in os.environ for key in ['GITHUB_TOKEN', 'NPM_TOKEN', 'AWS_ACCESS_KEY_ID', 'SSH_AUTH_SOCK']), 'credentials present')
    for tool in (SUPERVISOR, TESTS):
        binary = pathlib.Path(tool)
        info = binary.lstat()
        require(stat.S_ISREG(info.st_mode) and info.st_uid == 0 and info.st_gid == 0 and stat.S_IMODE(info.st_mode) == 0o555 and 0 < info.st_size <= 268435456 and binary.resolve(strict=True) == binary, 'fixture binary provenance differs')
        for parent in binary.parents:
            info = parent.lstat()
            require(stat.S_ISDIR(info.st_mode) and info.st_uid == 0 and info.st_gid == 0 and info.st_mode & 0o022 == 0 and parent.resolve(strict=True) == parent, 'fixture binary ancestor provenance differs')
    REPORT['fixtureProvenance'] = 'exclusive root-owned canonical fixture binaries and complete nonwritable ancestor chains'
    for name in ['home', 'tmp', 'project', 'personal', 'parents']:
        (WORK / name).mkdir(mode=0o700, parents=True, exist_ok=False)
    server = tls_fixture()
    try:
        run([SUPERVISOR, 'shell', 'internal-check'])
        tests = run([TESTS, '-test.run=^TestUser', '-test.timeout=90s'], cwd=WORK / 'src/internal/shellinstaller', timeout=100)
        require('PASS' in tests, 'focused Go controls')
        personal = WORK / 'personal'
        (personal / 'sentinel').write_bytes(b'personal Pi must remain unchanged\n')
        before = physical_inventory(personal)
        if MODE == 'smoke':
            mvp_smoke(personal)
            return
        alan_pnpm_backend_probe()
        require(physical_inventory(personal) == before, 'Alan backend probe changed personal Pi')
        FAULT = 'node-sri'
        bad = WORK / 'parents/refused-node'
        args, token = inspect(bad)
        run(args + ['--confirm', token], good=False)
        require(not bad.exists() and physical_inventory(personal) == before, 'bad acquisition affected personal Pi')
        FAULT = ''
        acquisition_fault(WORK / 'parents/canceled-node')
        acquisition_fault(WORK / 'parents/cleanup-node', cleanup=True)
        require(physical_inventory(personal) == before, 'acquisition faults changed personal Pi')
        target = WORK / 'parents/separate'
        manifest = install(target)
        require(physical_inventory(personal) == before, 'separate touched personal Pi')
        install(target)  # Physical idempotent retry uses actual global readback.
        shutil.copytree(pathlib.Path(manifest['Prefix']), personal / 'prefix', symlinks=True)
        personal_agent = WORK / 'home/.pi/agent'
        personal_agent.mkdir(mode=0o700, parents=True)
        (personal_agent / 'settings.json').write_text('{"theme":"personal-preserved","packages":[]}\n')
        personal_before, personal_agent_before = physical_inventory(personal), physical_inventory(personal_agent)
        pty_status(pathlib.Path(SUPERVISOR), WORK / 'project', [SUPERVISOR, 'shell', 'install'], installer=target)
        pty_status(pathlib.Path(SUPERVISOR), WORK / 'project', [SUPERVISOR, 'shell', 'install'], installer='', cancel_installer=True)
        os.chmod(target.parent, 0o770)
        try:
            run([SUPERVISOR, 'shell', 'install', '--target', str(target), '--mode', 'separate', '--inspect'], good=False)
            for binding in ['gentle-shell', 'pi']:
                run([str(target / 'bin' / binding), '--version'], good=False)
        finally:
            os.chmod(target.parent, 0o700)
        REPORT['existingParentGuard'] = 'occupied installation refused under changed nonprivate parent'
        run(['git', '-c', 'core.hooksPath=/dev/null', 'init', '-q', str(WORK / 'project')])
        run(['git', '-c', 'core.hooksPath=/dev/null', '-c', 'user.name=Guest Fixture', '-c', 'user.email=guest@invalid.local', 'commit', '--allow-empty', '-m', 'fixture'], cwd=WORK / 'project')
        native = pathlib.Path(manifest['Prefix']) / 'lib/node_modules/gentle-pi/.gentle-ai/v4.0.0/gentle-ai'
        require('4.0.0' in run([str(native), '--version']), 'authenticated native v4 did not execute')
        backend = json.loads(run([str(native), 'review', 'status', '--contract', 'gentle-ai.review-integration/v2', '--cwd', str(WORK / 'project'), '--projection', 'workspace', '--next-transition'], cwd=WORK / 'project', timeout=35, stdout_only=True))
        require(backend.get('contract') == 'gentle-ai.review-integration/v2' and backend.get('operation') == 'review.status' and backend.get('applicability') == 'unrelated', 'native read-only negotiated status identity differs')
        require(backend.get('authority') is None and backend.get('candidates') == [], 'fresh native status unexpectedly contains review authority')
        require(backend.get('schema') in {'gentle-ai.review-integration.status/v' + str(v) for v in [3, 5, 6, 7, 8, 9]}, 'unsupported native status schema')
        REPORT['nativeBackend'] = backend['schema']
        pty_status(target / 'bin/gentle-shell', WORK / 'project')
        pty_status(target / 'bin/pi', WORK / 'project')
        prefix, agent = pathlib.Path(manifest['Prefix']), pathlib.Path(manifest['Agent'])
        roots_before = [(p.stat().st_dev, p.stat().st_ino) for p in [prefix, agent]]
        bindings_before = {name: (target / 'bin' / name).read_bytes() for name in ['pi', 'gentle-shell']}
        run([str(target / 'runtime/node/bin/node'), str(target / 'provision.mjs'), str(target), str(prefix), str(agent), str(prefix), str(target), 'separate', 'prepare-prior'], timeout=remaining())
        require('0.99.2' in run([str(target / 'bin/pi'), '--version']), 'authenticated prior did not launch')
        nested = prefix / 'lib/node_modules/@earendil-works/pi-coding-agent/node_modules/@earendil-works/pi-tui/package.json'
        require(json.loads(nested.read_text())['version'] == '0.99.2', 'actual prior nested TUI placement differs')
        run([str(target / 'bin/pi'), 'update', '--self'], cwd=WORK / 'project', timeout=180)
        require('1.0.0' in run([str(target / 'bin/pi'), '--version']), 'actual stock prior-to-next transition absent')
        require(roots_before == [(p.stat().st_dev, p.stat().st_ino) for p in [prefix, agent]], 'update replaced physical selected roots')
        require(all((target / 'bin' / name).read_bytes() == data for name, data in bindings_before.items()), 'update changed owned command bindings')
        REPORT['trueUpgrade'] = 'actual published 0.99.2 to 1.0.0 stock updater; TEST-only Pi API selection, not public latest authority'
        run([str(target / 'bin/pi'), 'update', '--self', '--force'], cwd=WORK / 'project', timeout=180)
        run([str(target / 'bin/pi'), '--version'], cwd=WORK / 'project')
        REPORT['stockForce'] = 'actual separate same-version reinstall after true upgrade'
        coding_metadata = prefix / 'lib/node_modules/@earendil-works/pi-coding-agent/package.json'
        expected_metadata = coding_metadata.read_bytes()
        corrupted = json.loads(expected_metadata)
        corrupted['version'] = '9.9.9'
        coding_metadata.write_text(json.dumps(corrupted))
        run([str(target / 'bin/pi'), '--version'], good=False)
        upgrade_recovery = run([SUPERVISOR, 'shell', 'recover', str(target), 'inspect'])
        upgrade_token = next(line.split(': ', 1)[1] for line in upgrade_recovery.splitlines() if line.startswith('Recovery confirmation: '))
        run([SUPERVISOR, 'shell', 'recover', str(target), upgrade_token], timeout=120)
        require(coding_metadata.read_bytes() == expected_metadata, 'separate upgrade preimages did not restore known bytes')
        require('1.0.0' in run([str(target / 'bin/pi'), '--version']), 'restored known cohort did not retry successfully')
        REPORT['upgradeRecovery'] = 'actual unknown cohort refused, full saved trees restored and authenticated retry observed'
        shared_prefix, shared_agent = WORK / 'shared-prefix', WORK / 'shared-agent'
        shutil.copytree(manifest['Prefix'], shared_prefix, symlinks=True)
        shared_agent.mkdir(mode=0o700)
        (shared_agent / 'settings.json').write_text('{}\n')
        os.chmod(shared_prefix, 0o700)
        os.chmod(shared_agent / 'settings.json', 0o600)
        shared = WORK / 'parents/shared'
        args, token = inspect(shared, (shared_prefix, shared_agent))
        (shared_agent / 'settings.json').write_text('{"theme":"dark"}\n')
        run(args + ['--confirm', token], good=False)
        tool_acquisition_fault(WORK / 'parents/tool-fault', (shared_prefix, shared_agent))
        shared_manifest = install(shared, (shared_prefix, shared_agent))
        require(shared_manifest['Prefix'] == str(shared_prefix) and shared_manifest['Agent'] == str(shared_agent), 'shared physical binding selection')
        pty_status(shared / 'bin/gentle-shell', WORK / 'project')
        pty_status(shared / 'bin/pi', WORK / 'project')
        REPORT['manager'] = 'UNQUALIFIED: real externally provided delegated manager required'
        if MODE == 'full':
            runtime = pathlib.Path('/run/user/1002')
            require(stat.S_ISSOCK((runtime / 'systemd/private').stat().st_mode), 'pre-existing user manager socket unavailable')
            manager_env = {'GENTLE_USER_MANAGER_GUEST': 'approved', 'GENTLE_USER_SUPERVISOR': SUPERVISOR,
                           'GENTLE_USER_INSTALLED': str(shared), 'GENTLE_USER_PROJECT': str(WORK / 'project'),
                           'XDG_RUNTIME_DIR': str(runtime), 'DBUS_SESSION_BUS_ADDRESS': 'unix:path=' + str(runtime / 'bus')}
            check = run([TESTS, '-test.run=^TestUserDelegatedManagerIntegration$', '-test.timeout=25s'], extra=manager_env, timeout=30)
            require('PASS' in check, 'real delegated controller check')
            pty_status(pathlib.Path(TESTS), WORK / 'project', [TESTS, '-test.run=^TestUserDelegatedManagerLaunch$', '-test.timeout=70s'], manager_env)
            REPORT['manager'] = 'actual manager/controller/PTY/stop-subtree readback observed'
        recovery = run([SUPERVISOR, 'shell', 'recover', str(shared), 'inspect'])
        recovery_token = next(line.split(': ', 1)[1] for line in recovery.splitlines() if line.startswith('Recovery confirmation: '))
        # Recovery intentionally accepts damaged live contents. Stale authority
        # must change a bound directory identity, not merely settings bytes.
        retained_agent = WORK / 'shared-agent-before-replacement'
        shared_agent.rename(retained_agent)
        shared_agent.mkdir(mode=0o700)
        (shared_agent / 'settings.json').write_text('{"theme":"after-inspect"}\n')
        os.chmod(shared_agent / 'settings.json', 0o600)
        run([SUPERVISOR, 'shell', 'recover', str(shared), recovery_token], good=False)
        require(json.loads((shared_agent / 'settings.json').read_text()) == {'theme': 'after-inspect'}, 'stale recovery altered replacement')
        require((retained_agent / 'settings.json').is_file(), 'replaced shared agent evidence lost')
        recovery = run([SUPERVISOR, 'shell', 'recover', str(shared), 'inspect'])
        recovery_token = next(line.split(': ', 1)[1] for line in recovery.splitlines() if line.startswith('Recovery confirmation: '))
        run([SUPERVISOR, 'shell', 'recover', str(shared), recovery_token], timeout=120)
        require(json.loads((shared_agent / 'settings.json').read_text()) == {'theme': 'dark'}, 'shared exact settings restoration')
        run([SUPERVISOR, 'shell', 'recover', str(shared), 'inspect'])
        REPORT['sharedRecovery'] = 'actual fresh-confirm restoration with retained quarantine'
        publication_fault(WORK / 'parents/publication-fault', (shared_prefix, shared_agent))
        REPORT['recoveryFaultMatrix'] = 'actual acquisition cancellation, cleanup denial, stale recovery consent and post-publication uncertainty controls'
        require(physical_inventory(personal) == personal_before and physical_inventory(personal_agent) == personal_agent_before, 'actual personalized published Pi prefix/configuration changed')
        REPORT['personalizedPi'] = 'actual authenticated published prefix, default agent settings and metadata preserved'
        REPORT['functionalReady'] = MODE == 'full'
        if MODE == 'full':
            format_data = WORK / 'gofmt.data'
            require(format_data.is_file() and format_data.stat().st_size == 0, 'whole source formatting not yet qualified')
            require(all(key in REPORT for key in ['manager', 'nativeBackend', 'trueUpgrade', 'upgradeRecovery', 'installerTUI', 'installerCancel', 'publicationUncertainty', 'sharedRecovery', 'recoveryFaultMatrix']), 'full observed qualification record incomplete')
    finally:
        server.shutdown()
        server.server_close()
    output = (json.dumps(REPORT, sort_keys=True, separators=(',', ':')) + '\n').encode()
    require(len(output) < 4096, 'entire raw Guest report withheld above bound')
    sys.stdout.buffer.write(output)


try:
    main()
    if MODE == 'smoke':
        output = (json.dumps(REPORT, sort_keys=True, separators=(',', ':')) + '\n').encode()
        require(len(output) < 4096, 'entire raw Guest report withheld above bound')
        sys.stdout.buffer.write(output)
except Exception as error:
    message = {'functionalReady': False, 'errorType': type(error).__name__, 'reason': str(error)[:240], 'rawStockOutput': 'withheld',
               'elapsedSeconds': round(time.monotonic() - START), 'reached': sorted(key for key in REPORT if not key.startswith('pty'))}
    if 'pnpmLayout' in REPORT:
        message['pnpmLayout'] = REPORT['pnpmLayout']
    if 'ptyFailure' in REPORT:
        message['ptyFailure'] = REPORT['ptyFailure']
    if 'projectMutation' in REPORT:
        message['projectMutation'] = REPORT['projectMutation']
    if 'ordinaryRequestFailure' in REPORT:
        message['ordinaryRequestFailure'] = REPORT['ordinaryRequestFailure']
    if 'nativeBackend' in REPORT:
        message['nativeBackend'] = REPORT['nativeBackend']
    if 'alanBackendProbe' in REPORT:
        message['alanBackendProbe'] = REPORT['alanBackendProbe']
    if COMMAND_FAILURE is not None:
        complete = 'stdoutBase64' in COMMAND_FAILURE and 'stderrBase64' in COMMAND_FAILURE
        message.update(commandFailure=COMMAND_FAILURE, rawStockOutput='complete-base64' if complete else 'withheld')
    if 'ptyFailureBuffer' in REPORT:
        candidate = {**message, 'observedTTYBufferBase64': REPORT['ptyFailureBuffer']}
        if len((json.dumps(candidate, sort_keys=True) + '\n').encode()) < 4096:
            message = candidate
    output = (json.dumps(message, sort_keys=True) + '\n').encode()
    if len(output) >= 4096 and 'stdoutTailBase64' in message.get('commandFailure', {}):
        message['commandFailure'] = {k: v for k, v in message['commandFailure'].items() if not k.endswith('TailBase64')}
        output = (json.dumps(message, sort_keys=True) + '\n').encode()
    if len(output) >= 4096:
        message = {'functionalReady': False, 'reason': 'entire command failure output withheld at original Guest bound', 'rawStockOutput': 'withheld', 'bytes': len(output), 'sha256': hashlib.sha256(output).hexdigest()}
        output = (json.dumps(message, sort_keys=True) + '\n').encode()
    if len(output) < 4096:
        sys.stdout.buffer.write(output)
    sys.exit(1)
