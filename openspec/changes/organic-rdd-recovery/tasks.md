# Tasks: Restore Organic Post-Candidate RDD

## Review Workload Forecast

| Field | Value |
|---|---|
| Estimated changed lines | 2,000–3,500 authored, plus generated assets |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | Single existing PR; maintainer `size:exception` required |
| Delivery strategy | single-pr |
| Chain strategy | size-exception |

Decision needed before apply: No — resolved by the maintainer on 2026-07-25
Delivery decision: continue on the existing pull request; `size:exception` already applied
Chain strategy: size-exception
400-line budget risk: High

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|---|---|---|---|---|---|
| 1 | Trace/delete proof | Existing PR | `go test ./internal/recoverytrace/...` | ledger validator fixtures | ledgers + recoverytrace |
| 2 | Organic RDD | Existing PR | `go test ./internal/reviewtransaction/... ./internal/cli/...` | frozen-candidate modes | RDD/mode surfaces |
| 3 | Assets/E2E | Existing PR | `go test ./e2e/organicruntime/...` | configured-agent journeys | assets/docs/E2E |

## Phase 1: Traceability and invariant transfer

- [ ] 1.1 RED: add `internal/recoverytrace/validate_test.go` table cases for the ledger-validator boundary only: deletion-without-proof, orphaned retained invariant, missing contributor credit, count reconciliation (241/92, 74 collisions, 499 overlaps, 16 decompositions), and publication classification with unauthorized early deviation. The five product boundaries (doc-like executable classification; common-dir identity; staged/`commit -a`/empty index; tracking/first-push/refspec; structured PR head/env/shell rejection) belong to their owning RAR/PAD packages and are covered by task 3.2, not by the ledger validator.
- [ ] 1.2 GREEN: create `internal/recoverytrace/{model,import,validate,generate}.go`, schemas and snapshot/change/invariant/contribution/test/deletion/release ledgers; import 241 issues/92 PRs, 74 collisions/499 overlaps, 16 decompositions, credits, refs/publication classification and authorized deviations.
- [ ] 1.3 RED→GREEN→REFACTOR: transplant each retained generic WorkRun invariant with destination tests or explicit no-retained-invariant proof; validator rejects unproven deletion and binds applicable exact-SHA/cross-OS/real-agent/release evidence.

## Phase 2: Remove control plane and install organic routing

- [ ] 2.1 RED→GREEN→REFACTOR: test then remove only ledger-proven-unpublished WorkRun dispatch, contracts/tests, `workprovider`/`workrun`, work flags, app/help and `sddstatus` binding/reservation residue; retain SDD attempt authority and compact reviewer capture.
- [ ] 2.2 RED→GREEN→REFACTOR: create `components/agentguidance` and wire `cli/{run,sync,doctor}` so every configured non-SDD agent receives equal baseline direct/delegated/optional-SDD routing; SDD assets remain optional.
- [ ] 2.3 RED→GREEN→REFACTOR: make capability manifests truthful per adapter, semantically equal on organic routing, and reject unknown agents; add `routing:sync-required` diagnostics and transactional managed-block sync preserving user content/idempotence.

## Phase 3: Post-candidate RDD and delivery

- [ ] 3.1 RED→GREEN→REFACTOR: add global plus clone-local off-only `review mode enable|disable|status`, source/effective diagnostics, future-candidate-only re-enable, disabled read-only frozen authority and `disabled/unmanaged` delivery.
- [ ] 3.2 RED→GREEN→REFACTOR: implement post-freeze tier-0 structural, tier-1 consolidated, and evidence-only tier-2 4R review; one ordinary correction, Judgment Day unchanged, immutable receipt reuse across policy-permitted delivery. Cover the five product threat-matrix boundaries under their owning RAR/PAD packages with `t.TempDir()`/fake-remote table tests: doc-like executable classification, common-dir identity, staged/`commit -a`/empty-index projection, tracking/first-push/refspec binding, and structured PR head with env-prefix/composed-shell rejection.
- [ ] 3.3 RED→GREEN→REFACTOR: bind applicability/cost/mutation/permission effects in RAR authority; cover N/A, auto quick, gap, immediate sensitive authorization, one frozen consent and replay rejection.

## Phase 4: Regeneration and release proof

- [ ] 4.1 RED→GREEN→REFACTOR: regenerate the 12 canonical orchestrator assets covering all 16 supported AgentIDs, the shared review contract and `docs/trigger-rules.md`; assert parity, no WorkRun/work-unit governance leakage, sync-required behavior, user-content preservation and second-sync no-op.
- [ ] 4.2 RED→GREEN→REFACTOR: rewrite `e2e/organicruntime/organic_runtime_test.go` for real agents: direct/delegated/optional-SDD accept-decline, all tiers, correction, and flexible delivery.
- [ ] 4.3 Run focused tests, `gofmt`, `go vet ./...`, `go test ./...`, applicable cross-OS/real-agent/exact-SHA checks and ledger/release gates; record results. Commit append-only conventional work units; rollback only corrective commits/`git revert`, never force-push.
