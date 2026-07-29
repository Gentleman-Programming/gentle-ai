# Testing Agents Deterministically

How Gentle AI proves that an agent did what it was asked — in CI, on every push, with no API keys and no token cost.

← [Back to README](../README.md)

---

## The problem this solves

Gentle AI is deterministic code that validates agent work. The ceremony that used to live in system prompts — freeze this candidate, hash it, verify the obligation, emit the receipt, revalidate the gate — moved into the CLI, because an agent performs that ceremony slowly and cannot be trusted to perform it honestly. A model asserting *"I verified it, it passes"* is prose, not proof.

That design creates a testing problem. Unit tests prove the code is internally consistent. They cannot prove the product invariant:

> A real agent does real work, and the deterministic CLI correctly decides whether that work is acceptable.

Proving that requires running an actual agent against an actual repository. But an actual agent calls an actual model, and a model is:

- **non-deterministic** — the same instruction yields different tool calls in a different order;
- **billed per token** — every push, every PR, on every platform in the matrix;
- **network-dependent** — rate limits and provider outages turn CI red for reasons unrelated to the code;
- **a secret to manage** — an API key in a public repository's CI is an exfiltration target.

A test with those properties gets disabled within a month. The solution is to keep the agent real and replace only its reasoning.

---

## Two E2E suites

| Suite | Location | Platforms | What it proves |
|---|---|---|---|
| Installer E2E | `e2e/docker-test.sh` | Ubuntu, Arch, Fedora (Docker) | Installation, layout, idempotency, optional SDD |
| Organic Runtime E2E | `e2e/organicruntime/` | Ubuntu, Windows (native runners) | A real agent driving the real CLI through the full work lifecycle |

The installer suite is documented separately in [Docker E2E Testing](./docker-e2e-testing.md). This document covers the Organic Runtime suite.

### Test scope by trigger

The installer suite runs all platform checks on every trigger; only depth changes. On `pull_request` it runs with `RUN_FULL_E2E=0` for fast feedback; on `push` and `schedule` it runs the full suite.

---

## Organic Runtime E2E

One test — `TestRealOpenCodeOrganicRuntimeJourneys` in `e2e/organicruntime/organic_runtime_test.go` — exercises four journeys through the real binary.

### What is real

Everything except the model's reasoning:

| Component | Real? | Detail |
|---|---|---|
| OpenCode binary | Yes | Pinned to `versions.OpenCode`; `requireExecutableVersion` fails the test on a mismatch |
| OpenCode plugin | Yes | `@opencode-ai/plugin` installed with `npm install` at the pinned version |
| Orchestrator prompt | Yes | Read from `internal/assets/opencode/sdd-orchestrator.md` — the same asset shipped to users |
| `gentle-ai` binary | Yes | Compiled from the working tree, exposed as `GENTLE_AI_TEST_BINARY` |
| Git repository | Yes | A real repository plus a bare remote; delivery ends in an `update-ref` CAS with exact tree and blob proof |
| Filesystem effects | Yes | Real files, real commits, isolated `$HOME` with `--pure` and per-test `XDG_*` directories |
| Model reasoning | **No** | A local HTTP server returning a scripted sequence |

Because the prompt is loaded from the shipped asset, changing that asset changes what the E2E exercises. There is no test-only copy to drift out of sync.

### The journeys

| Journey | Invariant under test |
|---|---|
| `direct inline implementation` | The `direct_inline` route stays inline and creates no SDD artifacts |
| `delegated direct implementation` | The `delegated_direct` route delegates without entering an SDD lifecycle |
| `direct route with common review actor` | A direct route may delegate the common review actor without changing its implementation route |
| `managed start kill switch before advance` | The activation kill switch stops the flow before it advances |

The first three are the routing invariants from the architecture plan. The fourth proves the brake works, which is what makes shipping a dormant-by-default capability safe.

---

## How the fixture works

### Step 1 — the agent accepts a custom provider

An agent's model call is an HTTP POST carrying messages and tool definitions, and the response names the next tool to call. That protocol is standardized, so anything that speaks it is, to the agent, a model.

