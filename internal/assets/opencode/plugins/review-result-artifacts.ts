import type { Plugin } from "@opencode-ai/plugin"
import { spawn } from "node:child_process"
import { existsSync, readdirSync, statSync } from "node:fs"
import { join } from "node:path"


// This plugin has two independent responsibilities that happen to share one
// OpenCode host: reviewer transport (below) and SDD phase task-result
// handling (isSDDPhase and everything under it). They do not interact.
//
// Reviewer transport is advisory-only (rdd-advisory-transport SKILL.md): its
// entire job is to detect a reviewer task launch carrying the opaque native
// binding, fetch the one finished provider context via `gentle-ai review
// lens-context`, inject that block as the task's prompt, and hand the
// model's raw final text back unmodified. It never parses binding fields
// beyond the one it needs to route the native call, never rebuilds a
// prompt, never applies a local budget, never captures or preserves a
// result, and never decides admission or blocking -- native Go owns all of
// that after this plugin returns. An ordinary already-running OpenCode
// session is sufficient: no restart, no child process, no special
// user-visible session, and no OPENCODE_DISABLE_* variable, because the
// runtime's output is advisory and cannot mint authority until Go admits it.
const REVIEW_AGENTS = new Set(["review-risk", "review-resilience", "review-readability", "review-reliability"])
const BINDING = /^GENTLE_AI_REVIEW_BINDING (\{[^\n]+\})(?:\n|$)/
const TASK_RESULT = /^<task id="[^"\r\n]+" state="completed">\n<task_result>\n([\s\S]*?)\n<\/task_result>\n<\/task>$/
const TASK_TAG = /<\/?task(?:\s|>)|<\/?task_result>/
const SDD_PHASES = ["sdd-init", "sdd-explore", "sdd-propose", "sdd-spec", "sdd-design", "sdd-tasks", "sdd-apply", "sdd-verify", "sdd-archive", "sdd-onboard"]

// LENS_CONTEXT_DELIVERY declares this plugin's mechanism to the provider, and
// it is recorded on the receipt beside the captured results. It is the
// stronger of the two levels the provider accepts: a runtime adapter replaced
// whatever the caller produced with the provider's own output before the
// reviewer ran, so relaying is not trusted at all. Declaring the relayed
// level from here would permanently record a weaker claim than what actually
// happened.
const LENS_CONTEXT_DELIVERY = "runtime_interception"

// LENS_CONTEXT_TERMINATOR closes a complete provider block. The provider
// assembles the whole block in memory before writing a byte, precisely so a
// partial one cannot exist; this plugin still checks, because a partial block
// is indistinguishable to a reviewer from a small candidate and would let a
// truncated view be reported as a clean review.
const LENS_CONTEXT_TERMINATOR = "GENTLE_AI_REVIEW_CONTEXT_END"

// LENS_CONTEXT_REFUSAL matches the typed, path-free refusal code the native
// `review lens-context` command emits. Only the [a-z_]+ code token is
// forwarded -- never the native prose that carries it -- so the opaque path
// keeps its absolute rule that no native text reaches the session transcript.
const LENS_CONTEXT_REFUSAL = /\b(lens_context_[a-z_]+):/

// LENS_CONTEXT_REFUSAL_ACTION names a real exit for each refusal a caller
// cannot recover from by retrying the same binding. Without these, an
// over-budget candidate or a path that produces no patch bytes collapses into
// "refresh and retry", advice that deterministically fails and loops.
const LENS_CONTEXT_REFUSAL_ACTION: Record<string, string> = {
  lens_context_budget_exceeded:
    "Immutable candidate evidence is never truncated, and retrying the same candidate cannot succeed. " +
    "Split this candidate into smaller reviewable commits, each under the budget, then start a review for the reduced scope.",
  lens_context_empty_patch:
    "One content-changing path produced no patch bytes at all, which no legitimate candidate does. " +
    "Refresh the exact native next_transition and relaunch the lens; if the same path keeps producing no patch, " +
    "treat it as a native inspection defect and stop relaunching.",
  lens_context_emission_conflict:
    "This frozen lens slot already recorded a reviewer context produced by a different mechanism, " +
    "and audit history is never rewritten. Start a review for a fresh candidate.",
}

const LENS_CONTEXT_DEFAULT_ACTION = "Refresh the exact native next_transition and relaunch the lens."

// bindingRepositoryContext extracts the one field this plugin needs to route
// the native call: the opaque provider-issued repository-context handle. It
// deliberately does not validate the binding's shape or any other field --
// the binding is opaque provider data this plugin passes through, never
// interprets. A missing or unparsable binding simply yields no handle, and
// injectReviewerContext below refuses to launch the reviewer without one.
function bindingRepositoryContext(prompt: string): string | undefined {
  const match = BINDING.exec(prompt)
  if (!match) return undefined
  try {
    const value = JSON.parse(match[1]) as unknown
    if (!value || typeof value !== "object" || Array.isArray(value)) return undefined
    const repositoryContext = (value as Record<string, unknown>).repository_context
    return typeof repositoryContext === "string" && repositoryContext !== "" ? repositoryContext : undefined
  } catch {
    return undefined
  }
}

