#!/usr/bin/env python3
"""Unit tests for the pure helpers of e2e/shell-tui-acceptance.py (no tmux, no install)."""
import importlib.util
import json
import os
import pathlib
import tempfile
import unittest

_SPEC = importlib.util.spec_from_file_location('acceptance', pathlib.Path(__file__).with_name('shell-tui-acceptance.py'))
acc = importlib.util.module_from_spec(_SPEC)
_SPEC.loader.exec_module(acc)

# Trimmed captures of the real TUI (macOS, origin/main cd19f274e).
CONFIGURE = '''selected additional experience preferences require a compatible installed runtime; run gentle-ai shell install --help
Configure Gentle-Shell experience
PERSONA                                                  ││ CURRENT VALUE                    │
▸ (*) Gentleman                                          ││ gentleman                        │
  ( ) Neutral                                            ││                                  │
  ( ) Custom                                             ││                                  │
RECOMMENDED                                              ││                                  │
  [x] Background subagents: ON                           ││                                  │
  [ ] Strict TDD: Inherited                              │
  [ ] Context7: OFF                                      │
SDK SOURCE                                               │
  (*) Stable                                             │
  ( ) Main preview                                       │
INSTALL TARGET                                           │
  ( ) On my existing Pi  · locked                        │
  (*) Separate install (Gentle-Shell)  · locked          │
ACTIONS                                                  │
  Apply changes to installation                          │
j/k: navigate • enter: select • End: continue • esc: back'''

DETECTED = '''╭────────────────────────────────────────╮
│  ◆  Gentle-Shell detected                │
│  MODE          Separate install          │
│   ▸ Manage this installation             │
│     Back                                 │
╰────────────────────────────────────────╯'''

WELCOME = '''║  │ ▸ Install Gentle-Shell, our own agent        │    ║
║  Menu                                                 ║
║    Start installation                                 ║'''

MANAGE = '''Manage installation
~/.local/share/gentle-shell/install
TYPE          Separate install  ·  opens with gentle-shell

PREFERENCE CHANGES
  Persona        Gentleman → Neutral

COMPONENT CHANGES (not applied here)
  CodeGraph      On → Off
These need a component update; changing preferences does not reinstall the stack.

 ▸ Apply preferences
   Back

   Uninstall this installation

j/k: navigate  •  enter: select  •  esc: back'''

FAILED = '''Installation failed
The installation stopped before publishing a verified result:
  published installation refused: the delegated unit launch probe failed:
  delegated user manager required

Enter: back to menu'''


def canonical(value):
    return (json.dumps(value, indent=2) + '\n').encode()


