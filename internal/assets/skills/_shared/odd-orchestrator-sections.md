# ODD Orchestrator — Shared Sections

Canonical bodies for the orchestrator subsections shared across runtimes.

<!-- sdd-orchestrator-section:Language Domain Contract:start -->
- The active persona controls direct user/orchestrator conversation only. Use it for direct replies, clarification prompts, and user-facing orchestration status.
- Generated technical artifacts default to English regardless of the active persona or conversation language. This includes tasks, code comments, UI copy, tests, fixtures, and delegated outputs.
- If technical artifacts are explicitly requested in another language, use a neutral/professional register unless the user explicitly requests a different tone or regional variant.
- Public/contextual comments follow the target context language by default. Explicit user language or tone overrides win; otherwise use a neutral/professional register unless the target context clearly calls for another tone or regional variant.
- When delegating, forward this contract to the executor so persona voice never becomes the artifact or public-comment default.
<!-- sdd-orchestrator-section:Language Domain Contract:end -->

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

<!-- sdd-orchestrator-section:Organic Driven Development Is The Default Workflow (MANDATORY):start -->
Organic Driven Development (ODD) is this orchestrator's predefined workflow for every request. Its ordered protocol is installed for this agent under `## Implementation Routing` (`### ODD protocol`) and runs first, on every request, without the user asking about workflow, planning, or task tracking.
<!-- sdd-orchestrator-section:Organic Driven Development Is The Default Workflow (MANDATORY):end -->
