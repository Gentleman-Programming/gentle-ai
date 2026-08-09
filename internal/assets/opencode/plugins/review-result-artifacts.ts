import { tool, type Plugin } from "@opencode-ai/plugin"
import { spawn } from "node:child_process"

// This plugin has two independent responsibilities that share an OpenCode
// host: v2 bounded-review transport and SDD phase task-result handling. They
// do not interact.
//
// A v2 reviewer gets only its parent-supplied opaque binding and manifest. It
// has no filesystem or shell access; its only candidate byte source is the
// constrained indexed native inspection tool below. Its final JSON returns via
// task stdout, then this parent-side plugin passes it directly to native
// capture on stdin. Native Go verifies the binding and decides admission.
const REVIEW_AGENTS = new Set(["review-risk", "review-resilience", "review-readability", "review-reliability"])
const BINDING = /^GENTLE_AI_REVIEW_BINDING (\{[^\n]+\})(?:\n|$)/
const REVIEW_INSPECTION_TOOL = "gentle_ai_review_inspect"
const TASK_RESULT = /^<task id="[^"\r\n]+" state="completed">\n<task_result>\n([\s\S]*?)\n<\/task_result>\n<\/task>$/
const TASK_TAG = /<\/?task(?:\s|>)|<\/?task_result>/
const SDD_PHASES = ["sdd-init", "sdd-explore", "sdd-propose", "sdd-spec", "sdd-design", "sdd-tasks", "sdd-apply", "sdd-verify", "sdd-archive", "sdd-onboard"]

type ReviewBinding = {
  repository_context: string
  revision: string
  lineage: string
  target: string
  lens: string
  order: number
  subject_hash: string
}

function reviewBinding(prompt: string): ReviewBinding | undefined {
  const match = BINDING.exec(prompt)
  if (!match) return undefined
  try {
    const value = JSON.parse(match[1]) as unknown
    if (!value || typeof value !== "object" || Array.isArray(value)) return undefined
    const fields = value as Record<string, unknown>
    const order = fields.order
    if (
      typeof fields.repository_context !== "string" || fields.repository_context === "" ||
      typeof fields.revision !== "string" || fields.revision === "" ||
      typeof fields.lineage !== "string" || fields.lineage === "" ||
      typeof fields.target !== "string" || fields.target === "" ||
      typeof fields.lens !== "string" || fields.lens === "" ||
      typeof fields.subject_hash !== "string" || fields.subject_hash === "" ||
      !Number.isInteger(order) || order < 0
    ) return undefined
    return {
      repository_context: fields.repository_context,
      revision: fields.revision,
      lineage: fields.lineage,
      target: fields.target,
      lens: fields.lens,
      order,
      subject_hash: fields.subject_hash,
    }
  } catch {
    return undefined
  }
}

function hasRepositoryContext(prompt: string): boolean {
  const match = BINDING.exec(prompt)
  if (!match) return false
  try {
    const value = JSON.parse(match[1]) as Record<string, unknown>
    return typeof value?.repository_context === "string" && value.repository_context !== ""
  } catch {
    return false
  }
}

function taskResult(output: unknown, subject: string, classification?: string): string {
  const fail = (message: string, taskResultClass: string): never => {
    if (classification) throw Object.assign(new Error(message), { [classification]: taskResultClass })
    throw new Error(message)
  }
  if (typeof output !== "string" || output.trim() === "") {
    fail(`${subject} output must not be empty`, "empty_result")
  }
  const trimmed = (output as string).trim()
  const envelope = TASK_RESULT.exec(trimmed)
  if (!envelope) {
    if (TASK_TAG.test(trimmed)) fail(`${subject} output contains a malformed task result envelope`, "malformed_result")
    return trimmed
  }
  if (envelope[1].trim() === "") fail(`${subject} task result is empty`, "empty_result")
  if (TASK_TAG.test(envelope[1])) fail(`${subject} task result contains a nested task envelope`, "nested_envelope")
  return envelope[1]
}

function reviewerResult(output: unknown): string {
  return taskResult(output, "reviewer")
}

function extractionClass(cause: unknown, property: string): string | undefined {
  const value = (cause as Record<string, unknown> | null)?.[property]
  return typeof value === "string" ? value : undefined
}

function isSDDPhase(agent: string): boolean {
  return SDD_PHASES.some((phase) => agent === phase || agent.startsWith(phase + "-"))
}

const SDD_TASK_FAILURE_PREFIX = "GENTLE_AI_SDD_FAILURE "
type SDDTaskFailure = { phase: string, code: string, handoff: string }
type SDDTaskFailureError = Error & { sddFailure: SDDTaskFailure }

function shellQuote(value: string): string {
  return `'${value.replace(/'/g, "'\\''")}'`
}

function sddTaskFailure(phase: string, cwd: string, cause: unknown): SDDTaskFailureError {
  const classification = extractionClass(cause, "sddClass")
  const code = classification === "empty_result" ? "sdd_task_result_empty" : "sdd_task_result_malformed"
  const failure: SDDTaskFailure = {
    phase,
    code,
    handoff: SDD_TASK_FAILURE_PREFIX + JSON.stringify({
      schemaName: "gentle-ai.sdd-task-result-failure/v1",
      status: "blocked",
      code,
      phase,
      summary: `${phase} returned no valid task result. Do not retry or advance SDD; inspect the existing artifact state and surface the terminal failure to the user.`,
      continuation: `gentle-ai sdd-status --cwd ${shellQuote(cwd)} --json`,
    }),
  }
  return Object.assign(new Error(failure.handoff), { sddFailure: failure }) as SDDTaskFailureError
}

