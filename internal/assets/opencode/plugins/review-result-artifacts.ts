import type { Plugin } from "@opencode-ai/plugin"
import { spawn } from "node:child_process"

const REVIEW_AGENTS = new Set(["review-risk", "review-resilience", "review-readability", "review-reliability"])
const BINDING = /^GENTLE_AI_REVIEW_BINDING (\{[^\n]+\})(?:\n|$)/
const FROZEN_CONTEXT = "GENTLE_AI_FROZEN_CANDIDATE_CONTEXT "
const TASK_RESULT = /^<task id="[^"\r\n]+" state="completed">\n<task_result>\n([\s\S]*?)\n<\/task_result>\n<\/task>$/
const TASK_TAG = /<\/?task(?:\s|>)|<\/?task_result>/

type ReviewBinding = {
  lineage: string
  target: string
  lens: string
  order: number
  revision?: string
  repository_context?: string
  subject_hash?: string
}

interface ReviewArtifactSubject {
  subject_hash: string
}

interface ReviewCapturePreflight {
  artifact_subject: ReviewArtifactSubject
  candidate_diff: Record<string, unknown>
  changed_path_manifest: Array<Record<string, unknown>>
}

function validManifestPath(path: unknown): path is string {
  return typeof path === "string" && path !== "" && !path.startsWith("/") && !path.includes("\\") &&
    path.split("/").every((segment) => segment !== "" && segment !== "." && segment !== "..")
}

