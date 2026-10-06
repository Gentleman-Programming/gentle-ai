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

function captureResult(cwd: string, binding: ReviewBinding, result: string): Promise<string> {
  return new Promise((resolve, reject) => {
    const child = spawn("gentle-ai", [
      "review", "capture-result", "--cwd", cwd,
      "--lineage", binding.lineage, "--target", binding.target,
      "--lens", binding.lens, "--order", String(binding.order), "--input", "-",
    ], { cwd, stdio: ["pipe", "pipe", "pipe"] })
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
      reject(new Error(`gentle-ai review capture-result failed (${code ?? "signal"}): ${Buffer.concat(stderr).toString("utf8").trim()}`))
    })
    child.stdin.end(result)
  })
}

const ReviewResultArtifactsPlugin: Plugin = async ({ directory, worktree }) => ({
  "tool.execute.before": async (input, output) => {
    if (input.tool === "task" && typeof output.args?.subagent_type === "string" &&
        REVIEW_AGENTS.has(output.args.subagent_type) && BINDING.test(output.args.prompt) && output.args.background === true) {
      throw new Error("bound review tasks must run in the foreground for native result capture")
    }
  },
  "tool.execute.after": async (input, output) => {
    if (input.tool !== "task" || typeof input.args?.subagent_type !== "string" || !REVIEW_AGENTS.has(input.args.subagent_type)) return
    if (typeof input.args.prompt !== "string" || !BINDING.test(input.args.prompt)) return
    const lens = input.args.subagent_type.slice("review-".length)
    const binding = parseBinding(input.args.prompt, lens)
    output.output = await captureResult(worktree || directory, binding, reviewerResult(output.output))
  },
})

export default ReviewResultArtifactsPlugin
