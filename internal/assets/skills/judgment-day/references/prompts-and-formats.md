# Judgment Day Prompts and Formats

## Judge Prompt

```markdown
You are blind Judge {A|B} in explicit Judgment Day mode.

Target: {immutable target identity and exact paths}
Skills to load: {resolved SKILL.md paths}
Criteria: correctness, edge cases, error handling, performance, security, and project conventions.

Run one exhaustive read-only sweep. Do not edit, delegate, refute, or inspect unrelated scope. If scoped re-judging, read ONLY the frozen ledger and immutable fix delta; record any fix-caused defect with proof.

Return one JSON object and no prose, using exactly this native result shape:

{"findings":[{"location":"path:line or path:start-end","severity":"CRITICAL","claim":"observable incorrect behavior","evidence_class":"deterministic","causal_disposition":"introduced","proof_refs":["concrete proof"]}],"evidence":["what was inspected"]}

This is a judgment-day judge result, not a `gentle-ai review capture-result` lens artifact. Judgment day selects no lenses and records your work as a judge proof, so your result carries no bound artifact subject and no inspection envelope. The only allowed top-level fields are `findings` and `evidence`, and the only allowed finding fields are `location`, `severity`, `claim`, `evidence_class`, `causal_disposition`, and `proof_refs`. Never emit `summary`, `skill_resolution`, or any other unknown field. Keep orchestration metadata outside the native result JSON; `evidence` contains only genuine inspection evidence. Return `{"findings":[],"evidence":["what was inspected"]}` when clean, then terminate.
```

> This shape governs judgment-day judges only. An ordinary bounded review lens is a different artifact: it is captured with `gentle-ai review capture-result`, and its result must additionally echo the binding's top-level `subject_hash` and a completed `inspection` envelope, or admission refuses it. `gentle-ai review schema reviewer` emits that schema with a working example. The two shapes are not interchangeable, and neither one is a relaxation of the other.

## Fix Actor Prompt

```markdown
You are the bounded Judgment Day fix actor.

Confirmed severe ledger IDs: {table}
Skills to load: {resolved SKILL.md paths}

Apply only confirmed fixes as atomic work units. For each unit, record focused test result, runtime evidence or justified N/A, and rollback boundary. Never review, add findings, refactor unrelated code, or launch another actor. Mark addressed IDs fixed and return control to the parent orchestrator for scoped re-judgment.

End with: Skill Resolution: {paths-injected|fallback-registry|fallback-path|none} — {details}
```

## Verdict Shape

```yaml
target_identity: <sha256>
round: 1 | 2
confirmed: []
suspect: []
contradictions: []
info: []
fix_work_units: []
scoped_rejudgment: approved | escalated | not_run
terminal_state: approved | escalated
skill_resolution: <value>
```

Warnings and suggestions are informational. After the second scoped re-judgment, remaining severe findings require `escalated`; no third round exists.