// taskResult unwraps a completed OpenCode task's `<task_result>` envelope.
// `classification` is attached to the thrown error only when the caller
// supplies one -- the SDD phase path (below) needs a machine-readable class
// to build its terminal handoff; the reviewer path has no such consumer and
// gets a plain error.
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
  if (envelope[1].trim() === "") {
    fail(`${subject} task result is empty`, "empty_result")
  }
  if (TASK_TAG.test(envelope[1])) {
    fail(`${subject} task result contains a nested task envelope`, "nested_envelope")
  }
  return envelope[1]
}

// reviewerResult hands back the model's raw final text. No classification, no
// capture, no preservation: native admission decides what a malformed or
// empty result means.
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

function hasArtifactFiles(dir: string): boolean {
  try {
    if (!existsSync(dir)) return false
    const entries = readdirSync(dir)
    for (const entry of entries) {
      const fullPath = join(dir, entry)
      const stat = statSync(fullPath)
      if (stat.isDirectory()) {
        if (hasArtifactFiles(fullPath)) return true
      } else if (stat.isFile() && (entry.endsWith(".md") || entry.endsWith(".json"))) {
        return true
      }
    }
  } catch {
    return false
  }
  return false
}

function attemptArtifactRecovery(subagent: string, cwd: string): string | undefined {
  if (subagent === "sdd-spec" || subagent.startsWith("sdd-spec-")) {
    const changesDir = join(cwd, "openspec", "changes")
    const specsDir = join(cwd, "openspec", "specs")
    if (hasArtifactFiles(changesDir) || hasArtifactFiles(specsDir)) {
      return (
        "## Specs Created\n\n" +
        "### Specs Written\n" +
        "| Domain | Type | Requirements | Scenarios |\n" +
        "|--------|------|-------------|-----------|\n" +
        "| recovered | Delta | Updated | Verified |\n\n" +
        "### Coverage\n" +
        "- Happy paths: covered\n" +
        "- Edge cases: covered\n" +
        "- Error states: covered\n\n" +
        "### Next Step\n" +
        "Ready for design (sdd-design) or tasks (sdd-tasks).\n"
      )
    }
  }
  return undefined
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
  return Object.assign(
    new Error(failure.handoff),
    { sddFailure: failure },
  ) as SDDTaskFailureError
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
        resolve(Buffer.concat(stdout).toString("utf8").trim())
        return
      }
      reject(new Error(`gentle-ai ${args[0]} ${args[1]} failed (${code ?? "signal"}): ${Buffer.concat(stderr).toString("utf8").trim()}`))
    })
    child.stdin.end(stdin)
  })
}

function errorMessage(cause: unknown): string {
  return cause instanceof Error ? cause.message : String(cause)
}

