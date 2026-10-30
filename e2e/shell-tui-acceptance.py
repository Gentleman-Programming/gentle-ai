#!/usr/bin/env python3
"""Drive real Gentle Shell installs through the gentle-ai TUI (tmux keystrokes).

Every scenario runs in its own canonical HOME and asserts the correct product
behavior; known defects are never encoded as expected. A scenario is SKIP only
when the TUI shows one of GATES verbatim. See docs/gentle-shell-tui-acceptance.md.
Standard library only.
"""
import argparse
import hashlib
import json
import os
import pathlib
import pwd
import re
import shlex
import shutil
import stat
import subprocess
import sys
import tempfile
import time

SCENARIOS = 'ABCDEFG'
TARGET_REL = '.local/share/gentle-shell/install'
REGISTRY_REL = '.gentle-ai/shell-installs.json'
LAUNCHER_REL = '.local/bin/gentle-shell'
CONTEXT7_URL = 'https://mcp.context7.com/mcp'
# Intended product gates. Extend only with exact TUI text of a deliberate gate.
GATES = ('SDK Main + stock provider is qualified on Linux amd64 today',)
CONFIGURE = 'Configure Gentle-Shell experience'
DETECTED = 'Gentle-Shell detected'
HERO = 'Install Gentle-Shell, our own agent'
PLAN = 'Installation plan'
FAILURE_HEAD = 'The installation stopped before publishing a verified result:'
EXIT_MARK = '__GSA_EXIT__='
BORDER = '║│╭╮╰╯ '
SEMVER = re.compile(r'\b\d+\.\d+\.\d+\b')

DEFAULTS = {'persona': 'gentleman', 'background': 'on', 'tdd': 'on', 'rdd': 'on',
            'codegraph': 'on', 'context7': 'on', 'sdk': 'stable', 'target': 'separate'}
PERSONA_LABEL = {'gentleman': 'Gentleman', 'neutral': 'Neutral', 'unmanaged': 'Custom'}
TOGGLE_LABEL = {'background': 'Background subagents', 'tdd': 'Strict TDD', 'rdd': 'Global RDD',
                'codegraph': 'CodeGraph', 'context7': 'Context7'}
SDK_LABEL = {'stable': 'Stable', 'main-preview': 'Main preview'}
TARGET_LABEL = {'separate': 'Separate install (Gentle-Shell)', 'agent': 'On my existing Pi'}


class Fail(Exception):
    pass


class Skip(Exception):
    pass


# ---------------------------------------------------------------- pure helpers

def parse_scenarios(text):
    picked = [part.strip().upper() for part in re.split(r'[,\s]+', text or '') if part.strip()]
    for item in picked:
        if item not in SCENARIOS:
            raise ValueError(f'unknown scenario {item!r}; choose from {",".join(SCENARIOS)}')
    return [s for s in SCENARIOS if s in picked] if picked else list(SCENARIOS)


def _content(row):
    return row.strip().strip(BORDER).split('│')[0].strip()


def cursor_label(screen):
    for row in screen.split('\n'):
        stripped = row.strip().strip(BORDER)
        if stripped.startswith('▸'):
            return stripped[1:].split('│')[0].strip()
    return None


def option_text(label):
    """Drop the radio/checkbox marker so labels compare by name."""
    return re.sub(r'^(\(\*\)|\( \)|\[x\]|\[ \])\s*', '', label or '')


def label_matches(current, wanted):
    return current is not None and option_text(current).startswith(wanted)


def toggle_state(screen, label):
    for row in screen.split('\n'):
        text = _content(row).lstrip('▸').strip()
        match = re.match(r'^\[[x ]\] ' + re.escape(label) + r': (ON|OFF|Inherited)\b', text)
        if match:
            return match.group(1).lower()
    return None


def radio_selected(screen, label):
    for row in screen.split('\n'):
        text = _content(row).lstrip('▸').strip()
        if option_text(text).startswith(label) and text.startswith('('):
            return text.startswith('(*)')
    return None


def screen_error(screen, title=CONFIGURE):
    """Text the Configure screen renders above its title (validation errors)."""
    rows = screen.split('\n')
    for index, row in enumerate(rows):
        if title in row:
            return ' '.join(r.strip().strip(BORDER) for r in rows[:index] if r.strip().strip(BORDER)).strip()
    return ''


def find_gate(screen):
    return next((gate for gate in GATES if gate in screen), None)


def failure_reason(screen):
    rows = screen.split('\n')
    for index, row in enumerate(rows):
        if FAILURE_HEAD in row:
            tail = []
            for line in rows[index + 1:]:
                text = line.strip().strip(BORDER)
                if text.startswith('Enter:'):
                    break
                if text:
                    tail.append(text)
            return ' '.join(tail)
    return ''


def manage_actions(screen):
    """Selectable actions of the Manage screen, in order."""
    rows = [row.strip().strip(BORDER) for row in screen.split('\n')]
    start = 0
    for index, text in enumerate(rows):
        if '→' in text or text.startswith('TYPE'):
            start = index + 1
    actions = []
    for text in rows[start:]:
        if text.startswith('j/k:'):
            break
        name = text.lstrip('▸').strip()
        if not name or name.isupper() or name.endswith('.') or name.startswith(('Enter:', 'These need')):
            continue
        actions.append(name)
    return actions


