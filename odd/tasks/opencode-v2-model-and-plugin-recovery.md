# OpenCode V2 model and managed-plugin recovery

## Objective
Restore model discovery in Gentle AI against OpenCode 2.0.18 and ensure Gentle AI-owned installed plugins use the V2 assets without touching plugins owned by other projects.

## Problem and why
The picker invokes `opencode models --verbose`, which OpenCode 2.0.18 rejects. The installed managed Gentle AI plugin bytes match V1 assets, while V2 assets exist in the repository. A screenshot also shows other failed plugins and missing commands, but their exact loader errors and requested command names have distinct ownership and remain outside this repair.

## Scope and constraints
- Authorized scope: local Gentle AI Go catalog discovery, its tests, Gentle AI-managed OpenCode plugin install/refresh behavior and its tests, necessary task documentation. No user-owned or third-party plugin edits.
- Preserve the original checkout and its untracked files. Work in `fix/opencode-v2-recovery` based on `main`.
- Test-first where a deterministic regression exists: observe RED, implement GREEN, then refactor with green tests.
- Advisory task size: approximately 400 authored changed lines per task, not a cap.
- Delivery strategy: ask-on-risk. Forecast: approximately 300–400 authored changed lines across the work units; revisit before the next commit if this grows past 400.

## Tasks
- [x] T1 — Make V2 model catalog discovery use supported OpenCode API/CLI output, preserving V1 behavior. Route: delegated direct (source and tests are non-trivial; preparation belongs with writer). Acceptance: `opencode models --verbose` never runs for V2; available tool-capable models can populate the picker; focused tests pass. Checks: `go test ./internal/opencode ./internal/tui/screens -count=1` passed in writer and independent verifier; opt-in real V2 API integration passed in both. RED for the unsupported V2 flag and explicit reasoning capability was observed before GREEN. The V2 API does not always expose reasoning metadata, so absent values remain unknown. Commit evidence: pending. Risk and RDD assessment: pending.
- [ ] T2 — Recover Gentle AI-owned stale V1 plugin installs on V2 without overwriting user edits. Route: delegated direct (lifecycle and tests across components). Acceptance: owned assets refresh to V2 bytes, modified/unowned files remain untouched; focused lifecycle and asset tests pass. Checks: `go test ./internal/components/opencoderuntimeplugins ./internal/components/telemetryruntime ./internal/assets -count=1`; isolated V2 plugin activation. Commit evidence: pending. Risk and RDD assessment: pending.

## Progress and evidence
- Global OpenCode agent assignments were updated separately in the user's existing configuration and verified via `/api/agent` and `/api/model`; no repository files were involved.
- Local reproduction: `opencode models --verbose` exits 1 with `Unrecognized flag: --verbose in command opencode models` on OpenCode 2.0.18.
- Read-only investigation found installed V1 assets for four Gentle AI-owned plugins; community and other owner's plugins have separate failures.
- Engram mirror update pending after the T1 commit evidence is recorded.
- T1 source verification observed. The source candidate contains API schema parsing and is treated as high-risk pending native assessment; no T2 source edits yet.

## Next step
Complete T1 test-first, then T2. Obtain the exact missing slash-command name before attributing that symptom to a repository defect.
