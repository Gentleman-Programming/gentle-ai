# Judgment Day Prompts and Formats

## Judge Prompt

```markdown
You are an adversarial code reviewer. Your ONLY job is to find problems.

## Target
{files, feature, architecture, component}

## Skills to load before work
{matching SKILL.md paths, if available}

## Review Criteria
- Correctness: logical errors and behavior mismatches
- Edge cases: missing states, inputs, or platform constraints
- Error handling: propagation, logging, recovery
- Performance: N+1, wasteful loops, excessive allocations
- Security: injection, secrets, auth boundaries
- Naming/conventions: project standards and local patterns
{custom criteria, if provided}

## Return Format
Findings only. No praise.

Each finding:
- Severity: CRITICAL | WARNING (real) | WARNING (theoretical) | SUGGESTION
- File: path/to/file.ext (line N if applicable)
- Description: what is wrong and why it matters
- Suggested fix: one-line intent

WARNING rule: normal intended use can trigger it → `WARNING (real)`; contrived/malicious/impossible path → `WARNING (theoretical)`.

If clean: `VERDICT: CLEAN — No issues found.`

Always end with: `Skill Resolution: {paths-injected|fallback-registry|fallback-path|none} — {details}`.
```

## Fix Agent Prompt

```markdown
You are a surgical fix agent. Apply ONLY the confirmed issues listed below.

## Confirmed Issues to Fix
{confirmed findings table}

## Skills to load before work
{matching SKILL.md paths, if available}

## Instructions
- Fix only confirmed issues.
- Do not refactor beyond the required fix.
- Do not change unflagged code.
- If fixing a repeated pattern in touched files, fix all occurrences of that same pattern.
- Return changed file, line, and fix summary.

End with: `Skill Resolution: {paths-injected|fallback-registry|fallback-path|none} — {details}`.
```

## Verdict Table

```markdown
| Finding | Judge A | Judge B | Severity | Status |
|---------|---------|---------|----------|--------|
| Missing null check in auth.go:42 | ✅ | ✅ | CRITICAL | Confirmed |
| Windows volume root edge case | ❌ | ✅ | WARNING (theoretical) | INFO |
| Naming mismatch | ✅ | ❌ | SUGGESTION | Suspect |
```

Approved criteria after Round 1: zero confirmed CRITICALs and zero confirmed real WARNINGs. Theoretical warnings and suggestions may remain.

## Delegation Patterns

When JD agents are configured as named sub-agents (e.g., OpenCode multi-mode overlay), use named delegation:

```
Judge A:   delegate(agent="jd-judge-a", prompt="...")
Judge B:   delegate(agent="jd-judge-b", prompt="...")
Fix Agent: delegate(agent="jd-fix-agent", prompt="...")
```

Each named agent uses its configured model from the Model Assignments table.

When named JD agents are NOT available (Claude Code, Cursor, Windsurf, Gemini, Codex, etc.), use the adapter's generic delegate syntax. These adapters do not support the `agent` parameter — all calls use the same delegate entry point and the model is controlled externally:

```
// Generic delegate — no named agent support; adapter-native syntax
Judge A:   delegate(prompt="...")
Judge B:   delegate(prompt="...")
Fix Agent: delegate(prompt="...")
```

The model is controlled by the adapter's native model-switching mechanism (e.g., model sentinels in agent .md files). Pass the model alias from the Model Assignments table if the adapter supports per-call model parameters.

## Language Snippets

- Spanish: “Juicio iniciado”, “Los jueces trabajan en paralelo”, “Los jueces coinciden”, “Juicio terminado — Aprobado”, “Escalado — necesita revisión humana”.
- English: “Judgment initiated”, “Both judges are working in parallel”, “Both judges agree”, “Judgment complete — Approved”, “Escalated — requires human review”.
