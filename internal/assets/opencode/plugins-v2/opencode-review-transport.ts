// gentle-ai:managed opencode-review-transport/v2
// Staged wire adapter. Native capability remains unavailable pending runtime proof.
import { Plugin } from "@opencode/plugin"
import { spawn } from "node:child_process"
const RELAY_CONTRACT = "gentle-ai.opencode-relay/v2-staged"
const RELAY_CONTRACT_ENV = "GENTLE_AI_OPENCODE_RELAY_CONTRACT"
const REVIEW_AGENTS = new Set(["review-risk", "review-resilience", "review-readability", "review-reliability", "review-refuter", "review-validator"])
const TRANSPORT = {
  Command: "gentle-ai",
  Schema: "gentle-ai.provider-transport/v1",
  Start: "start",
  Prompt: "prompt",
  Complete: "complete",
  Result: "result",
  Refused: "refused",
} as const

// Bounded refusal reasons. Go names one in its refused frame; the plugin only
// forwards an exact member, so relay text, paths, or child bytes never reach
// the parent. dispatch_refused is the plugin's own pre-relay refusal.
const REFUSAL_REASON = {
  CapabilityUnavailable: "capability_unavailable",
  EnvelopeInvalid: "envelope_invalid",
  AgentMismatch: "agent_mismatch",
  BindingMismatch: "binding_mismatch",
  StaleAuthority: "stale_authority",
  OutputRefused: "output_refused",
  ProviderFailed: "provider_failed",
  RelayUnavailable: "relay_unavailable",
  DispatchRefused: "dispatch_refused",
} as const
type RefusalReason = (typeof REFUSAL_REASON)[keyof typeof REFUSAL_REASON]
const REFUSAL_REASONS: ReadonlySet<string> = new Set(Object.values(REFUSAL_REASON))

class RelayRefusal extends Error {
  readonly reason: RefusalReason
  constructor(reason: RefusalReason) {
    super("Go review relay refused the Task")
    this.reason = reason
  }
}

function refusalReason(cause: unknown): RefusalReason {
  return cause instanceof RelayRefusal ? cause.reason : REFUSAL_REASON.RelayUnavailable
}