def new_rows(before, after):
    """Rows that appear only after an action: inline errors of Manage/update screens."""
    seen = {_content(row) for row in before.split('\n')}
    return [text for text in (_content(row) for row in after.split('\n')) if text and text not in seen]


def screen_digest(screen, limit=600):
    return ' | '.join(t for t in (_content(r) for r in screen.split('\n')) if t)[:limit]


def backup_path(screen):
    match = re.search(r'Backup: (\S+)', screen)
    return match.group(1) if match else None


def experience_steps(profile, base):
    """Key plan from base to profile: persona, toggles, target, SDK (gate last)."""
    steps = []
    if profile['persona'] != base['persona']:
        steps.append(('radio', PERSONA_LABEL[profile['persona']]))
    for key, label in TOGGLE_LABEL.items():
        if profile[key] != base[key]:
            steps.append(('toggle', label, profile[key]))
    if profile['target'] != base['target']:
        steps.append(('radio', TARGET_LABEL[profile['target']]))
    if profile['sdk'] != base['sdk']:
        steps.append(('radio', SDK_LABEL[profile['sdk']]))
    return steps


def canonical_json(value):
    """Bytes of Go json.MarshalIndent(value, "", "  ") plus newline."""
    return (json.dumps(value, indent=2) + '\n').encode()


def expected_config_files(profile):
    files = {'background-subagents.json': canonical_json(
        {'schema': 'gentle-pi.background-subagents/v1', 'policy': profile['background']})}
    if profile['persona'] != 'unmanaged':
        files['persona.json'] = canonical_json({'mode': profile['persona']})
    files['strict-tdd.json'] = canonical_json({'schema': 'gentle-pi.strict-tdd/v1', 'policy': profile['tdd']})
    return files


def _read_json(path, problems, name):
    try:
        return json.loads(pathlib.Path(path).read_bytes())
    except (OSError, ValueError) as error:
        problems.append(f'{name}: unreadable ({error.__class__.__name__}: {error})')
        return None


def registry_entries(home):
    path = pathlib.Path(home) / REGISTRY_REL
    if not path.exists():
        return []
    data = json.loads(path.read_bytes())
    if data.get('schema') != 'gentle-ai.shell-installs/v1':
        raise ValueError('registry schema is not gentle-ai.shell-installs/v1')
    return data.get('installs') or []


def verify_install(home, profile):
    """Return the list of problems; empty means the installation matches profile."""
    home = pathlib.Path(home)
    target = home / TARGET_REL
    problems = []
    manifest = _read_json(target / 'installation.json', problems, 'installation.json')
    if manifest is None:
        return problems
    if manifest.get('Schema') != 'gentle-shell-user-install/v1':
        problems.append(f'installation.json Schema={manifest.get("Schema")!r}')
    if manifest.get('Mode') != profile['target']:
        problems.append(f'installation.json Mode={manifest.get("Mode")!r} want {profile["target"]!r}')
    if manifest.get('Destination') != str(target):
        problems.append(f'installation.json Destination={manifest.get("Destination")!r} want {str(target)!r}')
    if bool(manifest.get('codegraph')) != (profile['codegraph'] == 'on'):
        problems.append(f'installation.json codegraph={manifest.get("codegraph")!r} want {profile["codegraph"]}')
    if ('main_sdk' in manifest) != (profile['sdk'] == 'main-preview'):
        problems.append(f'installation.json main_sdk present={"main_sdk" in manifest} want sdk {profile["sdk"]}')
    config = target / 'config'
    for name, want in expected_config_files(profile).items():
        try:
            got = (config / name).read_bytes()
        except OSError:
            problems.append(f'config/{name}: missing')
            continue
        if got != want:
            problems.append(f'config/{name}: {got!r} want canonical {want!r}')
    if profile['persona'] == 'unmanaged' and (config / 'persona.json').exists():
        problems.append('config/persona.json: present for Custom (unmanaged) persona')
    state = _read_json(target / 'home/.gentle-ai/state.json', problems, 'home/.gentle-ai/state.json')
    if state is not None and state.get('rdd_mode') != profile['rdd']:
        problems.append(f'home/.gentle-ai/state.json rdd_mode={state.get("rdd_mode")!r} want {profile["rdd"]!r}')
    agent = pathlib.Path(manifest.get('Agent') or target / 'agent')
    mcp = _read_json(agent / 'mcp.json', problems, f'{agent}/mcp.json')
    if mcp is not None:
        want = {'url': CONTEXT7_URL, 'enabled': profile['context7'] == 'on', 'exposure': 'codemode'}
        got = (mcp.get('mcpServers') or {}).get('context7')
        if got != want:
            problems.append(f'agent mcp.json context7={got!r} want {want!r}')
    channel_file = config / 'update-channel.json'
    if channel_file.exists():
        channel = _read_json(channel_file, problems, 'config/update-channel.json') or {}
        want_channel = 'main' if profile['sdk'] == 'main-preview' else 'stable'
        if channel.get('channel') != want_channel:
            problems.append(f'config/update-channel.json channel={channel.get("channel")!r} want {want_channel!r}')
    if profile['target'] == 'separate':
        launcher = home / LAUNCHER_REL
        if not launcher.is_symlink() or os.path.realpath(launcher) != os.path.realpath(target / 'bin/gentle-shell'):
            problems.append(f'{LAUNCHER_REL}: not a link to {target}/bin/gentle-shell')
    try:
        entries = [e for e in registry_entries(home) if e.get('target') == str(target)]
    except ValueError as error:
        problems.append(f'{REGISTRY_REL}: {error}')
        entries = None
    if entries is not None:
        want = {'mode': profile['target'], 'channel': profile['sdk']}
        if len(entries) != 1 or {k: entries[0].get(k) for k in want} != want:
            problems.append(f'{REGISTRY_REL}: entries for target {entries!r} want one {want!r}')
    return problems


