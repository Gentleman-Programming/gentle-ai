---
name: rdd-advisory-transport
description: "Trigger: reviewer transport, advisory transport, review adapter, lens prompt/schema, OpenCode reviewer plugin, Codex reviewer, ReviewProviderContract. Enforce the shared Go advisory reviewer transport contract."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Activation Contract

Load when changing how any runtime (Claude Code, OpenCode, Codex, future) invokes an RDD reviewer, refuter, or fix validator, or when touching reviewer prompts, lens evidence, result schemas, adapters, or transport capability policy.

## Hard Rules

- One Go provider contract owns binding, frozen evidence, lens mandate, prompt, result schema, byte budget, JSON extraction, and admission. Start from existing native sources (`reviewLensContextBlock`, `ReviewerResultSchema`, `RunReviewCaptureResult`, `CaptureAdmittedReviewerResult`); never build a parallel system.
- Adapters only invoke the reviewer and return raw bytes plus a transport error. An adapter must never parse bindings, rebuild prompts, copy schemas, apply local budgets, parse JSON, capture or preserve results, hold retry state, or decide blocking.
- Runtime output is advisory: it cannot create authority, mutate review state, mint a receipt, or open a gate. Only Go admission does.
- Byte budget refuses before invocation; never truncate. Malformed, truncated, or incomplete input never becomes PASS, complete, or a clean receipt.
- No OpenCode restart, child isolation, special session, or `OPENCODE_DISABLE_*` variables. An ordinary running session is sufficient.
- The RDD semantic pipeline is untouchable: lens selection, severity meanings, candidate causality, refuter adjudication, bounded correction, verification, terminal receipt, and read-only review-context evaluation. These are review-lifecycle concerns only; ordinary repository policy owns delivery.
- Refuter and fix validator consume the same provider contract. A validator that could not inspect produced no verdict — surface a blocked decision; never record it as failed (that irreversibly consumes the single correction).
- Capability advertisement requires shared-contract conformance plus organic runtime proof. Codex stays unadvertised until both positive and fail-closed proofs pass.
- Transport failures never consume a correction budget, duplicate a capture, or strand a lineage.
- Historical receipts stay readable; never rewrite `provider_command` or `runtime_interception` descriptors.

## Decision Gates

| Condition | Action |
| --- | --- |
| Logic fits "prompt/evidence/schema/validation" | It belongs in the Go contract, never an adapter. |
| Adapter cannot expose raw final output | Runtime stays typed unavailable; no artifact-parser workaround. |
| Cutover slice exceeds 1000 lines/PR | Chained PRs on the tracker; advertisement flip lands last. |
| Shared prompt changes any semantic outcome | STOP; regression against baseline blocks the slice. |
| Slice touches negotiated START / `repository_context` | Coordinate with the deferred Wave 7 switch-removal finding first. |

## Execution Steps

1. Freeze the boundary: contract-level tests around current native prompt/evidence/schema/admission.
2. Extract the Go provider contract; differential-test against current admission.
3. Add adapter-minimality static guards (no binding types, prompt text, schema copies, budgets, parsers, capture, retry in adapters) — from the first adapter slice, not last.
4. Cut Claude + OpenCode over without an advertised intermediate path; delete the OpenCode plugin and isolation machinery in the same release slice.
5. Prove Codex organically before advertising; retire historical transport code last.
6. Gate every slice on the bench journey corpus diff: all journeys unchanged.

## Output Contract

Report per slice: contract surface touched, adapter minimality guard status, bench corpus diff result, semantic regression result, capability advertisement state, and any unresolved maintainer decision (e.g. receipt `provider_contract` descriptor).

## References

- `references/shared-advisory-transport-proposal.md` — full proposal: architecture, deletion candidates, test plan, risks.
- `references/issue-impact-matrix.md` — per-issue dispositions under the shared contract; a maintainer condition in an issue's own thread always outranks its row.
