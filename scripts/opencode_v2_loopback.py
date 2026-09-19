"""Deterministic test-only provider and observer; never real review admission."""
from contextlib import contextmanager
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import threading
import time


def scripted_reply(body):
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
def local_provider():
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
            if not 0 < length <= 1024 * 1024 or len(requests) >= 12:
                return self.reject("request bound exceeded")
            try:
                body = json.loads(self.rfile.read(length))
                requests.append(body)
                reply = scripted_reply(body)
            except (ValueError, KeyError) as error:
                return self.reject(str(error))
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


def prepare_fixture(root, plugin_directory, provider):
    config = {
        "model": "fixture/parent",
        "providers": {"fixture": {
            "name": "Loopback fixture", "package": "@opencode/ai/providers/openai-compatible",
            "settings": {"baseURL": provider + "/v1", "apiKey": "fixture-only"},
            "models": {name: {"limit": {"context": 32768, "output": 1024}, "capabilities": {
                "tools": True, "input": ["text"], "output": ["text"],
            }} for name in ("parent", "child")},
        }},
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
    (root / "project/opencode.json").write_text(json.dumps(config))
    (root / "project/AGENTS.md").write_text("PROJECT_INSTRUCTION_SENTINEL\n")
    log = root / "observations.jsonl"
    observer = '''import { Plugin } from "@opencode/plugin"
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
'''.replace("LOG", json.dumps(str(log))).replace("ORIGIN", json.dumps(provider))
    (plugin_directory / "zz-fixture-observer.ts").write_text(observer)
    return log


def prove_dispatch(request, observation_log, requests, failures):
    # Exercise the real native gate independently of hook-error projection.
    shell = request("/api/shell", {"command": "gentle-ai review opencode-transport </dev/null", "timeout": 5000})["data"]
    deadline = time.monotonic() + 10
    while shell["status"] == "running" and time.monotonic() < deadline:
        time.sleep(0.1)
        shell = request("/api/shell/" + shell["id"])["data"]
    assert shell["status"] == "exited" and shell.get("exit") != 0, "native shell result: " + repr(shell)
    refused = request("/api/shell/" + shell["id"] + "/output")["data"]["output"]
    assert "immutable_review_transport_unsupported" in refused, "real native gate did not refuse"

    def run(text):
        session = request("/api/session", {"agent": "fixture-parent", "model": {"providerID": "fixture", "id": "parent"}})["data"]
        request("/api/session/" + session["id"] + "/prompt", {"text": text})
        request("/api/experimental/session/" + session["id"] + "/wait", {}, timeout=25)

    run("FOREGROUND_PARENT")
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
    run("NEGATIVE_REVIEW")
    assert sum(body["model"] == "child" for body in requests) == child_requests, "refused review reached child provider"
    observations = [json.loads(line) for line in observation_log.read_text().splitlines()]
    assert not any(item.get("agent") == "review-risk" for item in observations), "review child context reached after refusal"
    negative_results = [message for body in requests if body["model"] == "parent" for message in body["messages"]
                        if message.get("role") == "tool" and "opencode_review_transport_relay_refused" in str(message.get("content"))]
    assert negative_results, "native refusal did not reach parent tool result"
    assert not failures, "unexpected network/provider requests: " + repr(failures)
    assert not any(item["type"] == "forbidden-network" for item in observations)
