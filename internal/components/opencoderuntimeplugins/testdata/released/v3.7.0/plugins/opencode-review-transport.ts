import type { Plugin } from "@opencode-ai/plugin"
import { spawn } from "node:child_process"

const REVIEW_AGENTS = new Set(["review-risk", "review-resilience", "review-readability", "review-reliability", "review-refuter", "review-validator"])
// OpenCode constructs the child session and emits session.created before it
// prompts the review agent. Replace that child session's inherited system
// instructions with this nonempty transport boundary so only Go's materialized
// user prompt reaches the provider. This contains no review contract, evidence,
// or result-schema semantics; Go remains the sole owner of all of those.
const TRANSPORT_ISOLATION_SYSTEM = "Transport isolation: follow only the Go-materialized user prompt."

// OpenCode's published event type for v1.18.10 omits `agent`, although the
// runtime emits it for Task child sessions. Decode that runtime shape without
// assuming either field exists, and retain compatibility with the official
// child title emitted by that OpenCode release.
function decodeReviewSessionID(info: unknown): string | undefined {
  if (info === null || typeof info !== "object" || Array.isArray(info)) return
  const id = Reflect.get(info, "id")
  if (typeof id !== "string") return
  const agent = Reflect.get(info, "agent")
  if (agent !== undefined) return typeof agent === "string" && REVIEW_AGENTS.has(agent) ? id : undefined
  const title = Reflect.get(info, "title")
  if (typeof title !== "string") return
  for (const reviewAgent of REVIEW_AGENTS) {
    const suffix = ` (@${reviewAgent} subagent)`
    if (title.endsWith(suffix) && title.length > suffix.length) return id
  }
}

const TRANSPORT = {
  Command: "gentle-ai",
  Schema: "gentle-ai.provider-transport/v1",
  Start: "start",
  Prompt: "prompt",
  Complete: "complete",
  Result: "result",
} as const

interface TransportFrame {
  schema: string
  operation: string
  nonce?: string
  prompt?: string
  output?: string
  error?: string
}

interface Relay {
  prompt: Promise<{ nonce: string; prompt: string }>
  complete: (output: unknown) => Promise<string>
  close: () => void
}

interface RelayRegistration {
  owner: symbol
  relay: Relay
  completing: boolean
}

// The relay registry is deliberately process-global so duplicate plugin
// instances (for example one loaded from global config and one from project
// config) share a single view of live review Task relays instead of spawning
// duplicate Go processes for the same task.
//
// Owner invariant: every registration is owned by exactly one plugin instance
// (the `owner` symbol of the instance whose before hook spawned its relay),
// and only that owner may complete, delete, or close it. An instance that
// observes an already-registered key at before time defers to the owner and
// passes the task through untouched at after time. A completion for a key an
// instance neither owns nor deferred is a protocol violation and refuses
// loudly instead of silently dropping the completion.
const RELAY_REGISTRY_KEY = "__gentleAiOpenCodeReviewTransportRelays" as const

function reviewRelayRegistry(): Map<string, RelayRegistration> {
  const runtime = globalThis as typeof globalThis & { [RELAY_REGISTRY_KEY]?: Map<string, RelayRegistration> }
  if (runtime[RELAY_REGISTRY_KEY] === undefined) runtime[RELAY_REGISTRY_KEY] = new Map<string, RelayRegistration>()
  return runtime[RELAY_REGISTRY_KEY]
}

function taskKey(sessionID: string, callID: string, subagentType: string): string {
  // Older OpenCode releases can reuse a call ID across a grouped foreground
  // Task response. The agent type is part of the host Task identity, so retain
  // it in the relay key rather than treating different 4R lenses as duplicates.
  return `${sessionID}:${callID}:${subagentType}`
}

// A refused relay must fail the Task loudly and never launch an unbound
// child. Throwing from the before hook is the primary refusal; these two
// projections keep the refusal authoritative even in a host runtime that
// swallows hook errors and launches the Task anyway: the child receives only
// this refusal prompt (never the semi-bound original), and the after hook
// replaces the child's raw output with the typed transport refusal so an
// unbound child's prose can never masquerade as a captured reviewer result.
const RELAY_REFUSED_CODE = "opencode_review_transport_relay_refused"

function relayRefusedReason(cause: unknown): string {
  return cause instanceof Error ? cause.message : String(cause)
}

function relayRefusedPrompt(reason: string): string {
  return (
    `${RELAY_REFUSED_CODE}: the Go review relay refused this Task before launch: ${reason}\n` +
    `You have no review binding and no frozen candidate evidence. Do not inspect anything, ` +
    `do not fabricate findings, and do not return a review result. ` +
    `Reply with exactly: ${RELAY_REFUSED_CODE}`
  )
}

