import type { Plugin } from "@opencode-ai/plugin"
import { spawn } from "node:child_process"

const REVIEW_AGENTS = new Set(["review-risk", "review-resilience", "review-readability", "review-reliability", "review-refuter", "review-validator"])
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

function taskKey(sessionID: string, callID: string): string {
  return `${sessionID}:${callID}`
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
  const relays = new Map<string, Relay>()
  const cwd = () => worktree || directory
  const clear = (key: string) => {
    const relay = relays.get(key)
    relays.delete(key)
    relay?.close()
  }
  return {
    dispose: async () => { for (const key of relays.keys()) clear(key) },
    event: async ({ event }) => {
      if (event.type !== "session.deleted") return
      const prefix = `${event.properties.info.id}:`
      for (const key of relays.keys()) if (key.startsWith(prefix)) clear(key)
    },
    "tool.execute.before": async (input, output) => {
      if (input.tool !== "task" || typeof output.args?.subagent_type !== "string" || !REVIEW_AGENTS.has(output.args.subagent_type)) return
      if (typeof output.args.prompt !== "string") throw new Error("review task prompt is unavailable for Go relay materialization")
      const key = taskKey(input.sessionID, input.callID)
      if (relays.has(key)) throw new Error("duplicate review Task relay before hook")
      const relay = startRelay(cwd(), output.args.prompt)
      relays.set(key, relay)
      try {
        output.args.prompt = (await relay.prompt).prompt
      } catch (cause) {
        clear(key)
        throw cause
      }
    },
    "tool.execute.after": async (input, output) => {
      if (input.tool !== "task" || typeof input.args?.subagent_type !== "string" || !REVIEW_AGENTS.has(input.args.subagent_type)) return
      const key = taskKey(input.sessionID, input.callID)
      const relay = relays.get(key)
      if (!relay) throw new Error("review Task relay completion has no matching live before hook")
      relays.delete(key)
      try {
        output.output = await relay.complete(output.output)
      } finally {
        relay.close()
      }
    },
  }
}

export default OpenCodeReviewTransportPlugin
