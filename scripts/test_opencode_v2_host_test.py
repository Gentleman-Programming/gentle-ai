import importlib.util
from pathlib import Path
import unittest
import json
import io
import os
import tempfile
from unittest.mock import Mock, patch

spec = importlib.util.spec_from_file_location("host", Path(__file__).with_name("test-opencode-v2-host.py"))
host = importlib.util.module_from_spec(spec)
spec.loader.exec_module(host)

class HostVersionTests(unittest.TestCase):
    def test_explicit_selectors(self):
        with tempfile.TemporaryDirectory() as root:
            for selector in ('2.x', '2.0.18', '2.0.19'):
                with self.subTest(selector=selector):
                    args = host.parse_args(['host', 'deps', '--host-version', selector, '--temp-root', root])
                    self.assertEqual(args.host_version, selector)
            for selector in ('1.x', '3.x', '1.0.18', '3.0.0', '2', '2.0', 'unknown'):
                with self.subTest(selector=selector), patch('sys.stderr', new_callable=io.StringIO):
                    with self.assertRaises(SystemExit):
                        host.parse_args(['host', 'deps', '--host-version', selector, '--temp-root', root])

    def test_observed_host_version_matches_selector(self):
        for version in ('2.0.18', '2.0.19'):
            for selector in ('2.x', version):
                with self.subTest(version=version, selector=selector):
                    self.assertEqual(host.validate_host_version('opencode v' + version + '\n', selector), version)
        for output in ('opencode v1.0.18', 'opencode v3.0.0', 'unknown', '', '2.0.18',
                       'opencode v2.0', 'opencode v2.0.19-dev', 'opencode v2.0.19 extra',
                       'warning\nopencode v2.0.19', 'opencode v2.０.19'):
            with self.subTest(output=output), self.assertRaisesRegex(RuntimeError, 'host version'):
                host.validate_host_version(output, '2.x')
        with self.assertRaisesRegex(RuntimeError, 'host version'):
            host.validate_host_version('opencode v2.0.19', '2.0.18')


class ActivationTests(unittest.TestCase):
    def poll(self, replies):
        now = [0]
        def sleep(seconds):
            now[0] += seconds
        iterator = iter(replies)
        return host.wait_for_plugins(lambda: next(iterator), {"one"}, timeout=2,
                                     clock=lambda: now[0], sleep=sleep)

    def test_cold_inventory_waits_for_active_same_host(self):
        active = {"data": [{"id": "one", "state": {"status": "active"}}]}
        self.assertEqual(self.poll([{"data": []}, active]), active)

    def test_builtin_inventory_does_not_hide_active_managed_plugins(self):
        response = {"data": [{"id": name, "state": {"status": "active"}} for name in ("builtin", "one")]}
        self.assertEqual(self.poll([response] * 10), response)

    def test_failed_activation_is_not_success(self):
        with self.assertRaisesRegex(RuntimeError, "activation failed"):
            self.poll([{"data": [{"state": {"status": "failed", "error": "Duplicate plugin ID"}}]}])

    def test_missing_activation_is_bounded(self):
        with self.assertRaisesRegex(TimeoutError, "activation"):
            self.poll([{"data": []}] * 10)

    def test_duplicate_active_inventory_is_not_success(self):
        item = {"id": "one", "state": {"status": "active"}}
        with self.assertRaisesRegex(RuntimeError, "duplicate"):
            self.poll([{"data": [item, item]}])


class LoopbackResponderTests(unittest.TestCase):
    def test_parent_requests_foreground_child(self):
        from opencode_v2_loopback import scripted_reply
        reply = scripted_reply({"model": "parent", "messages": [{"role": "user", "content": "FOREGROUND_PARENT"}]})
        call = reply["tool_calls"][0]
        import json
        args = json.loads(call["function"]["arguments"])
        self.assertEqual(call["function"]["name"], "subagent")
        self.assertEqual(args["agent"], "fixture-child")
        self.assertNotIn("background", args)
        self.assertNotIn("sessionID", args)

    def test_child_returns_raw_sentinel(self):
        from opencode_v2_loopback import scripted_reply
        self.assertEqual(scripted_reply({"model": "child", "messages": []})["content"], "RAW_CHILD_SENTINEL")

    def test_unexpected_model_refuses(self):
        from opencode_v2_loopback import scripted_reply
        with self.assertRaisesRegex(ValueError, "unexpected model"):
            scripted_reply({"model": "external-model", "messages": []})

    def test_parent_returns_only_after_tool_result(self):
        from opencode_v2_loopback import scripted_reply
        self.assertEqual(scripted_reply({"model": "parent", "messages": [{"role": "tool", "content": "result"}]})["content"], "PARENT_DONE")


