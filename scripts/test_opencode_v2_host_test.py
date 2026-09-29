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
                host.main([str(binary), str(dependencies), "--host-version", "2.0.18", "--temp-root", directory, "--generic-task-only"])
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

if __name__ == "__main__":
    unittest.main()