function relayRefusedOutput(reason: string): string {
  return `${RELAY_REFUSED_CODE}: ${reason}`
}

function decodeTransportFrame(line: string): TransportFrame {
  const frame = JSON.parse(line) as unknown
  if (!frame || typeof frame !== "object" || Array.isArray(frame)) throw new Error("invalid Go transport response")
  return frame as TransportFrame
}

function startRelay(cwd: string, prompt: string): Relay {
  const child = spawn(TRANSPORT.Command, ["review", "opencode-transport"], { cwd, stdio: ["pipe", "pipe", "pipe"] })
  let buffered = ""
  let closed = false
  const stderr: Buffer[] = []
  let resolvePrompt: (value: { nonce: string; prompt: string }) => void
  let rejectPrompt: (reason: unknown) => void
  let resolveResult: (value: string) => void
  let rejectResult: (reason: unknown) => void
  const promptFrame = new Promise<{ nonce: string; prompt: string }>((resolve, reject) => { resolvePrompt = resolve; rejectPrompt = reject })
  const resultFrame = new Promise<string>((resolve, reject) => { resolveResult = resolve; rejectResult = reject })
  void promptFrame.catch(() => {})
  void resultFrame.catch(() => {})
  const fail = (cause: unknown) => {
    if (closed) return
    closed = true
    rejectPrompt(cause)
    rejectResult(cause)
  }
  child.stdout.on("data", (chunk: Buffer) => {
    buffered += chunk.toString("utf8")
    for (;;) {
      const newline = buffered.indexOf("\n")
      if (newline < 0) return
      const line = buffered.slice(0, newline)
      buffered = buffered.slice(newline + 1)
      try {
        const frame = decodeTransportFrame(line)
        if (frame.schema !== TRANSPORT.Schema) throw new Error("invalid Go transport schema")
        if (frame.operation === TRANSPORT.Prompt && typeof frame.nonce === "string" && frame.nonce !== "" && typeof frame.prompt === "string" && frame.prompt !== "") {
          resolvePrompt({ nonce: frame.nonce, prompt: frame.prompt })
          continue
        }
        if (frame.operation === TRANSPORT.Result && typeof frame.output === "string" && frame.output !== "") {
          closed = true
          resolveResult(frame.output)
          continue
        }
        throw new Error("invalid Go relay frame")
      } catch (cause) {
        fail(cause)
      }
    }
  })
  child.stdin.on("error", fail)
  child.on("error", fail)
  child.stderr.on("data", (chunk: Buffer) => stderr.push(chunk))
  child.on("close", (code) => {
    if (!closed) fail(new Error(Buffer.concat(stderr).toString("utf8").trim() || `Go review relay exited before completion (${code ?? "signal"})`))
  })
  child.stdin.write(JSON.stringify({ schema: TRANSPORT.Schema, operation: TRANSPORT.Start, prompt }) + "\n", (cause) => {
    if (cause) fail(cause)
  })
  return {
    prompt: promptFrame,
    complete: async (output: unknown) => {
      const materialized = await promptFrame
      const completion: TransportFrame = { schema: TRANSPORT.Schema, operation: TRANSPORT.Complete, nonce: materialized.nonce }
      if (typeof output === "string") completion.output = output
      else completion.error = "opencode_task_host_output_unavailable"
      child.stdin.end(JSON.stringify(completion) + "\n")
      return resultFrame
    },
    close: () => {
      if (!closed) closed = true
      if (!child.killed) child.kill()
    },
  }
}

