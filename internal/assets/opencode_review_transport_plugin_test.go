package assets

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestOpenCodeReviewTransportPluginDeduplicatesDuplicateInstances(t *testing.T) {
	source, err := Read("opencode/plugins/opencode-review-transport.ts")
	if err != nil {
		t.Fatal(err)
	}
	const harness = `import plugin from "./plugin.mts"
const first = await plugin({ directory: process.cwd(), worktree: process.cwd() })
const second = await plugin({ directory: process.cwd(), worktree: process.cwd() })
const before = { args: { subagent_type: "review-risk", prompt: "Go must receive this original host prompt" } }
await first["tool.execute.before"]({ tool: "task", sessionID: "session", callID: "call" }, before)
await second["tool.execute.before"]({ tool: "task", sessionID: "session", callID: "call" }, before)
const after = { output: "untrusted reviewer output", metadata: {} }
await second["tool.execute.after"]({ tool: "task", sessionID: "session", callID: "call", args: { subagent_type: "review-risk" } }, after)
if (after.output !== "untrusted reviewer output") throw new Error("non-owner after hook must pass through instead of completing the owner's relay; output = " + after.output)
await first["tool.execute.after"]({ tool: "task", sessionID: "session", callID: "call", args: { subagent_type: "review-risk" } }, after)
const beforeReversed = { args: { subagent_type: "review-risk", prompt: "Go must receive this original host prompt" } }
await first["tool.execute.before"]({ tool: "task", sessionID: "session", callID: "reversed" }, beforeReversed)
await second["tool.execute.before"]({ tool: "task", sessionID: "session", callID: "reversed" }, beforeReversed)
const reversed = { output: "untrusted reviewer output", metadata: {} }
await first["tool.execute.after"]({ tool: "task", sessionID: "session", callID: "reversed", args: { subagent_type: "review-risk" } }, reversed)
await second["tool.execute.after"]({ tool: "task", sessionID: "session", callID: "reversed", args: { subagent_type: "review-risk" } }, reversed)
if (reversed.output !== "captured") throw new Error("non-owner after an owner-delivered completion must pass through; output = " + reversed.output)
console.log(JSON.stringify({ prompt: before.args.prompt, output: after.output }))
`
	output, log := runOpenCodeTransportPluginHarness(t, map[string]string{"plugin.mts": string(source)}, harness, posixRelayFixture)
	var result struct {
		Prompt string `json:"prompt"`
		Output string `json:"output"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decode relay harness output %q: %v", output, err)
	}
	if result.Prompt != "Go-materialized immutable prompt" || result.Output != "captured" {
		t.Fatalf("relay result = %#v", result)
	}
	frames := "{\"schema\":\"gentle-ai.provider-transport/v1\",\"operation\":\"start\",\"prompt\":\"Go must receive this original host prompt\"}\n{\"schema\":\"gentle-ai.provider-transport/v1\",\"operation\":\"complete\",\"nonce\":\"nonce\",\"output\":\"untrusted reviewer output\"}\n"
	if log != frames+frames {
		t.Fatalf("relay frames = %q", log)
	}
}

func TestOpenCodeReviewTransportPluginStripsAmbientSystemOnlyForRegisteredReviewChildren(t *testing.T) {
	source, err := Read("opencode/plugins/opencode-review-transport.ts")
	if err != nil {
		t.Fatal(err)
	}
	const harness = `import plugin from "./plugin.mts"
const hooks = await plugin({ directory: process.cwd(), worktree: process.cwd() })
const transform = hooks["experimental.chat.system.transform"]
if (typeof transform !== "function") throw new Error("review transport must install a system transform")
const sentinels = ["GENTLE_AI_AMBIENT_AGENT", "GENTLE_AI_AMBIENT_PROJECT", "GENTLE_AI_AMBIENT_SKILL"]
const unchanged = (name, system) => {
  if (JSON.stringify(system) !== JSON.stringify(sentinels)) throw new Error(name + " session system was changed: " + JSON.stringify(system))
}

const runtimeAgentSystem = [...sentinels]
const runtimeAgentIdentity = runtimeAgentSystem
await hooks.event({ event: { type: "session.created", properties: { info: { id: "runtime-agent-session", agent: "review-risk" } } } })
await transform({ sessionID: "runtime-agent-session" }, { system: runtimeAgentSystem })
if (runtimeAgentSystem !== runtimeAgentIdentity) throw new Error("runtime-agent review system array was replaced instead of mutated in place")
if (runtimeAgentSystem.length !== 1 || !runtimeAgentSystem[0].includes("Go-materialized user prompt")) throw new Error("runtime-agent review system was not reduced to the transport isolation instruction: " + JSON.stringify(runtimeAgentSystem))
if (sentinels.some((sentinel) => runtimeAgentSystem.some((entry) => entry.includes(sentinel)))) throw new Error("runtime-agent review system retained ambient agent, project, or skill content: " + JSON.stringify(runtimeAgentSystem))

await hooks.event({ event: { type: "session.created", properties: {} } })
const legacyTitleSystem = [...sentinels]
await hooks.event({ event: { type: "session.created", properties: { info: { id: "legacy-title-session", title: "Review task (@review-readability subagent)" } } } })
await transform({ sessionID: "legacy-title-session" }, { system: legacyTitleSystem })
if (legacyTitleSystem.length !== 1 || !legacyTitleSystem[0].includes("Go-materialized user prompt")) throw new Error("legacy review title did not register review-session isolation: " + JSON.stringify(legacyTitleSystem))

await hooks.event({ event: { type: "session.created", properties: { info: { id: "ordinary-agent-session", agent: "ordinary-agent", title: "Review task (@review-risk subagent)" } } } })
await hooks.event({ event: { type: "session.created", properties: { info: { id: "partial-title-session", title: "Review task (@review-risk subagent) extra" } } } })
const ordinaryAgentSystem = [...sentinels]
const partialTitleSystem = [...sentinels]
const malformedSystem = [...sentinels]
const undefinedSessionIDSystem = [...sentinels]
await transform({ sessionID: "ordinary-agent-session" }, { system: ordinaryAgentSystem })
await transform({ sessionID: "partial-title-session" }, { system: partialTitleSystem })
await transform({ sessionID: "malformed-session" }, { system: malformedSystem })
await transform({}, { system: undefinedSessionIDSystem })
unchanged("ordinary-agent", ordinaryAgentSystem)
unchanged("partial-title", partialTitleSystem)
unchanged("malformed", malformedSystem)
unchanged("undefined-sessionID", undefinedSessionIDSystem)

await hooks.event({ event: { type: "session.deleted", properties: { info: { id: "runtime-agent-session" } } } })
const deletedSystem = [...sentinels]
await transform({ sessionID: "runtime-agent-session" }, { system: deletedSystem })
unchanged("deleted", deletedSystem)
await hooks.event({ event: { type: "session.created", properties: { info: { id: "disposed-session", agent: "review-risk" } } } })
await hooks.dispose()
const disposedSystem = [...sentinels]
await transform({ sessionID: "disposed-session" }, { system: disposedSystem })
unchanged("disposed", disposedSystem)
console.log(JSON.stringify({ runtimeAgentSystem, legacyTitleSystem, ordinaryAgentSystem, partialTitleSystem, malformedSystem, undefinedSessionIDSystem, deletedSystem, disposedSystem }))
`
	output, _ := runOpenCodeTransportPluginHarness(t, map[string]string{"plugin.mts": string(source)}, harness, posixRelayFixture)
	var result struct {
		RuntimeAgentSystem       []string `json:"runtimeAgentSystem"`
		LegacyTitleSystem        []string `json:"legacyTitleSystem"`
		OrdinaryAgentSystem      []string `json:"ordinaryAgentSystem"`
		PartialTitleSystem       []string `json:"partialTitleSystem"`
		MalformedSystem          []string `json:"malformedSystem"`
		UndefinedSessionIDSystem []string `json:"undefinedSessionIDSystem"`
		DeletedSystem            []string `json:"deletedSystem"`
		DisposedSystem           []string `json:"disposedSystem"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decode system transform harness output %q: %v", output, err)
	}
	for _, system := range [][]string{result.RuntimeAgentSystem, result.LegacyTitleSystem} {
		if len(system) != 1 || !strings.Contains(system[0], "Go-materialized user prompt") {
			t.Fatalf("review system = %#v, want only a transport isolation instruction", system)
		}
	}
	for _, system := range [][]string{result.OrdinaryAgentSystem, result.PartialTitleSystem, result.MalformedSystem, result.UndefinedSessionIDSystem, result.DeletedSystem, result.DisposedSystem} {
		if got, want := strings.Join(system, ","), "GENTLE_AI_AMBIENT_AGENT,GENTLE_AI_AMBIENT_PROJECT,GENTLE_AI_AMBIENT_SKILL"; got != want {
			t.Fatalf("non-review or cleaned-up system = %#v, want ambient system unchanged", system)
		}
	}
}

