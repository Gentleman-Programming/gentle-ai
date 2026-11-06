import type { Plugin } from "@opencode-ai/plugin"

const TASK_RESULT = /^<task id="[^"\r\n]+" state="completed">\n(?:<summary>[^<>\r\n]+<\/summary>\n)?<task_result>\n([\s\S]*?)\n<\/task_result>\n<\/task>$/
const TASK_TAG = /<\/?(?:task|task_result|summary)(?:\s|>)/
const SDD_PHASES = ["sdd-init", "sdd-explore", "sdd-research", "sdd-propose", "sdd-spec", "sdd-design", "sdd-tasks", "sdd-apply", "sdd-verify", "sdd-archive", "sdd-onboard"]
const SDD_TASK_FAILURE_PREFIX = "GENTLE_AI_SDD_FAILURE "
const SDD_TASK_ROUTE_TOKEN = /^[A-Za-z0-9][A-Za-z0-9._:@-]{0,127}$/
const SDD_PREFLIGHT_QUESTION_PREFIX = "Gentle AI SDD preflight "
const SDD_PREFLIGHT_HEADING = "## SDD Session Preflight"
// #2855: host cwd does not identify the coordinator's selected change/store.
const SDD_TASK_CONTINUATION_GUIDANCE = "Return to the active SDD coordinator and inspect only its retained structured status for the selected change and artifact store. If that status is unavailable, report this terminal failure and ask the user to select the change and artifact store. Do not infer either, run unscoped status discovery, retry, or launch another phase."

type SDDTaskFailure = { phase: string, code: string, handoff: string }
type SDDTaskFailureError = Error & { sddFailure: SDDTaskFailure }
type SDDPreflightQuestion = { question: string, options: Array<{ label: string }>, multiple?: boolean }
type SDDPreflightGroup = { header: string, options: Array<{ label: string, description: string }> }

// Canonical grouped preflight shape. The runtime — not the model — owns
// these labels and descriptions, so a user who picks a rendered option is
// always accepted downstream regardless of the model's phrasing or locale.
const SDD_PREFLIGHT_GROUPS: SDDPreflightGroup[] = [
  {
    header: "Pace",
    options: [
      { label: "Interactive", description: "Confirm before each SDD phase advances." },
      { label: "Automatic", description: "Advance through SDD phases without per-phase confirmation." },
    ],
  },
  {
    header: "Artifacts",
    options: [
      { label: "OpenSpec", description: "Track this change with OpenSpec proposal, spec, design, and task files." },
      { label: "Engram", description: "Track this change with Engram memory topics." },
      { label: "Both", description: "Track this change with OpenSpec files and Engram memory together." },
    ],
  },
  {
    header: "PR strategy",
    options: [
      { label: "Ask me", description: "Ask before opening a pull request when review risk is high." },
      { label: "Single PR", description: "Deliver the change as a single pull request." },
      { label: "Auto", description: "Chain pull requests automatically as work completes." },
    ],
  },
]
const SDD_PREFLIGHT_MARKER = /^Gentle AI SDD preflight \d\/3:\s*/
const SDD_PREFLIGHT_RETRY_GUIDANCE = "ask the grouped preflight again with the question tool; typed chat answers cannot create preflight authority"

function sddPreflightGuidance(message: string): Error {
  return new Error(`${message}; ${SDD_PREFLIGHT_RETRY_GUIDANCE}`)
}

function isSDDPreflightQuestion(question: unknown): boolean {
  return !!question && typeof question === "object" && !Array.isArray(question) && typeof (question as Record<string, unknown>).question === "string" && ((question as Record<string, unknown>).question as string).startsWith(SDD_PREFLIGHT_QUESTION_PREFIX)
}

