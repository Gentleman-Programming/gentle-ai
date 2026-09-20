// Cross-lane battery hook harness.
//
// Emulates ONLY the OpenCode Task hook surface (tool.execute.before /
// tool.execute.after) around the REAL transport plugin bytes copied next to
// this file as plugin.mts. The harness accepts only the provider-owned Task
// fields returned by STATUS, exactly as the live orchestration contract does.
//
// Input: one case config JSON path as argv[2]:
//   {
//     "name": string,
//     "subagent": string,                     // e.g. review-reliability
//     "prompt": string,                       // provider_task.prompt verbatim
//     "task_output": string,                  // raw reviewer/validator output
//     "skip_after": boolean                    // materialize only, then dispose
//   }
//
// skip_after models the field shape this battery previously missed: a host
// whose Task never executed after the before hook rewrote its prompt argument.
// The relay is disposed instead of completed, so the next process starts from
// an uncaptured slot exactly as the real host did.
// Output: one JSON result line on stdout:
//   { name, before_ok, after_ok, child_prompt, output, error }
import { readFileSync } from "node:fs"
import plugin from "./plugin.mts"

interface CaseConfig {
  name: string
  subagent: string
  prompt: string
  task_output: string
  skip_after?: boolean
}

const config = JSON.parse(readFileSync(process.argv[2], "utf8")) as CaseConfig
const prompt = config.prompt

const hooks = await plugin({
  directory: process.cwd(),
  worktree: process.cwd(),
} as never)

const result = { name: config.name, before_ok: false, after_ok: false, child_prompt: "", output: "", error: "" }
const before = { args: { subagent_type: config.subagent, prompt } }
try {
  await hooks["tool.execute.before"]!(
    { tool: "task", sessionID: "battery", callID: config.name } as never,
    before as never,
  )
  result.before_ok = true
  result.child_prompt = before.args.prompt
  if (config.skip_after === true) {
    await hooks.dispose?.()
    console.log(JSON.stringify(result))
    process.exit(0)
  }
  const after = { output: config.task_output, metadata: {} }
  await hooks["tool.execute.after"]!(
    { tool: "task", sessionID: "battery", callID: config.name, args: { subagent_type: config.subagent } } as never,
    after as never,
  )
  result.after_ok = true
  result.output = after.output
} catch (cause) {
  result.error = cause instanceof Error ? cause.message : String(cause)
}
console.log(JSON.stringify(result))