OpenCode already supports custom providers so users can point it at local runtimes. The test declares one:

```json
{
  "provider": {
    "fixture": {
      "npm": "@ai-sdk/openai-compatible",
      "name": "Organic E2E Fixture",
      "options": {
        "baseURL": "http://127.0.0.1:<port>/v1",
        "apiKey": "fixture"
      },
      "models": { "fixture": { "name": "Fixture" } }
    }
  }
}
```

`@ai-sdk/openai-compatible` is a generic client for the OpenAI wire protocol. The `baseURL` points at loopback, and `apiKey` is the literal string `fixture` — nothing validates it, because nothing on the other end is a vendor API.

No agent code is patched. This is a door OpenCode already left open.

### Step 2 — the fixture server is started, then its URL is injected

```go
fixture.Server = httptest.NewServer(http.HandlerFunc(fixture.serveHTTP))
```

`httptest` binds a free port and returns a URL. That URL becomes the `baseURL` above, which is why the configuration is generated at runtime rather than committed.

### Step 3 — the agent is defined with the shipped prompt

```json
{
  "agent": {
    "organic": {
      "mode": "primary",
      "model": "fixture/fixture",
      "prompt": "<contents of internal/assets/opencode/sdd-orchestrator.md>",
      "permission": { "bash": "allow", "task": "allow", "edit": "deny" }
    }
  }
}
```

`edit: deny` is deliberate: the agent may run commands and delegate subagents, but it cannot write files directly. Every mutation must travel through the CLI, which forces the path under test.

### Step 4 — the agent runs

```bash
opencode run --pure --format json \
  --agent organic --model fixture/fixture \
  --dir <repo> "<requested outcome>"
```

OpenCode starts normally, resolves `fixture/fixture` to loopback, and POSTs exactly as it would to a vendor endpoint.

### Step 5 — the fixture answers from a script

There is no reasoning. There is a counter and a switch:

```go
fixture.mainCalls++
switch fixture.mainCalls {
case 1: writeTool(w, "capabilities", "bash", organicCapabilityCommand())
case 2: writeTool(w, "start",        "bash", organicStartCommand())
case 3: writeTool(w, "status",       "bash", organicStatusCommand())
case 4: writeTool(w, "actor",        "task", /* delegate the real work */)
case 5: writeTool(w, "advance",      "bash", organicAdvanceCommand(...))
// ...
}
```

The agent executes each step for real: real `bash`, the real binary, a real repository. Only the *choice* of step is scripted.

### Step 6 — the fixture also inspects the request

This is what makes it a test rather than a playback. Before dictating the next step, the fixture audits what the agent brought back:

```go
if !strings.Contains(lastText, fixture.scenario.actorMarker) {
    fixture.fail(w, "work-advance was requested before actor commit evidence: %s", lastText)
    return
}
```

An agent that asks to advance without commit evidence fails the test. The same guard applies to common review (`common review was requested before committed implementation evidence`). The fixture rejects a request with no tools or no messages, and fails on an unexpected extra call.

So the contract is checked in both directions: the fixture drives the agent, and it verifies that the agent asked for the right things in the right order.

### The second fixture: the hosting plane

A separate TLS server (`httptest.NewTLSServer`) stands in for the delivery destination, exposing `resolvePADGitBinding`, `observeDelivery`, and `compareAndSwapBranch`, with an `assertCalls` check that the expected calls — and only those — occurred. TLS is genuine, with a self-signed certificate, so the code paths that validate secure connections are exercised as in production.

---

## End-to-end flow

