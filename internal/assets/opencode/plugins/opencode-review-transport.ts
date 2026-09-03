import type { Plugin } from "@opencode-ai/plugin"
import { spawn } from "node:child_process"
import { statSync } from "node:fs"
import { delimiter, join } from "node:path"

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

// Binary handshake (issue #3049): a stale PATH `gentle-ai` can answer the
// relay spawn for a newer binary's authority without ever knowing the
// provider-transport/v1 capability. Probe `gentle-ai --version` before the
// relay spawn, refuse on skew or ENOENT, and cache by resolved path + mtime
// so a mid-session upgrade or fresh session both re-probe. The two typed
// codes route through the same refused-prompt / refused-output machinery so
// a refused handshake still fails the Task loudly.
const BINARY_SKEW_CODE = "opencode_review_transport_binary_skew"
const BINARY_UNAVAILABLE_CODE = "opencode_review_transport_binary_unavailable"

// Version that shipped the `gentle-ai.provider-transport/v1` capability
// baked into this plugin. A PATH binary reporting a strictly older semver
// is refused with BINARY_SKEW_CODE before the relay spawn.
const MIN_GENTLE_AI_VERSION = "2.4.0"

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

// Resolve the PATH entry that `spawn("gentle-ai", ...)` would pick. Walking
// PATH ourselves is the only way to get a stable cache key for the
// mtime-based invalidation the spec requires. `statSync` follows symlinks
// so a swap of the symlink target surfaces as an mtime drift on the same
// path.
function resolveGentleAiPath(): { path: string; mtime: number } | null {
  const isWindows = process.platform === "win32"
  const extensions = isWindows ? [".exe", ".cmd", ".bat", ""] : [""]
  for (const dir of (process.env.PATH ?? "").split(delimiter)) {
    if (dir === "") continue
    for (const ext of extensions) {
      const candidate = join(dir, `gentle-ai${ext}`)
      try {
        const stat = statSync(candidate)
        if (stat.isFile()) return { path: candidate, mtime: stat.mtimeMs }
      } catch {
        continue
      }
    }
  }
  return null
}

const RELAY_PROBE_CACHE_KEY = "__gentleAiOpenCodeReviewTransportProbeCache" as const
type ProbeCacheEntry = { mtime: number; version: string }
type ProbeResult = { path: string; mtime: number; version: string }

function relayProbeCache(): Map<string, ProbeCacheEntry> {
  const runtime = globalThis as typeof globalThis & { [RELAY_PROBE_CACHE_KEY]?: Map<string, ProbeCacheEntry> }
  if (runtime[RELAY_PROBE_CACHE_KEY] === undefined) runtime[RELAY_PROBE_CACHE_KEY] = new Map<string, ProbeCacheEntry>()
  return runtime[RELAY_PROBE_CACHE_KEY]
}

function clearRelayProbeCache(): void {
  relayProbeCache().clear()
}

// Parse `gentle-ai <semver>\n` from `--version` stdout. Anything else is
// treated as a probe failure so a binary that does not implement the
// version command cannot be mistaken for a healthy handshake.
function parseGentleAiVersion(stdout: string): string | undefined {
  const match = /^gentle-ai\s+(\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?)\s*$/m.exec(stdout)
  return match?.[1]
}

// Compare two dot-separated semvers segment by segment: numeric segments
// as integers, non-numeric segments lexicographically. Intentionally
// narrower than full semver ordering because the contract is "PATH version
// >= the version that shipped provider-transport/v1"; build-metadata and
// pre-release precedence edge cases are out of scope for the refusal.
function compareSemver(pathVersion: string, minVersion: string): number {
  const parts = (version: string) => version.split(/[.-]/).map((segment) => /^\d+$/.test(segment) ? Number(segment) : segment)
  const [left, right] = [parts(pathVersion), parts(minVersion)]
  const length = Math.max(left.length, right.length)
  for (let index = 0; index < length; index++) {
    const a = left[index] ?? 0
    const b = right[index] ?? 0
    if (typeof a === "number" && typeof b === "number") {
      if (a !== b) return a < b ? -1 : 1
      continue
    }
    const sa = String(a)
    const sb = String(b)
    if (sa !== sb) return sa < sb ? -1 : 1
  }
  return 0
}

function runGentleAiVersion(resolved: string): Promise<{ code: number | null; stdout: string } | null> {
  return new Promise((settle) => {
    const child = spawn(resolved, ["--version"], { stdio: ["ignore", "pipe", "pipe"] })
    let stdout = ""
    let done = false
    const finish = (value: { code: number | null; stdout: string } | null) => {
      if (done) return
      done = true
      settle(value)
    }
    child.stdout.on("data", (chunk: Buffer) => { stdout += chunk.toString("utf8") })
    child.on("error", () => finish(null))
    child.on("close", (code) => finish({ code, stdout }))
  })
}

async function probeGentleAiBinary(): Promise<ProbeResult | null> {
  const resolved = resolveGentleAiPath()
  if (resolved === null) return null
  const cache = relayProbeCache()
  const cached = cache.get(resolved.path)
  if (cached !== undefined && cached.mtime === resolved.mtime) {
    return { path: resolved.path, mtime: cached.mtime, version: cached.version }
  }
  const probe = await runGentleAiVersion(resolved.path)
  if (probe === null || probe.code !== 0) return null
  const version = parseGentleAiVersion(probe.stdout)
  if (version === undefined) return null
  // Re-stat the resolved path: between resolveGentleAiPath and the spawn
  // closing, an upgrade could have swapped the file under us, and the
  // cached mtime must match the binary we actually probed.
  let mtime = resolved.mtime
  try {
    mtime = statSync(resolved.path).mtimeMs
  } catch {
    return null
  }
  cache.set(resolved.path, { mtime, version })
  return { path: resolved.path, mtime, version }
}

function binarySkewReason(resolvedPath: string, pathVersion: string): string {
  return (
    `${BINARY_SKEW_CODE}: PATH resolves gentle-ai to ${resolvedPath} version ${pathVersion}, ` +
    `which is older than the minimum ${MIN_GENTLE_AI_VERSION} this plugin requires. ` +
    `Inspect the path with: which -a gentle-ai`
  )
}

function binaryUnavailableReason(): string {
  return (
    `${BINARY_UNAVAILABLE_CODE}: gentle-ai --version could not be spawned (ENOENT or spawn error); ` +
    `the relay child cannot start. See issue #2971 for the install-side fix.`
  )
}

async function runBinaryHandshake(): Promise<void> {
  const result = await probeGentleAiBinary()
  if (result === null) throw new Error(binaryUnavailableReason())
  if (compareSemver(result.version, MIN_GENTLE_AI_VERSION) < 0) {
    throw new Error(binarySkewReason(result.path, result.version))
  }
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
      // Clear the probe cache so the next relay in a new session re-probes.
      // Within a session, relays still reuse the cached mtime probe.
      clearRelayProbeCache()
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
      try {
        await runBinaryHandshake()
        const relay = startRelay(cwd(), output.args.prompt)
        relays.set(key, { owner, relay, completing: false })
        output.args.prompt = (await relay.prompt).prompt
      } catch (cause) {
        const registration = relays.get(key)
        if (registration !== undefined && registration.owner === owner) {
          clearOwned(key)
        }
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