def verify_uninstalled(home, backup):
    home = pathlib.Path(home)
    target = home / TARGET_REL
    problems = []
    if not backup or not re.fullmatch(re.escape(str(target)) + r'\.removed-\S+', backup):
        problems.append(f'backup {backup!r} is not {target}.removed-<ts>')
    elif not pathlib.Path(backup).is_dir():
        problems.append(f'backup {backup} does not exist')
    if os.path.lexists(target):
        problems.append(f'{target} still exists')
    if os.path.lexists(home / LAUNCHER_REL):
        problems.append(f'{LAUNCHER_REL} still exists')
    try:
        if any(e.get('target') == str(target) for e in registry_entries(home)):
            problems.append(f'{REGISTRY_REL} still lists {target}')
    except ValueError as error:
        problems.append(f'{REGISTRY_REL}: {error}')
    return problems


PATH_MARK = '# gentle-ai: gentle-shell launcher'
PATH_LINES = {'sh': 'export PATH="$HOME/.local/bin:$PATH" ' + PATH_MARK, 'fish': 'set -gx PATH $HOME/.local/bin $PATH ' + PATH_MARK}


def startup_candidates(home, env):
    """Every startup file the installer may choose for env, per shell (profileFor)."""
    home = pathlib.Path(home)
    zdot = env.get('ZDOTDIR', '')
    config = env.get('XDG_CONFIG_HOME', '')
    zdot = pathlib.Path(zdot) if os.path.isabs(zdot) else home
    config = pathlib.Path(config) if os.path.isabs(config) else home / '.config'
    bash = [home / name for name in ('.bash_profile', '.bash_login', '.profile')] if sys.platform == 'darwin' else [home / '.bashrc']
    return {'zsh': [zdot / '.zshrc'], 'bash': bash, 'fish': [config / 'fish/conf.d/gentle-ai-shell.fish'], 'other': [home / '.profile']}


def startup_file(home, env):
    """The startup file the installer chooses for the SHELL the harness sets."""
    files = startup_candidates(home, env)
    shell = os.path.basename(env.get('SHELL', ''))
    if shell == 'bash' and sys.platform == 'darwin':
        return next((path for path in files['bash'] if path.exists()), files['bash'][0])
    return files.get(shell, files['other'])[0]


def startup_files_present(home, env):
    return {path for paths in startup_candidates(home, env).values() for path in paths if os.path.lexists(path)}


def verify_startup(home, env, served, preexisting):
    """Marked PATH lines against the PATH and SHELL the harness gives the TUI.

    served: a Separate launcher is published. With ~/.local/bin on that PATH,
    or nothing served, no marked line may exist anywhere; otherwise the chosen
    file holds exactly one, the exact line for its shell. With nothing served,
    no startup file absent at preexisting time may be left behind.
    """
    home = pathlib.Path(home)
    on_path = str(home / '.local/bin') in [os.path.normpath(entry) for entry in env.get('PATH', '').split(':') if entry]
    chosen = startup_file(home, env) if served and not on_path else None
    problems = []
    for path in sorted({p for paths in startup_candidates(home, env).values() for p in paths}):
        if not path.is_file() or path.is_symlink():
            continue
        lines = [line for line in path.read_text().split('\n') if PATH_MARK in line]
        want = [PATH_LINES['fish' if path.suffix == '.fish' else 'sh']] if path == chosen else []
        if lines != want:
            problems.append(f'{path.relative_to(home)}: marked PATH lines {lines!r} want {want!r}')
    if chosen is not None and not chosen.is_file():
        problems.append(f'{chosen.relative_to(home)}: missing the marked PATH line')
    if not served:
        left = sorted(str(path.relative_to(home)) for path in startup_files_present(home, env) - set(preexisting))
        if left:
            problems.append(f'startup files created by the install remain: {left}')
    return problems


def snapshot_tree(root):
    root = pathlib.Path(root)
    entries = {}
    for directory, dirs, files in os.walk(root, followlinks=False):
        for name in sorted(dirs + files):
            path = pathlib.Path(directory) / name
            info = path.lstat()
            rel = str(path.relative_to(root))
            if stat.S_ISLNK(info.st_mode):
                entries[rel] = ('link', os.readlink(path))
            elif stat.S_ISDIR(info.st_mode):
                entries[rel] = ('dir', stat.S_IMODE(info.st_mode))
            else:
                entries[rel] = ('file', stat.S_IMODE(info.st_mode), hashlib.sha256(path.read_bytes()).hexdigest())
    entries['.'] = ('dir', stat.S_IMODE(root.lstat().st_mode))
    return entries


def diff_snapshots(before, after):
    changes = [f'removed {k}' for k in sorted(set(before) - set(after))]
    changes += [f'added {k}' for k in sorted(set(after) - set(before))]
    changes += [f'changed {k}' for k in sorted(set(before) & set(after)) if before[k] != after[k]]
    return changes