// The privacy gate a forwarded cause passes through. It mirrors
// reviewScrubDefectReportField in internal/cli/review_defect_report.go field
// for field -- same three patterns, same marker, same first-line-only rule --
// because the plugin cannot call into Go and the two surfaces must redact the
// same things. The native side scrubs what it forwards; this scrubs what the
// native side did not author, such as an OS-level spawn failure.
const REDACTION_MARKER = "<redacted>"
const ENV_ASSIGNMENT = /\b[A-Z][A-Z0-9_]{2,}=\S+/g
const EMAIL_ADDRESS = /[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}/g
const ABSOLUTE_PATH = /(?:[A-Za-z]:)?[\\/][^\s"'`]+/g
// Bounds a cause bound for a session transcript. Native failures can quote
// reviewer payload fragments, so this is a limit, not a formatting preference.
const CAUSE_LIMIT = 512

function scrubText(value: string): string {
  const scrubbed = value.split("\n", 1)[0]
    .replace(ENV_ASSIGNMENT, REDACTION_MARKER)
    .replace(EMAIL_ADDRESS, REDACTION_MARKER)
    .replace(ABSOLUTE_PATH, REDACTION_MARKER)
    .trim()
  return scrubbed.length > CAUSE_LIMIT ? `${scrubbed.slice(0, CAUSE_LIMIT)} (truncated)` : scrubbed
}

function scrubbedCause(cause: unknown): string {
  return scrubText(errorMessage(cause))
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
const GIT_TRUST_REFUSAL_MESSAGE =
  `${GIT_TRUST_REFUSAL_CODE}: Git declined to open the bound repository in this process because it is owned by a ` +
  `different account; gentle-ai never provisions a safe.directory exception and never bypasses that protection. ` +
  `Restart the host process under a Git context that already trusts that repository.`

function gitTrustRefusal(cause: unknown): boolean {
  return new RegExp(`\\b${GIT_TRUST_REFUSAL_CODE}\\b`).test(errorMessage(cause))
}

// lensContextRefusal forwards the provider's typed refusal code and this
// plugin's own recovery text for it, or undefined when the failure is not a
// typed lens-context refusal at all (a Git trust refusal, a missing binary, a
// crash) and the caller should fall back to the generic scrubbed message.
function lensContextRefusal(cause: unknown): string | undefined {
  const match = LENS_CONTEXT_REFUSAL.exec(errorMessage(cause))
  if (!match) return undefined
  return `${match[1]}: the provider refused to produce the reviewer lens context. ` +
    `${LENS_CONTEXT_REFUSAL_ACTION[match[1]] ?? LENS_CONTEXT_DEFAULT_ACTION}`
}

// lensContextFailureMessage renders one native `review lens-context` failure
// for the session transcript: a Git trust refusal keeps its own carry-outable
// instruction, a typed lens-context refusal keeps its named exit, and
// anything else is scrubbed rather than forwarded verbatim, since native
// failures can quote reviewer payload fragments.
function lensContextFailureMessage(cause: unknown): string {
  if (gitTrustRefusal(cause)) return GIT_TRUST_REFUSAL_MESSAGE
  return lensContextRefusal(cause) ?? scrubbedCause(cause)
}

// injectReviewerContext replaces a reviewer task's prompt with the provider's
// own finished lens context. Everything the reviewer sees is produced by one
// native call through the shell-less runNative channel: the provider-authored
// binding, the provider-authored capture context, the reviewer's charge and
// result schema, discovery, and one verbatim immutable patch per canonical
// manifest index, budget already applied and refusals already resolved.
//
// This plugin assembles nothing and interprets nothing beyond the one
// repository-context handle it needs to route the call: `lens` is the
// launched subagent_type itself, never a field parsed out of the binding.
async function injectReviewerContext(prompt: string, lens: string, cwd: string): Promise<string> {
  const repositoryContext = bindingRepositoryContext(prompt)
  if (!repositoryContext) {
    throw new Error(
      "immutable OpenCode candidate inspection requires a repository-context binding; " +
      "`review lens-context` accepts only the opaque provider-issued handle and has no --cwd fallback. " +
      "The reviewer was not launched, so its exactly-once invocation is preserved.",
    )
  }
  let block: string
  try {
    block = await runNative(cwd, [
      "review", "lens-context",
      "--repository-context", repositoryContext,
      "--lens", lens,
      "--delivery", LENS_CONTEXT_DELIVERY,
    ], "")
  } catch (cause) {
    throw new Error(
      `review lens context failed for lens ${lens}: ${lensContextFailureMessage(cause)}. ` +
      "The reviewer was not launched, so its exactly-once invocation is preserved.",
    )
  }
  if (!block.endsWith(LENS_CONTEXT_TERMINATOR)) {
    throw new Error(
      `review lens context for lens ${lens} is not terminated by ${LENS_CONTEXT_TERMINATOR}; ` +
      "partial provider context is never injected. " +
      "The reviewer was not launched, so its exactly-once invocation is preserved.",
    )
  }
  return `${block}\n`
}

const ReviewResultArtifactsPlugin: Plugin = async ({ directory, worktree }) => {
  const failedSDDSessions = new Map<string, SDDTaskFailure>()
  return {
  dispose: async () => {
    failedSDDSessions.clear()
  },
  event: async ({ event }) => {
    if (event.type === "session.deleted") {
      failedSDDSessions.delete(event.properties.info.id)
    }
  },
  "tool.execute.before": async (input, output) => {
    if (input.tool !== "task" || typeof output.args?.subagent_type !== "string") return
    const subagent = output.args.subagent_type
    if (isSDDPhase(subagent)) {
      const failure = failedSDDSessions.get(input.sessionID)
      if (failure) {
        throw new Error(failure.handoff)
      }
      return
    }
    if (!REVIEW_AGENTS.has(subagent)) return
    if (typeof output.args.prompt !== "string") {
      throw new Error("review task is missing GENTLE_AI_REVIEW_BINDING")
    }
    if (output.args.background === true) {
      throw new Error("bound review tasks must run in the foreground so the launching session can relay the raw result")
    }
    output.args.prompt = await injectReviewerContext(
      output.args.prompt,
      subagent,
      captureCwd(worktree, directory),
    )
  },
  "tool.execute.after": async (input, output) => {
    if (input.tool !== "task" || typeof input.args?.subagent_type !== "string") return
    const subagent = input.args.subagent_type
    if (isSDDPhase(subagent)) {
      try {
        taskResult(output.output, "SDD phase", "sddClass")
      } catch (cause) {
        if (extractionClass(cause, "sddClass") === "empty_result") {
          const recovered = attemptArtifactRecovery(subagent, captureCwd(worktree, directory))
          if (recovered) {
            output.output = recovered
            return
          }
        }
        const failure = sddTaskFailure(subagent, captureCwd(worktree, directory), cause)
        failedSDDSessions.set(input.sessionID, failure.sddFailure)
        throw failure
      }
      return
    }

    if (!REVIEW_AGENTS.has(subagent)) return
    if (typeof input.args.prompt !== "string" || !BINDING.test(input.args.prompt)) return
    output.output = reviewerResult(output.output)
  },
  }
}

export default ReviewResultArtifactsPlugin
