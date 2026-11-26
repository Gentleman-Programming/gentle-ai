// gentle-ai:managed opencode-review-transport/v2
// Thin wire adapter: Go owns binding, materialization, admission, and capture.
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
  LensAlreadyInProgress: "lens_already_in_progress",
  OutputRefused: "output_refused",
  ProviderFailed: "provider_failed",
  RelayUnavailable: "relay_unavailable",
  DispatchRefused: "dispatch_refused",
} as const
type RefusalReason = (typeof REFUSAL_REASON)[keyof typeof REFUSAL_REASON]
const REFUSAL_REASONS: ReadonlySet<string> = new Set(Object.values(REFUSAL_REASON))

// Bounded refusal causes. Go may name one beside the reason; the plugin only
// forwards an exact member, so admission prose, paths, or child bytes never
// reach the parent. An absent or unknown cause keeps the reason-only refusal.
const REFUSAL_CAUSES: ReadonlySet<string> = new Set([
  "reviewer_result_not_admissible",
  "validator_result_not_admissible",
  "targeted_validation_inconclusive",
  "role_capture_failed",
  "inspection_coverage",
  "invalid_finding_location",
  "proof_path_out_of_scope",
  "candidate_causality_unclaimed_id",
  "candidate_causality_evidence_degraded",
])

class RelayRefusal extends Error {
  readonly reason: RefusalReason
  readonly refusalCause: string | undefined
  constructor(reason: RefusalReason, refusalCause?: string) {
    super("Go review relay refused the Task")
    this.reason = reason
    this.refusalCause = refusalCause
  }
}

function refusalReason(cause: unknown): RefusalReason {
  return cause instanceof RelayRefusal ? cause.reason : REFUSAL_REASON.RelayUnavailable
}

function refusalCause(cause: unknown): string | undefined {
  return cause instanceof RelayRefusal ? cause.refusalCause : undefined
}

interface TransportFrame {
  schema: string
  operation: string
  nonce?: string
  prompt?: string
  agent?: string
  output?: string
  error?: string
  cause?: string
}

interface Relay {
  prompt: Promise<{ nonce: string; prompt: string }>
  complete: (output: unknown) => Promise<string>
  signal: AbortSignal
  close: () => void
}

