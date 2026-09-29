"""Deterministic test-only provider and observer; never real review admission."""
from contextlib import contextmanager
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import hashlib
import json
from pathlib import Path
import re
import threading
import time

MATERIALIZATION_HEADER = "GENTLE_AI_REVIEW_PROVIDER_MATERIALIZATION"
RELAY_REFUSED = "opencode_review_transport_relay_refused"
REVIEW_AGENTS = ("review-risk", "review-resilience", "review-readability", "review-reliability",
                 "review-refuter", "review-validator")
REVIEW_STEP = re.compile(r"REVIEW_STEP ([0-9]+)")
# Observed on OpenCode 2.0.19: the host prepends this line to every subagent
# prompt, so the child never receives the Go materialization byte-first.
HOST_SUBAGENT_PREAMBLE = "You are a subagent spawned by another session.\n"


def strip_host_preamble(prompt):
    return prompt[len(HOST_SUBAGENT_PREAMBLE):] if prompt.startswith(HOST_SUBAGENT_PREAMBLE) else prompt


def message_text(content):
    """Normalize OpenAI-compatible string or text-part message content."""
    if isinstance(content, str):
        return content
    if isinstance(content, list):
        return "".join(part.get("text", "") for part in content if isinstance(part, dict) and part.get("type") == "text")
    return ""


class ReviewScript:
    """Replays Go-computed review steps: the parent dispatches, the child answers.

    The model is replaced; every byte the child returns was computed by the Go
    test before the host launched, so this never judges a review.
    """

    def __init__(self, steps):
        self.steps = steps
        self.current = None
        self.lock = threading.Lock()

    def reply(self, body):
        model = body["model"]
        with self.lock:
            if model == "child":
                if self.current is None:
                    raise ValueError("child request with no dispatched review step")
                step = self.steps[self.current]
                if step.get("child_http_error"):
                    return {"http_error": 400}
                return {"content": step.get("child") or ""}
            if model != "parent":
                raise ValueError("unexpected model: " + str(model))
            messages = body["messages"]
            if any(message.get("role") == "tool" for message in messages):
                return {"content": "PARENT_DONE"}
            marks = [int(match[1]) for message in messages if message.get("role") == "user"
                     for match in [REVIEW_STEP.search(message_text(message.get("content")))] if match]
            if len(marks) != 1 or not 0 <= marks[0] < len(self.steps):
                raise ValueError("parent request lacks exactly one valid review step marker")
            self.current = marks[0]
            step = self.steps[self.current]
            arguments = {"agent": step["agent"], "description": "Review step " + step["name"], "prompt": step["prompt"]}
            if step.get("background"):
                arguments["background"] = True
            if step.get("session_id"):
                arguments["sessionID"] = step["session_id"]
            return {"tool_calls": [{"index": 0, "id": "review-call-" + str(self.current), "type": "function", "function": {
                "name": "subagent", "arguments": json.dumps(arguments),
            }}]}


def scripted_reply(body, generic_task_only=False):
    if generic_task_only and "NEGATIVE_REVIEW" in json.dumps(body):
        raise ValueError("review disabled in generic-task-only fixture")
    model = body["model"]
    if model == "child":
        return {"content": "RAW_CHILD_SENTINEL"}
    if model != "parent":
        raise ValueError("unexpected model: " + str(model))
    if any(message.get("role") == "tool" for message in body["messages"]):
        return {"content": "PARENT_DONE"}
    negative = any("NEGATIVE_REVIEW" in str(message.get("content", "")) for message in body["messages"])
    return {"tool_calls": [{"index": 0, "id": "fixture-call", "type": "function", "function": {
        "name": "subagent", "arguments": json.dumps({
            "agent": "review-risk" if negative else "fixture-child",
            "description": "Fixture foreground child", "prompt": "CHILD_REQUEST",
        }),
    }}]}