// Canonicalizes a recognized grouped SDD preflight before OpenCode renders
// it: forces the exact host marker, single-select, non-empty headers, and
// the canonical option labels in canonical order, in place. A description
// the model supplied is kept only for the canonical label it was written
// for (matched tolerantly by label, never by array position), and only when
// every canonical label finds exactly one such option; otherwise the
// built-in descriptions are used so a label is never shown next to another
// option's description. This runs for every session (root or child) because
// it only shapes what the user sees; authority recording stays root-only.
function canonicalizeSDDPreflightQuestions(args: unknown): void {
  if (!args || typeof args !== "object" || Array.isArray(args)) return
  const questions = (args as Record<string, unknown>).questions
  if (!Array.isArray(questions)) return
  if (!questions.some((question) => isSDDPreflightQuestion(question))) return
  if (questions.length !== 3) {
    throw new Error("SDD preflight refused: ask Pace, Artifacts, and PR strategy in ONE `question` tool call with exactly three questions; typed chat answers cannot create preflight authority")
  }
  questions.forEach((raw, index) => {
    if (!raw || typeof raw !== "object" || Array.isArray(raw)) return
    const question = raw as Record<string, unknown>
    const group = SDD_PREFLIGHT_GROUPS[index]
    const marker = `${SDD_PREFLIGHT_QUESTION_PREFIX}${index + 1}/3:`
    question.question = typeof question.question === "string"
      ? (SDD_PREFLIGHT_MARKER.test(question.question) ? question.question.replace(SDD_PREFLIGHT_MARKER, `${marker} `) : `${marker} ${question.question}`.trim())
      : marker
    question.multiple = false
    if (typeof question.header !== "string" || question.header.trim() === "") question.header = group.header
    const supplied = Array.isArray(question.options) ? (question.options as unknown[]) : []
    const described = group.options.map((canonical) => {
      const matches = supplied.filter((option) => !!option && typeof option === "object" && !Array.isArray(option)
        && typeof (option as Record<string, unknown>).label === "string"
        && normalizeSDDPreflightAnswer((option as Record<string, unknown>).label as string) === normalizeSDDPreflightAnswer(canonical.label))
      if (matches.length !== 1) return undefined
      const description = (matches[0] as Record<string, unknown>).description
      return typeof description === "string" && description.trim() !== "" ? description : undefined
    })
    const keepDescriptions = supplied.length === group.options.length && described.every((description) => description !== undefined)
    question.options = group.options.map((canonical, optionIndex) => ({
      label: canonical.label,
      description: keepDescriptions ? (described[optionIndex] as string) : canonical.description,
    }))
  })
}

// Normalizes a user-facing answer or option label for tolerant matching:
// trims, collapses internal whitespace, lowercases, then strips diacritics
// via NFD decomposition so a typed or localized answer still binds.
function normalizeSDDPreflightAnswer(value: string): string {
  return value.trim().replace(/\s+/g, " ").toLowerCase().normalize("NFD").replace(/[\u0300-\u036f]/g, "")
}

function isSDDPhase(agent: string): boolean {
  return SDD_PHASES.some((phase) => agent === phase || agent.startsWith(phase + "-"))
}

function isBackgroundTask(args: unknown): boolean {
  return !!args && typeof args === "object" && !Array.isArray(args) && (args as Record<string, unknown>).background === true
}

async function isRootSession(client: any, sessionID: string): Promise<boolean> {
  try {
    const result = await client.session.get({ path: { id: sessionID } })
    const info = result?.data ?? result
    return !!info && info.id === sessionID && (info.parentID === undefined || info.parentID === null)
  } catch {
    return false
  }
}