interface RelayRegistration {
  owner: symbol
  relay: Relay
  originalPrompt: string
  completing: boolean
  childID?: string
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
  const abort = new AbortController()
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
    abort.abort()
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
          const detail = typeof frame.cause === "string" && REFUSAL_CAUSES.has(frame.cause) ? frame.cause : undefined
          fail(new RelayRefusal(reason, detail))
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
    signal: abort.signal,
    complete: async (output: unknown) => {
      const materialized = await promptFrame
      if (abort.signal.aborted) return resultFrame
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
const refusalText = (reason: RefusalReason, cause?: string) => `${REFUSED} (reason: ${reason}${cause === undefined ? "" : `, cause: ${cause}`})`
const refusal = (reason: RefusalReason, cause?: string) => Object.assign(new Error(refusalText(reason, cause)), { code: REFUSED, reason }, cause === undefined ? {} : { cause })

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
    const keyFor = (call: { sessionID: string; messageID?: string; id: string }, agent: string) => JSON.stringify([prefix, call.sessionID, call.messageID, call.id, agent])
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
    const backgroundOutput = async (registration: RelayRegistration, childID: string, parentID: string, agent: string, prompt: string) => {
      const { signal } = registration.relay
      if (signal.aborted) throw refusal(REFUSAL_REASON.RelayUnavailable)
      registration.childID = childID
      const read = async () => {
        const session = await ctx.session.get({ sessionID: childID })
        if (signal.aborted || session?.id !== childID || session.parentID !== parentID || session.agent !== agent) return
        await ctx.session.wait({ sessionID: childID })
        if (signal.aborted) return
        const messages = await ctx.session.context({ sessionID: childID })
        // A newly spawned child has exactly one user turn at the start. A
        // subsequent user turn (even with identical text) is not this review.
        const first = messages[0]
        if (first?.type !== "user" || first.text !== "You are a subagent spawned by another session.\n" + prompt) return
        const idle = messages.at(-1)
        if (idle?.type !== "idle" || idle.outcome !== "succeeded" || ("error" in idle && idle.error !== undefined)) return
        const terminal = messages.at(-2)
        if (terminal?.type !== "assistant" || terminal.finish !== "stop") return
        for (const message of messages.slice(1, -1)) {
          if (message.type !== "assistant" || message.agent !== agent || message.error !== undefined || typeof message.time.completed !== "number") return
          if (message !== terminal && message.finish !== "tool-calls") return
        }
        // The terminal text is opaque to the adapter. Go rejects empty or
        // invalid payloads; neither reasoning nor tool content is forwarded.
        return terminal.content.flatMap(part => part.type === "text" ? [part.text] : []).join("")
      }
      let cancel: () => void = () => {}
      const disconnected = new Promise<never>((_resolve, reject) => {
        cancel = () => reject(new RelayRefusal(REFUSAL_REASON.RelayUnavailable))
      })
      signal.addEventListener("abort", cancel, { once: true })
      try { return await Promise.race([read().catch(() => undefined), disconnected]) }
      finally { signal.removeEventListener("abort", cancel) }
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
        const suppliedPrompt = input.prompt
        // The host may remove background from the executable input. Inspect
        // the exact calling message/tool while its original input is running.
        const messages = await ctx.session.context({ sessionID: call.sessionID }).catch(() => [])
        if (disposed) return refuse(REFUSAL_REASON.RelayUnavailable)
        const matches = messages.filter(message => message.id === call.messageID)
        const [message] = matches
        if (matches.length !== 1 || message?.type !== "assistant" || message.agent !== call.agent) return refuse(REFUSAL_REASON.DispatchRefused)
        const parts = message.content.filter(part => part.type === "tool" && part.id === call.id)
        const [part] = parts
        if (parts.length !== 1 || part?.type !== "tool" || part.name !== call.tool || part.state.status !== "running") return refuse(REFUSAL_REASON.DispatchRefused)
        const original = reviewInput({ tool: part.name, input: part.state.input })
        if (!original || original.agent !== input.agent || original.background === true || original.sessionID !== undefined) return refuse(REFUSAL_REASON.DispatchRefused)
        const existing = relays.get(key)
        if (existing) {
          if (original.prompt !== existing.originalPrompt) return refuse(REFUSAL_REASON.DispatchRefused)
          // Concurrent duplicate instances must also wait for Go's materialization.
          let prompt: string
          try { prompt = (await existing.relay.prompt).prompt } catch (cause) { return refuse(refusalReason(cause)) }
          if (existing.relay.signal.aborted) return refuse(REFUSAL_REASON.RelayUnavailable)
          if (suppliedPrompt !== existing.originalPrompt && suppliedPrompt !== prompt) return refuse(REFUSAL_REASON.DispatchRefused)
          if (existing.owner !== owner) deferred.set(key, existing)
          input.prompt = prompt
          return
        }
        if (original.prompt !== suppliedPrompt) return refuse(REFUSAL_REASON.DispatchRefused)
        try {
          const relay = startRelay(ctx.location.directory, suppliedPrompt, input.agent as string)
          relays.set(key, { owner, relay, originalPrompt: suppliedPrompt, completing: false })
          const { prompt } = await relay.prompt
          relay.signal.throwIfAborted()
          input.prompt = prompt
        } catch (cause) {
          clearOwned(key)
          return refuse(refusalReason(cause))
        }
      }))
      registrations.push(await ctx.tool.hook("execute.after", async call => {
        const input = reviewInput(call)
        if (!input) return
        const key = keyFor(call, input.agent as string)
        const refuse = (reason: RefusalReason, cause?: string) => {
          if (call.status === "completed") call.result = { output: { status: "unavailable", code: REFUSED, reason, ...(cause === undefined ? {} : { cause }) }, content: refusalText(reason, cause) }
          throw refusal(reason, cause)
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
          const output = call.status === "completed" ? call.result.output : undefined
          const child = output && typeof output === "object" ? output as Record<string, unknown> : undefined
          const { prompt } = await registration.relay.prompt
          let raw: string | undefined
          if (input.prompt === prompt && typeof child?.sessionID === "string" && child.sessionID !== "" && child.sessionID !== call.sessionID) {
            if (child.status === "completed" && typeof child.output === "string") raw = child.output
            // A completed hook may only acknowledge a running child. Never
            // forward its acknowledgement or the independent parent inbox item.
            if (child.status === "running") raw = await backgroundOutput(registration, child.sessionID, call.sessionID, input.agent as string, prompt)
          }
          const result = await registration.relay.complete(raw)
          if (call.status === "completed") call.result = { ...call.result, output: { ...child, status: "completed", output: result }, content: result }
        } catch (cause) { return refuse(refusalReason(cause), refusalCause(cause)) }
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
            for (const [key, registration] of relays) if (key.startsWith(sessionPrefix) || registration.childID === event.data.sessionID) clearOwned(key)
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