@contextmanager
def local_provider(generic_task_only=False, review_script=None, max_requests=12):
    requests, failures = [], []

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *_args):
            pass

        def reject(self, reason):
            failures.append(reason)
            self.send_error(400, "fixture request refused")

        def do_CONNECT(self):
            self.reject("nonloopback proxy request: " + self.path)

        def do_GET(self):
            if self.path == "/catalog/api.json":
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", "2")
                self.end_headers()
                self.wfile.write(b"{}")
                return
            self.reject("unexpected GET target: " + self.path)

        def do_POST(self):
            if self.path != "/v1/chat/completions":
                return self.reject("unexpected request target: " + self.path)
            length = int(self.headers.get("Content-Length", "0"))
            if not 0 < length <= 1024 * 1024 or len(requests) >= max_requests:
                return self.reject("request bound exceeded")
            try:
                body = json.loads(self.rfile.read(length))
                requests.append(body)
                reply = review_script.reply(body) if review_script else scripted_reply(body, generic_task_only=generic_task_only)
            except (ValueError, KeyError) as error:
                return self.reject(str(error))
            if "http_error" in reply:
                # A scripted provider failure, not a fixture failure.
                payload = json.dumps({"error": {"message": "fixture child provider error", "type": "invalid_request_error"}}).encode()
                self.send_response(reply["http_error"])
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(payload)))
                self.send_header("Connection", "close")
                self.end_headers()
                self.wfile.write(payload)
                return
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("Connection", "close")
            self.end_headers()
            base = {"id": "fixture-response", "object": "chat.completion.chunk", "created": 1, "model": body["model"]}
            chunks = [
                {**base, "choices": [{"index": 0, "delta": {"role": "assistant", **reply}, "finish_reason": None}]},
                {**base, "choices": [{"index": 0, "delta": {}, "finish_reason": "tool_calls" if "tool_calls" in reply else "stop"}],
                 "usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2}},
            ]
            for chunk in chunks:
                self.wfile.write(("data: " + json.dumps(chunk) + "\n\n").encode())
            self.wfile.write(b"data: [DONE]\n\n")
            self.wfile.flush()

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        yield f"http://127.0.0.1:{server.server_port}", requests, failures
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=5)


def prepare_fixture(root, plugin_directory, provider, generic_task_only=False):
    config = {
        "model": "fixture/parent",
        "providers": fixture_provider_config(provider),
        "agents": {
            "fixture-parent": {"mode": "primary", "model": "fixture/parent", "system": "PARENT_AGENT_SENTINEL", "steps": 4,
                               "permissions": [{"action": "*", "resource": "*", "effect": "deny"},
                                               {"action": "subagent", "resource": "*", "effect": "allow"}]},
            "fixture-child": {"mode": "subagent", "model": "fixture/child", "system": "CHILD_AGENT_SENTINEL", "steps": 2,
                              "permissions": [{"action": "*", "resource": "*", "effect": "deny"}]},
            "review-risk": {"mode": "subagent", "model": "fixture/child", "system": "REVIEW_SENTINEL", "steps": 2,
                            "permissions": [{"action": "*", "resource": "*", "effect": "deny"}]},
        },
    }
    if generic_task_only:
        del config["agents"]["review-risk"]
        config["agents"]["fixture-parent"]["permissions"][-1]["resource"] = "fixture-child"
    (root / "project/opencode.json").write_text(json.dumps(config))
    (root / "project/AGENTS.md").write_text("PROJECT_INSTRUCTION_SENTINEL\n")
    log = root / "observations.jsonl"
    write_observer(plugin_directory, log, provider)
    return log


OBSERVER = '''import { Plugin } from "@opencode/plugin"
import { appendFileSync } from "node:fs"
const log = (value: unknown) => appendFileSync(LOG, JSON.stringify(value)+"\\n")
export default Plugin.define({id:"fixture.observer", async setup(ctx) {
 const registrations=[]
 registrations.push(await ctx.session.hook("title", call => {call.result="Fixture title"}))
 registrations.push(await ctx.session.hook("context", call => {log({type:"context",agent:call.agent,system:call.system,tools:Object.keys(call.tools)})}))
 registrations.push(await ctx.session.hook("http.request", call => {
   const url=new URL(call.request.url)
   if(url.origin!==ORIGIN) { log({type:"forbidden-network"}); throw Error("nonloopback model request") }
 }))
 registrations.push(await ctx.tool.hook("execute.before", call => {if(call.tool==="subagent")log({type:"before",id:call.id,input:call.input})}))
 registrations.push(await ctx.tool.hook("execute.after", call => {if(call.tool==="subagent")log({type:"after",id:call.id,status:call.status,...(call.status==="completed"?{result:call.result}:{error:call.error})})}))
 return async()=>{await Promise.all(registrations.map(r=>r.dispose()))}
}})
'''


def write_observer(plugin_directory, log, provider):
    source = OBSERVER.replace("LOG", json.dumps(str(log))).replace("ORIGIN", json.dumps(provider))
    (plugin_directory / "zz-fixture-observer.ts").write_text(source)


def fixture_provider_config(provider):
    return {"fixture": {
        "name": "Loopback fixture", "package": "@opencode/ai/providers/openai-compatible",
        "settings": {"baseURL": provider + "/v1", "apiKey": "fixture-only"},
        "models": {name: {"limit": {"context": 32768, "output": 1024}, "capabilities": {
            "tools": True, "input": ["text"], "output": ["text"],
        }} for name in ("parent", "child")},
    }}


