#!/usr/bin/env python3
"""Isolated released-host activation and negative shell-hook smoke; no model calls."""
from contextlib import ExitStack
import base64
import json
import os
from pathlib import Path
import selectors
import shutil
import signal
import subprocess
import sys
import tempfile
import time
import urllib.parse
import urllib.request

PLUGIN_IDS = {"gentle-ai." + name for name in (
    "model-variants", "skill-registry", "telemetry-runtime",
    "opencode-review-transport", "sdd-task-result-artifacts",
)}
DECLARATION = "gentle-ai.opencode-relay/v2-staged"


def wait_for_plugins(fetch, expected, timeout=15, clock=time.monotonic, sleep=time.sleep):
    deadline = clock() + timeout
    while clock() < deadline:
        response = fetch()
        entries = response["data"]
        if any(item["state"]["status"] == "failed" for item in entries):
            raise RuntimeError("plugin activation failed: " + json.dumps(entries))
        ids = [item.get("id") for item in entries]
        if len(ids) != len(set(ids)):
            raise RuntimeError("duplicate active plugin identity")
        if expected.issubset(set(ids)) and all(item["state"]["status"] == "active" for item in entries):
            return response
        sleep(0.25)
    raise TimeoutError("plugin activation did not complete within bounded polling: " + json.dumps(response))


def stop(process):
    try:
        os.killpg(process.pid, signal.SIGTERM)
    except ProcessLookupError:
        pass
    try:
        process.wait(timeout=5)
    except subprocess.TimeoutExpired:
        os.killpg(process.pid, signal.SIGKILL)
        process.wait()
    for stream in (process.stdin, process.stdout, process.stderr):
        if stream:
            stream.close()