class ScreenParsing(unittest.TestCase):
    def test_cursor_label_on_each_screen_shape(self):
        self.assertEqual(acc.cursor_label(CONFIGURE), '(*) Gentleman')
        self.assertEqual(acc.cursor_label(DETECTED), 'Manage this installation')
        self.assertEqual(acc.cursor_label(WELCOME), 'Install Gentle-Shell, our own agent')
        self.assertEqual(acc.cursor_label(MANAGE), 'Apply preferences')
        self.assertIsNone(acc.cursor_label('no cursor here'))

    def test_label_matches_ignores_markers_and_suffixes(self):
        self.assertTrue(acc.label_matches('(*) Gentleman', 'Gentleman'))
        self.assertTrue(acc.label_matches('( ) On my existing Pi  · locked', 'On my existing Pi'))
        self.assertTrue(acc.label_matches('[ ] Strict TDD: Inherited', 'Strict TDD'))
        self.assertFalse(acc.label_matches('( ) Neutral', 'Gentleman'))
        self.assertFalse(acc.label_matches(None, 'Back'))

    def test_toggle_and_radio_state(self):
        self.assertEqual(acc.toggle_state(CONFIGURE, 'Background subagents'), 'on')
        self.assertEqual(acc.toggle_state(CONFIGURE, 'Strict TDD'), 'inherited')
        self.assertEqual(acc.toggle_state(CONFIGURE, 'Context7'), 'off')
        self.assertIsNone(acc.toggle_state(CONFIGURE, 'CodeGraph'))
        self.assertTrue(acc.radio_selected(CONFIGURE, 'Gentleman'))
        self.assertFalse(acc.radio_selected(CONFIGURE, 'Main preview'))
        self.assertTrue(acc.radio_selected(CONFIGURE, 'Separate install (Gentle-Shell)'))
        self.assertIsNone(acc.radio_selected(CONFIGURE, 'Apply changes'))

    def test_error_gate_and_failure_text(self):
        self.assertTrue(acc.screen_error(CONFIGURE).startswith('selected additional experience preferences'))
        self.assertEqual(acc.screen_error(CONFIGURE.split('\n', 1)[1]), '')
        self.assertIsNone(acc.find_gate(CONFIGURE))
        gate = 'SDK Main + stock provider is qualified on Linux amd64 today; macOS support is being completed'
        self.assertEqual(acc.find_gate(gate + '\n' + CONFIGURE), acc.GATES[0])
        self.assertEqual(acc.failure_reason(FAILED),
                         'published installation refused: the delegated unit launch probe failed: delegated user manager required')
        self.assertEqual(acc.failure_reason(MANAGE), '')

    def test_manage_actions_and_new_rows(self):
        self.assertEqual(acc.manage_actions(MANAGE), ['Apply preferences', 'Back', 'Uninstall this installation'])
        after = MANAGE.replace(' ▸ Apply preferences', 'experience write refused\n\n ▸ Apply preferences')
        self.assertEqual(acc.new_rows(MANAGE, after), ['experience write refused'])
        self.assertEqual(acc.new_rows(MANAGE, MANAGE), [])

    def test_backup_path(self):
        self.assertEqual(acc.backup_path('Installation uninstalled.\nBackup: /h/.local/share/gentle-shell/install.removed-20261024T1Z\n'),
                         '/h/.local/share/gentle-shell/install.removed-20261024T1Z')
        self.assertIsNone(acc.backup_path(MANAGE))


class KeyPlans(unittest.TestCase):
    def test_defaults_need_no_keys(self):
        self.assertEqual(acc.experience_steps(acc.DEFAULTS, acc.DEFAULTS), [])

    def test_manage_preferences_plan(self):
        want = dict(acc.DEFAULTS, persona='neutral', tdd='off', context7='off')
        self.assertEqual(acc.experience_steps(want, acc.DEFAULTS),
                         [('radio', 'Neutral'), ('toggle', 'Strict TDD', 'off'), ('toggle', 'Context7', 'off')])

    def test_fresh_custom_main_plan_selects_sdk_last(self):
        want = {'persona': 'unmanaged', 'background': 'off', 'tdd': 'off', 'rdd': 'off',
                'codegraph': 'off', 'context7': 'off', 'sdk': 'main-preview', 'target': 'agent'}
        steps = acc.experience_steps(want, acc.DEFAULTS)
        self.assertEqual(steps[0], ('radio', 'Custom'))
        self.assertEqual(len([s for s in steps if s[0] == 'toggle']), 5)
        self.assertEqual(steps[-2:], [('radio', 'On my existing Pi'), ('radio', 'Main preview')])

    def test_scenario_filter(self):
        self.assertEqual(acc.parse_scenarios(''), list('ABCDEFG'))
        self.assertEqual(acc.parse_scenarios('e, a b'), ['A', 'B', 'E'])
        with self.assertRaises(ValueError):
            acc.parse_scenarios('A,Z')

    def test_summary_exit_code(self):
        self.assertEqual(acc.summarize({'A': ('PASS', ''), 'F': ('SKIP', 'gate')})[1], 0)
        summary, code = acc.summarize({'A': ('PASS', ''), 'B': ('FAIL', 'x')})
        self.assertEqual((code, summary['B']), (1, {'status': 'FAIL', 'reason': 'x'}))


