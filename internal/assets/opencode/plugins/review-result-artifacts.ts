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
  revision?: string
  repository_context?: string
  subject_hash?: string
}

interface ReviewArtifactSubject {
  schema: string
  subject_hash: string
  lineage_id: string
  authority_revision: string
  target_identity: string
  base_tree: string
  candidate_tree: string
  changed_path_manifest_sha256: string
  lens: string
  selected_order: number
}

interface ChangedPathManifestEntry {
  path: string
  status: string
  old_mode: string
  new_mode: string
  deleted: boolean
  type_changed: boolean
  mode_only: boolean
  intended_untracked: boolean
}

interface ReviewCapturePreflight {
  schema: string
  capability: string
  lineage_id: string
  target_identity: string
  lens: string
  selected_order: number
  artifact_subject: ReviewArtifactSubject
  base_tree: string
  candidate_tree: string
  changed_path_manifest: ChangedPathManifestEntry[]
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
  const subjectArgs = binding.subject_hash ? ["--subject-hash", binding.subject_hash] : []
  return runNative(cwd, [
    "review", "capture-result", ...repositoryBindingArgs(cwd, binding),
    "--lineage", binding.lineage, "--target", binding.target,
    "--lens", binding.lens, "--order", String(binding.order), ...subjectArgs, "--input", "-",
  ], result)
}

