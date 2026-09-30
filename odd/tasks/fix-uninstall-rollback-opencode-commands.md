# Feature: fix(lifecycle): track standalone OpenCode commands in uninstall/rollback

## Status
Registered only — NOT implemented. Created per explicit user instruction after closing
`smartops-ui-skill-mvp`, to track a transversal, pre-existing gap surfaced (not introduced) by
that feature's T9 smoke test and independently confirmed by its RDD review (findings
`R4-uninstall-command-leak`, `R3-002`, `R3-001`).

## Problem
`gentle-ai uninstall` removes standalone skills' directories correctly, but never removes their
paired OpenCode standalone command files under `opencode/commands/*.md`. Confirmed for every
existing standalone QA command (`qa-evidence.md`, `qa-supervisor.md`, `qa-doc-access.md`,
`qa-doc-reference.md`, `qa-locator-hunting.md`) and now also for `smartops-ui.md` — same gap,
same root cause, not specific to any one skill.

## Why
- Real, observed behavior (not theoretical): a live isolated smoke test (T9 of
  `smartops-ui-skill-mvp`) ran a real `uninstall` and confirmed the leak firsthand.
- RDD review of that same feature flagged it independently across two lenses (resilience,
  reliability) as a real gap in rollback completeness, worsened (one more orphaned-file class)
  by adding smartops-ui without fixing the underlying mechanism.
- Repeated install/uninstall/toggle cycles accumulate orphaned command files under a real user's
  `~/.config/opencode/commands/` over time.

## Scope (when this is picked up — NOT done yet)
- Cover both `/qa-*` standalone commands and `/smartops-ui` — the gap is transversal, not
  per-skill, so the fix must be in the shared uninstall/rollback mechanism, not duplicated per
  skill's injector.
- Investigate whether `qaStandaloneCommands`/`smartopsCommandAssetDir`-style command paths need to
  be registered into whatever manifest `uninstall` already uses to decide "managed" vs
  "non-managed" files (the same manifest that correctly removes skill directories today).
- Likely touches: `internal/cli/run.go`/`sync.go` wiring sites already found in
  `smartops-ui-skill-mvp` (T5), the uninstall command path in `internal/cli`, and possibly a
  shared helper both `qa_commands.go` and `smartops_commands.go` could register against instead of
  each tracking removal separately.

## Constraints (carried over, still apply when implemented)
- Strict TDD Mode: enabled — RED (reproduce the leak with a test) before GREEN.
- Should NOT require re-litigating whether `smartops-ui`/qa-* commands should exist — this is
  purely a lifecycle completeness fix.

## Acceptance criteria (draft, refine when picked up)
- A real `uninstall` run leaves zero orphaned `opencode/commands/*.md` files for any standalone
  skill's command, for both the qa-* set and `smartops-ui`.
- Regression test added (unit and/or e2e) that would have caught this gap before it shipped.

## Tasks
- [ ] Not started.
