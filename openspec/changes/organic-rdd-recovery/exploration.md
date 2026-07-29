## Exploration: organic-rdd-recovery

### Current State
The branch has a committed productive-runtime implementation plus a large uncommitted continuation (43 modified and 4 untracked files; 3,124 additions / 344 deletions). It still exposes pre-implementation `work-capabilities`, `work-start`, WorkRun routing/advance, connector assumptions, and generated orchestrator instructions. CodeGraph confirms this coupling in `internal/cli/work_capabilities.go`, `internal/cli/work_start.go`, `internal/workprovider/*`, `internal/components/sdd/*`, and adapter assets. The native review kernel remains separately valuable: review start/finalize, compact immutable reviewer-result capture, candidate identity, and content-bound gate validation.

The valid architecture is the systemic nine-owner model, not the productive-runtime control plane. The recovery keeps the following invariants under their owners: HCR local command/install facts; MMI mutation and cross-OS path/mode safety; ACI deterministic agent instruction projection; MCA catalog provenance; RAR immutable candidate identity, proportional review, one correction, and receipts; EPD typed evidence/policy/diagnostics; DSR desired-state reconciliation; optional SDD lifecycle; and PAD flexible, exact-head delivery. The 241-issue / 92-PR ledger remains authoritative, including the 74-PR collision component, 499 overlaps, and 16 cross-context PR decompositions.

### Affected Areas
- `internal/cli/work_capabilities.go`, `internal/cli/work_start.go`, `internal/app/help.go` — remove universal normal-work negotiation and public command/help surface.
- `internal/workprovider/{runtime_*,productive_*,coordination_*,owner_*,production_repository.go,outcome_service.go}` — selectively delete remote/control-plane orchestration while transplanting generic CAS, replay, isolation, result-admission, and recovery safeguards to their owning local packages.
- `contracts/work-routing/v1/{schemas,fixtures}` and CLI/runtime tests — delete or regenerate only transport/dormant/connector-specific contracts; preserve invariant tests under retained owners.
- `internal/components/sdd/{boundedreview.go,inject.go}` and `internal/assets/*/sdd-orchestrator.md` — restore organic direct/delegated/optional-SDD guidance; remove pre-request WorkRun handshake and prompt-owned provider control-plane rules.
- `internal/reviewtransaction/{compact_reviewer_capture.go,*_test.go}` — keep and revalidate admitted immutable reviewer results, exact result/receipt pairing, candidate binding, and one-correction constraints.
- `e2e/organicruntime/organic_runtime_test.go` — replace TLS/runtime-fixture proof with real configured-agent journeys: direct, delegated, optional SDD, tier-0 docs, tier-1 consolidated review, tier-2 focused 4R, one correction, and flexible delivery.
- `docs/audits/2026-07-24-organic-rdd-recovery-plan.md` and the superseded `docs/audits/2026-07-23-organic-recovery-implementation-plan.md` — use the current plan as migration authority and retain the superseded implementation plan only as drift evidence. `docs/audits/2026-07-23-systemic-remediation-architecture.md` is not superseded and remains the authoritative systemic source of truth.

### Approaches
1. **Selective extraction and owner-based recovery** — classify every branch artifact as KEEP, TRANSPLANT, REWRITE, DELETE, REGENERATE, or DEFER; move generic invariants before deleting obsolete callers.
   - Pros: Preserves issue/PR coverage, contributor credit, Windows and worktree fixes, and the proven review trust kernel; aligns with the nine contexts.
   - Cons: Requires traceability ledgers and careful test relocation before deletion.
   - Effort: High.

2. **Branch reset or broad runtime deletion** — discard the productive-runtime work and reimplement an organic flow.
   - Pros: Lower short-term code volume.
   - Cons: Violates the preservation rule; risks losing generic safety fixes, historical coverage, and contributor work; breaks 241/92 reconciliation.
   - Effort: High risk; rejected.

### Recommendation
Use selective extraction, never a branch reset. First freeze/classify every current commit and dirty file against the authoritative issue/PR and invariant ledgers. Then remove the pre-implementation control plane (`work-capabilities`, universal WorkRun intake, productive HTTPS connector, URL/token/CA/global-agent activation, remote assumptions) only after focused retained-owner tests cover each generic invariant. Rebuild generated schemas, fixtures, and agent assets from corrected organic sources. Keep execution governance SDD-only for this change; RDD is the product behavior being corrected, not this change's planning or delivery meta-workflow.

### Risks
- Deleting `workprovider`/`workrun` callers before transplanting their generic CAS, common-dir/worktree, Windows DACL, replay, and immutable-result protections would create silent regressions.
- Generated orchestrator assets, schemas, fixtures, help, and E2E can retain obsolete WorkRun language after source removal unless regenerated and parity-checked.
- A simple package-ancestry test purge would lose valid edge-case coverage; test retention must follow invariant ownership.
- The current dirty diff is large and mixed; no file may be deleted without a disposition, source issue/PR mapping, contributor credit, and regression proof.
- Delivery must remain direct-main/direct-push/PR-without-issue/PR-with-issue/emergency where repository policy permits; do not accidentally reintroduce mandatory GitHub or connector credentials.

### Ready for Proposal
Yes — the product boundary, rejected implementation, retained trust kernel, affected areas, and migration rule are evidence-backed. The proposal must require the snapshot/change/invariant/contribution/test/deletion/release ledgers, exact one-time reconciliation of all 241 issues and 92 PRs, and SDD-only execution governance.
