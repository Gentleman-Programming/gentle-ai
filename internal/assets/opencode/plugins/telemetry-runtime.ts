// gentle-ai:managed telemetry-runtime/v1
// Published plugin 1.18.30 imports SDK V1, not /v2:
// https://unpkg.com/@opencode-ai/plugin@1.18.30/dist/index.d.ts
import type { Plugin } from "@opencode-ai/plugin"
import { execFile } from "node:child_process"

const MAX_IN_FLIGHT = 32
const MAX_BYTES = 16384

function veto(): boolean {
  const truthy = (value: string | undefined) => !["", "0", "false"].includes((value ?? "").trim().toLowerCase())
  return truthy(process.env.DO_NOT_TRACK) || process.env.GENTLE_AI_TELEMETRY === "0"
    || truthy(process.env.CI) || truthy(process.env.GITHUB_ACTIONS)
}

// One event -> one native process -> one send attempt. No queue, retry, metric
// files, session/message identities, dedupe map, policy cache, or SDK retrieval.
// The hook never awaits the process (and therefore never awaits network).
const telemetryRuntime: Plugin = async () => {
  const children = new Set<ReturnType<typeof execFile>>()
  let disposed = false
  return {
    event: async ({ event }) => {
      if (disposed || veto() || children.size >= MAX_IN_FLIGHT || event.type !== "message.updated") return
      const info = event.properties.info
      if (info.role !== "assistant" || info.summary || !Number.isSafeInteger(info.time?.completed)) return
      try {
        const tokens = info.tokens
        const error = info.error
        const agent = typeof info.mode === "string" && info.mode.length > 0 && info.mode.length <= 64
          && /^[\x20-\x7e]+$/.test(info.mode) ? info.mode : undefined
        const knownErrors = ["ProviderAuthError", "UnknownError", "MessageOutputLengthError", "MessageAbortedError", "APIError"]
        const body = JSON.stringify({
          schema: "gentle-ai.telemetry-opencode/v1",
          info: {
            role: "assistant",
            time: { created: info.time.created, completed: info.time.completed },
            providerID: info.providerID,
            modelID: info.modelID,
            agent,
            tokens: tokens && {
              input: tokens.input, output: tokens.output, reasoning: tokens.reasoning,
              cache: tokens.cache && { read: tokens.cache.read, write: tokens.cache.write },
            },
            error: error && {
              name: knownErrors.includes(error.name) ? error.name : "UnknownError",
              data: { statusCode: error.name === "APIError" ? error.data.statusCode : undefined },
            },
          },
        })
        if (Buffer.byteLength(body) > MAX_BYTES || veto()) return
        const child = execFile("gentle-ai", ["telemetry", "runtime", "opencode", "--json"],
          { timeout: 4000, killSignal: "SIGKILL", maxBuffer: 1024, windowsHide: true },
          () => { children.delete(child) })
        children.add(child)
        child.stdin?.on("error", () => {})
        child.stdin?.end(body)
      } catch {
        // Missing binaries, invalid events and IO failures silently lose coverage.
      }
    },
    dispose: async () => {
      disposed = true
      for (const child of children) child.kill("SIGKILL")
      children.clear()
    },
  }
}

export default telemetryRuntime