class FileAssertions(unittest.TestCase):
    def setUp(self):
        self.home = pathlib.Path(os.path.realpath(tempfile.mkdtemp()))
        self.target = self.home / acc.TARGET_REL

    def install(self, profile, **overrides):
        target, home = self.target, self.home
        manifest = {'Schema': 'gentle-shell-user-install/v1', 'Destination': str(target), 'Mode': profile['target'],
                    'Agent': str(target / 'agent')}
        if profile['codegraph'] == 'on':
            manifest['codegraph'] = True
        if profile['sdk'] == 'main-preview':
            manifest['main_sdk'] = {'name': 'gentle-shell'}
        files = {'installation.json': json.dumps(manifest).encode(),
                 'home/.gentle-ai/state.json': json.dumps({'rdd_mode': profile['rdd']}).encode(),
                 'agent/mcp.json': json.dumps({'mcpServers': {'context7': {
                     'url': acc.CONTEXT7_URL, 'enabled': profile['context7'] == 'on', 'exposure': 'codemode'}}}).encode()}
        files.update({f'config/{k}': v for k, v in acc.expected_config_files(profile).items()})
        files.update(overrides)
        for name, data in files.items():
            if data is None:
                continue
            (target / name).parent.mkdir(parents=True, exist_ok=True)
            (target / name).write_bytes(data)
        (target / 'bin').mkdir(exist_ok=True)
        (target / 'bin/gentle-shell').write_text('#!/bin/sh\n')
        (home / '.local/bin').mkdir(parents=True, exist_ok=True)
        os.symlink(target / 'bin/gentle-shell', home / acc.LAUNCHER_REL)
        (home / '.gentle-ai').mkdir()
        (home / acc.REGISTRY_REL).write_text(json.dumps({'schema': 'gentle-ai.shell-installs/v1', 'installs': [
            {'target': str(target), 'mode': profile['target'], 'channel': profile['sdk']}]}))

    def test_canonical_bytes_match_the_go_writer(self):
        files = acc.expected_config_files(acc.DEFAULTS)
        self.assertEqual(files['strict-tdd.json'], b'{\n  "schema": "gentle-pi.strict-tdd/v1",\n  "policy": "on"\n}\n')
        self.assertEqual(files['persona.json'], b'{\n  "mode": "gentleman"\n}\n')
        self.assertNotIn('persona.json', acc.expected_config_files(dict(acc.DEFAULTS, persona='unmanaged')))

    def test_matching_install_has_no_problems(self):
        self.install(acc.DEFAULTS)
        self.assertEqual(acc.verify_install(self.home, acc.DEFAULTS), [])

    def test_apply_preferences_defect_shapes_are_reported(self):
        want = dict(acc.DEFAULTS, tdd='off', context7='off')
        self.install(want, **{'config/strict-tdd.json': b'{"policy":"off"}',
                              'agent/mcp.json': json.dumps({'mcpServers': {'context7': {'enabled': False}}}).encode()})
        problems = acc.verify_install(self.home, want)
        self.assertTrue(any(p.startswith('config/strict-tdd.json') for p in problems), problems)
        self.assertTrue(any('context7' in p for p in problems), problems)

    def test_channel_codegraph_and_registry_mismatches(self):
        self.install(acc.DEFAULTS, **{'config/update-channel.json': b'{"channel":"main"}'})
        problems = acc.verify_install(self.home, dict(acc.DEFAULTS, codegraph='off'))
        self.assertTrue(any('codegraph' in p for p in problems), problems)
        self.assertTrue(any('update-channel' in p for p in problems), problems)
        self.assertTrue(any('main_sdk' in p for p in acc.verify_install(self.home, dict(acc.DEFAULTS, sdk='main-preview'))))

    def test_missing_installation_is_one_problem(self):
        problems = acc.verify_install(self.home, acc.DEFAULTS)
        self.assertEqual(len(problems), 1)
        self.assertIn('installation.json', problems[0])

    def test_uninstalled_requires_backup_and_no_leftovers(self):
        backup = pathlib.Path(str(self.target) + '.removed-20261024T234121Z')
        backup.mkdir(parents=True)
        self.assertEqual(acc.verify_uninstalled(self.home, str(backup)), [])
        self.install(acc.DEFAULTS)
        problems = acc.verify_uninstalled(self.home, str(backup))
        self.assertEqual(len(problems), 3, problems)
        self.assertTrue(acc.verify_uninstalled(self.home, '/elsewhere/install.removed-1'))

    def test_snapshot_detects_content_mode_and_new_files(self):
        agent = self.home / '.pi/agent'
        (agent / 'bin').mkdir(parents=True)
        (agent / 'settings.json').write_text('{}')
        before = acc.snapshot_tree(agent)
        self.assertEqual(acc.diff_snapshots(before, acc.snapshot_tree(agent)), [])
        (agent / 'settings.json').write_text('{"packages":[]}')
        (agent / 'mcp.json').write_text('{}')
        os.chmod(agent / 'bin', 0o700)
        self.assertEqual(acc.diff_snapshots(before, acc.snapshot_tree(agent)),
                         ['added mcp.json', 'changed bin', 'changed settings.json'])