// sddPreflightBlock builds the parent-confirmed preflight block from a
// question tool call and its answers. The before-hook already canonicalizes
// a recognized preflight, so these structural checks are a safety net for a
// host that skipped it or a caller that bypassed it; only answer matching
// against the offered option labels is intentionally tolerant (trim,
// whitespace collapse, case, diacritics) so a typed or slightly-off answer
// still binds to the option the user meant.
function sddPreflightBlock(args: unknown, metadata: unknown): string | undefined {
  if (!args || typeof args !== "object" || Array.isArray(args)) return undefined
  const questions = (args as Record<string, unknown>).questions
  if (!Array.isArray(questions)) return undefined
  const recognized = questions.some((question) => isSDDPreflightQuestion(question))
  if (!recognized) return undefined
  if (questions.length !== 3) throw sddPreflightGuidance("SDD preflight requires exactly three questions")
  const parsed = questions.map((raw, index) => {
    const group = SDD_PREFLIGHT_GROUPS[index]
    if (!raw || typeof raw !== "object" || Array.isArray(raw)) throw sddPreflightGuidance(`SDD preflight question ${index + 1} is malformed`)
    const question = raw as SDDPreflightQuestion
    if (typeof question.question !== "string" || !question.question.startsWith(`${SDD_PREFLIGHT_QUESTION_PREFIX}${index + 1}/3:`) || question.multiple === true || !Array.isArray(question.options) || question.options.length !== group.options.length) {
      throw sddPreflightGuidance(`SDD preflight question ${index + 1} is malformed`)
    }
    if (question.options.some((option, optionIndex) => option?.label !== group.options[optionIndex].label)) throw sddPreflightGuidance(`SDD preflight question ${index + 1} changed the canonical option semantics`)
    return question
  })
  if (!metadata || typeof metadata !== "object" || Array.isArray(metadata)) throw sddPreflightGuidance("SDD preflight response metadata is missing")
  const answers = (metadata as Record<string, unknown>).answers
  if (!Array.isArray(answers) || answers.length !== 3) throw sddPreflightGuidance("SDD preflight response must contain three answers")
  const indexes = parsed.map((question, index) => {
    const groupName = SDD_PREFLIGHT_GROUPS[index].header
    const answer = answers[index]
    if (!Array.isArray(answer) || answer.length === 0) throw sddPreflightGuidance(`SDD preflight answer for ${groupName} is missing`)
    if (answer.length !== 1 || typeof answer[0] !== "string") throw sddPreflightGuidance(`SDD preflight answer for ${groupName} is not single-select`)
    const normalizedAnswer = normalizeSDDPreflightAnswer(answer[0])
    const matches = question.options.flatMap((option, optionIndex) => normalizeSDDPreflightAnswer(option.label) === normalizedAnswer ? [optionIndex] : [])
    if (matches.length !== 1) throw sddPreflightGuidance(`SDD preflight answer for ${groupName} is outside the offered domain`)
    return matches[0]
  })
  return [
    SDD_PREFLIGHT_HEADING,
    "Parent-confirmed by the runtime; models and child agents cannot create or modify this block.",
    `- Pace: ${["interactive", "auto"][indexes[0]]}`,
    `- Artifact store: ${["openspec", "engram", "hybrid"][indexes[1]]}`,
    `- Delivery strategy: ${["ask-on-risk", "single-pr", "auto-chain"][indexes[2]]}`,
    "- Review policy: 400 changed lines",
  ].join("\n")
}

function taskResult(output: unknown): void {
  if (typeof output !== "string" || output.trim() === "") {
    throw Object.assign(new Error("SDD phase output must not be empty"), { sddClass: "empty_result" })
  }
  const trimmed = output.trim()
  const envelope = TASK_RESULT.exec(trimmed)
  if (!envelope) {
    if (TASK_TAG.test(trimmed)) throw Object.assign(new Error("SDD phase output contains a malformed task result envelope"), { sddClass: "malformed_result" })
    return
  }
  if (envelope[1].trim() === "") throw Object.assign(new Error("SDD phase task result is empty"), { sddClass: "empty_result" })
  if (TASK_TAG.test(envelope[1])) throw Object.assign(new Error("SDD phase task result contains a nested task envelope"), { sddClass: "malformed_result" })
}

function taskRouteModel(metadata: unknown): string | undefined {
  if (!metadata || typeof metadata !== "object" || Array.isArray(metadata)) return undefined
  const model = (metadata as Record<string, unknown>).model
  if (!model || typeof model !== "object" || Array.isArray(model)) return undefined
  const providerID = (model as Record<string, unknown>).providerID
  const modelID = (model as Record<string, unknown>).modelID
  if (typeof providerID !== "string" || typeof modelID !== "string") return undefined
  if (!SDD_TASK_ROUTE_TOKEN.test(providerID) || !SDD_TASK_ROUTE_TOKEN.test(modelID)) return undefined
  return `${providerID}/${modelID}`
}

function sddTaskFailure(phase: string, cause: unknown, metadata?: unknown): SDDTaskFailureError {
  const empty = (cause as Record<string, unknown> | null)?.sddClass === "empty_result"
  const code = empty ? "sdd_task_result_empty" : "sdd_task_result_malformed"
  const taskModel = taskRouteModel(metadata)
  const guidance = "Do not retry or advance SDD; inspect the existing artifact state and surface the terminal failure to the user."
  const summary = empty
    ? `${phase} produced no task output at all. The child task returned nothing, which most often means the provider rejected the request before generation (authentication, region, or model access), the task was interrupted, or the phase genuinely wrote nothing. ${guidance}`
    : `${phase} returned no valid task result. ${guidance}`
  const failure: SDDTaskFailure = {
    phase,
    code,
    handoff: SDD_TASK_FAILURE_PREFIX + JSON.stringify({
      schemaName: "gentle-ai.sdd-task-result-failure/v1",
      status: "blocked",
      code,
      phase,
      ...(taskModel === undefined ? {} : { taskModel }),
      summary,
      continuation: SDD_TASK_CONTINUATION_GUIDANCE,
    }),
  }
  return Object.assign(new Error(failure.handoff), { sddFailure: failure }) as SDDTaskFailureError
}

