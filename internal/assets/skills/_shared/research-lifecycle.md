# SDD Research Lifecycle Contract

Research is optional until selected. Selection makes this fail-closed contract mandatory; research produces evidence, while the orchestrator owns product decisions and proposal admission.

## Evidence artifact

`research.md` and `sdd/{change-name}/research` use `gentle-ai.sdd-research/v1` and record:

- a positive `revision`, explicit `done | partial | blocked` outcome, and the questions;
- admission and observed exact grants;
- sources with `id, class, title, publisher, URL, accessed_at, excerpt`;
- validated claims where each claim maps to source IDs;
- contradictions, uncertainty and freshness; and
- product choices are separate and non-authoritative.

Selected `done` evidence must satisfy every field above; partial or blocked outcomes MUST exclude unvalidated claims. Admission denial produces no source claims.

## Pre-proposal state

`gentle-ai.sdd-preproposal/v1` records the positive `revision`, exploration outcome/reference, research request and classes, admission and outcome, OpenSpec and Engram evidence references, product decisions (`pending | confirmed`), and `proposal_ready`.

Selected research is ready only when evidence is valid and `done`, decisions are confirmed, references are valid, and its selected store mode is ready: OpenSpec-only or Engram-only validates that store, hybrid requires equal revision and bytes in both, and no-store remains unready. Unselected research skips only this research condition. The proposal handoff carries the state revision, confirmed decisions, and optional evidence references.

<!-- research-lifecycle-gate:start -->### Research and Pre-Proposal Gate (MANDATORY) — Offer `sdd-research` immediately after `sdd-explore`; selection makes completion mandatory. Before every `propose`, invoke `sdd-propose` only when selected research is `done` or research is unselected, product decisions are `confirmed`, evidence references are valid, and the selected artifact-store state is ready. The orchestrator owns product discovery. Automatic unresolved choices require one lossless grouped prompt with all context, options, consequences, allowed answers, and exact tokens; it MUST persist the pending state before prompting, then STOP without invoking `sdd-propose`. The proposer receives a confirmed pre-proposal handoff and MUST NOT interview or infer consent. Native `gentle-ai.sdd-status/v2` is the sole status contract.<!-- research-lifecycle-gate:end -->