def main():
    loopback = len(sys.argv) == 5 and sys.argv[3] == "--loopback"
    if len(sys.argv) != 3 and not loopback:
        raise SystemExit("usage: test-opencode-v2-host.py <2.0.4-binary> <isolated-node_modules> [--loopback <native-gentle-ai>]")
    if loopback and (sys.platform != "darwin" or not Path("/usr/bin/sandbox-exec").is_file()):
        raise RuntimeError("loopback conformance requires verified per-process network denial")
    binary = Path(sys.argv[1]).resolve(strict=True)
    dependencies = Path(sys.argv[2]).resolve(strict=True)
    package = json.loads((dependencies / "@opencode/plugin/package.json").read_text())
    assert package["version"] == "2.0.4", "requires the released SDK dependency fixture"
    assets = Path(__file__).resolve().parents[1] / "internal/assets/opencode/plugins-v2"
    base = "/private/tmp" if sys.platform == "darwin" else None
    # Separate fixtures prove each discovery scope without duplicate plugin IDs.
    for scope in ("global", "project"):
        with tempfile.TemporaryDirectory(prefix="gentle-ai-opencode-v2-host-", dir=base) as directory, ExitStack() as stack:
            root = Path(directory).resolve()
            for name in ("home", "config", "data", "state", "cache", "tmp", "project"):
                (root / name).mkdir()
            shutil.copytree(dependencies, root / "node_modules", symlinks=True)
            (root / "package.json").write_text('{"private":true,"type":"module"}\n')
            config = root / "config/opencode" if scope == "global" else root / "project/.opencode"
            shutil.copytree(assets, config / "plugins")
            provider = None
            if loopback:
                from opencode_v2_loopback import local_provider, prepare_fixture
                provider, provider_requests, provider_failures = stack.enter_context(local_provider())
                observation_log = prepare_fixture(root, config / "plugins", provider)
                (root / "bin").mkdir()
                shutil.copy2(Path(sys.argv[4]).resolve(strict=True), root / "bin/gentle-ai")
            env = {
                "HOME": str(root / "home"), "XDG_CONFIG_HOME": str(root / "config"),
                "XDG_DATA_HOME": str(root / "data"), "XDG_STATE_HOME": str(root / "state"),
                "XDG_CACHE_HOME": str(root / "cache"), "TMPDIR": str(root / "tmp"),
                "OPENCODE_CONFIG_DIR": str(root / "config/opencode"),
                "OPENCODE_TEST_HOME": str(root / "home"), "PATH": "/usr/bin:/bin",
                "SHELL": "/bin/sh", "TERM": "dumb", "DO_NOT_TRACK": "1",
                "OPENCODE_PASSWORD": "isolated-conformance-only",
            }
            prefix = []
            if loopback:
                env.update({"OPENCODE_MODELS_URL": provider + "/catalog", "HTTP_PROXY": provider, "HTTPS_PROXY": provider,
                            "NO_PROXY": "127.0.0.1,localhost", "PATH": str(root / "bin") + ":/usr/bin:/bin"})
                profile = '(version 1)(allow default)(deny network*)(allow network-inbound (local ip "localhost:*"))(allow network-outbound (remote ip "localhost:*"))'
                prefix = ["/usr/bin/sandbox-exec", "-p", profile]
            version = subprocess.run([str(binary), "--version"], cwd=root / "project", env=env,
                                     capture_output=True, text=True, timeout=10, check=True)
            assert version.stdout.strip() == "opencode v2.0.4"
            process = subprocess.Popen(
                prefix + [str(binary), "serve", "--stdio", "--port", "0"], cwd=root / "project", env=env,
                stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
                text=True, start_new_session=True,
            )
            try:
                with selectors.DefaultSelector() as selector:
                    selector.register(process.stdout, selectors.EVENT_READ)
                    if not selector.select(timeout=20):
                        raise TimeoutError("host lease readiness unavailable")
                    address = json.loads(process.stdout.readline())["url"]
                url = urllib.parse.urlparse(address)
                assert url.scheme == "http" and url.hostname == "127.0.0.1"
                authorization = "Basic " + base64.b64encode(b"opencode:isolated-conformance-only").decode()
                opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))

                def request(path, body=None, timeout=5):
                    data = json.dumps(body).encode() if body is not None else None
                    req = urllib.request.Request(address + path, data=data, headers={
                        "Authorization": authorization, "Content-Type": "application/json",
                    })
                    with opener.open(req, timeout=timeout) as response:
                        return None if response.status == 204 else json.load(response)

                spec = request("/openapi.json")
                plugin_route = next(path for path, methods in spec["paths"].items()
                                    if any(isinstance(value, dict) and value.get("operationId") == "plugin.list"
                                           for value in methods.values()))
                location = ""  # The private host is bound to its fixture working directory.
                inventory = wait_for_plugins(lambda: request(plugin_route + location), PLUGIN_IDS | ({"fixture.observer"} if loopback else set()))
                assert inventory["location"]["directory"] == str(root / "project")
                catalog = request("/api/model" + location)
                assert isinstance(catalog["data"], list)
                assert catalog["location"]["directory"] == str(root / "project")
                shell = request("/api/shell" + location, {
                    "command": "printf '%s' \"$GENTLE_AI_OPENCODE_RELAY_CONTRACT\"",
                    "cwd": str(root / "project"), "timeout": 5000,
                })["data"]
                deadline = time.monotonic() + 10
                while shell["status"] == "running" and time.monotonic() < deadline:
                    time.sleep(0.1)
                    shell = request("/api/shell/" + shell["id"] + location)["data"]
                assert shell["status"] == "exited" and shell["exit"] == 0
                output = request("/api/shell/" + shell["id"] + "/output" + location)["data"]
                assert output["output"] == DECLARATION, "host shell did not receive negative capability declaration"
                print(f"PASS: {scope}: all five plugins active; negative shell declaration; location-scoped catalog")
                if loopback:
                    from opencode_v2_loopback import prove_dispatch
                    prove_dispatch(request, observation_log, provider_requests, provider_failures)
                    print(f"PASS: {scope}: foreground raw child output; inherited sentinels; tool inventory; native review refusal")
            finally:
                stop(process)
    print("NOT PROVEN: reviewer quality or positive native review admission" if loopback else "NOT PROVEN: model/subagent hooks, inherited instructions, review admission")


if __name__ == "__main__":
    main()