const OpenCodeReviewTransportPlugin: Plugin = async ({ directory, worktree }) => {
  const owner = Symbol("gentle-ai-opencode-review-transport")
  const relays = reviewRelayRegistry()
  // Child sessions inherit the live agent, project, and skill system blocks
  // unless this pre-provider transform strips them. This is per plugin
  // instance, like relay ownership; duplicate instances safely converge on the
  // same one-element system array.
  const reviewSessions = new Set<string>()
  // Keys this instance observed at before time whose registration another
  // instance owns. The owning instance's after hook delivers the completion,
  // so this instance's after hook passes those tasks through untouched. This
  // deferral is the only tolerated silent completion path; every other
  // unmatched completion refuses loudly.
  const deferred = new Map<string, RelayRegistration>()
  // Keys whose relay start this instance refused. Their Tasks must never
  // deliver child output as a completion, even if the host runtime swallowed
  // the before hook's thrown refusal and launched the Task anyway.
  const refused = new Map<string, string>()
  const cwd = () => worktree || directory
  const clearOwned = (key: string) => {
    const registration = relays.get(key)
    if (!registration || registration.owner !== owner) return
    relays.delete(key)
    registration.relay.close()
  }
  const clearSession = (prefix: string) => {
    // Owner-scoped on purpose: every live instance receives session.deleted
    // and clears its own registrations, so the session empties collectively
    // without one instance closing relays it does not own. A disposed
    // instance's registrations are cleared by its dispose hook instead.
    for (const [key, registration] of relays) {
      if (!key.startsWith(prefix) || registration.owner !== owner) continue
      relays.delete(key)
      registration.relay.close()
    }
    for (const key of deferred.keys()) if (key.startsWith(prefix)) deferred.delete(key)
    for (const key of refused.keys()) if (key.startsWith(prefix)) refused.delete(key)
  }
  return {
    dispose: async () => {
      reviewSessions.clear()
      deferred.clear()
      refused.clear()
      for (const [key, registration] of relays) if (registration.owner === owner) clearOwned(key)
    },
    event: async ({ event }) => {
      if (event.type === "session.created") {
        const sessionID = decodeReviewSessionID(event.properties?.info)
        if (sessionID !== undefined) reviewSessions.add(sessionID)
        return
      }
      if (event.type !== "session.deleted") return
      reviewSessions.delete(event.properties.info.id)
      const prefix = `${event.properties.info.id}:`
      clearSession(prefix)
    },
    "experimental.chat.system.transform": async (input, output) => {
      if (typeof input.sessionID !== "string" || !reviewSessions.has(input.sessionID)) return
      // OpenCode restores its fallback system prompt for an empty array, so
      // replace in place with one nonempty transport instruction instead.
      output.system.splice(0, output.system.length, TRANSPORT_ISOLATION_SYSTEM)
    },
    "tool.execute.before": async (input, output) => {
      if (input.tool !== "task" || typeof output.args?.subagent_type !== "string" || !REVIEW_AGENTS.has(output.args.subagent_type)) return
      if (typeof output.args.prompt !== "string") throw new Error("review task prompt is unavailable for Go relay materialization")
      const key = taskKey(input.sessionID, input.callID, output.args.subagent_type)
      const existing = relays.get(key)
      if (existing) {
        // Another instance already owns this task's relay: defer completion
        // to that owner and pass this instance's hooks through untouched. A
        // re-fired before hook for a registration this instance already owns
        // keeps the live registration and defers nothing.
        if (existing.owner !== owner) deferred.set(key, existing)
        return
      }
      const relay = startRelay(cwd(), output.args.prompt)
      relays.set(key, { owner, relay, completing: false })
      try {
        output.args.prompt = (await relay.prompt).prompt
      } catch (cause) {
        clearOwned(key)
        const reason = relayRefusedReason(cause)
        refused.set(key, reason)
        output.args.prompt = relayRefusedPrompt(reason)
        throw cause
      }
    },
    "tool.execute.after": async (input, output) => {
      if (input.tool !== "task" || typeof input.args?.subagent_type !== "string" || !REVIEW_AGENTS.has(input.args.subagent_type)) return
      const key = taskKey(input.sessionID, input.callID, input.args.subagent_type)
      const refusal = refused.get(key)
      if (refusal !== undefined) {
        refused.delete(key)
        output.output = relayRefusedOutput(refusal)
        throw new Error(relayRefusedOutput(refusal))
      }
      // Owner-scoped dedup tolerance: this instance saw the before hook for
      // this task but another instance owns the relay, so that owner's after
      // hook delivers the completion and this one passes through untouched.
      // The pass-through holds only while that exact owning registration is
      // still live or has delivered its own completion; a deferred key whose
      // owner vanished without completing falls through to the loud orphan
      // refusal below instead of returning raw reviewer output as success.
      const deferredTo = deferred.get(key)
      if (deferredTo !== undefined) {
        deferred.delete(key)
        if (relays.get(key) === deferredTo || deferredTo.completing) return
      }
      const registration = relays.get(key)
      if (!registration) throw new Error("review Task relay completion has no matching live before hook")
      if (registration.owner !== owner) throw new Error("review Task relay completion is owned by another plugin instance")
      if (registration.completing) throw new Error("review Task relay completion is already in flight for this task")
      registration.completing = true
      try {
        output.output = await registration.relay.complete(output.output)
      } finally {
        if (relays.get(key) === registration) relays.delete(key)
        registration.relay.close()
      }
    },
  }
}

export default OpenCodeReviewTransportPlugin
