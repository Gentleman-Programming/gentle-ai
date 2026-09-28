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
- Delivery strategy: exception-ok, explicitly requested by the user for one branch/one PR despite a forecast of approximately 800–1,200 authored changed lines across model discovery, managed plugins, and authentic V2 review transport. Preserve reviewable work-unit commits and evidence; do not weaken the immutable-transport proof or existing repository size gates.

## Tasks
- [x] T1 — Make V2 model catalog discovery use supported OpenCode API/CLI output, preserving V1 behavior. Route: delegated direct (source and tests are non-trivial; preparation belongs with writer). Acceptance: `opencode models --verbose` never runs for V2; available tool-capable models can populate the picker; focused tests pass. Checks: `go test ./internal/opencode ./internal/tui/screens -count=1` passed in writer and independent verifier; opt-in real V2 API integration passed in both. RED for the unsupported V2 flag and explicit reasoning capability was observed before GREEN. The V2 API does not always expose reasoning metadata, so absent values remain unknown. Commit evidence: `cc8900334`. Risk/RDD: high (unassessable), due; preflight returned terminal `immutable_review_transport_unsupported`, with no authority started. Native receipt unavailable on this build/runtime; no clone-level mode change made.
- [x] T2a — Fail closed before V2 plugin and telemetry writes when the installed V2 SDK is missing. Route: delegated direct (install/sync orchestration and tests). Acceptance: no broken V2 assets are written; V1 bypasses the new guard; ambiguous package-manager ownership is not guessed; printed install commands are runnable for their stated shells. Checks: focused Go tests passed in writer and independent verifier (before the final scoped portability correction), parent spot check pending. Broader `go test ./internal/cli ./internal/components/opencoderuntimeplugins ./internal/components/telemetryruntime ./internal/assets -count=1` was partial: CLI had 54 failures, including one introduced refusal message since corrected and many native review tests refusing V2; not rerun after correction. Commit evidence: pending. Risk/RDD: high, independent verifier used; native review unavailable on this runtime.
- [ ] T2b — Provision the version-matched SDK during explicit V2 install using the existing package-manager route; keep `sync` offline and preserve user-owned package manifest/lockfile semantics. Route: delegated direct. Acceptance: install succeeds only when the SDK is actually resolvable and otherwise fails before asset writes with an actionable exit; no unapproved network calls or irreversible package-manager mutation are hidden. Checks: bounded mocked package-manager tests and real V2-host activation. Commit evidence: pending. Risk/RDD assessment: pending.
- [ ] T3 — Establish real OpenCode 2.0.18 foreground reviewer transport behavior before enabling any authority. Route: delegated direct (host E2E plus test harness). Acceptance: positive lens/refuter/validator Task and bound final-result evidence from an installed V2 host; negative cases cannot be admitted. Checks: opt-in real V2 E2E and Go transport/asset tests. Commit evidence: pending. Risk and RDD assessment: pending.
- [ ] T4 — Correct only transport differences proven by T3, then admit V2 capability last. Route: delegated direct (plugin, native gate, tests, docs). Acceptance: immutable target-bound capture and host conformance proven; stale/wrong/partial output fails closed; no change to V1 behavior. Checks: targeted Go tests, authentic V2 E2E, applicable bench-driven journey if representable. Commit evidence: pending. Risk and RDD assessment: pending.

## Progress and evidence
- Global OpenCode agent assignments were updated separately in the user's existing configuration and verified via `/api/agent` and `/api/model`; no repository files were involved.
- Local reproduction: `opencode models --verbose` exits 1 with `Unrecognized flag: --verbose in command opencode models` on OpenCode 2.0.18.
- `gentle-ai sync --agent opencode` migrated the four Gentle AI-owned files to V2; after explicit anonymous installation of `@opencode/plugin@2.0.4` and a user-initiated shared-service restart, `/api/plugin` reports all four active. Community and other owner's plugins remain separate.
- T1 committed; running authored branch change is 330 lines including task document. Engram mirror synchronized from this document.
- T1 source verification observed. Native risk assessment and preflight refused immutable OpenCode transport; no T2 source edits yet.

## Next step
Native review is stopped without starting authority. Preserve T1 candidate; do not bypass V2's gate without authentic host proof. Finish T2a readback and evidence, then T2b and T3–T4. The user explicitly chose a single-branch size exception. Obtain the exact missing slash-command name before attributing that symptom to a repository defect.
