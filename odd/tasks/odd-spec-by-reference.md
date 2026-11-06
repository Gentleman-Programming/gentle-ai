# ODD spec-by-reference parity with gentle-pi (Gentleman-Programming/gentle-shell#1713)
Branch: `fix/odd-spec-by-reference` (base `origin/main` f4d3f3e8) · Delivery: single-pr · Runner: `go test ./internal/components/agentguidance/ ./internal/assets/`; full `go test ./...`
Engram mirror: `odd/odd-spec-by-reference/tasks` (project gentle-pi session) · Route: inline (user: no delegation today; RDD reviewers allowed)

## Specs
S1. Parity: "luego una vez que esto lo tengamos tenemos que hcer la paridad en gentle-ai" — the canonical ODD routing block carries the same contract gentle-pi merged in gentle-shell#1718.
S2. Feature document is the reference with a fixed order: header (2-3 lines) → `## Specs` (numbered `S#`, user's exact strings, error messages, and examples verbatim, never summarized, no unrequested requirements) → `## Tasks` (one line per task: ID, linked `S#`, route, commit) → `## Log` last (`L1` = original request verbatim; later user corrections verbatim; evidence and decisions).
S3. Change = only the affected spec and task: "A diferencia de SDD, un cambio no re hace todo, solo re hace esa spec y la tarea asociada".
S4. Handoffs pass a reference, never a paraphrase: `Spec: odd/tasks/<feature>.md (read until ## Log). Do T#; S#.`; without a document, the user's request verbatim. Workers report covered `S#`.
S5. Verify reads the whole document, runs the spec's examples (isolated state when they mutate), and returns a verdict per `S#`. User-reported failures are reproduced before deciding they already work.
S6. The shipped OpenCode `gentle-ai-worker`, `gentle-ai-verify`, and `gentle-ai-explore` agents match gentle-pi's merged wording.

## Tasks
- [x] T1 (S2-S5) inline · canonical routing.go + routing_test.go + docs/usage.md · RED→GREEN · 6c664915
- [x] T2 (S6) inline · internal/assets/opencode/agents/gentle-ai-{worker,verify,explore}.md + assets test · RED→GREEN · 122e85b7
- [x] T4 (S5) inline · review follow-ups: guard verify examples in the canon (authorized, isolated state) and pin docs/usage.md to the contract · RED→GREEN · 5c460fce
- [x] T5 (S5) inline · review follow-up: verify's command scope names authorized spec example commands; record T2/T4 commit identities · RED→GREEN · recorded in L8
- [ ] T3 (S1) pending merge · in gentle-pi, `npm run mirror:odd-routing` to regenerate `fixtures/odd-routing-canonical.md`

## Log
L1 2026-10-03 user (verbatim): > luego una vez que esto lo tengamos tenemos que hcer la paridad en gentle-ai
L2 2026-10-03 user (verbatim): > ahh si dale haz la paridad
L3 2026-10-03 context: gentle-pi PR gentle-shell#1718 merged (cd4ba5a7) with the always-on contract in extensions/gentle-ai.ts ODD steps 5-6, assets/orchestrator-memory.md, and the three agents. Canon still prescribed "objective, problem, why, scope, constraints..." and "passes the locator and relevant context".
L4 2026-10-03 T1 evidence (risk: medium, canonical prompt contract for every agent):
   RED: TestRenderRoutingOrganicTaskContinuity failed for all 17 catalog agents on the new spec-by-reference clauses.
   GREEN: `go test ./internal/components/agentguidance/ ./internal/assets/` ok; `go test ./...` 80 packages ok; `go vet` and `gofmt -l` clean.
   Decision: the handoff example is `Spec: odd/tasks/<feature>.md, T2, S3-S4` and "read until `## Log`" is stated outside the code span (backticks cannot nest; gentle-pi's always-on example nests them).
L5 2026-10-03 T2 evidence (risk: medium, shipped agent prompts):
   RED: TestOpenCodeGenericAgentsReadTheFeatureSpecByReference missing all 8 clauses.
   GREEN: the three #1713 sentences ported verbatim from gentle-pi; runtime-specific differences (OpenCode `task` tool, codegraph lifecycle, handoff size wording) kept. `go test ./...` 80 packages ok; `gofmt -l` clean.
   Follow-up for gentle-pi: its always-on step 6 example nests backticks (`... (read until `## Log`) ...`), which breaks the inline code span; T3's PR can adopt this canon wording.
L6 2026-10-03 next: push + PR to Gentleman-Programming/gentle-ai need user authorization; then T3 in gentle-pi.
L7 2026-10-03 RDD: lineage review-27a499d50fc565d6 (risk medium, 8 files, 107 lines) granted by the user, approved, acknowledged. Findings: R3-verify-examples-unguarded-in-canon (WARNING, step 6 told verify to run examples unconditionally; every runtime renders it) and R3-usage-doc-untested (SUGGESTION).
   T4 evidence: RED on 17 agents for the guarded clause and on docs/usage.md; GREEN after the fix. gentle-pi's merged always-on step 6 carries the same unguarded wording: fix it in T3's PR together with the nested-backtick example.
L8 2026-10-03 RDD: lineage for 5c460fce (8 files, 139 lines) granted, approved, acknowledged. Findings: R3-001 (SUGGESTION) verify's lead sentence limited it to test/build/lint commands, so a literal model could refuse spec examples; R3-002 (SUGGESTION) T2/T4 lacked commit identities.
   T5 evidence: RED on the new verify clause in TestOpenCodeGenericAgentsReadTheFeatureSpecByReference, GREEN after widening the scope to "test, build, lint, or spec example commands". T5's own identity is the commit after 5c460fce (a commit cannot contain its own hash). gentle-pi's verify agent needs the same widening in T3's PR.
