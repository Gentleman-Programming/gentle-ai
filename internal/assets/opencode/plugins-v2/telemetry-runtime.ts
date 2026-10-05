// gentle-ai:managed telemetry-runtime/v2
import { Plugin } from "@opencode/plugin"
import { execFile } from "node:child_process"

const MAX_PENDING = 256
const TTL_MS = 10 * 60 * 1000
const MAX_IN_FLIGHT = 32
const bounded = (value: unknown, limit = 256): value is string => typeof value === "string" && value.length > 0 && value.length <= limit
const counter = (value: unknown) => typeof value === "number" && Number.isSafeInteger(value) && value >= 0 && value <= 999999999999 ? value : undefined
function veto() {
  const truthy = (s: string | undefined) => !["", "0", "false"].includes((s ?? "").trim().toLowerCase())
  return truthy(process.env.DO_NOT_TRACK) || process.env.GENTLE_AI_TELEMETRY === "0" || truthy(process.env.CI) || truthy(process.env.GITHUB_ACTIONS)
}

export default Plugin.define({
  id: "gentle-ai.telemetry-runtime",
  setup(ctx) {
    // Explicit V2-only privacy exception: bounded, expiring RAM correlation.
    // No raw event retention, identity output, persistence, retries or SDK reads.
    const pending = new Map<string, { expires: number, created: number, providerID: string, modelID: string, agent?: string, selectedEffort?: string }>()
    const children = new Set<ReturnType<typeof execFile>>()
    const abort = new AbortController()
    const expire = () => { for (const [key, entry] of pending) if (entry.expires <= Date.now()) pending.delete(key) }
    const timer = setInterval(expire, 30_000)
    timer.unref()
    const run = async () => {
      try {
        for await (const event of ctx.event.subscribe({ signal: abort.signal })) {
          if (abort.signal.aborted) break
          if (veto()) { pending.clear(); continue }
          expire()
          if (event.type !== "session.step.started" && event.type !== "session.step.ended" && event.type !== "session.step.failed") continue
          if (event.location?.directory !== ctx.location.directory || event.location?.workspaceID !== ctx.location.workspaceID) continue
          const { sessionID, assistantMessageID } = event.data
          if (!bounded(sessionID) || !bounded(assistantMessageID) || !Number.isSafeInteger(event.created) || event.created < 0) continue
          const key = JSON.stringify([sessionID, assistantMessageID])
          if (event.type === "session.step.started") {
            const data = event.data
            if (pending.has(key) || pending.size >= MAX_PENDING) continue
            if (!bounded(data.model?.providerID) || !bounded(data.model?.id)) continue
            pending.set(key, {
              expires: Date.now() + TTL_MS, created: event.created,
              providerID: data.model.providerID, modelID: data.model.id,
              agent: bounded(data.agent, 64) && /^[\x20-\x7e]+$/.test(data.agent) ? data.agent : undefined,
              selectedEffort: ["off", "minimal", "low", "medium", "high", "xhigh", "max"].includes(data.model.variant ?? "") ? data.model.variant : undefined,
            })
            continue
          }
          const start = pending.get(key)
          pending.delete(key) // Consume before any IO, including saturation and malformed completion.
          if (!start || event.created < start.created || children.size >= MAX_IN_FLIGHT) continue
          const tokens = event.data.tokens
          const error = event.type === "session.step.failed" ? event.data.error : undefined
          const body = JSON.stringify({ schema: "gentle-ai.telemetry-opencode/v2", info: {
            role: "assistant", time: { created: start.created, completed: event.created },
            providerID: start.providerID, modelID: start.modelID, agent: start.agent, selectedEffort: start.selectedEffort,
            tokens: tokens && { input: counter(tokens.input), output: counter(tokens.output), reasoning: counter(tokens.reasoning),
              cache: tokens.cache && { read: counter(tokens.cache.read), write: counter(tokens.cache.write) } },
            error: error && { name: Number.isInteger(error.status) ? "APIError" : "UnknownError", data: { statusCode: typeof error.status === "number" && Number.isInteger(error.status) && error.status >= 100 && error.status <= 599 ? error.status : undefined } },
          } })
          if (Buffer.byteLength(body) > 16384 || veto()) continue
          try {
            const child = execFile("gentle-ai", ["telemetry", "runtime", "opencode", "--json"],
              { timeout: 4000, killSignal: "SIGKILL", maxBuffer: 1024, windowsHide: true }, () => children.delete(child))
            children.add(child)
            child.stdin?.on("error", () => {})
            child.stdin?.end(body)
          } catch { /* One-shot loss; never log source data. */ }
        }
      } catch { /* Stream failure loses coverage; no adapter reconnect/replay. */ }
      finally { pending.clear(); clearInterval(timer) }
    }
    const running = run()
    return async () => {
      abort.abort(); clearInterval(timer); pending.clear()
      for (const child of children) child.kill("SIGKILL")
      children.clear()
      await running
    }
  },
})