function bindingKey(binding: ReviewBinding): string {
  return JSON.stringify([
    binding.lineage, binding.target, binding.lens, binding.order,
    binding.revision ?? "", binding.repository_context ?? "",
  ])
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
  const legacy = fields === "lens,lineage,order,target"
  const legacyBound = fields === "lens,lineage,order,subject_hash,target"
  const priorCurrent = fields === "lens,lineage,order,repository_context,revision,target"
  const current = fields === "lens,lineage,order,repository_context,revision,subject_hash,target"
  if ((!legacy && !legacyBound && !priorCurrent && !current) ||
      typeof value.lineage !== "string" || !/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(value.lineage) ||
      typeof value.target !== "string" || !/^sha256:[a-f0-9]{64}$/.test(value.target) ||
      ((priorCurrent || current) && (typeof value.revision !== "string" || !/^sha256:[a-f0-9]{64}$/.test(value.revision) ||
        typeof value.repository_context !== "string" || !/^rctx1_[a-f0-9]{64}$/.test(value.repository_context))) ||
      ((legacyBound || current) && (typeof value.subject_hash !== "string" || !/^sha256:[a-f0-9]{64}$/.test(value.subject_hash))) ||
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
  if (envelope[1].trim() === "") {
    throw Object.assign(new Error("reviewer task result is empty"), { reviewClass: "empty_result" })
  }
  if (TASK_TAG.test(envelope[1])) {
    throw Object.assign(new Error("reviewer task result contains a nested task envelope"), { reviewClass: "nested_envelope" })
  }
  return envelope[1]
}

function extractionClass(cause: unknown): string | undefined {
  const value = (cause as { reviewClass?: unknown } | null)?.reviewClass
  return typeof value === "string" ? value : undefined
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

function repositoryBindingArgs(cwd: string, binding: ReviewBinding): string[] {
  if (binding.repository_context && binding.revision) {
    return ["--repository-context", binding.repository_context, "--expected-revision", binding.revision]
  }
  return ["--cwd", cwd]
}

function captureResult(cwd: string, binding: ReviewBinding, result: string): Promise<string> {
  return runNative(cwd, [
    "review", "capture-result", ...repositoryBindingArgs(cwd, binding),
    "--lineage", binding.lineage, "--target", binding.target,
    "--lens", binding.lens, "--order", String(binding.order), "--input", "-",
  ], result)
}

async function preflightCapture(cwd: string, binding: ReviewBinding): Promise<ReviewCapturePreflight | undefined> {
  try {
    const response = await runNative(cwd, [
      "review", "capture-result", ...repositoryBindingArgs(cwd, binding),
      "--lineage", binding.lineage, "--target", binding.target,
      "--lens", binding.lens, "--order", String(binding.order), "--preflight",
    ], "")
    let parsed: unknown
    try {
      parsed = JSON.parse(response)
    } catch {
      throw new Error("review capture preflight returned malformed artifact-subject JSON")
    }
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
      throw new Error("review capture preflight returned malformed artifact-subject JSON")
    }
    const value = parsed as Record<string, unknown>
    const subject = value.artifact_subject as Record<string, unknown> | undefined
    const manifest = value.changed_path_manifest as Array<Record<string, unknown>> | undefined
    if (!subject || typeof subject.subject_hash !== "string" || !/^sha256:[a-f0-9]{64}$/.test(subject.subject_hash) ||
        !value.candidate_diff || typeof value.candidate_diff !== "object" || Array.isArray(value.candidate_diff) ||
        !Array.isArray(manifest) || manifest.some((entry) => !entry || typeof entry !== "object" || Array.isArray(entry) || !validManifestPath(entry.path)) ||
        new Set(manifest.map((entry) => entry.path)).size !== manifest.length) {
      throw new Error("review capture preflight returned incomplete frozen candidate context")
    }
    if (binding.subject_hash && subject.subject_hash !== binding.subject_hash) {
      throw new Error("review capture preflight returned a different artifact subject")
    }
    return value as unknown as ReviewCapturePreflight
  } catch (cause) {
    // An older installed gentle-ai binary rejects the flag itself ("flag
    // provided but not defined: -preflight"). That is version skew, not a
    // binding problem: degrade gracefully and let the real capture path
    // behave exactly as it did before preflight existed.
    const message = errorMessage(cause)
    if (message.includes("flag provided but not defined") && message.includes("-preflight")) return undefined
    const scope = binding.repository_context ? "the provider-issued repository context" : cwd
    const recovery = binding.repository_context
      ? `Refresh the exact native next_transition for lineage ${binding.lineage} before relaunching the lens.`
      : `If lineage ${binding.lineage} was started in a different repository (for example a nested one), ` +
        `set GENTLE_AI_REVIEW_CWD to that repository and relaunch the lens.`
    throw new Error(
      `review capture preflight failed for lens ${binding.lens} under ${scope}: ` +
      `${sessionErrorMessage(binding, cause, "repository_context_preflight_failed")}. ` +
      `The reviewer was not launched, so its exactly-once invocation is preserved. ` +
      recovery,
    )
  }
}

async function injectReviewerContext(prompt: string, lens: string, cwd: string): Promise<{prompt: string, preflight?: ReviewCapturePreflight}> {
  const binding = parseBinding(prompt, lens)
  const preflight = await preflightCapture(cwd, binding)
  if (!preflight) {
    if (binding.subject_hash) throw new Error("provider-bound review task requires capture preflight")
    return { prompt }
  }
  const injectedBinding = { ...binding, subject_hash: preflight.artifact_subject.subject_hash }
  const boundPrompt = prompt.replace(BINDING, `GENTLE_AI_REVIEW_BINDING ${JSON.stringify(injectedBinding)}\n`)
  const frozen = JSON.stringify({
    artifact_subject: preflight.artifact_subject,
    candidate_diff: preflight.candidate_diff,
    changed_path_manifest: preflight.changed_path_manifest,
  })
  return { prompt: `${boundPrompt.trimEnd()}\n${FROZEN_CONTEXT}${frozen}`, preflight }
}

function enrichedReviewerResult(binding: ReviewBinding, result: string, preflight: ReviewCapturePreflight | undefined): string {
  if (!preflight) {
    if (binding.subject_hash) throw new Error("review task is missing retained provider context")
    return result
  }
  if (binding.subject_hash !== preflight.artifact_subject.subject_hash) {
    throw new Error("review task binding does not match retained provider context")
  }
  const paths = preflight.changed_path_manifest.map((entry) => entry.path)
  if (paths.some((path) => typeof path !== "string" || path === "")) {
    throw new Error("retained provider context contains a malformed changed-path manifest")
  }
  let parsed: unknown
  try {
    parsed = JSON.parse(result)
  } catch {
    throw new Error("reviewer result is not strict JSON")
  }
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
    throw new Error("reviewer result must be an object")
  }
  const value = parsed as Record<string, unknown>
  if (Object.keys(value).sort().join(",") !== "evidence,findings" || !Array.isArray(value.findings) || !Array.isArray(value.evidence)) {
    throw new Error("reviewer result must contain only findings and evidence")
  }
  return JSON.stringify({
    subject_hash: preflight.artifact_subject.subject_hash,
    inspection: { status: "completed", paths },
    findings: value.findings,
    evidence: value.evidence,
  })
}