class GenericSafetyTests(unittest.TestCase):
    def test_cli_requires_explicit_version_and_temp_root_before_launch(self):
        for args, environment in [(["host", "deps"], {}),
                                  (["host", "deps", "--host-version", "2.0.18"], {}),
                                  (["host", "deps", "--host-version", "2.0.18", "--temp-root", "/missing/fixture-root"], {})]:
            with self.subTest(args=args), patch.dict(os.environ, environment, clear=True), patch.object(host.subprocess, "Popen") as launch, patch("sys.stderr", new_callable=io.StringIO):
                with self.assertRaises((SystemExit, ValueError)):
                    host.main(args)
                launch.assert_not_called()

    def test_modes_are_exclusive_and_tmpdir_is_explicit_fallback(self):
        with tempfile.TemporaryDirectory() as root, patch.dict(os.environ, {"TMPDIR": root}, clear=True):
            args = host.parse_args(["host", "deps", "--host-version", "2.0.18", "--generic-task-only"])
            self.assertEqual(args.temp_root, Path(root).resolve())
            self.assertIsNone(args.loopback)
            with self.assertRaises(SystemExit), patch("sys.stderr", new_callable=io.StringIO):
                host.parse_args(["host", "deps", "--host-version", "2.0.18", "--generic-task-only", "--loopback", "native"])

    def test_generic_responder_refuses_review_even_after_tool_result(self):
        from opencode_v2_loopback import scripted_reply
        for model in ("parent", "child"):
            with self.subTest(model=model), self.assertRaisesRegex(ValueError, "review disabled"):
                scripted_reply({"model": model, "messages": [{"role": "tool", "content": "NEGATIVE_REVIEW"}]}, generic_task_only=True)

    def test_generic_config_allows_only_fixture_child(self):
        from opencode_v2_loopback import prepare_fixture
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "project").mkdir()
            (root / "plugins").mkdir()
            prepare_fixture(root, root / "plugins", "http://127.0.0.1:1234", generic_task_only=True)
            config = json.loads((root / "project/opencode.json").read_text())
            self.assertEqual(set(config["agents"]), {"fixture-parent", "fixture-child"})
            self.assertEqual(config["agents"]["fixture-parent"]["permissions"][-1],
                             {"action": "subagent", "resource": "fixture-child", "effect": "allow"})

    def test_generic_dispatch_never_requests_shell_or_review(self):
        from opencode_v2_loopback import prove_generic_dispatch
        calls = []
        def request(path, body=None, **kwargs):
            calls.append((path, body))
            return {"data": {"id": "fixture"}}
        observations = [
            {"type": "before", "id": "call"},
            {"type": "after", "id": "call", "status": "completed", "result": {"output": {
                "status": "completed", "sessionID": "child", "output": "RAW_CHILD_SENTINEL"}}},
            {"type": "context", "agent": "fixture-child", "system": ["CHILD_AGENT_SENTINEL", "PROJECT_INSTRUCTION_SENTINEL"], "tools": []},
        ]
        log = Mock()
        log.read_text.return_value = "\n".join(map(json.dumps, observations))
        prove_generic_dispatch(request, log, [{"model": "child", "messages": [{"content": "CHILD_AGENT_SENTINEL PROJECT_INSTRUCTION_SENTINEL"}]}], [])
        self.assertEqual([path for path, _ in calls], ["/api/session", "/api/session/fixture/prompt", "/api/experimental/session/fixture/wait"])
        self.assertNotIn("review", json.dumps(calls).lower())

    def test_generic_host_sandbox_includes_version_and_cleanup(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            binary = root / "host"
            binary.touch()
            dependencies = root / "node_modules"
            package = dependencies / "@opencode/plugin"
            package.mkdir(parents=True)
            (package / "package.json").write_text('{"version":"2.0.4"}')
            args = [str(binary), str(dependencies), "--host-version", "2.0.18", "--temp-root", directory, "--generic-task-only"]
            with patch.object(host.sys, "platform", "darwin"), patch.object(host, "network_prefix", return_value=["sandbox"]), \
                 patch.object(host.subprocess, "run", return_value=Mock(stdout="opencode v2.0.18\n")) as version, \
                 patch.object(host.subprocess, "Popen") as launch, patch.object(host, "read_address", side_effect=TimeoutError), \
                 patch.object(host, "stop") as stop, patch.object(host.shutil, "copy2") as native_copy:
                with self.assertRaises(TimeoutError):
                    host.main(args)
                self.assertEqual(version.call_args.args[0], ["sandbox", str(binary.resolve()), "--version"])
                self.assertEqual(launch.call_args.args[0][:2], ["sandbox", str(binary.resolve())])
                env = launch.call_args.kwargs["env"]
                for key in ("HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "TMPDIR", "OPENCODE_CONFIG_DIR", "OPENCODE_TEST_HOME"):
                    self.assertTrue(env[key].startswith(str(root.resolve()) + "/"), key)
                native_copy.assert_not_called()
                stop.assert_called_once_with(launch.return_value)
            self.assertEqual(sorted(p.name for p in root.iterdir()), ["host", "node_modules"])

    def test_generic_main_uses_only_inventory_and_generic_dispatch(self):
        import urllib.parse
        paths = []
        def open_request(req, **kwargs):
            path = urllib.parse.urlparse(req.full_url).path
            paths.append(path)
            cwd = str(launch.call_args.kwargs["cwd"])
            replies = {
                "/openapi.json": {"paths": {"/api/plugin": {"get": {"operationId": "plugin.list"}}}},
                "/api/plugin": {"location": {"directory": cwd}, "data": [
                    {"id": name, "state": {"status": "active"}} for name in host.PLUGIN_IDS | {"fixture.observer"}]},
            }
            response = io.StringIO(json.dumps(replies[path]))
            response.status = 200
            return response
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            binary = root / "host"
            binary.touch()
            dependencies = root / "node_modules"
            package = dependencies / "@opencode/plugin"
            package.mkdir(parents=True)
            (package / "package.json").write_text('{"version":"2.0.4"}')
            with patch.object(host, "network_prefix", return_value=["sandbox"]), \
                 patch.object(host.subprocess, "run", return_value=Mock(stdout="opencode v2.0.18")), \
                 patch.object(host.subprocess, "Popen") as launch, patch.object(host, "stop") as stop, \
                 patch.object(host, "read_address", return_value="http://127.0.0.1:1234"), \
                 patch.object(host.urllib.request, "build_opener", return_value=Mock(open=open_request)), \
                 patch("opencode_v2_loopback.prove_generic_dispatch") as generic, \
                 patch("opencode_v2_loopback.prove_dispatch") as review, patch("sys.stdout", new_callable=io.StringIO) as output:
                host.main([str(binary), str(dependencies), "--host-version", "2.x", "--temp-root", directory, "--generic-task-only"])
                self.assertIn('host opencode v2.0.18', output.getvalue())
                self.assertEqual(paths, ["/openapi.json", "/api/plugin"] * 2)
                self.assertEqual(generic.call_count, 2)
                self.assertEqual(stop.call_count, 2)
                review.assert_not_called()
                self.assertIn("NOT PROVEN: reviewer/refuter/validator; native admission/receipt/capability", output.getvalue())

    def test_sandbox_unavailable_refuses_before_any_process(self):
        with tempfile.TemporaryDirectory() as directory, patch.object(host.sys, "platform", "linux"), \
             patch.object(host.subprocess, "run") as version, patch.object(host.subprocess, "Popen") as launch:
            with self.assertRaisesRegex(RuntimeError, "network denial"):
                host.main(["host", "deps", "--host-version", "2.0.18", "--temp-root", directory, "--generic-task-only"])
            version.assert_not_called()
            launch.assert_not_called()

    def test_startup_partial_line_is_bounded(self):
        selector = Mock()
        selector.select.return_value = [True]
        with patch.object(host.selectors, "DefaultSelector") as factory, \
             patch.object(host.os, "read", return_value=b'{"url":'), \
             patch.object(host.time, "monotonic", side_effect=[0, 0, 0, 21]):
            factory.return_value.__enter__.return_value = selector
            with self.assertRaises(TimeoutError):
                host.read_address(Mock())


class RequestBoundaryTests(unittest.TestCase):
    def test_redirects_never_leave_fixture_or_forward_auth(self):
        import urllib.error
        import urllib.request
        from email.message import Message
        from urllib.response import addinfourl
        for destination in ("https://external.invalid/secret", "http://127.0.0.1:9999/shared", "/same-origin"):
            sent = []
            class FakeHTTP(urllib.request.HTTPHandler):
                def http_open(self, req):
                    sent.append(req)
                    headers = Message()
                    headers["Location"] = destination
                    response = addinfourl(io.BytesIO(b""), headers, req.full_url, 302)
                    response.msg = "Found"
                    return response
            class FakeHTTPS(urllib.request.HTTPSHandler):
                https_open = FakeHTTP.http_open
            build_opener = urllib.request.build_opener
            with self.subTest(destination=destination), patch.object(host.urllib.request, "build_opener", side_effect=lambda *handlers: build_opener(*handlers, FakeHTTP(), FakeHTTPS())):
                request = host.fixture_request("http://127.0.0.1:1234", "Basic fixture-secret")
                with self.assertRaises(urllib.error.HTTPError) as caught:
                    request("/api/plugin")
                caught.exception.close()
                self.assertEqual([req.full_url for req in sent], ["http://127.0.0.1:1234/api/plugin"])

    def test_initial_request_origin_is_checked_before_open(self):
        with patch.object(host.urllib.request, "build_opener") as factory:
            request = host.fixture_request("http://127.0.0.1:1234", "Basic fixture-secret")
            for path in ("https://external.invalid/api", "//127.0.0.1:9999/api", "http://127.0.0.1:9999/api"):
                with self.subTest(path=path), self.assertRaises(ValueError):
                    request(path)
            factory.return_value.open.assert_not_called()


class InstalledActivationTests(unittest.TestCase):
    def fixture(self, root):
        root = Path(root).resolve()
        for name in ('home', 'config/opencode/plugins', 'data', 'state', 'cache', 'tmp', 'run', 'workspace'):
            (root / name).mkdir(parents=True, exist_ok=True)
        config = root / 'config/opencode'
        package = config / 'node_modules/@opencode/plugin'
        package.mkdir(parents=True)
        (package / 'package.json').write_text('{"version":"2.0.4"}')
        (config / 'opencode.json').write_text('{"default_agent":"gentle-orchestrator"}')
        for name in host.PLUGIN_IDS:
            (config / 'plugins' / (name.removeprefix('gentle-ai.') + '.ts')).write_text(name)
        binary = root / 'host'
        binary.touch()
        return [str(binary), str(config / 'node_modules'), '--host-version', '2.x',
                '--temp-root', str(root), '--installed-activation-only',
                '--installed-root', str(root), '--installed-config', str(config),
                '--installed-workspace', str(root / 'workspace')]

    def test_installed_mode_preserves_inputs_and_uses_get_only_exact_location(self):
        with tempfile.TemporaryDirectory() as directory:
            args = self.fixture(directory)
            root = Path(directory).resolve()
            config = root / 'config/opencode'
            paths = []
            def open_request(req, **kwargs):
                paths.append(req.full_url)
                self.assertEqual(req.get_method(), 'GET')
                path = host.urllib.parse.urlsplit(req.full_url)
                self.assertEqual(path.netloc, '127.0.0.1:1234')
                if path.path == '/openapi.json':
                    reply = {'paths': {'/api/plugin': {'get': {'operationId': 'plugin.list'}}}}
                else:
                    self.assertEqual(path.path, '/api/plugin')
                    self.assertEqual(host.urllib.parse.parse_qs(path.query), {'location[directory]': [str(root / 'workspace')]})
                    reply = {'location': {'directory': str(root / 'workspace')}, 'data': [
                        {'id': name, 'state': {'status': 'active'}, 'source': {'type': 'local',
                         'path': str(config / 'plugins' / (name.removeprefix('gentle-ai.') + '.ts'))}}
                        for name in host.PLUGIN_IDS]}
                response = io.StringIO(json.dumps(reply))
                response.status = 200
                return response
            before = {p: p.read_bytes() for p in config.rglob('*') if p.is_file()}
            with patch.object(host, 'network_prefix', return_value=['sandbox']), \
                 patch.object(host.subprocess, 'Popen') as launch, patch.object(host, 'stop') as stop, \
                 patch.object(host, 'read_address', return_value='http://127.0.0.1:1234'), \
                 patch.object(host.urllib.request, 'build_opener', return_value=Mock(open=open_request)), \
                 patch.object(host.shutil, 'copytree') as copy, patch.object(host.shutil, 'copy2') as copy2, \
                 patch.object(host.os, 'symlink') as link, patch.object(host.os, 'killpg'), \
                 patch('sys.stdout', new_callable=io.StringIO) as output:
                launch.return_value.communicate.return_value = ('opencode v2.0.19\n', '')
                launch.return_value.returncode = 0
                host.main(args)
                self.assertIn('host opencode v2.0.19', output.getvalue())
                self.assertIn('PASS: installed restart:', output.getvalue())
                # Initial launch and restart each read the spec and one inventory.
                self.assertEqual(len(paths), 4)
                self.assertEqual(stop.call_count, 3)
                commands = [call.args[0] for call in launch.call_args_list]
                self.assertTrue(all(command[0] == 'sandbox' for command in commands))
                self.assertEqual(commands[0][-1], '--version')
                self.assertEqual(commands[1][-4:], ['serve', '--stdio', '--port', '0'])
                self.assertEqual(commands[2], commands[1])
                env = launch.call_args.kwargs['env']
                self.assertEqual(env['OPENCODE_CONFIG_DIR'], str(config))
                self.assertEqual(env['HOME'], str(root / 'home'))
                self.assertEqual(launch.call_args.kwargs['cwd'], root / 'workspace')
                self.assertNotIn('OPENCODE_CONFIG_CONTENT', env)
                copy.assert_not_called()
                copy2.assert_not_called()
                link.assert_not_called()
            self.assertEqual(before, {p: p.read_bytes() for p in config.rglob('*') if p.is_file()})

    def test_installed_failures_always_stop_host(self):
        for failure in ('timeout', 'cancel', 'opencode.json', 'plugins/model-variants.ts',
                        'node_modules/@opencode/plugin/package.json', 'source', 'missing-source',
                        'null-source', 'package-source', 'file-mode', 'directory-mode', 'empty-directory',
                        'location', 'route'):
            with self.subTest(failure=failure), tempfile.TemporaryDirectory() as directory:
                args = self.fixture(directory)
                root = Path(directory).resolve()
                def request(path):
                    if path == '/openapi.json':
                        route = '/api/session' if failure == 'route' else '/api/plugin'
                        return {'paths': {route: {'get': {'operationId': 'plugin.list'}}}}
                    if failure == 'timeout':
                        raise TimeoutError('poll timeout')
                    if failure == 'cancel':
                        host.signal.getsignal(host.signal.SIGTERM)(host.signal.SIGTERM, None)
                    if failure.endswith(('.json', '.ts')):
                        (root / 'config/opencode' / failure).write_text('{}')
                    if failure == 'file-mode':
                        target = root / 'config/opencode/plugins/model-variants.ts'
                        target.chmod((target.stat().st_mode & 0o7777) ^ 0o100)
                    if failure == 'directory-mode':
                        target = root / 'config/opencode/node_modules'
                        target.chmod((target.stat().st_mode & 0o7777) ^ 0o010)
                    if failure == 'empty-directory':
                        (root / 'config/opencode/added-empty').mkdir()
                    def source(name):
                        if failure == 'missing-source':
                            return {}
                        if failure == 'null-source':
                            return {'source': None}
                        return {'source': {'type': 'package' if failure == 'package-source' else 'local',
                                'path': '/wrong' if failure == 'source' else str(root / 'config/opencode/plugins' /
                                         (name.removeprefix('gentle-ai.') + '.ts'))}}
                    return {'location': {'directory': '/wrong' if failure == 'location' else str(root / 'workspace')},
                            'data': [{'id': name, 'state': {'status': 'active'}, **source(name)}
                                for name in host.PLUGIN_IDS]}
                with patch.object(host, 'network_prefix', return_value=['sandbox']), \
                     patch.object(host.subprocess, 'Popen') as launch, patch.object(host, 'stop') as stop, \
                     patch.object(host, 'read_address', return_value='http://127.0.0.1:1234'), \
                     patch.object(host, 'fixture_request', return_value=request), patch.object(host.os, 'killpg'):
                    launch.return_value.communicate.return_value = ('opencode v2.0.18', '')
                    launch.return_value.returncode = 0
                    with self.assertRaises((RuntimeError, TimeoutError, ValueError)):
                        host.main(args)
                    self.assertEqual(stop.call_count, 2)

    def test_installed_snapshot_records_entry_types_modes_and_empty_directories(self):
        with tempfile.TemporaryDirectory() as directory:
            self.fixture(directory)
            config = Path(directory).resolve() / 'config/opencode'
            empty = config / 'empty'
            empty.mkdir()
            before = host.installed_hashes(config)
            self.assertIn('.', before)
            self.assertIn('empty', before)
            empty.rmdir()
            empty.touch()
            self.assertNotEqual(before['empty'], host.installed_hashes(config)['empty'])
            target = config / 'opencode.json'
            target.chmod((target.stat().st_mode & 0o7777) ^ 0o100)
            self.assertNotEqual(before['opencode.json'], host.installed_hashes(config)['opencode.json'])

    def test_installed_sandbox_allows_only_runtime_directory_writes(self):
        with patch.object(host.sys, 'platform', 'darwin'), patch.object(host.Path, 'is_file', return_value=True):
            profile = host.network_prefix(Path('/fixture'))[-1]
        self.assertIn('(deny network*)', profile)
        self.assertIn('(deny file-write*)', profile)
        for name in ('data', 'state', 'cache', 'tmp', 'run'):
            self.assertIn('(subpath "/fixture/' + name + '")', profile)
        for name in ('', '/config', '/config/opencode', '/workspace', '/home'):
            self.assertNotIn('(subpath "/fixture' + name + '")', profile)

    def test_installed_snapshot_accepts_internal_npm_links_without_traversing_them(self):
        with tempfile.TemporaryDirectory() as directory:
            self.fixture(directory)
            config = Path(directory).resolve() / 'config/opencode'
            package = config / 'node_modules/arborist'
            package.mkdir()
            target = package / 'bin.js'
            target.write_text('original')
            bindir = config / 'node_modules/.bin'
            bindir.mkdir()
            link = bindir / 'arborist'
            link.symlink_to('../arborist/bin.js')
            (config / 'package-alias').symlink_to(package, target_is_directory=True)
            before = host.installed_hashes(config)
            self.assertIn('node_modules/.bin/arborist', before)
            self.assertIn('node_modules/arborist/bin.js', before)
            self.assertNotIn('package-alias/bin.js', before)
            target.write_text('changed')
            self.assertNotEqual(before, host.installed_hashes(config))
            target.write_text('original')
            replacement = bindir / 'replacement'
            replacement.symlink_to('../arborist/bin.js')
            replacement.replace(link)
            self.assertNotEqual(before, host.installed_hashes(config))
            link.unlink()
            link.symlink_to('../arborist/./bin.js')
            self.assertNotEqual(before, host.installed_hashes(config))
            other = package / 'other.js'
            other.write_text('original')
            before_retarget = host.installed_hashes(config)
            link.unlink()
            link.symlink_to('../arborist/other.js')
            self.assertNotEqual(before_retarget, host.installed_hashes(config))
            # Replacing the link with a regular file also changes entry type.
            link.unlink()
            link.write_text('original')
            self.assertNotEqual(before, host.installed_hashes(config))

    def test_installed_snapshot_rejects_escaping_dangling_and_looping_links(self):
        for kind in ('external', 'dangling', 'loop', 'directory-escape'):
            with self.subTest(kind=kind), tempfile.TemporaryDirectory() as directory:
                self.fixture(directory)
                root = Path(directory).resolve()
                config = root / 'config/opencode'
                link = config / 'link'
                target = {'external': root / 'host', 'dangling': config / 'missing',
                          'loop': link, 'directory-escape': root / 'home'}[kind]
                link.symlink_to(target)
                with self.assertRaises(ValueError):
                    host.installed_hashes(config)

    def test_installed_request_allowlist_rejects_mutations_and_wrong_location(self):
        with patch.object(host.urllib.request, 'build_opener') as opener:
            request = host.fixture_request('http://127.0.0.1:1234', 'Basic private',
                                           allowed_gets={'/openapi.json', '/api/plugin?location%5Bdirectory%5D=%2Ffixture'})
            for path, body in [('/api/session', None), ('/api/model', None), ('/api/shell', {}),
                               ('/api/plugin', None), ('/openapi.json', {})]:
                with self.assertRaises(ValueError):
                    request(path, body)
            opener.return_value.open.assert_not_called()

    def test_installed_cancellation_during_launch_reaps_owned_process(self):
        process = Mock()
        def launch(*args, **kwargs):
            host.signal.getsignal(host.signal.SIGINT)(host.signal.SIGINT, None)
            return process
        with patch.object(host.subprocess, 'Popen', side_effect=launch), \
             patch.object(host, 'stop') as stop, patch.object(host.os, 'killpg') as kill:
            with self.assertRaisesRegex(TimeoutError, 'during process launch'):
                with host.installed_process(['host'], Path('/fixture'), {}):
                    self.fail('cancelled launch must not run requests')
            stop.assert_called_once_with(process)
            kill.assert_called_once_with(process.pid, host.signal.SIGKILL)

    def test_installed_version_timeout_reaps_before_any_server(self):
        with tempfile.TemporaryDirectory() as directory:
            args = self.fixture(directory)
            with patch.object(host, 'network_prefix', return_value=['sandbox']), \
                 patch.object(host.subprocess, 'Popen') as launch, patch.object(host, 'stop') as stop, \
                 patch.object(host.os, 'killpg'):
                launch.return_value.communicate.side_effect = host.subprocess.TimeoutExpired('host', 10)
                with self.assertRaises(host.subprocess.TimeoutExpired):
                    host.main(args)
                self.assertEqual(launch.call_count, 1)
                stop.assert_called_once_with(launch.return_value)

    def test_installed_invalid_inputs_refuse_before_launch(self):
        for failure in ('missing-root', 'different-config', 'symlink', 'wrong-sdk', 'no-sandbox'):
            with self.subTest(failure=failure), tempfile.TemporaryDirectory() as directory:
                args = self.fixture(directory)
                config = Path(directory) / 'config/opencode'
                if failure == 'missing-root':
                    args[args.index('--installed-root') + 1] = 'relative'
                elif failure == 'different-config':
                    args[args.index('--installed-config') + 1] = directory
                elif failure == 'symlink':
                    (config / 'link').symlink_to(Path(directory) / 'host')
                elif failure == 'wrong-sdk':
                    (config / 'node_modules/@opencode/plugin/package.json').write_text('{"version":"2.0.3"}')
                with patch.object(host, 'network_prefix', side_effect=RuntimeError('no sandbox') if failure == 'no-sandbox' else None), \
                     patch.object(host.subprocess, 'Popen') as launch, patch('sys.stderr', new_callable=io.StringIO):
                    with self.assertRaises((SystemExit, ValueError, RuntimeError)):
                        host.main(args)
                    launch.assert_not_called()


class InstalledRestartAndMissingSDKTests(unittest.TestCase):
    fixture = InstalledActivationTests.fixture

    def source(self, root, name):
        return str(Path(root).resolve() / 'config/opencode/plugins' / (name.removeprefix('gentle-ai.') + '.ts'))

    def run_main(self, args, request, extra_patches=()):
        with patch.object(host, 'network_prefix', return_value=['sandbox']), \
             patch.object(host.subprocess, 'Popen') as launch, patch.object(host, 'stop') as stop, \
             patch.object(host, 'read_address', return_value='http://127.0.0.1:1234'), \
             patch.object(host, 'fixture_request', return_value=request), patch.object(host.os, 'killpg'), \
             patch('sys.stdout', new_callable=io.StringIO) as output:
            launch.return_value.communicate.return_value = ('opencode v2.0.19', '')
            launch.return_value.returncode = 0
            launch.return_value.poll.return_value = -15
            try:
                host.main(args)
            finally:
                self.launches, self.stops, self.output = launch.call_count, stop.call_count, output.getvalue()

    def inventory(self, root, status='active', ids=True, error='Plugin failed to load'):
        state = {'status': status}
        if status == 'failed':
            state.update({'error': error, 'ref': 'err_' + status})
        return {'location': {'directory': str(Path(root).resolve() / 'workspace')}, 'data': [
            {**({'id': name} if ids else {}), 'state': dict(state, ref='err_' + name[-4:]) if status == 'failed' else state,
             'source': {'type': 'local', 'path': self.source(root, name)}} for name in sorted(host.PLUGIN_IDS)]}

    def counting_request(self, replies):
        calls = {'inventory': 0}
        def request(path):
            if path == '/openapi.json':
                return {'paths': {'/api/plugin': {'get': {'operationId': 'plugin.list'}}}}
            calls['inventory'] += 1
            return replies(calls['inventory'])
        return request

    def test_restart_relaunches_same_root_and_requires_active_again(self):
        with tempfile.TemporaryDirectory() as directory:
            args = self.fixture(directory)
            request = self.counting_request(lambda n: self.inventory(directory, 'active' if n == 1 else 'failed'))
            with self.assertRaisesRegex(RuntimeError, 'activation failed'):
                self.run_main(args, request)
            # Version probe, initial host, restarted host: all reaped.
            self.assertEqual((self.launches, self.stops), (3, 3))
            self.assertNotIn('PASS', self.output)

    def test_restart_detects_installed_input_change_between_launches(self):
        with tempfile.TemporaryDirectory() as directory:
            args = self.fixture(directory)
            plugin = Path(self.source(directory, 'gentle-ai.skill-registry'))
            def replies(n):
                if n == 2:
                    plugin.write_text('replaced during restart')
                return self.inventory(directory)
            with self.assertRaisesRegex(RuntimeError, 'hashes changed during restart'):
                self.run_main(args, self.counting_request(replies))
            self.assertEqual(self.stops, 3)

    def test_restart_refuses_to_relaunch_over_live_host(self):
        with tempfile.TemporaryDirectory() as directory:
            args = self.fixture(directory)
            request = self.counting_request(lambda n: self.inventory(directory))
            with patch.object(host, 'network_prefix', return_value=['sandbox']), \
                 patch.object(host.subprocess, 'Popen') as launch, patch.object(host, 'stop'), \
                 patch.object(host, 'read_address', return_value='http://127.0.0.1:1234'), \
                 patch.object(host, 'fixture_request', return_value=request), patch.object(host.os, 'killpg'):
                launch.return_value.communicate.return_value = ('opencode v2.0.19', '')
                launch.return_value.returncode = 0
                launch.return_value.poll.return_value = None
                with self.assertRaisesRegex(RuntimeError, 'still running after initial'):
                    host.main(args)
                self.assertEqual(launch.call_count, 2)

    def missing_fixture(self, directory):
        args = self.fixture(directory)
        sdk = Path(directory).resolve() / 'config/opencode/node_modules/@opencode/plugin'
        for path in sorted(sdk.rglob('*'), reverse=True):
            path.unlink()
        sdk.rmdir()
        return args + ['--installed-sdk-missing']

    def write_log(self, directory, cause):
        log = Path(directory).resolve() / 'data/opencode/log/host.log'
        log.parent.mkdir(parents=True, exist_ok=True)
        log.write_text(''.join(f'level=WARN message="failed to load plugin" ref=err_{name[-4:]} cause="{cause}"\n'
                               for name in host.PLUGIN_IDS))

    def test_missing_sdk_reports_logged_resolution_cause_without_restart(self):
        with tempfile.TemporaryDirectory() as directory:
            args = self.missing_fixture(directory)
            self.write_log(directory, "Die(ResolveMessage: Cannot find package '@opencode/plugin' imported from x)")
            request = self.counting_request(lambda n: self.inventory(directory, 'failed', ids=False))
            self.run_main(args, request)
            self.assertIn('PASS: missing SDK refused:', self.output)
            self.assertEqual(self.output.count("Cannot find package '@opencode/plugin'"), 4)
            self.assertEqual((self.launches, self.stops), (2, 2))

    def test_missing_sdk_rejects_unexpected_or_unlogged_cause(self):
        for cause in ('SyntaxError: unexpected token', None):
            with self.subTest(cause=cause), tempfile.TemporaryDirectory() as directory:
                args = self.missing_fixture(directory)
                if cause:
                    self.write_log(directory, cause)
                request = self.counting_request(lambda n: self.inventory(directory, 'failed', ids=False))
                with self.assertRaisesRegex(RuntimeError, 'unexpected or unlogged reason'):
                    self.run_main(args, request)
                self.assertNotIn('PASS', self.output)

    def test_missing_sdk_active_plugin_is_an_unsafe_finding(self):
        with tempfile.TemporaryDirectory() as directory:
            args = self.missing_fixture(directory)
            request = self.counting_request(lambda n: self.inventory(directory))
            with self.assertRaisesRegex(RuntimeError, 'active without the installed SDK'):
                self.run_main(args, request)
            self.assertEqual(self.stops, 2)

    def test_missing_sdk_mode_refuses_present_sdk_before_launch(self):
        with tempfile.TemporaryDirectory() as directory:
            args = self.fixture(directory) + ['--installed-sdk-missing']
            with patch.object(host, 'network_prefix', return_value=['sandbox']), \
                 patch.object(host.subprocess, 'Popen') as launch:
                with self.assertRaisesRegex(ValueError, 'SDK to be absent'):
                    host.main(args)
                launch.assert_not_called()

    def test_missing_sdk_flag_requires_installed_mode(self):
        with tempfile.TemporaryDirectory() as directory, patch('sys.stderr', new_callable=io.StringIO):
            with self.assertRaises(SystemExit):
                host.parse_args(['host', 'deps', '--host-version', '2.x', '--temp-root', directory,
                                 '--installed-sdk-missing'])


class MissingSDKRefusalPollTests(unittest.TestCase):
    expected = {'/fixture/one.ts': 'gentle-ai.one', '/fixture/two.ts': 'gentle-ai.two'}

    def poll(self, replies):
        now = [0]
        def sleep(seconds):
            now[0] += seconds
        iterator = iter(replies)
        return host.wait_for_missing_sdk_refusal(lambda: next(iterator), self.expected, timeout=2,
                                                 clock=lambda: now[0], sleep=sleep)

    @staticmethod
    def entry(path, status, error='Plugin failed to load', **extra):
        state = {'status': status, **({'error': error, 'ref': 'err_' + path[-6:-3]} if status == 'failed' else {})}
        return {'source': {'type': 'local', 'path': path}, 'state': state, **extra}

    def test_idless_failures_are_matched_by_installed_source(self):
        cold = {'data': [self.entry('/fixture/one.ts', 'failed')]}
        done = {'data': [self.entry(path, 'failed') for path in self.expected] +
                [{'id': 'builtin', 'source': {'type': 'builtin'}, 'state': {'status': 'active'}},
                 self.entry('/other/plugin.ts', 'failed')]}
        failed = self.poll([cold, done])
        self.assertEqual(set(failed), set(self.expected.values()))
        self.assertEqual(failed['gentle-ai.one']['ref'], 'err_one')

    def test_active_managed_plugin_is_never_a_refusal(self):
        for item in (self.entry('/fixture/two.ts', 'active'), {'id': 'gentle-ai.two', 'state': {'status': 'active'}}):
            with self.subTest(item=item), self.assertRaisesRegex(RuntimeError, 'active without the installed SDK'):
                self.poll([{'data': [self.entry('/fixture/one.ts', 'failed'), item]}])

    def test_failure_without_reason_or_duplicate_is_rejected(self):
        with self.assertRaisesRegex(RuntimeError, 'without a reported reason'):
            self.poll([{'data': [self.entry(path, 'failed', error='') for path in self.expected]}])
        with self.assertRaisesRegex(RuntimeError, 'duplicate'):
            self.poll([{'data': [self.entry('/fixture/one.ts', 'failed')] * 2}])

    def test_absent_refusal_is_bounded(self):
        with self.assertRaisesRegex(TimeoutError, 'not observable'):
            self.poll([{'data': []}] * 10)

    def test_failure_causes_extract_bounded_cause_for_each_ref(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for name in ('data', 'state', 'cache', 'tmp'):
                (root / name).mkdir()
            (root / 'state/log.txt').write_text('ref=err_a cause="Cannot find package x"\nref=err_b other\n')
            (root / 'cache/huge.log').write_text('ref=err_c cause="too large"' + ' ' * 64)
            causes = host.failure_causes(root, ['err_a', 'err_b', 'err_c', 'err_d'], limit=64)
            self.assertEqual(causes, {'err_a': 'cause="Cannot find package x"', 'err_b': 'ref=err_b other'})


class CleanupTests(unittest.TestCase):
    def test_kill_wait_is_bounded_and_timeout_closes_streams(self):
        process = Mock()
        process.wait.side_effect = host.subprocess.TimeoutExpired("host", 5)
        with patch.object(host.os, "killpg") as kill:
            with self.assertRaises(host.subprocess.TimeoutExpired):
                host.stop(process)
        self.assertEqual([call.kwargs for call in process.wait.call_args_list], [{"timeout": 5}, {"timeout": 5}])
        self.assertEqual([call.args[1] for call in kill.call_args_list], [host.signal.SIGTERM, host.signal.SIGKILL])
        for stream in (process.stdin, process.stdout, process.stderr):
            stream.close.assert_called_once()

    def test_exit_race_after_term_timeout_still_reaps_and_closes(self):
        process = Mock()
        process.wait.side_effect = [host.subprocess.TimeoutExpired("host", 5), 0]
        with patch.object(host.os, "killpg", side_effect=[None, ProcessLookupError]):
            host.stop(process)
        self.assertEqual([call.kwargs for call in process.wait.call_args_list], [{"timeout": 5}, {"timeout": 5}])
        for stream in (process.stdin, process.stdout, process.stderr):
            stream.close.assert_called_once()


class LoopbackNetworkTests(unittest.TestCase):
    def test_generic_provider_rejects_review_request(self):
        from opencode_v2_loopback import local_provider
        import urllib.request
        import urllib.error
        with local_provider(generic_task_only=True) as (url, requests, failures):
            opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
            data = json.dumps({"model": "parent", "messages": [{"role": "user", "content": "NEGATIVE_REVIEW"}]}).encode()
            with self.assertRaises(urllib.error.HTTPError) as caught:
                opener.open(urllib.request.Request(url + "/v1/chat/completions", data=data), timeout=2)
            caught.exception.close()
            self.assertEqual(failures, ["review disabled in generic-task-only fixture"])
            self.assertEqual(len(requests), 1)

    def test_unexpected_target_fails_fixture(self):
        from opencode_v2_loopback import local_provider
        import urllib.request
        import urllib.error
        with local_provider() as (url, requests, failures):
            opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
            with self.assertRaises(urllib.error.HTTPError) as caught:
                opener.open(url + "/unexpected", timeout=2)
            caught.exception.close()
            self.assertEqual(requests, [])
            self.assertEqual(failures, ["unexpected GET target: /unexpected"])

    def test_catalog_is_fixture_only(self):
        from opencode_v2_loopback import local_provider
        import urllib.request
        with local_provider() as (url, requests, failures):
            opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
            with opener.open(url + "/catalog/api.json", timeout=2) as response:
                self.assertEqual(response.read(), b"{}")
            self.assertEqual(requests, [])
            self.assertEqual(failures, [])


HEADER = "GENTLE_AI_REVIEW_PROVIDER_MATERIALIZATION"


def review_step(**overrides):
    step = {"name": "lens", "agent": "review-risk", "prompt": "GENTLE_AI_REVIEW_LENS_CONTEXT {\"lineage\":\"l\"}",
            "child": "{\"subject_hash\":\"s\"}", "expect": "admitted", "tool_contains": ["\"state\":\"approved\""]}
    step.update(overrides)
    return step


class ReviewScriptTests(unittest.TestCase):
    def script(self, *steps):
        from opencode_v2_loopback import ReviewScript
        return ReviewScript(list(steps))

    def test_parent_dispatches_the_marked_step_as_a_foreground_subagent(self):
        script = self.script(review_step(), review_step(name="second", agent="review-refuter", prompt="P2"))
        reply = script.reply({"model": "parent", "messages": [{"role": "user", "content": [{"type": "text", "text": "REVIEW_STEP 1"}]}]})
        args = json.loads(reply["tool_calls"][0]["function"]["arguments"])
        self.assertEqual(reply["tool_calls"][0]["function"]["name"], "subagent")
        self.assertEqual((args["agent"], args["prompt"]), ("review-refuter", "P2"))
        self.assertNotIn("background", args)
        self.assertNotIn("sessionID", args)
        self.assertEqual(script.current, 1)

    def test_parent_forwards_requested_refusal_arguments(self):
        script = self.script(review_step(background=True, session_id="ses_fixture"))
        reply = script.reply({"model": "parent", "messages": [{"role": "user", "content": "REVIEW_STEP 0"}]})
        args = json.loads(reply["tool_calls"][0]["function"]["arguments"])
        self.assertIs(args["background"], True)
        self.assertEqual(args["sessionID"], "ses_fixture")

    def test_parent_finishes_after_tool_result(self):
        script = self.script(review_step())
        reply = script.reply({"model": "parent", "messages": [{"role": "user", "content": "REVIEW_STEP 0"}, {"role": "tool", "content": "x"}]})
        self.assertEqual(reply, {"content": "PARENT_DONE"})

    def test_child_replays_the_dispatched_step_or_its_http_error(self):
        script = self.script(review_step(child="PAYLOAD"), review_step(child=None, child_http_error=True))
        with self.assertRaisesRegex(ValueError, "no dispatched"):
            script.reply({"model": "child", "messages": []})
        script.reply({"model": "parent", "messages": [{"role": "user", "content": "REVIEW_STEP 0"}]})
        self.assertEqual(script.reply({"model": "child", "messages": []}), {"content": "PAYLOAD"})
        script.reply({"model": "parent", "messages": [{"role": "user", "content": "REVIEW_STEP 1"}]})
        self.assertEqual(script.reply({"model": "child", "messages": []}), {"http_error": 400})

    def test_unmarked_parent_and_unknown_model_refuse(self):
        script = self.script(review_step())
        with self.assertRaisesRegex(ValueError, "step marker"):
            script.reply({"model": "parent", "messages": [{"role": "user", "content": "hello"}]})
        with self.assertRaisesRegex(ValueError, "step marker"):
            script.reply({"model": "parent", "messages": [{"role": "user", "content": "REVIEW_STEP 9"}]})
        with self.assertRaisesRegex(ValueError, "unexpected model"):
            script.reply({"model": "external", "messages": []})

    def test_provider_serves_scripted_http_error_without_fixture_failure(self):
        from opencode_v2_loopback import ReviewScript, local_provider
        import urllib.request
        import urllib.error
        script = ReviewScript([review_step(child_http_error=True)])
        script.current = 0
        with local_provider(review_script=script, max_requests=3) as (url, requests, failures):
            opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
            data = json.dumps({"model": "child", "messages": []}).encode()
            with self.assertRaises(urllib.error.HTTPError) as caught:
                opener.open(urllib.request.Request(url + "/v1/chat/completions", data=data), timeout=2)
            self.assertEqual(caught.exception.code, 400)
            caught.exception.close()
            self.assertEqual(failures, [])
            self.assertEqual(len(requests), 1)


class ReviewFixtureTests(unittest.TestCase):
    def test_global_config_defines_six_deny_all_review_agents_and_observer(self):
        from opencode_v2_loopback import REVIEW_AGENTS, prepare_review_fixture
        with tempfile.TemporaryDirectory() as directory:
            config = Path(directory) / "config/opencode"
            (config / "plugins").mkdir(parents=True)
            log = prepare_review_fixture(config, "http://127.0.0.1:1234", Path(directory) / "obs.jsonl")
            settings = json.loads((config / "opencode.json").read_text())
            self.assertEqual(len(REVIEW_AGENTS), 6)
            for agent in REVIEW_AGENTS:
                self.assertEqual(settings["agents"][agent]["mode"], "subagent")
                self.assertEqual(settings["agents"][agent]["permissions"], [{"action": "*", "resource": "*", "effect": "deny"}])
            self.assertEqual(settings["agents"]["fixture-parent"]["permissions"][-1],
                             {"action": "subagent", "resource": "*", "effect": "allow"})
            self.assertIn(json.dumps(str(log)), (config / "plugins/zz-fixture-observer.ts").read_text())

    def test_authority_digest_tracks_content_and_absence(self):
        from opencode_v2_loopback import authority_digest
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory) / "lineage"
            self.assertEqual(authority_digest(root), "absent")
            (root / "sub").mkdir(parents=True)
            (root / "sub/state.json").write_text("one")
            first = authority_digest(root)
            self.assertEqual(authority_digest(root), first)
            (root / "sub/state.json").write_text("two")
            self.assertNotEqual(authority_digest(root), first)

    def test_message_text_joins_text_parts(self):
        from opencode_v2_loopback import message_text
        self.assertEqual(message_text("plain"), "plain")
        self.assertEqual(message_text([{"type": "text", "text": "a"}, {"type": "image"}, {"type": "text", "text": "b"}]), "ab")
        self.assertEqual(message_text(None), "")


class ReviewStepCheckTests(unittest.TestCase):
    def check(self, step, **observed):
        from opencode_v2_loopback import check_review_step
        values = {"child_prompts": [HEADER + " {\"task_prompt\":\"x\"}\nprovider"], "tool_texts": ["{\"state\":\"approved\"}"],
                  "observed_texts": [], "before": "a", "after": "b"}
        values.update(observed)
        return check_review_step(step, **values)

    def test_admitted_step_requires_materialized_child_and_bound_parent_result(self):
        self.assertEqual(self.check(review_step()), [])

    def test_admitted_step_tolerates_only_the_observed_host_subagent_preamble(self):
        from opencode_v2_loopback import HOST_SUBAGENT_PREAMBLE
        materialized = HEADER + " {\"task_prompt\":\"x\"}\nprovider"
        self.assertEqual(self.check(review_step(), child_prompts=[HOST_SUBAGENT_PREAMBLE + materialized]), [])
        self.assertIn("child prompt is not Go-materialized",
                      self.check(review_step(), child_prompts=["Injected instruction\n" + materialized]))

    def test_admitted_step_rejects_raw_host_prompt_and_unrewritten_child_output(self):
        step = review_step()
        self.assertIn("child prompt is not Go-materialized", self.check(step, child_prompts=[step["prompt"]]))
        injected = review_step(prompt=step["prompt"] + "\nHOST_INJECTED", host_injected=["HOST_INJECTED"])
        self.assertEqual(self.check(injected, child_prompts=[HEADER + " " + step["prompt"]]), [])
        self.assertIn("child prompt carries host-authored prompt bytes", self.check(injected, child_prompts=[HEADER + " x\nHOST_INJECTED"]))
        self.assertIn("parent received the raw child output", self.check(step, tool_texts=["{\"state\":\"approved\"} " + step["child"]]))
        self.assertIn("parent result lacks '\"state\":\"approved\"'", self.check(step, tool_texts=["other"]))
        self.assertIn("authority unchanged by an admitted step", self.check(step, after="a"))
        self.assertIn("expected exactly one child request, observed 2", self.check(step, child_prompts=["x", "y"]))

    def test_refused_step_requires_unchanged_authority_and_visible_refusal(self):
        step = review_step(expect="refused", child_requests=0, tool_contains=[])
        refusal = "opencode_review_transport_relay_refused"
        self.assertEqual(self.check(step, child_prompts=[], tool_texts=[refusal], after="a"), [])
        self.assertEqual(self.check(step, child_prompts=[], tool_texts=[], observed_texts=[refusal], after="a"), [])
        self.assertIn("authority changed by a refused step", self.check(step, child_prompts=[], tool_texts=[refusal]))
        self.assertIn("refusal was not observed", self.check(step, child_prompts=[], tool_texts=["ok"], after="a"))
        self.assertIn("expected 0 child requests, observed 1", self.check(step, tool_texts=[refusal], after="a"))
        self.assertIn("parent received the raw child output",
                      self.check(review_step(expect="refused", tool_contains=[]), tool_texts=[refusal + review_step()["child"]], after="a"))

    def test_refused_step_with_a_reason_requires_that_bounded_reason(self):
        step = review_step(expect="refused", child_requests=0, tool_contains=[], reason="agent_mismatch")
        refusal = "opencode_review_transport_relay_refused"
        self.assertEqual(self.check(step, child_prompts=[], tool_texts=[refusal + " (reason: agent_mismatch)"], after="a"), [])
        self.assertEqual(self.check(step, child_prompts=[], tool_texts=[], observed_texts=['{"reason": "agent_mismatch", "code": "' + refusal + '"}'], after="a"), [])
        self.assertIn("refusal reason 'agent_mismatch' was not observed",
                      self.check(step, child_prompts=[], tool_texts=[refusal + " (reason: relay_unavailable)"], after="a"))
        self.assertIn("refusal reason 'agent_mismatch' was not observed",
                      self.check(step, child_prompts=[], tool_texts=[refusal], after="a"))


class ReviewModeArgumentTests(unittest.TestCase):
    def test_review_mode_requires_existing_absolute_inputs(self):
        with tempfile.TemporaryDirectory() as root:
            scenario, shim, project = Path(root) / "s.json", Path(root) / "gentle-ai", Path(root) / "host"
            scenario.write_text("{}")
            shim.write_text("#!/bin/sh\n")
            project.mkdir()
            base = ["host", "deps", "--host-version", "2.x", "--temp-root", root]
            args = host.parse_args(base + ["--review-scenario", str(scenario), "--gentle-ai", str(shim),
                                           "--host-project", str(project), "--evidence", str(Path(root) / "e.json")])
            self.assertEqual(args.host_project, project.resolve())
            for extra in (["--review-scenario", str(scenario)],
                          ["--gentle-ai", str(shim), "--host-project", str(project)],
                          ["--review-scenario", "relative.json", "--gentle-ai", str(shim), "--host-project", str(project), "--evidence", str(Path(root) / "e.json")],
                          ["--review-scenario", str(scenario), "--gentle-ai", str(shim), "--host-project", str(project), "--evidence", str(Path(root) / "e.json"), "--generic-task-only"]):
                with self.subTest(extra=extra), patch("sys.stderr", new_callable=io.StringIO), self.assertRaises(SystemExit):
                    host.parse_args(base + extra)

    def test_capability_gate_label_is_review_only_and_defaults_to_stubbed(self):
        with tempfile.TemporaryDirectory() as root:
            scenario, shim, project = Path(root) / "s.json", Path(root) / "gentle-ai", Path(root) / "host"
            scenario.write_text("{}")
            shim.write_text("#!/bin/sh\n")
            project.mkdir()
            base = ["host", "deps", "--host-version", "2.x", "--temp-root", root]
            review = ["--review-scenario", str(scenario), "--gentle-ai", str(shim), "--host-project", str(project),
                      "--evidence", str(Path(root) / "e.json")]
            self.assertEqual(host.parse_args(base + review).capability_gate, "stubbed")
            self.assertEqual(host.parse_args(base + review + ["--capability-gate", "real"]).capability_gate, "real")
            self.assertIn("gate itself not proven", host.review_scope("stubbed"))
            self.assertIn("capability gate real", host.review_scope("real"))
            self.assertNotIn("not proven", host.review_scope("real"))
            for extra in (review + ["--capability-gate", "open"], ["--capability-gate", "real"]):
                with self.subTest(extra=extra), patch("sys.stderr", new_callable=io.StringIO), self.assertRaises(SystemExit):
                    host.parse_args(base + extra)

if __name__ == "__main__":
    unittest.main()