def summarize(results):
    summary = {name: {'status': status, 'reason': reason} for name, (status, reason) in results.items()}
    return summary, 1 if any(status == 'FAIL' for status, _ in results.values()) else 0



def pi_first_run_path(directory, node_path, tools_path):
    """A PATH directory exposing only node, npm and Pi's archive extractors.

    Pi uses fd/rg found on PATH instead of downloading its own; hosts that
    ship them (Homebrew, nix, apt) would never reproduce a stock agent bin.
    Pi extracts its downloads with tar (GNU tar calls gzip).
    """
    directory = pathlib.Path(directory)
    directory.mkdir(mode=0o700)
    for name, search in (('node', node_path), ('npm', node_path), ('tar', tools_path), ('gzip', tools_path)):
        found = shutil.which(name, path=search)
        if found:
            os.symlink(os.path.realpath(found), directory / name)
        elif name != 'gzip':
            raise Fail(f'{name} is required for the first Pi run and is not on {search!r}')
    return directory


def tool_images(bin_dir):
    """Mode, size and digest of the fd/rg in an agent bin, without following links."""
    images = {}
    for name in ('fd', 'rg'):
        path = pathlib.Path(bin_dir) / name
        if not os.path.lexists(path):
            continue
        info = path.lstat()
        data = path.read_bytes() if stat.S_ISREG(info.st_mode) else b''
        images[name] = {'regular': stat.S_ISREG(info.st_mode), 'mode': stat.S_IMODE(info.st_mode),
                        'size': len(data), 'sha256': hashlib.sha256(data).hexdigest()}
    return images


def verify_set_aside(target, agent, pi_tools):
    """Problems with the installation's backup of Pi's own fd/rg."""
    target = pathlib.Path(target)
    problems = []
    evidence = _read_json(target / 'state/agent-tools.json', problems, 'state/agent-tools.json')
    if evidence is None:
        return problems
    if evidence.get('schema') != 'gentle-shell-agent-tools/v1' or evidence.get('agent') != str(agent):
        problems.append(f'state/agent-tools.json schema/agent={evidence.get("schema")!r}/{evidence.get("agent")!r}')
    recorded = evidence.get('files') or {}
    if sorted(recorded) != sorted(pi_tools):
        problems.append(f'state/agent-tools.json files={sorted(recorded)} want {sorted(pi_tools)}')
    for name, image in pi_tools.items():
        want = {'sha256': image['sha256'], 'size': image['size'], 'mode': '%04o' % image['mode']}
        if recorded.get(name) != want:
            problems.append(f'set-aside {name}: evidence {recorded.get(name)!r} want {want!r}')
        backup = target / 'backup/agent-tools' / name
        digest = hashlib.sha256(backup.read_bytes()).hexdigest() if backup.is_file() else None
        if digest != image['sha256']:
            problems.append(f'set-aside {name}: backup digest {digest!r} want {image["sha256"]!r}')
    return problems

# ------------------------------------------------------------------ tmux driver

class Tui:
    """One program in a tmux server on a private socket; the exit code is printed in-pane."""

    def __init__(self, ctx, tag, program=None, env=None):
        self.ctx, self.tag, self.count = ctx, tag, 0
        self.sockdir = tempfile.mkdtemp(prefix='gsa-', dir='/tmp')
        self.socket = os.path.join(self.sockdir, 's')
        script = f'"$0"; printf "\\n{EXIT_MARK}%s\\n" "$?"; exec sleep 86400'
        variables = [f'{k}={v}' for k, v in (env or ctx.env).items()]
        command = ['env', '-i', *variables, '/bin/sh', '-c', script, program or ctx.binary]
        self.tmux('new-session', '-d', '-x', '240', '-y', '80', '-c', str(ctx.home), shlex.join(command))

    def tmux(self, *args):
        return subprocess.run(['tmux', '-S', self.socket, *args], check=True, capture_output=True, text=True).stdout

    def capture(self):
        return self.tmux('capture-pane', '-p')

    def send(self, *keys):
        for key in keys:
            self.tmux('send-keys', key)
            time.sleep(0.05)

    def snap(self, name, screen=None):
        self.count += 1
        screen = self.capture() if screen is None else screen
        path = self.ctx.evidence / 'screens' / f'{self.tag}-{self.count:02d}-{name}.txt'
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text('\n'.join(row.rstrip() for row in screen.split('\n')).rstrip() + '\n')
        return screen

    def wait(self, ready, timeout, what):
        deadline = time.monotonic() + timeout
        while True:
            screen = self.capture()
            if ready(screen):
                return screen
            if EXIT_MARK in screen:
                self.snap('exited', screen)
                raise Fail(f'gentle-ai exited while waiting for {what}')
            if time.monotonic() > deadline:
                self.snap('timeout', screen)
                raise Fail(f'timeout after {timeout}s waiting for {what}')
            time.sleep(0.25)

    def wait_text(self, text, timeout=30):
        return self.wait(lambda s: text in s, timeout, repr(text))

    def close(self):
        subprocess.run(['tmux', '-S', self.socket, 'kill-server'], capture_output=True)
        shutil.rmtree(self.sockdir, ignore_errors=True)

    def move_to(self, label, limit=45):
        screen = self.capture()
        for key in ('j', 'k'):
            for _ in range(limit):
                current = cursor_label(screen)
                if label_matches(current, label):
                    return screen
                self.send(key)
                try:
                    screen = self.wait(lambda s: cursor_label(s) != current, 2, 'cursor move')
                except Fail:
                    break
        self.snap('cursor-missing')
        raise Fail(f'cursor never reached {label!r}')

    def set_radio(self, label):
        self.move_to(label)
        self.send('Enter')
        try:
            return self.wait(lambda s: radio_selected(s, label) or find_gate(s) or screen_error(s), 3, f'select {label}')
        except Fail:
            return self.capture()

    def set_toggle(self, label, value):
        screen = self.move_to(label)
        for _ in range(3):
            state = toggle_state(screen, label)
            if state == value:
                return screen
            self.send('Enter')
            screen = self.wait(lambda s, before=state: toggle_state(s, label) != before, 3, f'toggle {label}')
        raise Fail(f'{label} never reached {value}')

    def configure(self, steps):
        for step in steps:
            screen = self.set_radio(step[1]) if step[0] == 'radio' else self.set_toggle(step[1], step[2])
            gate = find_gate(screen)
            if gate:
                self.snap('gate', screen)
                raise Skip(gate)
            if step[0] == 'radio' and not radio_selected(screen, step[1]):
                self.snap('radio-refused', screen)
                raise Fail(f'choosing {step[1]!r} was refused: {screen_error(screen) or "no selection"}')
        return self.snap('configured')