function sddDispatchLatched(requested: string, failure: SDDTaskFailure): Error {
  return new Error(SDD_TASK_FAILURE_PREFIX + JSON.stringify({
    schemaName: "gentle-ai.sdd-task-result-failure/v1",
    status: "blocked",
    code: "sdd_task_dispatch_latched",
    phase: requested,
    latchedPhase: failure.phase,
    latchedCode: failure.code,
    summary: `${requested} was not dispatched. Earlier in this session ${failure.phase} returned ${failure.code}, and SDD launches stay latched afterwards so a failed phase is never silently retried and no later phase advances on top of it. No provider call, no subagent, and no artifact write happened for this launch, so it produced no new evidence about the original failure.`,
    continuation: SDD_TASK_CONTINUATION_GUIDANCE,
    exit: "Inspect the artifact state the original failure left, surface it to the user, and start a new session to launch SDD phases again. Relaunching in this session cannot dispatch.",
  }))
}

const SDDTaskResultArtifactsPlugin: Plugin = async ({ client }) => {
  const failedSDDSessions = new Map<string, SDDTaskFailure>()
  const confirmedPreflights = new Map<string, string>()
  return {
    dispose: async () => { failedSDDSessions.clear(); confirmedPreflights.clear() },
    event: async ({ event }) => {
      if (event.type === "session.deleted") {
        failedSDDSessions.delete(event.properties.info.id)
        confirmedPreflights.delete(event.properties.info.id)
      }
    },
    "tool.execute.before": async (input, output) => {
      if (input.tool === "question") {
        canonicalizeSDDPreflightQuestions(output.args)
        return
      }
      if (input.tool !== "task" || typeof output.args?.subagent_type !== "string") return
      const subagent = output.args.subagent_type
      if (!isSDDPhase(subagent)) return
      if (!(await isRootSession(client, input.sessionID))) throw new Error("SDD child dispatch refused: only the interactive root session may carry parent-confirmed SDD preflight authority")
      const failure = failedSDDSessions.get(input.sessionID)
      if (failure) throw sddDispatchLatched(subagent, failure)
      if (typeof output.args.prompt !== "string") throw new Error("SDD child dispatch refused: task prompt is unavailable")
      if (output.args.prompt.includes(SDD_PREFLIGHT_HEADING)) throw new Error("SDD child dispatch refused: model-authored preflight text cannot create parent-confirmed authority")
      const preflight = confirmedPreflights.get(input.sessionID)
      if (!preflight) throw new Error("SDD child dispatch refused: parent-confirmed SDD preflight is missing; ask the canonical grouped preflight with the question tool; typed chat answers cannot create preflight authority")
      output.args.prompt = `${preflight}\n\n${output.args.prompt}`
    },
    "tool.execute.after": async (input, output) => {
      if (input.tool === "question") {
        if (!(await isRootSession(client, input.sessionID))) return
        try {
          const block = sddPreflightBlock(input.args, output.metadata)
          if (block !== undefined) confirmedPreflights.set(input.sessionID, block)
        } catch (cause) {
          confirmedPreflights.delete(input.sessionID)
          throw cause
        }
        return
      }
      if (input.tool !== "task" || typeof input.args?.subagent_type !== "string") return
      const subagent = input.args.subagent_type
      if (!isSDDPhase(subagent)) return
      // OpenCode invokes this hook for the background launch acknowledgement while
      // the child is still running. That signal has no terminal task result to
      // validate; artifact/status ownership observes eventual completion.
      if (isBackgroundTask(input.args)) return
      try {
        taskResult(output.output)
      } catch (cause) {
        const failure = sddTaskFailure(subagent, cause, output.metadata)
        failedSDDSessions.set(input.sessionID, failure.sddFailure)
        throw failure
      }
    },
  }
}

export default SDDTaskResultArtifactsPlugin