async function preflightCapture(cwd: string, binding: ReviewBinding): Promise<ReviewCapturePreflight> {
  try {
    const subjectArgs = binding.subject_hash ? ["--subject-hash", binding.subject_hash] : []
    const response = await runNative(cwd, [
      "review", "capture-result", ...repositoryBindingArgs(cwd, binding),
      "--lineage", binding.lineage, "--target", binding.target,
      "--lens", binding.lens, "--order", String(binding.order), ...subjectArgs, "--preflight",
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
    const manifest = value.changed_path_manifest
    if (!subject || subject.schema !== "gentle-ai.review-artifact-subject/v2" ||
        typeof subject.subject_hash !== "string" || !/^sha256:[a-f0-9]{64}$/.test(subject.subject_hash) ||
        typeof subject.authority_revision !== "string" || !/^sha256:[a-f0-9]{64}$/.test(subject.authority_revision) ||
        typeof subject.base_tree !== "string" || !/^[a-f0-9]{40}(?:[a-f0-9]{24})?$/.test(subject.base_tree) ||
        typeof subject.candidate_tree !== "string" || !/^[a-f0-9]{40}(?:[a-f0-9]{24})?$/.test(subject.candidate_tree) ||
        typeof subject.changed_path_manifest_sha256 !== "string" || !/^sha256:[a-f0-9]{64}$/.test(subject.changed_path_manifest_sha256) ||
        subject.lineage_id !== binding.lineage || subject.target_identity !== binding.target ||
        (binding.revision !== undefined && subject.authority_revision !== binding.revision) ||
        subject.lens !== binding.lens || subject.selected_order !== binding.order ||
        value.schema !== "gentle-ai.review-capture-preflight/v1" || value.capability !== "review.native_capture_preflight" ||
        value.lineage_id !== binding.lineage || value.target_identity !== binding.target || value.lens !== binding.lens ||
        value.selected_order !== binding.order || value.base_tree !== subject.base_tree || value.candidate_tree !== subject.candidate_tree ||
        !validManifest(manifest)) {
      throw new Error("review capture preflight returned an incomplete artifact subject")
    }
    if (binding.subject_hash && subject.subject_hash !== binding.subject_hash) {
      throw new Error("review capture preflight returned a different artifact subject")
    }
    return value as unknown as ReviewCapturePreflight
  } catch (cause) {
    const scope = binding.repository_context ? "the provider-issued repository context" : cwd
    const recovery = gitTrustRefusal(binding, cause)
      ? GIT_TRUST_REFUSAL_RECOVERY
      : binding.repository_context
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

function validManifest(value: unknown): value is ChangedPathManifestEntry[] {
  if (!Array.isArray(value)) return false
  let previous = ""
  for (const entry of value) {
    if (!validManifestEntry(entry) ||
        (previous !== "" && Buffer.compare(Buffer.from(previous, "utf8"), Buffer.from(entry.path, "utf8")) >= 0)) return false
    previous = entry.path
  }
  return true
}

function validManifestEntry(entry: unknown): entry is ChangedPathManifestEntry {
  if (!entry || typeof entry !== "object" || Array.isArray(entry)) return false
  const value = entry as Record<string, unknown>
  return Object.keys(value).sort().join(",") ===
    "deleted,intended_untracked,mode_only,new_mode,old_mode,path,status,type_changed" &&
    typeof value.path === "string" && value.path !== "" &&
    typeof value.status === "string" && /^[ADMT]$/.test(value.status) &&
    typeof value.old_mode === "string" && /^[0-7]{6}$/.test(value.old_mode) &&
    typeof value.new_mode === "string" && /^[0-7]{6}$/.test(value.new_mode) &&
    typeof value.deleted === "boolean" && typeof value.type_changed === "boolean" &&
    typeof value.mode_only === "boolean" && typeof value.intended_untracked === "boolean"
}

async function injectReviewerContext(prompt: string, lens: string, cwd: string): Promise<string> {
  const binding = parseBinding(prompt, lens)
  const preflight = await preflightCapture(cwd, binding)
  const injectedBinding = { ...binding, subject_hash: preflight.artifact_subject.subject_hash }
  return `GENTLE_AI_REVIEW_BINDING ${JSON.stringify(injectedBinding)}\n` +
    `GENTLE_AI_REVIEW_CONTEXT ${JSON.stringify(preflight)}\n`
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

// GIT_TRUST_REFUSAL_CODE is the typed, path-free code the native CLI emits
// when Git itself declines to open the bound repository because it is owned by
// a different account. It is the one native code an opaque binding surfaces,
// because the generic message ("refresh the exact native next_transition") is
// not merely vague for this cause: refreshing a transition cannot change the
// Git trust context of an already-running host process.
const GIT_TRUST_REFUSAL_CODE = "git_repository_untrusted"

// GIT_TRUST_REFUSAL_MESSAGE is authored here rather than forwarded from the
// native stderr, so the opaque path keeps its absolute rule that no native
// text ever reaches the session transcript. It mirrors the native wording in
// internal/cli/review_incident.go.
// It carries its own instruction, so every surface that renders it — including
// the post-launch capture path, which appends no separate recovery line —
// tells the caller something they can actually carry out.
const GIT_TRUST_REFUSAL_MESSAGE =
  `${GIT_TRUST_REFUSAL_CODE}: Git declined to open the bound repository in this process because it is owned by a ` +
  `different account; gentle-ai never provisions a safe.directory exception and never bypasses that protection. ` +
  `Restart the host process under a Git context that already trusts that repository.`

const GIT_TRUST_REFUSAL_RECOVERY =
  "Relaunch the lens once the host process runs under a Git context that trusts that repository."

function gitTrustRefusal(binding: ReviewBinding, cause: unknown): boolean {
  return Boolean(binding.repository_context) && new RegExp(`\\b${GIT_TRUST_REFUSAL_CODE}\\b`).test(errorMessage(cause))
}

// ADMISSION_REJECTION matches the typed decision the native CLI emits when it
// refused the reviewer RESULT itself (`reviewer artifact admission <decision>:`
// from internal/cli/review_artifact.go). Only the [a-z_]+ decision token is
// forwarded — never the native diagnostic, which can embed payload text — so
// the opaque path keeps its rule that no native prose reaches the transcript.
// Without this, an invalid result collapsed into "retry the same opaque
// binding", advice that deterministically fails: recapturing identical bytes
// can never satisfy admission, only a relaunched reviewer can.
const ADMISSION_REJECTION = /\breviewer artifact admission ([a-z_]+):/

function admissionRejection(cause: unknown): string | undefined {
  const match = ADMISSION_REJECTION.exec(errorMessage(cause))
  return match ? match[1] : undefined
}

function sessionErrorMessage(binding: ReviewBinding, cause: unknown, code: string): string {
  if (!binding.repository_context) return errorMessage(cause)
  if (gitTrustRefusal(binding, cause)) return GIT_TRUST_REFUSAL_MESSAGE
  const admission = admissionRejection(cause)
  if (admission) {
    return `${code}: native admission rejected the reviewer result as ${admission}; ` +
      "retrying capture with the same result cannot succeed; relaunch this lens reviewer to produce a corrected result"
  }
  return `${code}: provider-owned review operation failed; refresh the exact native next_transition or retry the same opaque binding`
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
    // Double failure: durable preservation itself failed, so the transcript is
    // the only remaining copy — embed the bounded payload in the error. Both
    // bindings need this identically: captureResult and preserveResult resolve
    // the same repository through the same binding path, so one environmental
    // refusal can fail both, and an opaque binding that omitted the payload
    // had no equivalent transcript fallback left.
    return new Error(
      `${captureFailure}; raw reviewer result could not be preserved: ${preserveFailure}; ` +
      `raw reviewer result follows for manual recovery:\n${embeddedRawPayload(raw)}`,
    )
  }
}

const ReviewResultArtifactsPlugin: Plugin = async ({ directory, worktree }) => ({
  "tool.execute.before": async (input, output) => {
    if (input.tool !== "task" || typeof output.args?.subagent_type !== "string" ||
        !REVIEW_AGENTS.has(output.args.subagent_type)) return
    if (typeof output.args.prompt !== "string") {
      throw new Error("review task is missing GENTLE_AI_REVIEW_BINDING")
    }
    if (output.args.background === true) {
      throw new Error("bound review tasks must run in the foreground for native result capture")
    }
    output.args.prompt = await injectReviewerContext(
      output.args.prompt,
      output.args.subagent_type,
      captureCwd(worktree, directory),
    )
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