class Context:
    def __init__(self, args, scenario):
        self.binary, self.args, self.scenario = args.gentle_ai, args, scenario
        root = pathlib.Path(tempfile.mkdtemp(prefix=f'{scenario}-', dir=args.work))
        self.home, self.tmp = root / 'home', root / 'tmp'
        self.home.mkdir(mode=0o700)
        self.tmp.mkdir(mode=0o700)
        if os.path.realpath(self.home) != str(self.home) or str(self.home) == args.real_home:
            raise Fail(f'scenario HOME {self.home} is not canonical and isolated')
        self.evidence = args.evidence / scenario
        self.evidence.mkdir(parents=True, exist_ok=True)
        self.env = {'HOME': str(self.home), 'PATH': args.path, 'TERM': 'xterm-256color', 'LANG': 'C.UTF-8',
                    'TMPDIR': str(self.tmp), 'GENTLE_AI_NO_SELF_UPDATE': '1', 'GENTLE_AI_NO_ANIMATION': '1'}
        for key in ('XDG_RUNTIME_DIR', 'DBUS_SESSION_BUS_ADDRESS'):
            if os.environ.get(key):
                self.env[key] = os.environ[key]
        self.startup_before = startup_files_present(self.home, self.env)
        self.notes, self.tuis = [], []

    def log(self, message):
        self.notes.append(message)
        print(f'[{self.scenario}] {message}', flush=True)

    def tui(self, tag, program=None, env=None):
        tui = Tui(self, tag, program, env)
        self.tuis.append(tui)
        return tui

    def keep_files(self, label):
        target = self.home / TARGET_REL
        names = ['installation.json', 'config/persona.json', 'config/background-subagents.json',
                 'config/strict-tdd.json', 'config/update-channel.json', 'home/.gentle-ai/state.json', 'agent/mcp.json']
        sources = [(target / n, n) for n in names] + [(self.home / REGISTRY_REL, REGISTRY_REL),
                                                      (self.home / '.pi/agent/mcp.json', '.pi/agent/mcp.json')]
        for source, name in sources:
            if source.is_file():
                destination = self.evidence / 'files' / label / name
                destination.parent.mkdir(parents=True, exist_ok=True)
                destination.write_bytes(source.read_bytes()[:1 << 20])

    def verify(self, profile, label):
        self.keep_files(label)
        problems = verify_install(self.home, profile)
        problems += verify_startup(self.home, self.env, profile['target'] == 'separate', self.startup_before)
        if problems:
            raise Fail(f'{label}: ' + '; '.join(problems))
        self.log(f'{label}: files match profile')

    def version(self, label):
        launcher = self.home / LAUNCHER_REL
        result = subprocess.run([str(launcher), '--version'], env=self.env, cwd=self.home,
                                capture_output=True, text=True, timeout=180)
        output = (result.stdout + result.stderr).strip()
        (self.evidence / f'{label}-version.txt').write_text(f'exit={result.returncode}\n{output}\n')
        if result.returncode != 0 or not SEMVER.search(result.stdout):
            raise Fail(f'{label}: gentle-shell --version exit={result.returncode}: {output[-300:]}')
        self.log(f'{label}: gentle-shell --version -> {SEMVER.search(result.stdout).group(0)}')

    def close(self):
        for tui in self.tuis:
            tui.close()


# -------------------------------------------------------------------- flows

def enter_installer(tui):
    tui.wait_text(HERO)
    tui.move_to('Install Gentle-Shell')
    tui.send('Enter')
    return tui.wait(lambda s: CONFIGURE in s or DETECTED in s, 30, 'installer entry')


def continue_to_plan(tui):
    tui.send('c')
    screen = tui.wait(lambda s: PLAN in s or 'Manage installation' in s or screen_error(s), 60, 'Continue')
    tui.snap('continue', screen)
    if find_gate(screen):
        raise Skip(find_gate(screen))
    if PLAN not in screen and 'Manage installation' not in screen:
        raise Fail(f'Continue refused: {screen_error(screen)}')
    return screen


