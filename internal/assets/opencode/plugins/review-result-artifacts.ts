import type { Plugin } from "@opencode-ai/plugin"
import { spawn } from "node:child_process"

const REVIEW_AGENTS = new Set(["review-risk", "review-resilience", "review-readability", "review-reliability"])
const BINDING = /^GENTLE_AI_REVIEW_BINDING (\{[^\n]+\})(?:\n|$)/
const TASK_RESULT = /^<task id="[^"\r\n]+" state="completed">\n<task_result>\n([\s\S]*?)\n<\/task_result>\n<\/task>$/
const TASK_TAG = /<\/?task(?:\s|>)|<\/?task_result>/

type ReviewBinding = {
  lineage: string
  target: string
  lens: string
  order: number
}

function parseBinding(prompt: unknown, lens: string): ReviewBinding {
  const match = BINDING.exec(typeof prompt === "string" ? prompt : "")
  if (!match) throw new Error("review task is missing GENTLE_AI_REVIEW_BINDING")

  let binding: unknown
  try {
    binding = JSON.parse(match[1])
  } catch {
    throw new Error("review task binding is malformed")
  }
  if (!binding || typeof binding !== "object" || Array.isArray(binding)) {
    throw new Error("review task binding must be an object")
  }
  const value = binding as Record<string, unknown>
  const fields = Object.keys(value).sort().join(",")
  if (fields !== "lens,lineage,order,target" ||
      typeof value.lineage !== "string" || !/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(value.lineage) ||
      typeof value.target !== "string" || !/^sha256:[a-f0-9]{64}$/.test(value.target) ||
      value.lens !== lens || !Number.isSafeInteger(value.order) || (value.order as number) < 0) {
    throw new Error("review task binding does not match the selected lens")
  }
  return value as ReviewBinding
}

function reviewerResult(output: unknown): string {
  if (typeof output !== "string" || output.trim() === "") throw new Error("reviewer output must not be empty")
  const trimmed = output.trim()
  const envelope = TASK_RESULT.exec(trimmed)
  if (!envelope) {
    if (TASK_TAG.test(trimmed)) throw new Error("reviewer output contains a malformed task result envelope")
    return trimmed
  }
  if (envelope[1].trim() === "" || TASK_TAG.test(envelope[1])) {
    throw new Error("reviewer task result is empty or contains a nested envelope")
  }
  return envelope[1]
}

function captureCwd(worktree: string | undefined, directory: string): string {
  const override = process.env["GENTLE_AI_REVIEW_CWD"]
  if (typeof override === "string" && override.trim() !== "") return override.trim()
  return worktree || directory
}

function runNative(cwd: string, args: string[], stdin: string): Promise<string> {
  return new Promise((resolve, reject) => {
    const child = spawn("gentle-ai", args, { cwd, stdio: ["pipe", "pipe", "pipe"] })
    const stdout: Buffer[] = []
    const stderr: Buffer[] = []
    child.stdout.on("data", (chunk: Buffer) => stdout.push(chunk))
    child.stderr.on("data", (chunk: Buffer) => stderr.push(chunk))
    child.stdin.on("error", reject)
    child.on("error", reject)
    child.on("close", (code) => {
      if (code === 0) {
        resolve(Buffer.concat(stdout).toString("utf8").trim())
        return
      }
      reject(new Error(`gentle-ai ${args[0]} ${args[1]} failed (${code ?? "signal"}): ${Buffer.concat(stderr).toString("utf8").trim()}`))
    })
    child.stdin.end(stdin)
  })
}

function captureResult(cwd: string, binding: ReviewBinding, result: string): Promise<string> {
  return runNative(cwd, [
    "review", "capture-result", "--cwd", cwd,
    "--lineage", binding.lineage, "--target", binding.target,
    "--lens", binding.lens, "--order", String(binding.order), "--input", "-",
  ], result)
}

async function preflightCapture(cwd: string, binding: ReviewBinding): Promise<void> {
  try {
    await runNative(cwd, [
      "review", "capture-result", "--cwd", cwd,
      "--lineage", binding.lineage, "--target", binding.target,
      "--lens", binding.lens, "--order", String(binding.order), "--preflight",
    ], "")
  } catch (cause) {
    // An older installed gentle-ai binary rejects the flag itself ("flag
    // provided but not defined: -preflight"). That is version skew, not a
    // binding problem: degrade gracefully and let the real capture path
    // behave exactly as it did before preflight existed.
    const message = errorMessage(cause)
    if (message.includes("flag provided but not defined") && message.includes("-preflight")) return
    throw new Error(
      `review capture preflight failed for lens ${binding.lens} under ${cwd}: ${errorMessage(cause)}. ` +
      `The reviewer was not launched, so its exactly-once invocation is preserved. ` +
      `If lineage ${binding.lineage} was started in a different repository (for example a nested one), ` +
      `set GENTLE_AI_REVIEW_CWD to that repository and relaunch the lens.`,
    )
  }
}

