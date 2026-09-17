import importlib.util
from pathlib import Path
import unittest

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


class LoopbackNetworkTests(unittest.TestCase):
    def test_unexpected_target_fails_fixture(self):
        from opencode_v2_loopback import local_provider
        import urllib.request
        import urllib.error
        with local_provider() as (url, requests, failures):
            opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
            with self.assertRaises(urllib.error.HTTPError):
                opener.open(url + "/unexpected", timeout=2)
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