func TestOpenCodeReviewTransportPluginKeysGroupedReviewersBySubagentType(t *testing.T) {
	source, err := Read("opencode/plugins/opencode-review-transport.ts")
	if err != nil {
		t.Fatal(err)
	}
	const harness = `import plugin from "./plugin.mts"
const hooks = await plugin({ directory: process.cwd(), worktree: process.cwd() })
const lenses = ["review-risk", "review-resilience", "review-readability", "review-reliability"]
const tasks = lenses.map((subagent_type) => ({ args: { subagent_type, prompt: "Go must receive this original host prompt" } }))
await Promise.all(tasks.map((task) => hooks["tool.execute.before"]({ tool: "task", sessionID: "session", callID: "grouped-call" }, task)))
const taskOutputs = tasks.map(() => ({ output: "untrusted reviewer output", metadata: {} }))
const completions = await Promise.allSettled(tasks.map((task, index) => hooks["tool.execute.after"](
  { tool: "task", sessionID: "session", callID: "grouped-call", args: { subagent_type: task.args.subagent_type } },
  taskOutputs[index],
)))
for (const [index, completion] of completions.entries()) {
  if (completion.status !== "fulfilled") throw new Error("grouped reviewer " + lenses[index] + " did not complete: " + completion.reason)
}
console.log(JSON.stringify({ prompts: tasks.map((task) => task.args.prompt), outputs: taskOutputs.map((task) => task.output) }))
`
	output, log := runOpenCodeTransportPluginHarness(t, map[string]string{"plugin.mts": string(source)}, harness, posixRelayFixture)
	var result struct {
		Prompts []string `json:"prompts"`
		Outputs []string `json:"outputs"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decode grouped relay harness output %q: %v", output, err)
	}
	if len(result.Prompts) != 4 || len(result.Outputs) != 4 {
		t.Fatalf("grouped reviewer results = %#v, want four prompts and outputs", result)
	}
	for index, prompt := range result.Prompts {
		if prompt != "Go-materialized immutable prompt" {
			t.Fatalf("grouped reviewer prompt[%d] = %q, want Go materialization", index, prompt)
		}
	}
	for index, output := range result.Outputs {
		if output != "captured" {
			t.Fatalf("grouped reviewer output[%d] = %q, want captured", index, output)
		}
	}
	if got := strings.Count(log, `"operation":"start"`); got != 4 {
		t.Fatalf("grouped reviewer relay starts = %d, want 4; log=%q", got, log)
	}
	if got := strings.Count(log, `"operation":"complete"`); got != 4 {
		t.Fatalf("grouped reviewer relay completions = %d, want 4; log=%q", got, log)
	}
}

func TestOpenCodeReviewTransportPluginUsesActiveHostWithoutVersionOrEnvironmentGates(t *testing.T) {
	source, err := Read("opencode/plugins/opencode-review-transport.ts")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		`spawn("opencode"`, `spawn('opencode'`, `exec("opencode"`, `exec('opencode'`,
		"OPENCODE_DISABLE_", "--version",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("OpenCode transport plugin must use the active host process, found forbidden %q", forbidden)
		}
	}
	if !strings.Contains(source, `spawn(TRANSPORT.Command, ["review", "opencode-transport"]`) {
		t.Fatal("OpenCode transport plugin must spawn only the shared Go transport")
	}
}

func TestOpenCodeReviewTransportPluginRefusesDeferredCompletionWhoseOwnerIsGone(t *testing.T) {
	source, err := Read("opencode/plugins/opencode-review-transport.ts")
	if err != nil {
		t.Fatal(err)
	}
	const harness = `import plugin from "./plugin.mts"
const first = await plugin({ directory: process.cwd(), worktree: process.cwd() })
const second = await plugin({ directory: process.cwd(), worktree: process.cwd() })
const before = { args: { subagent_type: "review-risk", prompt: "Go must receive this original host prompt" } }
await first["tool.execute.before"]({ tool: "task", sessionID: "session", callID: "call" }, before)
await second["tool.execute.before"]({ tool: "task", sessionID: "session", callID: "call" }, before)
await first.dispose()
const after = { output: "untrusted reviewer output", metadata: {} }
let refused = ""
try {
  await second["tool.execute.after"]({ tool: "task", sessionID: "session", callID: "call", args: { subagent_type: "review-risk" } }, after)
} catch (cause) {
  refused = cause instanceof Error ? cause.message : String(cause)
}
console.log(JSON.stringify({ refused, output: after.output }))
`
	output, _ := runOpenCodeTransportPluginHarness(t, map[string]string{"plugin.mts": string(source)}, harness, posixRelayFixture)
	var result struct {
		Refused string `json:"refused"`
		Output  string `json:"output"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decode relay harness output %q: %v", output, err)
	}
	if !strings.Contains(result.Refused, "no matching live before hook") {
		t.Fatalf("deferred completion without a live owner must refuse loudly, got refusal %q", result.Refused)
	}
	if result.Output != "untrusted reviewer output" {
		t.Fatalf("refused completion must not mutate host output, got %q", result.Output)
	}
}

// posixRelayFixture answers one start frame with a Go-materialized prompt and
// one completion frame with a captured result, logging both inbound frames.
const posixRelayFixture = `#!/bin/sh
IFS= read -r start
printf '%s\n' "$start" >> "$GENTLE_AI_RELAY_LOG"
printf '%s\n' '{"schema":"gentle-ai.provider-transport/v1","operation":"prompt","nonce":"nonce","prompt":"Go-materialized immutable prompt"}'
IFS= read -r complete
printf '%s\n' "$complete" >> "$GENTLE_AI_RELAY_LOG"
printf '%s\n' '{"schema":"gentle-ai.provider-transport/v1","operation":"result","output":"captured"}'
`

func TestOpenCodeReviewTransportPluginRefusesCompletionWithoutMatchingBeforeHook(t *testing.T) {
	source, err := Read("opencode/plugins/opencode-review-transport.ts")
	if err != nil {
		t.Fatal(err)
	}
	const harness = `import plugin from "./plugin.mts"
const hooks = await plugin({ directory: process.cwd(), worktree: process.cwd() })
const after = { output: "untrusted reviewer output", metadata: {} }
let refused = ""
try {
  await hooks["tool.execute.after"]({ tool: "task", sessionID: "session", callID: "orphan", args: { subagent_type: "review-risk" } }, after)
} catch (cause) {
  refused = cause instanceof Error ? cause.message : String(cause)
}
console.log(JSON.stringify({ refused, output: after.output }))
`
	output, _ := runOpenCodeTransportPluginHarness(t, map[string]string{"plugin.mts": string(source)}, harness, posixRelayFixture)
	var result struct {
		Refused string `json:"refused"`
		Output  string `json:"output"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decode relay harness output %q: %v", output, err)
	}
	if !strings.Contains(result.Refused, "no matching live before hook") {
		t.Fatalf("completion for an unknown key must refuse loudly, got refusal %q", result.Refused)
	}
	if result.Output != "untrusted reviewer output" {
		t.Fatalf("refused completion must not mutate host output, got %q", result.Output)
	}
}

func TestOpenCodeReviewTransportPluginRefusesCompletionForAnotherOwnersRegistration(t *testing.T) {
	source, err := Read("opencode/plugins/opencode-review-transport.ts")
	if err != nil {
		t.Fatal(err)
	}
	const harness = `import plugin from "./plugin.mts"
const first = await plugin({ directory: process.cwd(), worktree: process.cwd() })
const second = await plugin({ directory: process.cwd(), worktree: process.cwd() })
const before = { args: { subagent_type: "review-risk", prompt: "Go must receive this original host prompt" } }
await first["tool.execute.before"]({ tool: "task", sessionID: "session", callID: "call" }, before)
const after = { output: "untrusted reviewer output", metadata: {} }
let refused = ""
try {
  await second["tool.execute.after"]({ tool: "task", sessionID: "session", callID: "call", args: { subagent_type: "review-risk" } }, after)
} catch (cause) {
  refused = cause instanceof Error ? cause.message : String(cause)
}
const untouched = after.output
await first["tool.execute.after"]({ tool: "task", sessionID: "session", callID: "call", args: { subagent_type: "review-risk" } }, after)
console.log(JSON.stringify({ refused, untouched, output: after.output }))
`
	output, log := runOpenCodeTransportPluginHarness(t, map[string]string{"plugin.mts": string(source)}, harness, posixRelayFixture)
	var result struct {
		Refused   string `json:"refused"`
		Untouched string `json:"untouched"`
		Output    string `json:"output"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decode relay harness output %q: %v", output, err)
	}
	if !strings.Contains(result.Refused, "another plugin instance") {
		t.Fatalf("completion for another owner's registration must refuse, got refusal %q", result.Refused)
	}
	if result.Untouched != "untrusted reviewer output" {
		t.Fatalf("refused cross-owner completion must not mutate host output, got %q", result.Untouched)
	}
	if result.Output != "captured" {
		t.Fatalf("owner completion after a cross-owner refusal must still capture, got %q", result.Output)
	}
	if got := strings.Count(log, `"operation":"complete"`); got != 1 {
		t.Fatalf("relay must receive exactly one completion frame, got %d; log=%q", got, log)
	}
}

func TestOpenCodeReviewTransportPluginClearSessionLeavesOtherOwnersRegistrationsAlive(t *testing.T) {
	source, err := Read("opencode/plugins/opencode-review-transport.ts")
	if err != nil {
		t.Fatal(err)
	}
	const harness = `import plugin from "./plugin.mts"
const first = await plugin({ directory: process.cwd(), worktree: process.cwd() })
const second = await plugin({ directory: process.cwd(), worktree: process.cwd() })
const firstTask = { args: { subagent_type: "review-risk", prompt: "first owner prompt" } }
const secondTask = { args: { subagent_type: "review-risk", prompt: "second owner prompt" } }
await first["tool.execute.before"]({ tool: "task", sessionID: "session", callID: "first-call" }, firstTask)
await second["tool.execute.before"]({ tool: "task", sessionID: "session", callID: "second-call" }, secondTask)
await first.event({ event: { type: "session.deleted", properties: { info: { id: "session" } } } })
const after = { output: "untrusted reviewer output", metadata: {} }
await second["tool.execute.after"]({ tool: "task", sessionID: "session", callID: "second-call", args: { subagent_type: "review-risk" } }, after)
console.log(JSON.stringify({ output: after.output }))
`
	output, log := runOpenCodeTransportPluginHarness(t, map[string]string{"plugin.mts": string(source)}, harness, posixRelayFixture)
	var result struct {
		Output string `json:"output"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decode relay harness output %q: %v", output, err)
	}
	if result.Output != "captured" {
		t.Fatalf("another instance's session cleanup must not kill this owner's live relay, got output %q", result.Output)
	}
	if got := strings.Count(log, `"operation":"complete"`); got != 1 {
		t.Fatalf("surviving registration must deliver exactly one completion frame, got %d; log=%q", got, log)
	}
}

func TestOpenCodeReviewTransportPluginPassesThroughIsolatedLegacyHook(t *testing.T) {
	current, err := Read("opencode/plugins/opencode-review-transport.ts")
	if err != nil {
		t.Fatal(err)
	}
	// This pre-registry fixture intentionally has its own relay map. It models a
	// managed plugin from before current instances shared a global registry.
	const legacy = `import { spawn } from "node:child_process"
const REVIEW_AGENTS = new Set(["review-risk", "review-resilience", "review-readability", "review-reliability", "review-refuter", "review-validator"])
const TRANSPORT = { Command: "gentle-ai", Schema: "gentle-ai.provider-transport/v1", Start: "start", Prompt: "prompt", Complete: "complete", Result: "result" }
function startRelay(cwd, prompt) {
  const child = spawn(TRANSPORT.Command, ["review", "opencode-transport"], { cwd, stdio: ["pipe", "pipe", "pipe"] })
  let buffered = ""
  let closed = false
  let resolvePrompt
  let rejectPrompt
  let resolveResult
  let rejectResult
  const promptFrame = new Promise((resolve, reject) => { resolvePrompt = resolve; rejectPrompt = reject })
  const resultFrame = new Promise((resolve, reject) => { resolveResult = resolve; rejectResult = reject })
  void promptFrame.catch(() => {})
  void resultFrame.catch(() => {})
  const fail = (cause) => { if (!closed) { closed = true; rejectPrompt(cause); rejectResult(cause) } }
  child.stdout.on("data", (chunk) => {
    buffered += chunk.toString("utf8")
    for (;;) {
      const newline = buffered.indexOf("\n")
      if (newline < 0) return
      const line = buffered.slice(0, newline)
      buffered = buffered.slice(newline + 1)
      try {
        const frame = JSON.parse(line)
        if (frame.schema !== TRANSPORT.Schema) throw new Error("invalid Go transport schema")
        if (frame.operation === TRANSPORT.Prompt && typeof frame.nonce === "string" && frame.nonce !== "" && typeof frame.prompt === "string" && frame.prompt !== "") { resolvePrompt({ nonce: frame.nonce, prompt: frame.prompt }); continue }
        if (frame.operation === TRANSPORT.Result && typeof frame.output === "string" && frame.output !== "") { closed = true; resolveResult(frame.output); continue }
        throw new Error("invalid Go relay frame")
      } catch (cause) { fail(cause) }
    }
  })
  child.stdin.on("error", fail)
  child.on("error", fail)
  child.on("close", () => { if (!closed) fail(new Error("Go review relay exited before completion")) })
  child.stdin.write(JSON.stringify({ schema: TRANSPORT.Schema, operation: TRANSPORT.Start, prompt }) + "\n", (cause) => { if (cause) fail(cause) })
  return {
    prompt: promptFrame,
    complete: async (output) => {
      const materialized = await promptFrame
      const completion = { schema: TRANSPORT.Schema, operation: TRANSPORT.Complete, nonce: materialized.nonce }
      if (typeof output === "string") completion.output = output
      else completion.error = "opencode_task_host_output_unavailable"
      child.stdin.end(JSON.stringify(completion) + "\n")
      return resultFrame
    },
    close: () => { if (!closed) closed = true; if (!child.killed) child.kill() },
  }
}
const legacyPlugin = async ({ directory, worktree }) => {
  const relays = new Map()
  const cwd = () => worktree || directory
  const clear = (key) => { const relay = relays.get(key); relays.delete(key); relay?.close() }
  const taskKey = (sessionID, callID) => sessionID + ":" + callID
  return {
    "tool.execute.before": async (input, output) => {
      if (input.tool !== "task" || typeof output.args?.subagent_type !== "string" || !REVIEW_AGENTS.has(output.args.subagent_type)) return
      if (typeof output.args.prompt !== "string") throw new Error("review task prompt is unavailable for Go relay materialization")
      const key = taskKey(input.sessionID, input.callID)
      if (relays.has(key)) throw new Error("duplicate review Task relay before hook")
      const relay = startRelay(cwd(), output.args.prompt)
      relays.set(key, relay)
      try { output.args.prompt = (await relay.prompt).prompt } catch (cause) { clear(key); throw cause }
    },
    "tool.execute.after": async (input, output) => {
      if (input.tool !== "task" || typeof input.args?.subagent_type !== "string" || !REVIEW_AGENTS.has(input.args.subagent_type)) return
      const key = taskKey(input.sessionID, input.callID)
      const relay = relays.get(key)
      if (!relay) throw new Error("review Task relay completion has no matching live before hook")
      relays.delete(key)
      try { output.output = await relay.complete(output.output) } finally { relay.close() }
    },
  }
}
export default legacyPlugin
`
	const harness = `import current from "./current.mts"
import legacy from "./legacy.mts"

const original = "Go must receive this original host prompt"
const materialized = "GENTLE_AI_REVIEW_PROVIDER_MATERIALIZATION {\"task_prompt\":\"Go must receive this original host prompt\"}\nGo-materialized immutable prompt"
const currentHooks = await current({ directory: process.cwd(), worktree: process.cwd() })
const legacyHooks = await legacy({ directory: process.cwd(), worktree: process.cwd() })
const before = async (hooks, callID, task) => hooks["tool.execute.before"]({ tool: "task", sessionID: "session", callID }, task)
const after = async (hooks, callID, task) => hooks["tool.execute.after"]({ tool: "task", sessionID: "session", callID, args: { subagent_type: "review-risk" } }, task)

for (const [beforeName, first, second] of [["legacy-before-current", legacyHooks, currentHooks], ["current-before-legacy", currentHooks, legacyHooks]]) {
  for (const secondaryFirst of [true, false]) {
    const task = { args: { subagent_type: "review-risk", prompt: original } }
    const firstCallID = beforeName + "-origin"
    const secondCallID = beforeName + "-reintercepted"
    await before(first, firstCallID, task)
    await before(second, secondCallID, task)
    if (task.args.prompt !== materialized) throw new Error(beforeName + " did not preserve Go materialization")
    const result = { output: "untrusted reviewer output", metadata: {} }
    if (secondaryFirst) {
      await after(second, secondCallID, result)
      if (result.output !== "untrusted reviewer output") throw new Error(beforeName + " secondary relay did not pass through host output")
      await after(first, firstCallID, result)
    } else {
      await after(first, firstCallID, result)
      if (result.output !== "captured") throw new Error(beforeName + " primary relay did not capture host output")
      await after(second, secondCallID, result)
    }
    if (result.output !== "captured") throw new Error(beforeName + " final output = " + result.output)
  }
}
console.log(JSON.stringify({ ok: true }))
`
	const relay = `#!/usr/bin/env node
import { appendFileSync } from "node:fs"
import { createInterface } from "node:readline"
const materializationHeader = "GENTLE_AI_REVIEW_PROVIDER_MATERIALIZATION "
let start
const lines = createInterface({ input: process.stdin })
lines.on("line", (line) => {
  const frame = JSON.parse(line)
  if (!start) {
    start = frame
    const prompt = frame.prompt.startsWith(materializationHeader) ? frame.prompt : materializationHeader + JSON.stringify({ task_prompt: frame.prompt }) + "\nGo-materialized immutable prompt"
    process.stdout.write(JSON.stringify({ schema: "gentle-ai.provider-transport/v1", operation: "prompt", nonce: "nonce", prompt }) + "\n")
    return
  }
  const secondary = start.prompt.startsWith(materializationHeader)
  appendFileSync(process.env.GENTLE_AI_RELAY_LOG, JSON.stringify({ secondary, output: frame.output }) + "\n")
  const output = secondary ? frame.output : "captured"
  process.stdout.write(JSON.stringify({ schema: "gentle-ai.provider-transport/v1", operation: "result", output }) + "\n")
})
`
	output, log := runOpenCodeTransportPluginHarness(t, map[string]string{"current.mts": string(current), "legacy.mts": legacy}, harness, relay)
	if string(output) != "{\"ok\":true}\n" {
		t.Fatalf("mixed transport plugin harness output = %q", output)
	}
	for _, want := range []struct {
		frame string
		count int
	}{
		{frame: `{"secondary":false,"output":"untrusted reviewer output"}`, count: 4},
		{frame: `{"secondary":true,"output":"untrusted reviewer output"}`, count: 2},
		{frame: `{"secondary":true,"output":"captured"}`, count: 2},
	} {
		if got := strings.Count(log, want.frame); got != want.count {
			t.Fatalf("relay frame %q count = %d, want %d; log=%q", want.frame, got, want.count, log)
		}
	}
}

func runOpenCodeTransportPluginHarness(t *testing.T, modules map[string]string, harness, relay string) (string, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the relay fixture uses a POSIX shell")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, source := range modules {
		if err := os.WriteFile(filepath.Join(root, name), []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "harness.mts"), []byte(harness), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "gentle-ai"), []byte(relay), 0o700); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(root, "relay.log")
	command := exec.Command(node, "harness.mts")
	command.Dir = root
	command.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "GENTLE_AI_RELAY_LOG="+logPath)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("transport plugin harness failed: %v\n%s", err, output)
	}
	log, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			// A harness that never spawns a relay leaves no frame log behind.
			return string(output), ""
		}
		t.Fatal(err)
	}
	return string(output), string(log)
}