interface TransportFrame {
  schema: string
  operation: string
  nonce?: string
  prompt?: string
  agent?: string
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

function decodeTransportFrame(line: string): TransportFrame {
  const frame = JSON.parse(line) as unknown
  if (!frame || typeof frame !== "object" || Array.isArray(frame)) throw new Error("invalid Go transport response")
  return frame as TransportFrame
}

// The dispatched host agent travels with the prompt so Go can bind it to the
// Task role; the prompt alone never selects the admitted role.
function startRelay(cwd: string, prompt: string, agent: string): Relay {
  const child = spawn(TRANSPORT.Command, ["review", "opencode-transport"], { cwd, env: { ...process.env, [RELAY_CONTRACT_ENV]: RELAY_CONTRACT }, stdio: ["pipe", "pipe", "pipe"] })
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
        if (frame.operation === TRANSPORT.Refused) {
          const reason = typeof frame.error === "string" && REFUSAL_REASONS.has(frame.error) ? frame.error as RefusalReason : REFUSAL_REASON.RelayUnavailable
          fail(new RelayRefusal(reason))
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
  child.stdin.write(JSON.stringify({ schema: TRANSPORT.Schema, operation: TRANSPORT.Start, prompt, agent }) + "\n", (cause) => {
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
      fail(new Error("Go review relay disposed"))
      if (!child.killed) child.kill()
    },
  }
}


const REGISTRY = "__gentleAiOpenCodeV2ReviewRelays"
function registry(): Map<string, RelayRegistration> {
  const runtime = globalThis as typeof globalThis & { [REGISTRY]?: Map<string, RelayRegistration> }
  return runtime[REGISTRY] ??= new Map()
}
const REFUSED = "opencode_review_transport_relay_refused"
const refusalText = (reason: RefusalReason) => `${REFUSED} (reason: ${reason})`
const refusal = (reason: RefusalReason) => Object.assign(new Error(refusalText(reason)), { code: REFUSED, reason })

export default Plugin.define({
  id: "gentle-ai.opencode-review-transport",
  async setup(ctx) {
    const owner = Symbol("review-relay")
    const relays = registry()
    const deferred = new Map<string, RelayRegistration>()
    const refused = new Map<string, RefusalReason>()
    const registrations: Array<{ dispose(): Promise<void> }> = []
    const abort = new AbortController()
    const location = [ctx.location.directory, ctx.location.workspaceID ?? ""]
    const prefix = JSON.stringify(location)
    const keyFor = (call: { sessionID: string; id: string }, agent: string) => JSON.stringify([prefix, call.sessionID, call.id, agent])
    const reviewInput = (call: { tool: string; input: unknown }): Record<string, unknown> | undefined => {
      if (call.tool !== "subagent" || !call.input || typeof call.input !== "object") return
      const input = call.input as Record<string, unknown>
      if (typeof input.agent === "string" && REVIEW_AGENTS.has(input.agent)) return input
    }
    const clearOwned = (key: string) => {
      const registration = relays.get(key)
      if (registration?.owner !== owner) return
      relays.delete(key)
      registration.relay.close()
    }
    let disposed = false
    const cleanup = async () => {
      disposed = true
      abort.abort()
      for (const [key, registration] of relays) if (registration.owner === owner) clearOwned(key)
      deferred.clear()
      refused.clear()
      await Promise.all(registrations.map(registration => registration.dispose()))
    }
    try {
      registrations.push(await ctx.tool.hook("execute.before", async call => {
        const input = reviewInput(call)
        if (!input) return
        const key = keyFor(call, input.agent as string)
        const refuse = (reason: RefusalReason) => { refused.set(key, reason); input.prompt = REFUSED; throw refusal(reason) }
        if (disposed) return refuse(REFUSAL_REASON.RelayUnavailable)
        if (typeof input.prompt !== "string" || input.prompt === "" || input.background === true || input.sessionID !== undefined) return refuse(REFUSAL_REASON.DispatchRefused)
        const existing = relays.get(key)
        if (existing) {
          if (existing.owner !== owner) deferred.set(key, existing)
          // Concurrent duplicate instances must also wait for Go's materialization.
          try { input.prompt = (await existing.relay.prompt).prompt } catch (cause) { return refuse(refusalReason(cause)) }
          return
        }
        try {
          const relay = startRelay(ctx.location.directory, input.prompt, input.agent as string)
          relays.set(key, { owner, relay, completing: false })
          input.prompt = (await relay.prompt).prompt
        } catch (cause) {
          clearOwned(key)
          return refuse(refusalReason(cause))
        }
      }))
      registrations.push(await ctx.tool.hook("execute.after", async call => {
        const input = reviewInput(call)
        if (!input) return
        const key = keyFor(call, input.agent as string)
        const refuse = (reason: RefusalReason) => {
          if (call.status === "completed") call.result = { output: { status: "unavailable", code: REFUSED, reason }, content: refusalText(reason) }
          throw refusal(reason)
        }
        const earlier = refused.get(key)
        if (earlier !== undefined) {
          refused.delete(key)
          return refuse(earlier)
        }
        if (disposed) return refuse(REFUSAL_REASON.RelayUnavailable)
        const deferredTo = deferred.get(key)
        if (deferredTo) {
          deferred.delete(key)
          if (relays.get(key) === deferredTo || deferredTo.completing) return
        }
        const registration = relays.get(key)
        if (!registration || registration.owner !== owner || registration.completing) return refuse(REFUSAL_REASON.RelayUnavailable)
        registration.completing = true
        try {
          // Hook completion alone may acknowledge a running background child.
          const output = call.status === "completed" ? call.result.output : undefined
          const child = output && typeof output === "object" ? output as Record<string, unknown> : undefined
          const raw = child?.status === "completed" && typeof child.sessionID === "string" && child.sessionID !== "" && typeof child.output === "string" ? child.output : undefined
          const result = await registration.relay.complete(raw)
          if (call.status === "completed") call.result = { ...call.result, output: { ...child, output: result }, content: result }
        } catch (cause) { return refuse(refusalReason(cause)) }
        finally { clearOwned(key) }
      }))
      registrations.push(await ctx.shell.hook("create.before", invocation => {
        invocation.env[RELAY_CONTRACT_ENV] = RELAY_CONTRACT
      }))
      void (async () => {
        try {
          for await (const event of ctx.event.subscribe({ signal: abort.signal })) {
            if (event.type !== "session.deleted" || event.location?.directory !== location[0] || (event.location?.workspaceID ?? "") !== location[1]) continue
            const sessionPrefix = JSON.stringify([prefix, event.data.sessionID]).slice(0, -1) + ","
            for (const key of relays.keys()) if (key.startsWith(sessionPrefix)) clearOwned(key)
            for (const key of deferred.keys()) if (key.startsWith(sessionPrefix)) deferred.delete(key)
            for (const key of refused.keys()) if (key.startsWith(sessionPrefix)) refused.delete(key)
          }
        } catch { /* Disposal also aborts the subscription. */ }
        finally { for (const [key, registration] of relays) if (registration.owner === owner) clearOwned(key) }
      })()
    } catch (cause) { await cleanup(); throw cause }
    return cleanup
  },
})
