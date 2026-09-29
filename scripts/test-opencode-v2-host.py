#!/usr/bin/env python3
"""Isolated host activation with optional generic or legacy review conformance."""
from contextlib import ExitStack, contextmanager
import argparse
import base64
import hashlib
import json
import os
import re
from pathlib import Path
import selectors
import secrets
import shutil
import signal
import stat
import subprocess
import sys
import tempfile
import time
import urllib.parse
import urllib.request

PLUGIN_IDS = {"gentle-ai." + name for name in (
    "model-variants", "skill-registry", "telemetry-runtime",
    "opencode-review-transport",
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
        try:
            os.killpg(process.pid, signal.SIGTERM)
        except ProcessLookupError:
            pass
        try:
            process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            try:
                os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            process.wait(timeout=5)
    finally:
        with ExitStack() as streams:
            for stream in (process.stdin, process.stdout, process.stderr):
                if stream:
                    streams.callback(stream.close)


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def fixture_request(address, authorization, allowed_gets=None):
    origin = urllib.parse.urlsplit(address)
    if (origin.scheme != "http" or origin.hostname != "127.0.0.1" or not origin.port
            or origin.username is not None or origin.password is not None
            or origin.path not in ("", "/") or origin.query or origin.fragment):
        raise ValueError("invalid fixture origin")
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect())

    def request(path, body=None, timeout=5):
        if allowed_gets is not None and (body is not None or path not in allowed_gets):
            raise ValueError("installed activation permits only allowlisted GET requests")
        target = urllib.parse.urljoin(address, path)
        parsed = urllib.parse.urlsplit(target)
        if (not path.startswith("/") or path.startswith("//")
                or (parsed.scheme, parsed.netloc) != (origin.scheme, origin.netloc)):
            raise ValueError("request must remain on the exact fixture origin")
        data = json.dumps(body).encode() if body is not None else None
        req = urllib.request.Request(target, data=data, headers={
            "Authorization": authorization, "Content-Type": "application/json",
        })
        with opener.open(req, timeout=timeout) as response:
            return None if response.status == 204 else json.load(response)

    return request


def validate_host_version(stdout, selector):
    match = re.fullmatch(r"opencode v(2\.[0-9]+\.[0-9]+)", stdout.strip())
    if match is None or (selector != "2.x" and match[1] != selector):
        raise RuntimeError(f"unexpected host version: {stdout!r}; expected {selector}")
    return match[1]


def parse_args(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary")
    parser.add_argument("dependencies", help="existing isolated node_modules with SDK 2.0.4")
    parser.add_argument("--host-version", required=True)
    parser.add_argument("--temp-root", default=os.environ.get("TMPDIR"))
    modes = parser.add_mutually_exclusive_group()
    modes.add_argument("--loopback", metavar="NATIVE_GENTLE_AI")
    modes.add_argument("--generic-task-only", action="store_true")
    modes.add_argument("--installed-activation-only", action="store_true")
    for name in ("root", "config", "workspace"):
        parser.add_argument("--installed-" + name)
    args = parser.parse_args(argv)
    if args.host_version != "2.x" and not re.fullmatch(r"2\.[0-9]+\.[0-9]+", args.host_version):
        parser.error("--host-version must be 2.x or an exact V2 release, such as 2.0.18")
    if not args.temp_root:
        parser.error("--temp-root or an explicitly provided TMPDIR is required")
    root = Path(args.temp_root).expanduser()
    if not root.is_absolute() or not root.is_dir() or not os.access(root, os.W_OK | os.X_OK):
        parser.error("temporary root must be an existing writable absolute directory")
    args.temp_root = root.resolve(strict=True)
    if args.installed_activation_only:
        for name in ("binary", "dependencies", "installed_root", "installed_config", "installed_workspace"):
            value = getattr(args, name)
            if not value or not Path(value).is_absolute() or not Path(value).exists():
                parser.error(name + " must be an explicit existing absolute path")
            setattr(args, name, Path(value).resolve(strict=True))
        if (args.installed_root != args.temp_root
                or args.installed_config != args.installed_root / "config/opencode"
                or args.installed_workspace != args.installed_root / "workspace"
                or args.dependencies != args.installed_config / "node_modules"):
            parser.error("installed paths must match the existing Go fixture layout")
    elif any((args.installed_root, args.installed_config, args.installed_workspace)):
        parser.error("installed paths require --installed-activation-only")
    return args


def network_prefix(write_root=None):
    if sys.platform != "darwin" or not Path("/usr/bin/sandbox-exec").is_file():
        raise RuntimeError("loopback conformance requires verified per-process network denial")
    profile = '(version 1)(allow default)(deny network*)(allow network-inbound (local ip "localhost:*"))(allow network-outbound (remote ip "localhost:*"))'
    if write_root is not None:
        # Installed configuration, SDK, plugins, home, and workspace are read-only.
        # A host requiring config writes must fail rather than weaken isolation.
        runtime_paths = " ".join('(subpath ' + json.dumps(str(write_root / name)) + ')'
                                 for name in ("data", "state", "cache", "tmp", "run"))
        profile += '(deny file-write*)(allow file-write* ' + runtime_paths + ' (literal "/dev/null"))'
    return ["/usr/bin/sandbox-exec", "-p", profile]


def read_address(process, timeout=20):
    # A readable partial line must not turn startup into an unbounded readline.
    deadline, data = time.monotonic() + timeout, b""
    with selectors.DefaultSelector() as selector:
        selector.register(process.stdout, selectors.EVENT_READ)
        while time.monotonic() < deadline:
            if not selector.select(timeout=max(0, deadline - time.monotonic())):
                break
            chunk = os.read(process.stdout.fileno(), 4096)
            if not chunk:
                raise RuntimeError("host exited before lease readiness")
            data += chunk
            if len(data) > 65536:
                raise RuntimeError("host lease readiness exceeded byte bound")
            if b"\n" in data:
                return json.loads(data.split(b"\n", 1)[0])["url"]
    raise TimeoutError("host lease readiness unavailable")


def installed_hashes(config):
    """Snapshot all installed entries, including root and empty directories."""
    hashes = {}
    paths = [config]
    for directory, directories, files in os.walk(config, followlinks=False):
        paths.extend(Path(directory) / name for name in directories + files)
    for path in sorted(paths):
        info = path.lstat()
        mode = info.st_mode
        if stat.S_ISLNK(mode):
            try:
                target = path.resolve(strict=True)
                target.relative_to(config)
            except (OSError, RuntimeError, ValueError) as error:
                raise ValueError("installed symlink target must exist inside config: " + str(path)) from error
            # Record link identity and text, not followed bytes. The ordinary walk
            # snapshots its real target; directory links are never traversed.
            digest = (os.readlink(path), str(target), info.st_dev, info.st_ino)
        elif stat.S_ISREG(mode) or stat.S_ISDIR(mode):
            digest = hashlib.sha256(path.read_bytes()).hexdigest() if stat.S_ISREG(mode) else None
        else:
            raise ValueError("installed inputs must be regular files or directories: " + str(path))
        hashes[str(path.relative_to(config))] = (stat.S_IFMT(mode), stat.S_IMODE(mode), digest)
    required = {"opencode.json", "node_modules/@opencode/plugin/package.json"} | {
        "plugins/" + name.removeprefix("gentle-ai.") + ".ts" for name in PLUGIN_IDS}
    if not required.issubset(hashes) or any(hashes[name][0] != stat.S_IFREG for name in required):
        raise ValueError("installed settings, SDK, and four plugins are required")
    return hashes


@contextmanager
def installed_process(command, workspace, env):
    # Defer cancellation across Popen so every successfully created process is
    # owned by a finally block, including cancellation during --version.
    signals = {signal.SIGINT, signal.SIGTERM, signal.SIGALRM}
    pending = []
    previous_handlers = {sig: signal.signal(sig, lambda signum, frame: pending.append(signum)) for sig in signals}
    process = None
    try:
        process = subprocess.Popen(command, cwd=workspace, env=env, stdin=subprocess.PIPE,
                                   stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
                                   text=True, start_new_session=True)
    finally:
        try:
            for sig, handler in previous_handlers.items():
                signal.signal(sig, handler)
            if pending:
                raise TimeoutError("installed activation cancelled during process launch")
            if process is not None:
                yield process
        finally:
            # Repeated cancellation must not interrupt bounded group cleanup.
            handlers = {sig: signal.signal(sig, signal.SIG_IGN) for sig in signals}
            try:
                if process is not None:
                    try:
                        stop(process)
                    finally:
                        # A parent can exit on TERM while a descendant ignores it.
                        try:
                            os.killpg(process.pid, signal.SIGKILL)
                        except ProcessLookupError:
                            pass
            finally:
                for sig, handler in handlers.items():
                    signal.signal(sig, handler)


def installed_activation(args):
    root, config, workspace = args.installed_root, args.installed_config, args.installed_workspace
    # Deny writes outside this fixture, including binary auto-update locations.
    prefix = network_prefix(root)  # Mandatory even for the version probe.
    before = installed_hashes(config)
    package = json.loads((args.dependencies / "@opencode/plugin/package.json").read_text())
    if package["version"] != "2.0.4":
        raise ValueError("installed activation requires SDK 2.0.4")
    env = {key: str(root / name) for key, name in {
        "HOME": "home", "XDG_CONFIG_HOME": "config", "XDG_DATA_HOME": "data",
        "XDG_STATE_HOME": "state", "XDG_CACHE_HOME": "cache", "XDG_RUNTIME_DIR": "run",
        "TMPDIR": "tmp", "OPENCODE_TEST_HOME": "home"}.items()}
    if not all(Path(value).is_dir() and Path(value).resolve() == Path(value) for value in env.values()):
        raise ValueError("existing isolated HOME/XDG/TMPDIR directories required")
    env.update({"OPENCODE_CONFIG_DIR": str(config), "PATH": "/usr/bin:/bin", "SHELL": "/bin/sh",
                "TERM": "dumb", "DO_NOT_TRACK": "1", "OPENCODE_PASSWORD": secrets.token_urlsafe(32)})
    def cancelled(signum, frame):
        raise TimeoutError("installed activation cancelled or deadline exceeded")
    handlers = {sig: signal.signal(sig, cancelled) for sig in (signal.SIGTERM, signal.SIGINT, signal.SIGALRM)}
    signal.alarm(45)
    try:
        with installed_process(prefix + [str(args.binary), "--version"], workspace, env) as process:
            stdout, _ = process.communicate(timeout=10)
            if process.returncode != 0:
                raise RuntimeError("installed host version probe failed")
            host_version = validate_host_version(stdout, args.host_version)
        with installed_process(prefix + [str(args.binary), "serve", "--stdio", "--port", "0"], workspace, env) as process:
            address = read_address(process)
            authorization = "Basic " + base64.b64encode(("opencode:" + env["OPENCODE_PASSWORD"]).encode()).decode()
            route = "/api/plugin?" + urllib.parse.urlencode({"location[directory]": str(workspace)})
            request = fixture_request(address, authorization, allowed_gets={"/openapi.json", route})
            spec = request("/openapi.json")
            if spec["paths"].get("/api/plugin", {}).get("get", {}).get("operationId") != "plugin.list":
                raise RuntimeError("host does not expose the expected GET plugin route")
            def inventory():
                response = request(route)
                if response["location"]["directory"] != str(workspace):
                    raise RuntimeError("plugin inventory returned a different location")
                for item in response["data"]:
                    if item.get("id") in PLUGIN_IDS:
                        source = item.get("source")
                        expected = config / "plugins" / (item["id"].removeprefix("gentle-ai.") + ".ts")
                        # Plugin.Source documents local provenance as {type: "local", path: string}.
                        if (not isinstance(source, dict) or source.get("type") != "local"
                                or source.get("path") != str(expected.resolve(strict=True))):
                            raise RuntimeError("managed plugin source differs from installed input")
                return response
            wait_for_plugins(inventory, PLUGIN_IDS)
    finally:
        signal.alarm(0)
        for sig, handler in handlers.items():
            signal.signal(sig, handler)
        if installed_hashes(config) != before:
            raise RuntimeError("installed input hashes changed during activation")
    print(f"PASS: installed activation: host opencode v{host_version}; four managed plugins active; installed settings/SDK/assets hashes unchanged")
    print("NOT PROVEN: reviewer/refuter/validator; native admission/receipt/capability")


def main(argv=None):
    args = parse_args(argv)
    if args.installed_activation_only:
        return installed_activation(args)
    loopback, generic = bool(args.loopback), args.generic_task_only
    fixture = loopback or generic
    prefix = network_prefix() if fixture else []
    binary = Path(args.binary).resolve(strict=True)
    dependencies = Path(args.dependencies).resolve(strict=True)
    package = json.loads((dependencies / "@opencode/plugin/package.json").read_text())
    assert package["version"] == "2.0.4", "requires the released SDK dependency fixture"
    assets = Path(__file__).resolve().parents[1] / "internal/assets/opencode/plugins-v2"
    # Separate fixtures prove each discovery scope without duplicate plugin IDs.
    for scope in ("global", "project"):
        with tempfile.TemporaryDirectory(prefix="gentle-ai-opencode-v2-host-", dir=args.temp_root) as directory, ExitStack() as stack:
            root = Path(directory).resolve()
            for name in ("home", "config", "data", "state", "cache", "tmp", "project"):
                (root / name).mkdir()
            shutil.copytree(dependencies, root / "node_modules", symlinks=True)
            (root / "package.json").write_text('{"private":true,"type":"module"}\n')
            config = root / "config/opencode" if scope == "global" else root / "project/.opencode"
            shutil.copytree(assets, config / "plugins")
            provider = None
            if fixture:
                from opencode_v2_loopback import local_provider, prepare_fixture
                provider, provider_requests, provider_failures = stack.enter_context(local_provider(generic_task_only=generic))
                observation_log = prepare_fixture(root, config / "plugins", provider, generic_task_only=generic)
            if loopback:
                (root / "bin").mkdir()
                shutil.copy2(Path(args.loopback).resolve(strict=True), root / "bin/gentle-ai")
            env = {
                "HOME": str(root / "home"), "XDG_CONFIG_HOME": str(root / "config"),
                "XDG_DATA_HOME": str(root / "data"), "XDG_STATE_HOME": str(root / "state"),
                "XDG_CACHE_HOME": str(root / "cache"), "TMPDIR": str(root / "tmp"),
                "OPENCODE_CONFIG_DIR": str(root / "config/opencode"),
                "OPENCODE_TEST_HOME": str(root / "home"), "PATH": "/usr/bin:/bin",
                "SHELL": "/bin/sh", "TERM": "dumb", "DO_NOT_TRACK": "1",
                "OPENCODE_PASSWORD": "isolated-conformance-only",
            }
            if fixture:
                env.update({"OPENCODE_MODELS_URL": provider + "/catalog", "HTTP_PROXY": provider, "HTTPS_PROXY": provider,
                            "NO_PROXY": "127.0.0.1,localhost"})
            if loopback:
                env["PATH"] = str(root / "bin") + ":/usr/bin:/bin"
            version = subprocess.run(prefix + [str(binary), "--version"], cwd=root / "project", env=env,
                                     capture_output=True, text=True, timeout=10, check=True)
            host_version = validate_host_version(version.stdout, args.host_version)
            process = subprocess.Popen(
                prefix + [str(binary), "serve", "--stdio", "--port", "0"], cwd=root / "project", env=env,
                stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
                text=True, start_new_session=True,
            )
            try:
                address = read_address(process)
                authorization = "Basic " + base64.b64encode(b"opencode:isolated-conformance-only").decode()
                request = fixture_request(address, authorization)

                spec = request("/openapi.json")
                plugin_route = next(path for path, methods in spec["paths"].items()
                                    if any(isinstance(value, dict) and value.get("operationId") == "plugin.list"
                                           for value in methods.values()))
                location = ""  # The private host is bound to its fixture working directory.
                inventory = wait_for_plugins(lambda: request(plugin_route + location), PLUGIN_IDS | ({"fixture.observer"} if fixture else set()))
                assert inventory["location"]["directory"] == str(root / "project")
                if generic:
                    from opencode_v2_loopback import prove_generic_dispatch
                    prove_generic_dispatch(request, observation_log, provider_requests, provider_failures)
                    print(f"PASS: {scope}: host opencode v{host_version}; four managed plugins active; generic foreground dispatch; raw child output; hooks; inherited instructions; tool inventory")
                    continue
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
                print(f"PASS: {scope}: host opencode v{host_version}; all four managed plugins active; negative shell declaration; location-scoped catalog")
                if loopback:
                    from opencode_v2_loopback import prove_dispatch
                    prove_dispatch(request, observation_log, provider_requests, provider_failures)
                    print(f"PASS: {scope}: host opencode v{host_version}; foreground raw child output; inherited sentinels; tool inventory; native review refusal")
            finally:
                stop(process)
    if generic:
        print("NOT PROVEN: reviewer/refuter/validator; native admission/receipt/capability")
    else:
        print("NOT PROVEN: reviewer quality or positive native review admission" if loopback else "NOT PROVEN: model/subagent hooks, inherited instructions, review admission")


if __name__ == "__main__":
    main()