def run_plan(ctx, tui):
    tui.send('Enter')
    screen = tui.wait(lambda s: 'Enter: finish' in s or 'Installation failed' in s, ctx.args.install_timeout, 'install result')
    tui.snap('result', screen)
    if 'Installation failed' in screen:
        raise Fail(f'install failed: {failure_reason(screen)}')
    tui.send('Enter')
    screen = tui.wait(lambda s: EXIT_MARK in s, 30, 'exit after Enter: finish')
    if EXIT_MARK + '0' not in screen:
        raise Fail(f'gentle-ai exited non-zero after a completed install: {screen_digest(screen)}')


def fresh_install(ctx, profile, tag):
    tui = ctx.tui(tag)
    screen = enter_installer(tui)
    if DETECTED in screen:
        tui.snap('unexpected-detected', screen)
        raise Fail('fresh HOME shows a detected installation')
    tui.configure(experience_steps(profile, DEFAULTS))
    if PLAN not in continue_to_plan(tui):
        raise Fail('fresh Continue opened Manage instead of the installation plan')
    run_plan(ctx, tui)
    return tui


def setup_defaults(ctx):
    try:
        fresh_install(ctx, DEFAULTS, 'setup')
        ctx.verify(DEFAULTS, 'setup')
    except Fail as error:
        raise Fail(f'setup install (scenario A) failed: {error}')


def open_manage(ctx, tag, tui=None):
    tui = tui or ctx.tui(tag)
    screen = enter_installer(tui)
    if DETECTED not in screen:
        tui.snap('not-detected', screen)
        raise Fail('installed target is not detected by the TUI')
    tui.move_to('Manage this installation')
    tui.send('Enter')
    tui.wait(lambda s: CONFIGURE in s and 'Uninstall this installation' in s, 30, 'Manage configure')
    return tui


def apply_preferences(tui, labels):
    screen = tui.capture()
    if 'Apply changes to installation' not in screen:
        tui.snap('label', screen)
        raise Fail('Configure does not offer "Apply changes to installation"')
    screen = continue_to_plan(tui)
    missing = [label for label in labels if not re.search(re.escape(label) + r'\s+\S.*→', screen)]
    if missing or 'Apply preferences' not in manage_actions(screen):
        raise Fail(f'Manage shows no applicable change for {missing or labels}: actions {manage_actions(screen)}')
    before = tui.move_to('Apply preferences')
    tui.send('Enter')
    screen = tui.wait(lambda s: 'Preferences updated.' in s or new_rows(before, s), 30, 'apply')
    tui.snap('applied', screen)
    if 'Preferences updated.' not in screen:
        raise Fail(f'Apply preferences failed: {" ".join(new_rows(before, screen))}')
    tui.send('Enter')
    return tui.wait(lambda s: CONFIGURE in s or HERO in s, 15, 'leave Manage')


def update_now(ctx, tui):
    if CONFIGURE not in tui.capture():
        open_manage(ctx, 'update', tui)
    tui.move_to('Check for updates')
    tui.send('Enter')
    screen = tui.wait(lambda s: 'Gentle-Shell updates' in s and 'Checking this channel' not in s, 90, 'update check')
    tui.snap('update-check', screen)
    if 'Update available' not in screen:
        raise Fail(f'channel switch offers no update: {screen_digest(screen)}')
    tui.move_to('Update now')
    tui.send('Enter')
    screen = tui.wait(lambda s: PLAN in s or 'Gentle-Shell updates' not in s, 60, 'update plan')
    if PLAN not in screen:
        tui.snap('update-refused', screen)
        raise Fail(f'Update now did not open the installation plan: {screen_digest(screen)}')
    run_plan(ctx, tui)


def scenario_a(ctx):
    fresh_install(ctx, DEFAULTS, 'install')
    ctx.verify(DEFAULTS, 'install')
    ctx.version('install')


def scenario_b(ctx):
    setup_defaults(ctx)
    tui = open_manage(ctx, 'manage')
    want = dict(DEFAULTS, persona='neutral', tdd='off', context7='off')
    tui.configure(experience_steps(want, DEFAULTS))
    apply_preferences(tui, ['Persona', 'Strict TDD', 'Context7'])
    ctx.verify(want, 'applied')


def scenario_c(ctx):
    setup_defaults(ctx)
    tui = open_manage(ctx, 'manage')
    want = dict(DEFAULTS, codegraph='off')
    screen = tui.configure(experience_steps(want, DEFAULTS))
    if 'Update installation' not in screen:
        raise Fail('Configure does not offer "Update installation" for a component change')
    screen = continue_to_plan(tui)
    if PLAN not in screen:
        extra = [a for a in manage_actions(screen) if a not in ('Back', 'Uninstall this installation', 'Apply preferences')]
        if not extra:
            raise Fail(f'component change has no apply path; Manage offers {manage_actions(screen)}')
        tui.move_to(extra[0])
        tui.send('Enter')
        tui.wait_text(PLAN, 60)
    run_plan(ctx, tui)
    ctx.verify(want, 'updated')
    ctx.version('updated')


def scenario_d(ctx):
    setup_defaults(ctx)
    for leg, sdk in (('main', 'main-preview'), ('stable', 'stable')):
        current = dict(DEFAULTS, sdk='stable' if sdk == 'main-preview' else 'main-preview')
        want = dict(DEFAULTS, sdk=sdk)
        tui = open_manage(ctx, f'to-{leg}')
        tui.configure(experience_steps(want, current))
        apply_preferences(tui, ['SDK source'])
        update_now(ctx, tui)
        ctx.verify(want, f'to-{leg}')
        ctx.version(f'to-{leg}')