class ExistingPiTools(unittest.TestCase):
    def setUp(self):
        self.root = pathlib.Path(os.path.realpath(tempfile.mkdtemp()))

    def executable(self, path, body='#!/bin/sh\nexit 0\n', mode=0o755):
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(body)
        os.chmod(path, mode)
        return path

    def test_first_run_path_hides_fd_and_rg_from_pi(self):
        host = self.root / 'host-bin'
        for name in ('node', 'npm', 'tar', 'gzip', 'fd', 'fdfind', 'rg'):
            self.executable(host / name)
        directory = acc.pi_first_run_path(self.root / 'pi-path', str(host), str(host))
        self.assertEqual(sorted(os.listdir(directory)), ['gzip', 'node', 'npm', 'tar'])
        for name in os.listdir(directory):
            self.assertEqual(os.path.realpath(directory / name), str(host / name))

    def test_first_run_path_requires_node_npm_and_tar(self):
        host = self.root / 'host-bin'
        self.executable(host / 'node')
        with self.assertRaises(acc.Fail):
            acc.pi_first_run_path(self.root / 'pi-path', str(host), str(host))

    def agent_and_target(self):
        bin_dir = self.root / 'agent/bin'
        fd = self.executable(bin_dir / 'fd', 'pi fd')
        rg = self.executable(bin_dir / 'rg', 'pi rg')
        target = self.root / 'install'
        pi_tools = acc.tool_images(bin_dir)
        (target / 'backup/agent-tools').mkdir(parents=True)
        (target / 'state').mkdir()
        files = {}
        for path in (fd, rg):
            (target / 'backup/agent-tools' / path.name).write_bytes(path.read_bytes())
            image = pi_tools[path.name]
            files[path.name] = {'sha256': image['sha256'], 'size': image['size'], 'mode': '%04o' % image['mode']}
        evidence = {'schema': 'gentle-shell-agent-tools/v1', 'agent': str(self.root / 'agent'), 'files': files}
        (target / 'state/agent-tools.json').write_text(json.dumps(evidence))
        return bin_dir, target, pi_tools

    def test_tool_images_record_mode_size_and_digest(self):
        bin_dir, _, pi_tools = self.agent_and_target()
        self.assertEqual(sorted(pi_tools), ['fd', 'rg'])
        self.assertEqual(pi_tools['fd']['mode'], 0o755)
        self.assertEqual(pi_tools['fd']['size'], len('pi fd'))
        os.chmod(bin_dir / 'fd', 0o700)
        self.assertNotEqual(acc.tool_images(bin_dir), pi_tools)

    def test_set_aside_evidence_matches(self):
        _, target, pi_tools = self.agent_and_target()
        self.assertEqual(acc.verify_set_aside(target, self.root / 'agent', pi_tools), [])

    def test_set_aside_mismatches_are_reported(self):
        _, target, pi_tools = self.agent_and_target()
        (target / 'backup/agent-tools/rg').write_text('tampered')
        evidence = json.loads((target / 'state/agent-tools.json').read_text())
        evidence['files']['fd']['mode'] = '0700'
        (target / 'state/agent-tools.json').write_text(json.dumps(evidence))
        problems = acc.verify_set_aside(target, self.root / 'agent', pi_tools)
        self.assertTrue(any(p.startswith('set-aside rg') for p in problems), problems)
        self.assertTrue(any(p.startswith('set-aside fd') for p in problems), problems)
        self.assertTrue(acc.verify_set_aside(self.root / 'missing', self.root / 'agent', pi_tools))

if __name__ == '__main__':
    unittest.main()
