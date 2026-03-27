---
name: judgment-day
description: >
  Parallel adversarial review protocol that launches two independent blind judge sub-agents
  simultaneously to review the same target, synthesizes their findings, applies fixes,
  and re-judges until both pass or escalates after 2 iterations.
  Trigger: When user says "judgment day", "judgment-day", "review adversarial", "dual review",
  "doble review", "juzgar", "que lo juzguen".
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.1"
---

## When to Use

- User explicitly asks for "judgment day", "judgment-day", or equivalent trigger phrases
- After significant implementations before merging
- When high-confidence review of code, features, or architecture is needed
- When a single reviewer might miss edge cases or have blind spots
- When the cost of a production bug is higher than the cost of two review rounds

## Critical Patterns

### Pattern 0: Skill Resolution (BEFORE launching judges)

Follow the **Skill Resolver Protocol** (`_shared/skill-resolver.md`) before launching ANY sub-agent:

1. Obtain the skill registry: search engram (`mem_search(query: "skill-registry", project: "{project}")`) → fallback to `.atl/skill-registry.md` from the project root → skip if none
2. Identify the target files/scope — what code will the judges review?
3. Match relevant skills from the registry's **Compact Rules** by:
   - **Code context**: file extensions/paths of the target (e.g., `.go` → go-testing; `.tsx` → react-19, typescript)
   - **Task context**: "review code" → framework/language skills; "create PR" → branch-pr skill
4. Build a `## Project Standards (auto-resolved)` block with the matching compact rules
5. Inject this block into BOTH Judge prompts AND the Fix Agent prompt (identical for all)

This ensures judges review against project-specific standards, not just generic best practices.

**If no registry exists**: warn the user ("No skill registry found — judges will review without project-specific standards. Run `skill-registry` to fix this.") and proceed with generic review only.

### Pattern 1: Parallel Blind Review

- Launch **TWO** sub-agents via `delegate` (async, parallel — never sequential)
- Each agent receives the **same target** but works **independently**
- **Neither agent knows about the other** — no cross-contamination
- Both use identical review criteria but may find different issues
- NEVER do the review yourself as the orchestrator — your job is coordination only

### Pattern 2: Verdict Synthesis

The **orchestrator** (NOT a sub-agent) compares results after both `delegation_read` calls return:

```
Confirmed   → found by BOTH agents          → high confidence, fix immediately
Suspect A   → found ONLY by Judge A         → needs triage
Suspect B   → found ONLY by Judge B         → needs triage
Contradiction → agents DISAGREE on the same thing → flag for manual decision
```

Present findings as a structured verdict table (see Output Format).

### Pattern 3: Fix and Re-judge

1. If **confirmed issues** exist → delegate a **Fix Agent** (separate delegation)
2. After Fix Agent completes → re-launch **both judges in parallel** (same blind protocol, fresh delegates)
3. **Max 2 fix iterations.** If still failing → JUDGMENT: ESCALATED — report to user with full history
4. If both judges return clean → JUDGMENT: APPROVED ✅

---

## Decision Tree

```
User asks for "judgment day"
│
├── Target is specific files/feature/component?
│   ├── YES → continue
│   └── NO → ask user to specify scope before proceeding
│
▼
Resolve skills (Pattern 0): read registry → match by code + task context → build Project Standards block
▼
Launch Judge A + Judge B in parallel (delegate, async) — with Project Standards injected
▼
Wait for both to complete (delegation_read both)
▼
Synthesize verdict
│
├── No issues found?
│   └── JUDGMENT: APPROVED ✅ (stop here)
│
├── Issues found (confirmed, suspect, or contradictions)?
│   └── Delegate Fix Agent with confirmed issues list
│       ▼
│       Wait for Fix Agent to complete
│       ▼
│       Re-launch Judge A + Judge B in parallel (Round 2)
│       ▼
│       Synthesize verdict
│       │
│       ├── Clean → JUDGMENT: APPROVED ✅
│       │
│       └── Still issues → Delegate Fix Agent again (Round 3 / iteration 2)
│           ▼
│           Re-launch Judge A + Judge B in parallel (Round 3)
│           ▼
│           Synthesize verdict
│           │
│           ├── Clean → JUDGMENT: APPROVED ✅
│           └── Still issues → JUDGMENT: ESCALATED ⚠️ (report to user)
```

---

## Sub-Agent Prompt Templates

### Judge Prompt (use for BOTH Judge A and Judge B — identical)