function preserveResult(cwd: string, binding: ReviewBinding, raw: string): Promise<string> {
  return runNative(cwd, [
    "review", "preserve-result", "--cwd", cwd,
    "--lineage", binding.lineage, "--target", binding.target,
    "--lens", binding.lens, "--order", String(binding.order), "--input", "-",
  ], raw)
}

function errorMessage(cause: unknown): string {
  return cause instanceof Error ? cause.message : String(cause)
}

function preservedReference(manifest: string): string {
  try {
    const parsed = JSON.parse(manifest) as { path?: unknown }
    if (parsed && typeof parsed.path === "string" && parsed.path !== "") return parsed.path
  } catch {
    // fall through to the full manifest
  }
  return manifest
}

// Bound on the raw payload embedded in a double-failure error message. The
// native side already caps preserved payloads at 4 MiB; embedding is a last
// resort into the session transcript, so keep it far smaller.
const PRESERVE_EMBED_LIMIT = 64 * 1024

function embeddedRawPayload(raw: string): string {
  if (raw.length <= PRESERVE_EMBED_LIMIT) return raw
  return `${raw.slice(0, PRESERVE_EMBED_LIMIT)}\n[truncated: first ${PRESERVE_EMBED_LIMIT} of ${raw.length} characters embedded]`
}

async function preservedCaptureFailure(cwd: string, binding: ReviewBinding, raw: unknown, cause: unknown): Promise<Error> {
  if (typeof raw !== "string" || raw.trim() === "") {
    return new Error(`${errorMessage(cause)}; no raw reviewer result was available to preserve`)
  }
  try {
    const manifest = await preserveResult(cwd, binding, raw)
    return new Error(`${errorMessage(cause)}; raw reviewer result preserved for recovery at ${preservedReference(manifest)}`)
  } catch (preserveCause) {
    // Double failure: durable preservation itself failed, so the transcript
    // is the only remaining copy — embed the bounded payload in the error.
    return new Error(
      `${errorMessage(cause)}; raw reviewer result could not be preserved: ${errorMessage(preserveCause)}; ` +
      `raw reviewer result follows for manual recovery:\n${embeddedRawPayload(raw)}`,
    )
  }
}

const ReviewResultArtifactsPlugin: Plugin = async ({ directory, worktree }) => ({
  "tool.execute.before": async (input, output) => {
    if (input.tool !== "task" || typeof output.args?.subagent_type !== "string" ||
        !REVIEW_AGENTS.has(output.args.subagent_type) || !BINDING.test(output.args.prompt)) return
    if (output.args.background === true) {
      throw new Error("bound review tasks must run in the foreground for native result capture")
    }
    await preflightCapture(captureCwd(worktree, directory), parseBinding(output.args.prompt, output.args.subagent_type))
  },
  "tool.execute.after": async (input, output) => {
    if (input.tool !== "task" || typeof input.args?.subagent_type !== "string" || !REVIEW_AGENTS.has(input.args.subagent_type)) return
    if (typeof input.args.prompt !== "string" || !BINDING.test(input.args.prompt)) return
    const lens = input.args.subagent_type
    const binding = parseBinding(input.args.prompt, lens)
    const cwd = captureCwd(worktree, directory)
    // Extract the replayable payload exactly once, BEFORE capture: recovery
    // re-runs `review capture-result --input <preserved file>`, whose strict
    // decoder rejects the task envelope, so a capture failure must preserve
    // the extracted strict JSON — never the enveloped output.output.
    let result: string
    try {
      result = reviewerResult(output.output)
    } catch (cause) {
      // Extraction itself failed (malformed envelope): there is no extracted
      // payload, so preserve the raw envelope under the distinct extraction
      // cause for manual inspection.
      throw await preservedCaptureFailure(cwd, binding, output.output, cause)
    }
    try {
      output.output = await captureResult(cwd, binding, result)
    } catch (cause) {
      throw await preservedCaptureFailure(cwd, binding, result, cause)
    }
  },
})

export default ReviewResultArtifactsPlugin