def scenario_e(ctx):
    setup_defaults(ctx)
    tui = open_manage(ctx, 'uninstall')
    tui.move_to('Uninstall this installation')
    tui.send('Enter')
    tui.wait_text('Uninstall (keep backup)')
    tui.move_to('Uninstall (keep backup)')
    tui.send('Enter')
    before = tui.capture()
    screen = tui.wait(lambda s: 'Installation uninstalled.' in s or ('Working' not in s and new_rows(before, s)), 180, 'uninstall')
    tui.snap('uninstalled', screen)
    problems = verify_uninstalled(ctx.home, backup_path(screen))
    problems += verify_startup(ctx.home, ctx.env, False, ctx.startup_before)
    if 'Installation uninstalled.' not in screen or problems:
        raise Fail('uninstall: ' + ('; '.join(problems) or 'no success text'))
    ctx.log(f'uninstall: backup {backup_path(screen)}')
    tui.send('Enter')
    screen = enter_installer(tui)
    if DETECTED in screen:
        raise Fail('uninstalled target is still detected')
    if PLAN not in continue_to_plan(tui):
        raise Fail('reinstall Continue did not open the plan')
    run_plan(ctx, tui)
    ctx.verify(DEFAULTS, 'reinstall')
    ctx.version('reinstall')


def scenario_f(ctx):
    want = {'persona': 'unmanaged', 'background': 'off', 'tdd': 'off', 'rdd': 'off',
            'codegraph': 'off', 'context7': 'off', 'sdk': 'main-preview', 'target': 'separate'}
    fresh_install(ctx, want, 'install')
    ctx.verify(want, 'install')
    ctx.version('install')


def scenario_g(ctx):
    prefix = ctx.home / '.npm-global'
    npm, node = shutil.which('npm'), shutil.which('node')
    if not npm or not node:
        raise Fail('node and npm are required to install Pi as a user')
    host_env = dict(ctx.env, PATH=os.path.dirname(node) + ':' + ctx.env['PATH'])
    package = f'@earendil-works/pi-coding-agent@{ctx.args.pi_version}'
    subprocess.run([npm, 'install', '-g', '--prefix', str(prefix), package], env=host_env,
                   check=True, capture_output=True, timeout=600)
    pi = prefix / 'bin/pi'
    first = subprocess.run([str(pi), '--version'], env=host_env, cwd=ctx.home, capture_output=True, text=True, timeout=120)
    ctx.log(f'pi --version -> {first.stdout.strip()!r}')
    agent = ctx.home / '.pi/agent'
    bin_dir = agent / 'bin'
    # A real first interactive run creates the agent and downloads Pi's own
    # fd/rg into agent/bin; its PATH hides any fd/rg the host already has.
    first_path = pi_first_run_path(ctx.home.parent / 'pi-first-run-path', os.pathsep.join([os.path.dirname(node), os.path.dirname(npm)]), ctx.env['PATH'])
    session = ctx.tui('pi', str(pi), dict(ctx.env, PATH=str(first_path)))
    deadline, last = time.monotonic() + 300, None
    while time.monotonic() < deadline:
        current = tool_images(bin_dir)
        if sorted(current) == ['fd', 'rg'] and all(image['size'] for image in current.values()) and current == last:
            break
        last = current
        time.sleep(3)
    session.snap('pi-first-run')
    session.close()
    pi_tools = tool_images(bin_dir)
    if sorted(pi_tools) != ['fd', 'rg'] or not all(image['regular'] and image['size'] for image in pi_tools.values()):
        raise Fail(f'running Pi once did not download its own fd/rg into {bin_dir}: {pi_tools}')
    ctx.log('Pi downloaded its own fd/rg: ' + ', '.join(f'{n} {i["size"]} B {i["mode"]:04o}' for n, i in sorted(pi_tools.items())))
    before = snapshot_tree(agent)
    # The agent target has no CodeGraph producer; the TUI keeps it off (T11).
    want = dict(DEFAULTS, target='agent', codegraph='off')
    try:
        fresh_install(ctx, want, 'install')
    except Fail as error:
        changes = diff_snapshots(before, snapshot_tree(agent))
        (ctx.evidence / 'agent-diff.txt').write_text('\n'.join(changes) + '\n')
        state = 'byte-identical' if not changes else f'MODIFIED ({len(changes)} entries, e.g. {changes[:5]})'
        raise Fail(f'{error}; ~/.pi/agent after the failed attempt: {state}')
    target = ctx.home / TARGET_REL
    manifest = json.loads((target / 'installation.json').read_bytes())
    ctx.keep_files('install')
    problems = verify_install(ctx.home, want)
    problems += verify_startup(ctx.home, ctx.env, False, ctx.startup_before)
    if manifest.get('Agent') != str(agent):
        problems.append(f'installation.json Agent={manifest.get("Agent")!r} want {agent}')
    problems += verify_set_aside(target, agent, pi_tools)
    pinned = tool_images(bin_dir)
    if sorted(pinned) != ['fd', 'rg'] or any(image['mode'] != 0o700 for image in pinned.values()):
        problems.append(f'pinned fd/rg not bound into {bin_dir}: {pinned}')
    after = subprocess.run([str(pi), '--version'], env=host_env, cwd=ctx.home, capture_output=True, text=True, timeout=120)
    if after.returncode != 0:
        problems.append(f'pi --version after install exit={after.returncode}')
    if problems:
        raise Fail('install: ' + '; '.join(problems))
    ctx.log('install: files match profile; Pi fd/rg set aside with their evidence')
    tui = open_manage(ctx, 'uninstall')
    tui.move_to('Uninstall this installation')
    tui.send('Enter')
    tui.wait_text('Uninstall (keep backup)')
    tui.move_to('Uninstall (keep backup)')
    tui.send('Enter')
    screen_before = tui.capture()
    screen = tui.wait(lambda s: 'Installation uninstalled.' in s or ('Working' not in s and new_rows(screen_before, s)), 180, 'uninstall')
    tui.snap('uninstalled', screen)
    problems = verify_uninstalled(ctx.home, backup_path(screen))
    problems += verify_startup(ctx.home, ctx.env, False, ctx.startup_before)
    if 'Installation uninstalled.' not in screen:
        problems.append('no success text: ' + screen_digest(screen))
    restored = tool_images(bin_dir)
    if restored != pi_tools:
        problems.append(f'fd/rg after uninstall {restored} want Pi\'s own {pi_tools}')
    changes = diff_snapshots(before, snapshot_tree(agent))
    (ctx.evidence / 'agent-diff-after-uninstall.txt').write_text('\n'.join(changes) + '\n')
    if changes:
        problems.append(f'~/.pi/agent after uninstall differs from before install: {changes}')
    final = subprocess.run([str(pi), '--version'], env=host_env, cwd=ctx.home, capture_output=True, text=True, timeout=120)
    if final.returncode != 0:
        problems.append(f'pi --version after uninstall exit={final.returncode}')
    if problems:
        raise Fail('uninstall: ' + '; '.join(problems))
    ctx.log(f'uninstall: backup {backup_path(screen)}; ~/.pi/agent byte-identical to before install, Pi fd/rg restored')


