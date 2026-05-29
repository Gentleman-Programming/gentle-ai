// === Gentle AI x Workflow governance preamble (workflow-governance skill) ===
// Copy this block to the TOP of any Workflow that delegates real work under ultracode.
// It reproduces, on the Workflow agent() surface, the controls Gentle AI binds to the Agent tool.
// Workflow scripts cannot import files, so this is a copy-paste template, not a module.

// Model Assignments table (source: CLAUDE.md "Model Assignments"). Mandatory per delegation.
const MODEL_BY_ROLE = {
  'sdd-explore': 'sonnet', 'sdd-propose': 'opus', 'sdd-spec': 'sonnet',
  'sdd-design': 'opus', 'sdd-tasks': 'sonnet', 'sdd-apply': 'sonnet',
  'sdd-verify': 'sonnet', 'sdd-archive': 'haiku',
  'jd-judge-a': 'sonnet', 'jd-judge-b': 'sonnet', 'jd-fix-agent': 'sonnet',
  'explore': 'sonnet', 'default': 'sonnet',
}

// Roles that map to a registered project agentType (recovers SKILL.md gates: TDD, delivery hard-stop, engram rules).
// Note: sdd-onboard is intentionally omitted — it is an orchestrator meta-command, not a sub-agent role; it falls back to 'default'.
const AGENTTYPE_BY_ROLE = {
  'sdd-explore': 'sdd-explore', 'sdd-propose': 'sdd-propose', 'sdd-spec': 'sdd-spec',
  'sdd-design': 'sdd-design', 'sdd-tasks': 'sdd-tasks', 'sdd-apply': 'sdd-apply',
  'sdd-verify': 'sdd-verify', 'sdd-archive': 'sdd-archive',
  'jd-judge-a': 'jd-judge-a', 'jd-judge-b': 'jd-judge-b', 'jd-fix-agent': 'jd-fix-agent',
  'explore': 'Explore',
}

// Cross-cutting skill registry (source: .atl/skill-registry.md).
// Populate at AUTHOR time from .atl/skill-registry.md (its Path column gives per-user absolute paths).
// Do NOT ship hardcoded home paths — they break for every other user.
const SKILL_PATHS = {}

const TDD_ROLES = new Set(['sdd-apply', 'sdd-verify'])

// Assemble a governed prompt + opts for a delegation.
// ctx: { project, strictTdd, testCommand, engramTopicKey, priorContext, skills: [names] }
function buildGoverned(role, task, ctx = {}) {
  const requested = ctx.skills || []
  const skills = requested.map(s => SKILL_PATHS[s]).filter(Boolean)
  const unresolvedSkills = requested.filter(s => !SKILL_PATHS[s])
  const parts = []
  if (skills.length) {
    parts.push('## Skills to load before work\nRead these SKILL.md files fully BEFORE any work:\n' +
      skills.map(p => '- ' + p).join('\n'))
  }
  if (ctx.priorContext) parts.push('## Prior context (from engram)\n' + ctx.priorContext)
  if (ctx.engramTopicKey) {
    parts.push(`## Engram topic\nRead via mem_search("${ctx.engramTopicKey}", project:"${ctx.project || ''}") + mem_get_observation BEFORE work; MERGE, never overwrite.`)
  }
  parts.push(`If you make decisions, discoveries, or fixes, save them via mem_save with project:"${ctx.project || ''}" BEFORE returning.`)
  if (TDD_ROLES.has(role) && ctx.strictTdd) {
    parts.push(`STRICT TDD MODE IS ACTIVE. Test runner: ${ctx.testCommand || '<resolve from sdd-init>'}. You MUST follow strict-tdd.md. Do NOT fall back to Standard Mode.`)
  }
  parts.push('## Task\n' + task)
  return {
    prompt: parts.join('\n\n'),
    opts: {
      model: MODEL_BY_ROLE[role] || MODEL_BY_ROLE.default,
      agentType: AGENTTYPE_BY_ROLE[role], // undefined for generic roles -> default workflow subagent
    },
    unresolvedSkills,
  }
}

// Machine-checked governance gate. Returns an array of violations (empty = pass).
function assertGoverned(role, built, ctx) {
  const errs = []
  if (!built.opts.model) errs.push(`[${role}] missing model alias`)
  if (!(role in MODEL_BY_ROLE)) errs.push(`[${role}] unknown role '${role}' — not in MODEL_BY_ROLE; no agentType/TDD/cost gating applied`)
  if (!/mem_save/.test(built.prompt)) errs.push(`[${role}] missing engram save directive`)
  if (TDD_ROLES.has(role) && ctx && ctx.strictTdd && !/STRICT TDD MODE IS ACTIVE/.test(built.prompt)) errs.push(`[${role}] missing strict-TDD forwarding`)
  if (ctx && ctx.skills && ctx.skills.length && built.unresolvedSkills && built.unresolvedSkills.length) errs.push(`[${role}] skills requested but unresolved: ${built.unresolvedSkills.join(',')} — populate SKILL_PATHS from .atl/skill-registry.md`)
  if (ctx && !ctx.project) errs.push(`[${role}] empty engram project — pin a backed project (mem_current_project / explicit / .engram/config.json)`)
  return errs
}

// Drop-in replacement for bare agent() on substantive work.
function launchSubagent(role, task, { ctx = {}, schema, phase, label } = {}) {
  const built = buildGoverned(role, task, ctx)
  const errs = assertGoverned(role, built, ctx)
  if (errs.length) throw new Error('governance assertion failed: ' + errs.join('; '))
  return agent(built.prompt, { ...built.opts, schema, phase, label: label || `gov:${role}` })
}