```
You are an adversarial code reviewer. Your ONLY job is to find problems.

## Target
{describe target: files, feature, architecture, component}

{if compact rules were resolved in Pattern 0, inject the following block — otherwise OMIT this entire section}
## Project Standards (auto-resolved)
{paste matching compact rules blocks from the skill registry}

## Review Criteria
- Correctness: Does the code do what it claims? Are there logical errors?
- Edge cases: What inputs or states aren't handled?
- Error handling: Are errors caught, propagated, and logged properly?
- Performance: Any N+1 queries, inefficient loops, unnecessary allocations?
- Security: Any injection risks, exposed secrets, improper auth checks?
- Naming & conventions: Does it follow the project's established patterns AND the Project Standards above?
{if user provided custom criteria, add here}

## Return Format
Return a structured list of findings ONLY. No praise, no approval.

Each finding:
- Severity: CRITICAL | WARNING | SUGGESTION
- File: path/to/file.ext (line N if applicable)
- Description: What is wrong and why it matters
- Suggested fix: one-line description of the fix (not code, just intent)

Always include at the end: **Skill Resolution**: {injected|fallback-registry|fallback-path|none} — {details}

If you find NO issues, return:
VERDICT: CLEAN — No issues found.

## Instructions
Be thorough and adversarial. Assume the code has bugs until proven otherwise.
Your job is to find problems, NOT to approve. Do not summarize. Do not praise.
```

### Fix Agent Prompt

```
You are a surgical fix agent. You apply ONLY the confirmed issues listed below.

## Confirmed Issues to Fix
{paste the confirmed findings table from the verdict synthesis}

{if compact rules were resolved in Pattern 0, inject the following block — otherwise OMIT this entire section}
## Project Standards (auto-resolved)
{paste matching compact rules blocks from the skill registry}

## Context
- Original review criteria: {paste same criteria used for judges}
- Target: {same target description}

## Instructions
- Fix ONLY the confirmed issues listed above
- Do NOT refactor beyond what is strictly needed to fix each issue
- Do NOT change code that was not flagged
- After each fix, note: file changed, line changed, what was done

Return a summary:
## Fixes Applied
- [file:line] — {what was fixed}

**Skill Resolution**: {injected|fallback-registry|fallback-path|none} — {details}
```

---

## Output Format

```markdown
## Judgment Day — {target}

### Round {N} — Verdict

| Finding | Judge A | Judge B | Severity | Status |
|---------|---------|---------|----------|--------|
| Missing null check in auth.go:42 | ✅ | ✅ | CRITICAL | Confirmed |
| Race condition in worker.go:88 | ✅ | ❌ | WARNING | Suspect (A only) |
| Naming mismatch in handler.go:15 | ❌ | ✅ | SUGGESTION | Suspect (B only) |
| Error swallowed in db.go:201 | ✅ | ✅ | CRITICAL | Confirmed |

**Confirmed issues**: 2 CRITICAL
**Suspect issues**: 1 WARNING, 1 SUGGESTION
**Contradictions**: none

### Fixes Applied (Round {N})
- `auth.go:42` — Added nil check before dereferencing user pointer
- `db.go:201` — Propagated error instead of silently returning nil

### Round {N+1} — Re-judgment
- Judge A: PASS ✅ — No issues found
- Judge B: PASS ✅ — No issues found

---

### JUDGMENT: APPROVED ✅
Both judges pass clean. The target is cleared for merge.
```

### Escalation Format (after 2 failed iterations)

```markdown
## Judgment Day — {target}

### JUDGMENT: ESCALATED ⚠️

After 2 fix iterations, both judges still report issues.
Manual review required before proceeding.

### Remaining Issues
| Finding | Judge A | Judge B | Severity |
|---------|---------|---------|----------|
| {description} | ✅ | ✅ | CRITICAL |

### History
- Round 1: {N} confirmed issues found
- Fix 1: applied {list}
- Round 2: {N} issues remain
- Fix 2: applied {list}
- Round 3: {N} issues remain → escalated

Recommend: human review of the remaining issues above before re-running judgment day.
```

---

## Skill Resolution Feedback

After every delegation that returns a result, check the `**Skill Resolution**` field in each judge/fix-agent response:
- `injected` → skills were passed correctly ✅
- `fallback-registry`, `fallback-path`, or `none` → skill cache was lost (likely compaction). Re-read the registry immediately and inject compact rules in all subsequent delegations.

This is a self-correction mechanism. Do NOT ignore fallback reports.

---

## Language

- **Spanish input → Rioplatense**: "Juicio iniciado", "Los jueces están trabajando en paralelo...", "Los jueces coinciden", "Juicio terminado — Aprobado", "Escalado — necesita revisión humana"
- **English input**: "Judgment initiated", "Both judges are working in parallel...", "Both judges agree", "Judgment complete — Approved", "Escalated — requires human review"

---

## Rules

- The **orchestrator NEVER reviews code itself** — it only launches judges, reads results, and synthesizes
- Judges MUST be launched as `delegate` (async) so they run in **parallel**
- The **Fix Agent is a separate delegation** — never use one of the judges as the fixer
- If user provides **custom review criteria**, include them in BOTH judge prompts (identical)
- If target scope is **unclear**, stop and ask before launching — partial reviews are useless
- **Max 2 fix iterations** — on the third failure, escalate with full report, do not loop forever
- Always wait for BOTH judges to complete before synthesizing — never accept a partial verdict
- Suspect findings (only one judge) are reported but NOT automatically fixed — triage and escalate to user if needed

---

## Commands

```bash
# No CLI commands — this is a pure orchestration protocol.
# Execution happens via delegate() and delegation_read() tool calls.
```
