import type { Plugin } from "@opencode-ai/plugin"

const TASK_RESULT = /^<task id="[^"\r\n]+" state="completed">\n(?:<summary>[^<>\r\n]+<\/summary>\n)?<task_result>\n([\s\S]*?)\n<\/task_result>\n<\/task>$/
const TASK_TAG = /<\/?(?:task|task_result|summary)(?:\s|>)/
const SDD_PHASES = ["sdd-init", "sdd-explore", "sdd-propose", "sdd-spec", "sdd-design", "sdd-tasks", "sdd-apply", "sdd-verify", "sdd-archive", "sdd-onboard"]
const SDD_TASK_FAILURE_PREFIX = "GENTLE_AI_SDD_FAILURE "
const SDD_TASK_ROUTE_TOKEN = /^[A-Za-z0-9][A-Za-z0-9._:@-]{0,127}$/

type SDDTaskFailure = { phase: string, code: string, handoff: string }
type SDDTaskFailureError = Error & { sddFailure: SDDTaskFailure }

function isSDDPhase(agent: string): boolean {
  return SDD_PHASES.some((phase) => agent === phase || agent.startsWith(phase + "-"))
}

function isBackgroundTask(args: unknown): boolean {
  return !!args && typeof args === "object" && !Array.isArray(args) && (args as Record<string, unknown>).background === true
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

function shellQuote(value: string): string {
  return `'${value.replace(/'/g, "'\\''")}'`
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

function sddTaskFailure(phase: string, cwd: string, cause: unknown, metadata?: unknown): SDDTaskFailureError {
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
      continuation: `gentle-ai sdd-status --cwd ${shellQuote(cwd)} --json`,
    }),
  }
  return Object.assign(new Error(failure.handoff), { sddFailure: failure }) as SDDTaskFailureError
}

function sddDispatchLatched(requested: string, failure: SDDTaskFailure, cwd: string): Error {
  return new Error(SDD_TASK_FAILURE_PREFIX + JSON.stringify({
    schemaName: "gentle-ai.sdd-task-result-failure/v1",
    status: "blocked",
    code: "sdd_task_dispatch_latched",
    phase: requested,
    latchedPhase: failure.phase,
    latchedCode: failure.code,
    summary: `${requested} was not dispatched. Earlier in this session ${failure.phase} returned ${failure.code}, and SDD launches stay latched afterwards so a failed phase is never silently retried and no later phase advances on top of it. No provider call, no subagent, and no artifact write happened for this launch, so it produced no new evidence about the original failure.`,
    continuation: `gentle-ai sdd-status --cwd ${shellQuote(cwd)} --json`,
    exit: "Inspect the artifact state the original failure left, surface it to the user, and start a new session to launch SDD phases again. Relaunching in this session cannot dispatch.",
  }))
}

const SDDTaskResultArtifactsPlugin: Plugin = async ({ directory, worktree }) => {
  const failedSDDSessions = new Map<string, SDDTaskFailure>()
  const cwd = worktree || directory
  return {
    dispose: async () => { failedSDDSessions.clear() },
    event: async ({ event }) => {
      if (event.type === "session.deleted") failedSDDSessions.delete(event.properties.info.id)
    },
    "tool.execute.before": async (input, output) => {
      if (input.tool !== "task" || typeof output.args?.subagent_type !== "string") return
      const subagent = output.args.subagent_type
      if (!isSDDPhase(subagent)) return
      const failure = failedSDDSessions.get(input.sessionID)
      if (failure) throw sddDispatchLatched(subagent, failure, cwd)
    },
    "tool.execute.after": async (input, output) => {
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
        const failure = sddTaskFailure(subagent, cwd, cause, output.metadata)
        failedSDDSessions.set(input.sessionID, failure.sddFailure)
        throw failure
      }
    },
  }
}

export default SDDTaskResultArtifactsPlugin