def prepare_review_fixture(config_directory, provider, log):
    """Global-scope review fixture: settings live in the config dir, never the host project."""
    deny = [{"action": "*", "resource": "*", "effect": "deny"}]
    agents = {"fixture-parent": {"mode": "primary", "model": "fixture/parent", "system": "PARENT_AGENT_SENTINEL", "steps": 4,
                                 "permissions": deny + [{"action": "subagent", "resource": "*", "effect": "allow"}]}}
    for agent in REVIEW_AGENTS:
        agents[agent] = {"mode": "subagent", "model": "fixture/child", "system": "REVIEW_AGENT_SENTINEL", "steps": 2,
                         "permissions": list(deny)}
    config = {"model": "fixture/parent", "providers": fixture_provider_config(provider), "agents": agents}
    (config_directory / "opencode.json").write_text(json.dumps(config))
    write_observer(config_directory / "plugins", log, provider)
    return log


def authority_digest(directory):
    """Digest every regular file under a review lineage directory."""
    root = Path(directory)
    if not root.exists():
        return "absent"
    digest = hashlib.sha256()
    for path in sorted(root.rglob("*")):
        if path.is_file() and not path.is_symlink():
            digest.update(str(path.relative_to(root)).encode() + b"\0" + hashlib.sha256(path.read_bytes()).digest())
    return digest.hexdigest()


def check_review_step(step, child_prompts, tool_texts, observed_texts, before, after):
    """Return every violated expectation for one host-dispatched review step."""
    problems = []
    child = step.get("child") or ""
    if child and any(child in text for text in tool_texts):
        problems.append("parent received the raw child output")
    if step["expect"] == "admitted":
        if len(child_prompts) != 1:
            problems.append(f"expected exactly one child request, observed {len(child_prompts)}")
        for prompt in map(strip_host_preamble, child_prompts):
            if not prompt.startswith(MATERIALIZATION_HEADER + " "):
                problems.append("child prompt is not Go-materialized")
            elif any(fragment in prompt for fragment in step.get("host_injected", [])):
                # The host prompt equals Go's canonical binding when unmodified, so
                # only host-authored additions can prove Go-only materialization.
                problems.append("child prompt carries host-authored prompt bytes")
        for fragment in step.get("tool_contains", []):
            if not any(fragment in text for text in tool_texts):
                problems.append(f"parent result lacks {fragment!r}")
        if before == after:
            problems.append("authority unchanged by an admitted step")
    else:
        if "child_requests" in step and len(child_prompts) != step["child_requests"]:
            problems.append(f"expected {step['child_requests']} child requests, observed {len(child_prompts)}")
        if not any(RELAY_REFUSED in text for text in tool_texts + observed_texts):
            problems.append("refusal was not observed")
        reason = step.get("reason")
        # The parent sees "<code> (reason: <reason>)"; the after hook records the
        # structured {"code", "reason"} result, so either proves the bounded cause.
        if reason and not any(f"(reason: {reason})" in text or f'"reason": "{reason}"' in text
                              for text in tool_texts + observed_texts):
            problems.append(f"refusal reason {reason!r} was not observed")
        if before != after:
            problems.append("authority changed by a refused step")
    return problems


def run_review_scenario(request, scenario, observation_log, requests, failures, script, run=None):
    """Drive one parent session per step and record evidence for each."""
    run = run or run_parent
    evidence = []
    for index, step in enumerate(scenario["steps"]):
        first_request = len(requests)
        observed_before = len(observation_log.read_text().splitlines()) if observation_log.exists() else 0
        before = authority_digest(scenario["authority_dir"])
        host_error = None
        try:
            run(request, "REVIEW_STEP " + str(index))
        except Exception as error:  # A refused step may surface as a host error.
            host_error = repr(error)[:500]
        after = authority_digest(scenario["authority_dir"])
        bodies = requests[first_request:]
        child_contents = [next((m.get("content") for m in reversed(body["messages"]) if m.get("role") == "user"), "")
                          for body in bodies if body["model"] == "child"]
        child_prompts = [message_text(content) for content in child_contents]
        tool_texts = [message_text(message.get("content")) for body in bodies if body["model"] == "parent"
                      for message in body["messages"] if message.get("role") == "tool"]
        lines = observation_log.read_text().splitlines()[observed_before:] if observation_log.exists() else []
        observations = [json.loads(line) for line in lines]
        observed_texts = [json.dumps(item) for item in observations if item["type"] == "after"]
        problems = check_review_step(step, child_prompts, tool_texts, observed_texts, before, after)
        evidence.append({
            "name": step["name"], "agent": step["agent"], "expect": step["expect"], "problems": problems,
            "child_requests": len(child_prompts),
            "child_prompt_prefixes": [prompt[:120] for prompt in child_prompts],
            "child_user_content_shape": [json.dumps(content)[:300] for content in child_contents],
            "child_prompt_host_preamble": [prompt.startswith(HOST_SUBAGENT_PREAMBLE) for prompt in child_prompts],
            "child_prompt_materialized": [strip_host_preamble(prompt).startswith(MATERIALIZATION_HEADER + " ") for prompt in child_prompts],
            "tool_texts": [text[:2000] for text in tool_texts],
            "hooks": [{key: item.get(key) for key in ("type", "status", "error")} for item in observations if item["type"] in ("before", "after")],
            "after_hooks": [text[:2000] for text in observed_texts],
            "authority_before": before, "authority_after": after, "host_error": host_error,
        })
    if failures:
        evidence.append({"name": "fixture", "problems": ["unexpected network/provider requests: " + repr(failures)]})
    return evidence