function captureCwd(worktree: string | undefined, directory: string): string {
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
        resolve(Buffer.concat(stdout).toString("utf8"))
        return
      }
      reject(new Error(`gentle-ai ${args[0]} ${args[1]} failed (${code ?? "signal"}): ${Buffer.concat(stderr).toString("utf8").trim()}`))
    })
    child.stdin.end(stdin)
  })
}

function inspectionArgs(binding: ReviewBinding, operation: string, pathIndex?: string, side?: string): string[] | undefined {
  const pathOperation = operation === "stat" || operation === "patch"
  const objectOperation = operation === "object"
  const globalOperation = operation === "name-status" || operation === "numstat"
  if (!pathOperation && !objectOperation && !globalOperation) return undefined
  const args = [
    "review", "inspect-candidate",
    "--repository-context", binding.repository_context,
    "--expected-revision", binding.revision,
    "--lineage", binding.lineage,
    "--target", binding.target,
    "--lens", binding.lens,
    "--order", String(binding.order),
    "--operation", operation,
  ]
  if (globalOperation) return pathIndex === undefined && side === undefined ? args : undefined
  if (!/^(0|[1-9][0-9]*)$/.test(pathIndex ?? "")) return undefined
  args.push("--path-index", pathIndex!)
  if (!objectOperation) return side === undefined ? args : undefined
  if (side !== "base" && side !== "candidate") return undefined
  return [...args, "--side", side]
}

function captureArgs(binding: ReviewBinding): string[] {
  return [
    "review", "capture-result",
    "--repository-context", binding.repository_context,
    "--expected-revision", binding.revision,
    "--lineage", binding.lineage,
    "--target", binding.target,
    "--lens", binding.lens,
    "--order", String(binding.order),
    "--subject-hash", binding.subject_hash,
    "--input", "-",
  ]
}

const ReviewResultArtifactsPlugin: Plugin = async ({ directory, worktree }) => {
  const failedSDDSessions = new Map<string, SDDTaskFailure>()
  return {
    tool: {
      [REVIEW_INSPECTION_TOOL]: tool({
        description: "Inspect one immutable, provider-bound candidate view by canonical manifest index. It cannot access the live worktree.",
        args: {
          binding: tool.schema.string(),
          operation: tool.schema.string(),
          path_index: tool.schema.string().optional(),
          side: tool.schema.string().optional(),
        },
        async execute(args) {
          const binding = reviewBinding(`GENTLE_AI_REVIEW_BINDING ${args.binding}\n`)
          const nativeArgs = binding && inspectionArgs(binding, args.operation, args.path_index, args.side)
          if (!nativeArgs) return "GENTLE_AI_REVIEW_INSPECTION_UNAVAILABLE: return incomplete inspection without findings."
          try {
            return await runNative(captureCwd(worktree, directory), nativeArgs, "")
          } catch {
            return "GENTLE_AI_REVIEW_INSPECTION_UNAVAILABLE: return incomplete inspection without findings."
          }
        },
      }),
    },
    dispose: async () => {
      failedSDDSessions.clear()
    },
    event: async ({ event }) => {
      if (event.type === "session.deleted") failedSDDSessions.delete(event.properties.info.id)
    },
    "tool.execute.before": async (input, output) => {
      if (input.tool !== "task" || typeof output.args?.subagent_type !== "string") return
      const subagent = output.args.subagent_type
      if (isSDDPhase(subagent)) {
        const failure = failedSDDSessions.get(input.sessionID)
        if (failure) throw new Error(failure.handoff)
        return
      }
      if (!REVIEW_AGENTS.has(subagent)) return
      if (typeof output.args.prompt !== "string") throw new Error("review task is missing GENTLE_AI_REVIEW_BINDING")
      if (output.args.background === true) throw new Error("bound review tasks must run in the foreground so the launching session can relay the raw result")
    },
    "tool.execute.after": async (input, output) => {
      if (input.tool !== "task" || typeof input.args?.subagent_type !== "string") return
      const subagent = input.args.subagent_type
      if (isSDDPhase(subagent)) {
        try {
          taskResult(output.output, "SDD phase", "sddClass")
        } catch (cause) {
          const failure = sddTaskFailure(subagent, captureCwd(worktree, directory), cause)
          failedSDDSessions.set(input.sessionID, failure.sddFailure)
          throw failure
        }
        return
      }
      if (!REVIEW_AGENTS.has(subagent)) return
      if (typeof input.args.prompt !== "string" || !BINDING.test(input.args.prompt)) return
      const rawResult = reviewerResult(output.output)
      const binding = reviewBinding(input.args.prompt)
      if (!binding) {
        if (hasRepositoryContext(input.args.prompt)) {
          throw new Error("v2 reviewer binding is malformed; refresh the exact native next_transition before retrying")
        }
        // Legacy v1 prompts retain their existing parent-owned raw-output path.
        output.output = rawResult
        return
      }
      try {
        output.output = await runNative(captureCwd(worktree, directory), captureArgs(binding), rawResult)
      } catch {
        throw new Error("native reviewer result capture failed; refresh the exact native next_transition before retrying")
      }
    },
  }
}

export default ReviewResultArtifactsPlugin
