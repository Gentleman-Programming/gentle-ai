# SDD Orchestrator — Shared Sections

Canonical bodies for the orchestrator subsections every runtime states identically.
Each runtime keeps its own heading line and carries `{{GENTLE_AI_SDD_SECTION:<name>}}` in place
of the body; `composeOrchestratorPrompt` substitutes from here. Sections that genuinely differ
per runtime stay in the runtime asset (see #3817 for the measured drift inventory).

<!-- sdd-orchestrator-section:Native SDD Dispatcher Guard:start -->
For inspection and before routing an SDD change, invoke the native dispatcher using only `axiom sdd status [change] --cwd <repo> --json --instructions`. Inspection needs no execution preflight, review, delivery, or archive authorization; this read-only rule takes precedence over phase preflight/init guards. No recommendation is executed during inspection, including planning phases or a displayed preparation invocation.

Use native v2 for every declared artifact store, including Engram. The dispatcher resolves the store the workspace declares and returns `artifactStore` and `artifactPaths`. Do NOT determine the artifact store yourself, and do NOT branch on it or reconstruct readiness locally. Native JSON is authoritative over prompt inference. If native resolution fails or is invalid, report it and stop without a local dispatch fallback.

Only explicit authorized continuation may call `axiom sdd continue [change] --cwd <repo>`. First inspect with status and confirm the current human scope covers the selected change-directory marker. Read-only or excluded-marker scope forbids this mutating call; native allowed roots do not grant human consent. Preparation grants no source roots or attempts. Carry native `actionContext` intersected with the current narrower human scope into any executor.

For authorized phase routing only: Route only by `nextRecommended` and dependency states; honor `blockedReasons` and never infer from free text. If `blockedReasons` is non-empty, do not proceed to apply, archive, or terminal work. If `nextRecommended` is `resolve-blockers`, report `blockedReasons` and stop; if `nextRecommended` is a planning token (`propose`, `spec`, `design`, or `tasks`), launch the corresponding planning phase only within the authorized scope.

If the binary is unavailable, use the existing prompt contract for non-authoritative diagnostics only. Do not fabricate native-shaped status, readiness, or mutation authority, and never substitute continue for inspection.
<!-- sdd-orchestrator-section:Native SDD Dispatcher Guard:end -->

<!-- sdd-orchestrator-section:Language Domain Contract:start -->
- The active persona controls direct user/orchestrator conversation only. Use it for direct replies, clarification prompts, and user-facing orchestration status.
- In Axiom-governed projects or repositories with Spanish language rules declared, all SDD technical artifacts (`proposal.md`, `spec.md`, `design.md`, `tasks.md`, `verify-report.md`, `archive-report.md`) MUST be written in Spanish (castellano). English is strictly preserved for source code identifiers: package names, types, functions, structs, interfaces, methods, variables, and language keywords.
- For generic repositories without explicit Spanish language rules, technical artifacts follow the repository's prevailing convention or default to English.
- If technical artifacts are explicitly requested in another language, use a neutral/professional register unless the user explicitly requests a different tone or regional variant.
- Public/contextual comments follow the target context language by default. Explicit user language or tone overrides win; otherwise use a neutral/professional register unless the target context clearly calls for another tone or regional variant.
- When delegating, forward this contract to the executor so persona voice never becomes the artifact or public-comment default.
<!-- sdd-orchestrator-section:Language Domain Contract:end -->

<!-- sdd-orchestrator-section:Dependency Graph:start -->
```
proposal -> specs --> tasks -> apply -> archive
                                 \-> verify (optional diagnostics)
             ^
             |
           design
```
Verification is opt-in and may inspect partial work. Completed implementation normally goes directly to archive. Optional diagnostic failures, absent reports, and unfinished tasks are not archive certificates or admission gates; preserve actual findings and permissions. Do not run the automatic gatekeeper retry loop for diagnostic findings. An explicitly requested archive may close unfinished work honestly even while status recommends apply.
<!-- sdd-orchestrator-section:Dependency Graph:end -->

<!-- sdd-orchestrator-section:Recovery Rule:start -->
Recover from native status and the actual artifacts identified by `artifactStore` and `artifactPaths`, not a locally reconstructed DAG. In `openspec`, read resolved file paths; in `engram`, use project-scoped `mem_search` followed by full `mem_get_observation`; in `hybrid`, follow each resolved locator without substituting the other store. In `none`, use available conversation context and disclose what cannot be recovered.

Existing `state.yaml` and `sdd/{change-name}/state` snapshots are optional recovery hints, never required per-phase writes or a second authority. Preserve historical snapshots and `dependsOn` metadata; check progress and archive closure against actual artifacts. Missing or stale hints do not block recovery or establish active work.
<!-- sdd-orchestrator-section:Recovery Rule:end -->

<!-- sdd-orchestrator-section:Delegated Verification Gate (MANDATORY):start -->
SDD never offers or launches RDD, regardless of review mode. For SDD work, run the applicable functional checks and configured TDD, report actual results, and follow the SDD phase instructions; do not assess review risk or invoke review from this gate. The rules below apply only to non-SDD work.

Verification of a delegated writer's work is decided by two inputs the parent reads deterministically: the receipt-driven development (RDD) state for the repository (`on`, `off`, or `unknown`), and the native risk tier from `axiom review assess --cwd <repo> --json` (`gentle-ai.review-assessment/v1`, `risk` one of `passive`, `medium`, `high`). A runtime that already renders an RDD status line reads it from there; otherwise read `axiom review mode status` (read-only) and treat a failure as `unknown`. Any assessment failure or an unrecognized verb is treated as `high`.

The `on` branch below holds only while the native review reaches a terminal outcome for this candidate. When the human declines the consent envelope for this candidate (candidate-scoped; never the kill switch), when receipt-driven development is disabled for the clone after this status was read, or when START or STATUS refuses, the parent follows the RDD off path instead: run `axiom review assess --cwd <repo> --json` over the writer's diff and apply the tier table below. An unknown outcome is treated as not closed, never as terminal.

- **RDD on**: the bounded writer runs the parent-authorized `## Verification` commands in the foreground and reports `<command>: <observed result>`; that report is the verification of record, and the native review is the independent check. A separate verifier stays on-demand only — the writer reported `partial` or `blocked`, an expensive or external check the parent wants run on a cheaper profile, or a parent spot check. A passive candidate needs only the parent's structural readback.
- **RDD off or unknown**: after the writer returns, the parent runs `axiom review assess` over the writer's diff and follows the tier — passive: structural readback only; medium: writer self-verification, with a separate verifier only when the writer ran on a small-model profile (low effort or a mini model); high or unassessable: writer self-verification plus an independent verifier. `unknown` never lowers a tier, and the small-model bias raises the tier by one for verification purposes.
- The parent spot check — re-running one reported command before delivery — stays in every tier.
- The writer receives `## Verification` naming the exact commands to run, and may receive `## Known environmental failures` naming exact test names or command lines already failing on the base as evidence; any other failing required command still forces `partial`.
- Exploration stays a separate delegation only when the parent needs the map to decide or route; reading that prepares a write belongs to the writer doing that write.
<!-- sdd-orchestrator-section:Delegated Verification Gate (MANDATORY):end -->

<!-- sdd-orchestrator-section:Delegated Verification Gate (Reduced Form):start -->
SDD never offers or launches RDD, regardless of review mode. For SDD work, run the applicable functional checks and configured TDD, report actual results, and follow the SDD phase instructions; do not assess review risk or invoke review from this gate. The rules below apply only to non-SDD work.

This runtime has no subagent delegation mechanism, so there is no separate writer or verifier to gate: the orchestrator itself performs the bounded action and its own verification. The native risk tier from `axiom review assess --cwd <repo> --json` (`gentle-ai.review-assessment/v1`, `risk` one of `passive`, `medium`, `high`; any failure or an unrecognized verb is treated as `high`) still decides whether verification commands run at all:

- **Passive**: structural readback only; do not run the `## Verification` commands.
- **Medium or high**: run the exact `## Verification` commands yourself, in the foreground, and report `<command>: <observed result>`.

The parent spot check — re-running one reported command before delivery — still applies. The receipt-driven development state does not change this table: native review remains the independent check on top of whatever verification ran here. That independent check only stands once the native review reaches a terminal outcome for this candidate: a decline of the consent envelope for this candidate (candidate-scoped; never the kill switch), receipt-driven development disabled for the clone after this status was read, or a START or STATUS refusal are all treated as not closed, and never excuse the agent from running the tier's verification commands above.
<!-- sdd-orchestrator-section:Delegated Verification Gate (Reduced Form):end -->