function preserveResult(cwd: string, binding: ReviewBinding, raw: string, cls?: string): Promise<string> {
  const args = [
    "review", "preserve-result", ...repositoryBindingArgs(cwd, binding),
    "--lineage", binding.lineage, "--target", binding.target,
    "--lens", binding.lens, "--order", String(binding.order), "--input", "-",
  ]
  if (typeof cls === "string" && cls !== "") args.push("--class", cls)
  return runNative(cwd, args, raw)
}

function errorMessage(cause: unknown): string {
  return cause instanceof Error ? cause.message : String(cause)
}

function sessionErrorMessage(binding: ReviewBinding, cause: unknown, code: string): string {
  return binding.repository_context
    ? `${code}: provider-owned review operation failed; refresh the exact native next_transition or retry the same opaque binding`
    : errorMessage(cause)
}

function preservedReference(manifest: string): string {
  try {
    const parsed = JSON.parse(manifest) as { reference?: unknown; path?: unknown; sha256?: unknown }
    if (parsed && typeof parsed.reference === "string" && parsed.reference !== "") return parsed.reference
    if (parsed && typeof parsed.path === "string" && parsed.path !== "") return parsed.path
    if (parsed && typeof parsed.sha256 === "string" && parsed.sha256 !== "") return parsed.sha256
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
  const captureFailure = sessionErrorMessage(binding, cause, "repository_context_capture_failed")
  if (typeof raw !== "string" || raw.trim() === "") {
    return new Error(`${captureFailure}; no raw reviewer result was available to preserve`)
  }
  try {
    const reviewClass = extractionClass(cause)
    const manifest = await preserveResult(cwd, binding, raw, reviewClass)
    return new Error(`${captureFailure}; raw reviewer result preserved for recovery as ${preservedReference(manifest)}`)
  } catch (preserveCause) {
    const preserveFailure = sessionErrorMessage(binding, preserveCause, "repository_context_preserve_failed")
    if (binding.repository_context) {
      return new Error(
        `${captureFailure}; ${preserveFailure}; the reviewer task output remains the manual recovery source`,
      )
    }
    // Double failure: durable preservation itself failed, so the transcript
    // is the only remaining copy — embed the bounded payload in the error.
    return new Error(
      `${captureFailure}; raw reviewer result could not be preserved: ${preserveFailure}; ` +
      `raw reviewer result follows for manual recovery:\n${embeddedRawPayload(raw)}`,
    )
  }
}

const ReviewResultArtifactsPlugin: Plugin = async ({ directory, worktree }) => {
  const retainedPreflight = new Map<string, ReviewCapturePreflight>()
  return {
  "tool.execute.before": async (input, output) => {
    if (input.tool !== "task" || typeof output.args?.subagent_type !== "string" ||
        !REVIEW_AGENTS.has(output.args.subagent_type) || !BINDING.test(output.args.prompt)) return
    if (output.args.background === true) {
      throw new Error("bound review tasks must run in the foreground for native result capture")
    }
    const injected = await injectReviewerContext(
      output.args.prompt,
      output.args.subagent_type,
      captureCwd(worktree, directory),
    )
    output.args.prompt = injected.prompt
    if (injected.preflight) {
      retainedPreflight.set(bindingKey(parseBinding(output.args.prompt, output.args.subagent_type)), injected.preflight)
    }
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
      const key = bindingKey(binding)
      const preflight = retainedPreflight.get(key)
      retainedPreflight.delete(key)
      result = enrichedReviewerResult(binding, result, preflight)
      output.output = await captureResult(cwd, binding, result)
    } catch (cause) {
      throw await preservedCaptureFailure(cwd, binding, result, cause)
    }
  },
  }
}

export default ReviewResultArtifactsPlugin
