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
3. After the external run, call `gentle-ai sdd-attempt settle --cwd <repo> --change <change> --token <token> --request-id <settle-id> ...` with a request ID distinct from the acquire operation's request ID, outcome, and bounded evidence. Reuse each operation's own ID only for its idempotent replay. Settle derives native binding/remediation inputs; pass `--successor-lineage` only for a distinct approved successor, otherwise the bound lineage remains its own successor.
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