```
Go test
 ├─ start httptest server                → http://127.0.0.1:<port>
 ├─ compile gentle-ai                    → GENTLE_AI_TEST_BINARY
 ├─ create a real Git repo + bare remote + isolated $HOME
 ├─ write the OpenCode config pointing at that URL, with the shipped prompt
 └─ exec: opencode run --agent organic ...
       │
       │  POST /v1/chat/completions { messages, tools }
       ▼
    [model fixture]  call #1 → "bash: gentle-ai work capabilities"
       │
       ▼
    OpenCode executes that bash for real → the real binary, the real repo
       │  (real stdout / exit code returns as the next message)
       ▼
    [model fixture]  inspects the result
                     missing expected evidence → t.Fail
                     evidence present          → call #2 → "work start"
       │
       ⋮  (through the scripted sequence)
       ▼
    Final assertions: update-ref CAS against the bare repo,
    exact tree and blob proof, expiry-stable Ready, lost-response replay
```

---

## Why this costs nothing

The distinction that makes it free:

- **The agent is a local program.** OpenCode is a binary you run, like `git`. Running it costs nothing.
- **The model is a paid API.** What bills you is the HTTP call the agent makes when it needs to decide.

The fixture cuts that call and routes it to loopback:

```
before:  OpenCode (local, free) ──POST──▶  vendor API   ($ per token)
after:   OpenCode (local, free) ──POST──▶  127.0.0.1    ($0)
```

The agent still does all the real work. Only the destination of its reasoning request changed. Consequently the suite is:

- **free** — no tokens, on any number of runs;
- **offline** — no network dependency, runnable on an isolated runner;
- **deterministic** — the same sequence every time, so a failure means the code broke;
- **secret-free** — the only CI secret in the workflow is the `GITHUB_TOKEN` that Actions provides;
- **fast** — milliseconds of model latency instead of minutes.

A test that costs money is a test somebody eventually turns off.

---

## Running it locally

```bash
# Prerequisites: node, npm, and OpenCode pinned to versions.OpenCode
GENTLE_AI_REAL_AGENT_E2E=1 \
  go test -v ./e2e/organicruntime \
  -run TestRealOpenCodeOrganicRuntimeJourneys -count=1 -timeout=15m
```

Without `GENTLE_AI_REAL_AGENT_E2E=1` the test skips, so ordinary `go test ./...` runs stay fast. A version mismatch on the `opencode` executable fails rather than silently testing a different runtime.

In CI the `organic-runtime-e2e` job runs this across a matrix of `ubuntu-latest` and `windows-latest`, installing the pinned OpenCode runtime first.

---

## What it proves, and what it does not

**Proved.** Given a known agent behaviour, the CLI classifies the implementation route correctly, creates no SDD artifacts when it must not, freezes the candidate, runs applicable verification, emits a content-bound receipt, authorizes delivery, performs a real compare-and-swap against the remote, and stops when the kill switch is set — on Linux and Windows.

**Not proved.** That a live model, given the shipped prompt, produces the same tool calls the fixture scripts. That leap is non-deterministic by nature and does not belong in a merge gate; it is covered by real usage and by the cross-adapter asset parity fixtures.

The complement is the platform unit tests. The Windows job runs a curated set of release-blocker tests — handle rebinding, secure-open fallbacks, store-lock ownership and recovery, concurrent authority repair — because the E2E tells you *that* something broke while a unit test tells you *where*, in seconds instead of fifteen minutes.

---

## Applying the pattern

The approach generalizes to any agent-driven system:

1. **Keep the runtime real.** Same binary, same version pin, same shipped prompt, same permissions.
2. **Replace only the reasoning.** Serve the model protocol from a local server with a scripted sequence.
3. **Make the fixture adversarial.** Assert on the incoming request, not just the outgoing response. Fail when evidence arrives out of order.
4. **Fake nothing else.** Real filesystem, real Git, real TLS. Anything that can be deterministic should stay real.
5. **Keep the non-deterministic part out of the gate.** Whether a prompt reliably steers a live model is a product question, answered by usage, not by CI.

---

## References

- [Docker E2E Testing](./docker-e2e-testing.md) — the installer suite
- [Organic Recovery Architecture and Implementation Plan](./audits/2026-07-23-organic-recovery-implementation-plan.md) — the routes, verification axes, and acceptance criteria this suite exercises
- [Review Authority Threat Model](./review-authority-threat-model.md) — boundaries and assumptions of the trust kernel