def run_parent(request, text):
    session = request("/api/session", {"agent": "fixture-parent", "model": {"providerID": "fixture", "id": "parent"}})["data"]
    request("/api/session/" + session["id"] + "/prompt", {"text": text})
    request("/api/experimental/session/" + session["id"] + "/wait", {}, timeout=25)


def prove_generic_dispatch(request, observation_log, requests, failures):
    run_parent(request, "FOREGROUND_PARENT")
    observations = [json.loads(line) for line in observation_log.read_text().splitlines()]
    before = [item for item in observations if item["type"] == "before"]
    after = [item for item in observations if item["type"] == "after"]
    assert len(before) == len(after) == 1, "missing or repeated foreground hooks"
    assert observations.index(before[0]) < observations.index(after[0]), "hook order"
    assert before[0]["id"] == after[0]["id"]
    child = after[0]["result"]["output"]
    assert after[0]["status"] == child["status"] == "completed"
    assert child["sessionID"] and child["output"] == "RAW_CHILD_SENTINEL", "raw structured child output"
    contexts = [item for item in observations if item["type"] == "context" and item["agent"] == "fixture-child"]
    assert contexts, "child context not observed"
    system = json.dumps(contexts[0]["system"])
    assert "CHILD_AGENT_SENTINEL" in system and "PROJECT_INSTRUCTION_SENTINEL" in system, "inherited instructions missing"
    assert "PARENT_AGENT_SENTINEL" not in system, "parent agent system leaked into child"
    assert contexts[0]["tools"] == [], "deny-all child still exposes tools"
    child_requests = sum(body["model"] == "child" for body in requests)
    assert child_requests == 1
    child_request = next(body for body in requests if body["model"] == "child")
    wire_messages = json.dumps(child_request["messages"])
    assert "PROJECT_INSTRUCTION_SENTINEL" in wire_messages and "CHILD_AGENT_SENTINEL" in wire_messages
    assert "PARENT_AGENT_SENTINEL" not in wire_messages
    assert not child_request.get("tools"), "provider received denied child tools"
    assert not failures, "unexpected network/provider requests: " + repr(failures)
    assert not any(item["type"] == "forbidden-network" for item in observations)


def prove_dispatch(request, observation_log, requests, failures):
    # Legacy review conformance is deliberately separate from generic dispatch.
    shell = request("/api/shell", {"command": "gentle-ai review opencode-transport </dev/null", "timeout": 5000})["data"]
    deadline = time.monotonic() + 10
    while shell["status"] == "running" and time.monotonic() < deadline:
        time.sleep(0.1)
        shell = request("/api/shell/" + shell["id"])["data"]
    assert shell["status"] == "exited" and shell.get("exit") != 0, "native shell result: " + repr(shell)
    refused = request("/api/shell/" + shell["id"] + "/output")["data"]["output"]
    assert "immutable_review_transport_unsupported" in refused, "real native gate did not refuse"
    prove_generic_dispatch(request, observation_log, requests, failures)
    child_requests = sum(body["model"] == "child" for body in requests)
    run_parent(request, "NEGATIVE_REVIEW")
    assert sum(body["model"] == "child" for body in requests) == child_requests, "refused review reached child provider"
    observations = [json.loads(line) for line in observation_log.read_text().splitlines()]
    assert not any(item.get("agent") == "review-risk" for item in observations), "review child context reached after refusal"
    negative_results = [message for body in requests if body["model"] == "parent" for message in body["messages"]
                        if message.get("role") == "tool" and "opencode_review_transport_relay_refused" in str(message.get("content"))]
    assert negative_results, "native refusal did not reach parent tool result"
    assert not failures, "unexpected network/provider requests: " + repr(failures)
    assert not any(item["type"] == "forbidden-network" for item in observations)
