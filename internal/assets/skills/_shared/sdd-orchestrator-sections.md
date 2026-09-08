# SDD Orchestrator — Shared Sections

Canonical bodies for the orchestrator subsections every runtime states identically.
Each runtime keeps its own heading line and carries `{{GENTLE_AI_SDD_SECTION:<name>}}` in place
of the body; `composeOrchestratorPrompt` substitutes from here. Sections that genuinely differ
per runtime stay in the runtime asset (see #3817 for the measured drift inventory).

<!-- sdd-orchestrator-section:Native SDD Dispatcher Guard:start -->
Before routing, continuing, applying, verifying, or archiving an SDD change, **invoke the native dispatcher** (`gentle-ai sdd-continue [change] --cwd <repo>` or `gentle-ai sdd-status [change] --cwd <repo> --json --instructions`). It resolves the artifact store the workspace DECLARES, reports it in `artifactStore`, and returns that store's locators in `artifactPaths`. **Do NOT determine the artifact store yourself, and do NOT branch on it** — an actor that re-derives the store disagrees with the authority that launched it, which is how a phase ends up reading a store the workspace never declared. Use the dispatcher for every store when `gentle-ai` is available and treat its native status JSON as authoritative over prompt inference. Route only by `nextRecommended` and dependency states; never infer from free text. If `blockedReasons` is non-empty, do not proceed to apply, archive, or terminal work. If `nextRecommended` is `verify`, verification/remediation may run only to refresh evidence; if `nextRecommended` is `resolve-blockers`, report `blockedReasons` and stop; if `nextRecommended` is a planning token (`propose`, `spec`, `design`, or `tasks`), launch the corresponding planning phase. If the binary is unavailable, fall back to the existing prompt contract and manual status schema.
<!-- sdd-orchestrator-section:Native SDD Dispatcher Guard:end -->

<!-- sdd-orchestrator-section:Native Runtime Attempt Authority (MANDATORY):start -->
Use the provider-owned Git-common-dir runtime ledger for every runtime-bearing `sdd-apply`, `sdd-verify`, or remediation continuation. It is the single attempt/budget authority for both OpenSpec and Engram; never persist caller-authored counters in OpenSpec files, Engram topics, prompts, or Pi state.

1. Before an actor or harness launch, call `gentle-ai sdd-attempt acquire --cwd <repo> --change <change> --request-id <id> --work-unit <label> --evidence-goal <goal> --max-attempts <count> --max-changed-lines <count>`.
2. Launch only when acquire returns `state: proceed`, and retain its opaque `token`. `blocked` or `complete` stops the launch.
3. After a failed or passed run, call `gentle-ai sdd-attempt settle --cwd <repo> --change <change> --token <token> --request-id <settle-id> --outcome <passed|failed> --evidence-revision <sha256> --diagnosis "<proven-diagnosis>" --harness-disposition <reused|invalidated> --cleanup-evidence "<evidence>" --process-evidence "<evidence>"`. After an interrupted run, pass `--outcome interrupted` and omit `--evidence-revision`. When the acquire carried `--remediates-evidence-revision <sha256>`, settle with the same `--remediates-evidence-revision <sha256>`. Use a `<settle-id>` distinct from the acquire operation's request ID; reuse each operation's own ID only for its idempotent replay. Settle defines no other flag and derives native binding and remediation inputs itself.
4. On any failed external command (test command or non-test external command) before a later native block, disclose in this order: **Primary failure:** identify the command in a privacy-safe form, its failed/cancelled/non-zero outcome, and only bounded relevant error evidence; never persist or print secrets, private values, raw environment, or unbounded output. **Verification consequence:** state that the current SDD phase/verification did not pass. **Attempt settlement:** when the native contract requires it, settle the current token with the correct failed/interrupted outcome and diagnosis, and disclose the settlement result before any later acquire/refusal. **Secondary governance block:** label a later objective-change/acquire refusal as secondary, never as the cause of the external command failure, and preserve the exact provider-owned runnable continuation unchanged. Never imply Gentle AI or the native ledger caused the independent consumer command failure.
5. Route only from settle's `proceed`, `blocked`, or `complete` state. Full `status|begin|finish|reset` operations are diagnostic/compatibility surfaces; reset requires an explicit maintainer scope decision and is never automatic.
<!-- sdd-orchestrator-section:Native Runtime Attempt Authority (MANDATORY):end -->

<!-- sdd-orchestrator-section:Language Domain Contract:start -->
- The active persona controls direct user/orchestrator conversation only. Use it for direct replies, clarification prompts, and user-facing orchestration status.
- Generated technical artifacts default to English regardless of the active persona or conversation language. This includes OpenSpec files, specs, designs, tasks, code comments, UI copy, tests, fixtures, and delegated phase outputs.
- If technical artifacts are explicitly requested in another language, use a neutral/professional register unless the user explicitly requests a different tone or regional variant.
- Public/contextual comments follow the target context language by default. Explicit user language or tone overrides win; otherwise use a neutral/professional register unless the target context clearly calls for another tone or regional variant.
- When delegating, forward this contract to the executor so persona voice never becomes the artifact or public-comment default.
<!-- sdd-orchestrator-section:Language Domain Contract:end -->

<!-- sdd-orchestrator-section:Dependency Graph:start -->
```
proposal -> specs --> tasks -> apply -> verify -> archive
             ^
             |
           design
```
<!-- sdd-orchestrator-section:Dependency Graph:end -->

<!-- sdd-orchestrator-section:Recovery Rule:start -->
- `engram` → `mem_search(...)` → `mem_get_observation(...)`
- `openspec` → read `openspec/changes/*/state.yaml`
- `none` → state not persisted — explain to user
<!-- sdd-orchestrator-section:Recovery Rule:end -->

<!-- sdd-orchestrator-section:Delegated Verification Gate (MANDATORY):start -->
Verification of a delegated writer's work is decided by two inputs the parent reads deterministically: the receipt-driven development (RDD) state for the repository (`on`, `off`, or `unknown`), and the native risk tier from `gentle-ai review assess --cwd <repo> --json` (`gentle-ai.review-assessment/v1`, `risk` one of `passive`, `medium`, `high`). A runtime that already renders an RDD status line reads it from there; otherwise read `gentle-ai review mode status` (read-only) and treat a failure as `unknown`. Any assessment failure or an unrecognized verb is treated as `high`.

The `on` branch below holds only while the native review reaches a terminal outcome for this candidate. When the human declines the consent envelope for this candidate (candidate-scoped; never the kill switch), when receipt-driven development is disabled for the clone after this status was read, or when START or STATUS refuses, the parent follows the RDD off path instead: run `gentle-ai review assess --cwd <repo> --json` over the writer's diff and apply the tier table below. An unknown outcome is treated as not closed, never as terminal.

- **RDD on**: the bounded writer runs the parent-authorized `## Verification` commands in the foreground and reports `<command>: <observed result>`; that report is the verification of record, and the native review is the independent check. A separate verifier stays on-demand only — the writer reported `partial` or `blocked`, an expensive or external check the parent wants run on a cheaper profile, or a parent spot check. A passive candidate needs only the parent's structural readback.
- **RDD off or unknown**: after the writer returns, the parent runs `gentle-ai review assess` over the writer's diff and follows the tier — passive: structural readback only; medium: writer self-verification, with a separate verifier only when the writer ran on a small-model profile (low effort or a mini model); high or unassessable: writer self-verification plus an independent verifier. `unknown` never lowers a tier, and the small-model bias raises the tier by one for verification purposes.
- The parent spot check — re-running one reported command before delivery — stays in every tier.
- The writer receives `## Verification` naming the exact commands to run, and may receive `## Known environmental failures` naming exact test names or command lines already failing on the base as evidence; any other failing required command still forces `partial`.
- Exploration stays a separate delegation only when the parent needs the map to decide or route; reading that prepares a write belongs to the writer doing that write.
<!-- sdd-orchestrator-section:Delegated Verification Gate (MANDATORY):end -->

<!-- sdd-orchestrator-section:Delegated Verification Gate (Reduced Form):start -->
This runtime has no subagent delegation mechanism, so there is no separate writer or verifier to gate: the orchestrator itself performs the bounded action and its own verification. The native risk tier from `gentle-ai review assess --cwd <repo> --json` (`gentle-ai.review-assessment/v1`, `risk` one of `passive`, `medium`, `high`; any failure or an unrecognized verb is treated as `high`) still decides whether verification commands run at all:

- **Passive**: structural readback only; do not run the `## Verification` commands.
- **Medium or high**: run the exact `## Verification` commands yourself, in the foreground, and report `<command>: <observed result>`.

The parent spot check — re-running one reported command before delivery — still applies. The receipt-driven development state does not change this table: native review remains the independent check on top of whatever verification ran here. That independent check only stands once the native review reaches a terminal outcome for this candidate: a decline of the consent envelope for this candidate (candidate-scoped; never the kill switch), receipt-driven development disabled for the clone after this status was read, or a START or STATUS refusal are all treated as not closed, and never excuse the agent from running the tier's verification commands above.
<!-- sdd-orchestrator-section:Delegated Verification Gate (Reduced Form):end -->
