# Feature: Mandatory ODD Delegation

## Objective

Make ODD delegation behavioral instead of advisory, in both gentle-ai (canonical routing block)
and gentle-pi (pi orchestrator guidance), so the orchestrator delegates subagents when the
defined triggers fire instead of silently executing everything inline.

## Problem

The orchestrator runs the whole task inline regardless of size. Investigation found:

1. The canonical always-on ODD routing block (`internal/components/agentguidance/routing.go`,
   gentle-ai) describes delegation only as a route *description* ("delegate one narrow
   exploration when understanding needs 4+ files") — no mandatory trigger, no order to stop
   and delegate.
2. The "Mandatory Delegation Triggers" table (4-file rule, multi-file write rule, incident
   rule, verification rule, long-session rule) exists only in per-runtime SDD orchestrator
   assets (`internal/assets/<agent>/sdd-orchestrator.md`), SDD-scoped, and explicitly bound
   away from ODD work.
3. ODD routing is pure model judgment: nothing requires the route decision to be stated,
   recorded, or checked, so non-delegation is invisible (SDD delegates because a native
   dispatcher tells it to; ODD has no equivalent forcing function).
4. Permissive framing ("smallest useful topology", "small stays small") outcompetes the single
   buried trigger.

## Scope

- gentle-ai: `internal/components/agentguidance/routing.go` + tests. Add to the ODD section:
  mandatory delegation triggers (runtime-neutral, no SDD references), a per-task route
  declaration recorded in the feature document, and a mid-session backstop (long-session rule).
- gentle-pi: the orchestrator ODD guidance assets that mirror routing —
  `skills/gentle-ai` package assets (orchestrator/delegation-related AGENTS.md sections) so the
  pi parent gets the same mandatory triggers.

## Constraints

- Runtime-neutral wording in gentle-ai (no pi-only tool names).
- No SDD coupling: routing guidance must stay independent of optional SDD assets.
- Keep the drift ratchet tests green (orchestrator_drift_ratchet_test.go).
- TDD: observe RED before implementing, GREEN after, per repo test runner (go test).

## Tasks

- [x] T1. Explore gentle-pi asset layout and locate the mirrored ODD/orchestrator guidance files.
  - Route: delegated (gentle-ai-explore). Evidence: handoff mapped assets/orchestrator.md (triggers :54-60), assets/orchestrator-delegation.md (:124-133), extensions/gentle-ai.ts (:1272-1279), pinning tests.
- [x] T2. gentle-ai: write failing tests for mandatory triggers in RenderRouting output.
  - Route: inline (1 test file, single understood file). Evidence: RED observed — TestRenderRoutingMakesDelegationMandatory failed with missing clauses.
- [x] T3. gentle-ai: implement triggers + route declaration + backstop in routing.go; GREEN.
  - Route: inline (same file pair, already understood). Evidence: `go test ./internal/components/agentguidance/` ok; build + assets + sdd tests ok. Commit `6ad6843a` on feat/odd-mandatory-delegation.
- [x] T4. gentle-pi: mirror mandatory triggers into pi orchestrator ODD guidance.
  - Route: delegated (gentle-ai-worker). Evidence: RED 2 fail → GREEN 12 pass (odd-routing-contract), focused suites 181 pass, npm test EXIT=0 (2680 tests). Commit `f58774f2` on feat/odd-mandatory-delegation. Parent spot check re-ran odd-routing-contract: 12 pass. RDD assess: high → commit is the native review candidate (preflight STATUS next).
- [x] T5. Run validation in both repos; work-unit commits on feature branches.
  - gentle-ai: build + agentguidance + assets + sdd tests ok; RDD assess on `6ad6843a`: medium → slice closed at feature end; preflight STATUS run (outcome below).
  - gentle-pi: focused suites 181 pass, npm test EXIT=0 (2680 tests); native review of `f58774f2` (lineage review-cfc3e15af3258346, tier high, 4 lens reviewers: risk/resilience/readability/reliability): **approved**, acknowledged, authority burned.

## Outcome

- gentle-ai `6ad6843a`: Mandatory Delegation Triggers section (mapping/writer/preparation, long-session backstop, route declaration, never-selects-SDD) in the canonical ODD block; ODD step 6 bound.
- gentle-pi `f58774f2`: canonical port in orchestrator-delegation.md, core list reconciled (Verification = #5, matching RDD trigger-5 pin), ODD step 6 binding in extensions/gentle-ai.ts, pinning tests added. Native review approved.
- Delivery (push/PR) remains the user's decision under ordinary repository policy.

## Acceptance criteria

- RenderRouting output contains mandatory delegation triggers, route-declaration requirement,
  and long-session backstop.
- gentle-pi orchestrator guidance carries the same triggers for the pi parent.
- All tests pass in both repos.

## Progress

- Route decisions per task: recorded here as work proceeds.