RUNNERS = {'A': scenario_a, 'B': scenario_b, 'C': scenario_c, 'D': scenario_d,
           'E': scenario_e, 'F': scenario_f, 'G': scenario_g}


def remove_scratch(path, work):
    """Delete one scenario scratch tree; installed trees may hold read-only directories."""
    path = pathlib.Path(path)
    if path.parent != pathlib.Path(work):
        raise ValueError(f'refusing to delete {path} outside {work}')
    for directory, _, _ in os.walk(path):
        os.chmod(directory, 0o700)
    shutil.rmtree(path)


def run_scenario(args, name):
    ctx, result = None, ('FAIL', 'not started')
    try:
        ctx = Context(args, name)
        RUNNERS[name](ctx)
        result = ('PASS', '')
    except Skip as gate:
        result = ('SKIP', f'intended product gate: {gate}')
    except Fail as error:
        result = ('FAIL', str(error))
    except (subprocess.SubprocessError, OSError, ValueError, KeyError) as error:
        result = ('FAIL', f'{error.__class__.__name__}: {error}')
    finally:
        if ctx:
            ctx.close()
            record = {'status': result[0], 'reason': result[1], 'home': str(ctx.home), 'notes': ctx.notes}
            (ctx.evidence / 'result.json').write_text(json.dumps(record, indent=2) + '\n')
            if args.cleanup:
                remove_scratch(ctx.home.parent, args.work)
    return result


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__.split('\n')[0])
    parser.add_argument('--gentle-ai', required=True)
    parser.add_argument('--evidence', required=True, type=pathlib.Path)
    parser.add_argument('--scenarios', default='')
    parser.add_argument('--work', help='parent for per-scenario HOMEs (default: a new temp dir)')
    parser.add_argument('--path', default='/usr/bin:/bin', help='PATH given to gentle-ai')
    parser.add_argument('--install-timeout', type=int, default=1800)
    parser.add_argument('--pi-version', default='1.4.0')
    parser.add_argument('--cleanup', action='store_true', help='delete each scenario HOME after its evidence is kept')
    args = parser.parse_args(argv)
    try:
        selected = parse_scenarios(args.scenarios)
    except ValueError as error:
        parser.error(str(error))
    if os.name != 'posix' or os.geteuid() == 0:
        parser.error('run as a regular non-root user on Linux or macOS')
    if not shutil.which('tmux'):
        parser.error('tmux is required')
    args.gentle_ai = os.path.realpath(args.gentle_ai)
    args.real_home = pwd.getpwuid(os.getuid()).pw_dir
    args.work = os.path.realpath(args.work or tempfile.mkdtemp(prefix='gsa-work-', dir=os.environ.get('RUNNER_TEMP')))
    if args.real_home.startswith(args.work.rstrip('/') + '/') or args.work == args.real_home:
        parser.error('--work must not be the real HOME or one of its ancestors')
    args.evidence.mkdir(parents=True, exist_ok=True)
    results = {}
    for name in selected:
        print(f'=== scenario {name}', flush=True)
        results[name] = run_scenario(args, name)
        print(f'=== scenario {name}: {results[name][0]} {results[name][1]}', flush=True)
    summary, code = summarize(results)
    (args.evidence / 'summary.json').write_text(json.dumps(summary, indent=2) + '\n')
    print(json.dumps(summary, indent=2))
    return code


if __name__ == '__main__':
    sys.exit(main())
